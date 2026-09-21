package controller

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func RelayConfigurableResource(c *gin.Context) {
	profileID := strings.TrimSpace(c.GetString(middleware.ContextKeyConfigurableResourceProfileID))
	resourceID := strings.TrimSpace(c.GetString(middleware.ContextKeyConfigurableResourceID))
	// Resolve owned handles before routing. Another token belonging to the
	// same user must not need access to the original channel's group/model.
	assetAccess, accessErr := resolveAssetAccessRoute(c, profileID, resourceID)
	if accessErr != nil {
		respondAssetAccessError(c, accessErr)
		return
	}
	smart := service.IsRoutingStrategyTokenPolicy(c)
	param := &service.RetryParam{Ctx: c, TokenGroup: service.AutoGroupName, Retry: common.GetPointer(0)}
	var lastErr *types.NewAPIError
	for {
		if !param.BeginAttempt() {
			break
		}
		var channel *model.Channel
		var profile *configurable.Profile
		var resource *configurable.ResourceConfig
		var err error
		if assetAccess != nil && assetAccess.channel != nil {
			channel, profile, resource = assetAccess.channel, assetAccess.profile, assetAccess.resource
		} else if smart {
			channel, profile, resource, err = selectSmartConfigurableResourceRoute(c, profileID, resourceID)
		} else {
			channel, profile, resource, err = selectConfigurableResourceRoute(c, profileID, resourceID)
		}
		if err != nil {
			if errors.Is(err, errUnsupportedAssetOperation) {
				c.JSON(http.StatusNotImplemented, gin.H{"error": gin.H{"code": "unsupported_asset_operation", "message": err.Error()}})
				return
			}
			if lastErr == nil {
				lastErr = types.NewErrorWithStatusCode(err, types.ErrorCodeGetChannelFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
			}
			break
		}
		if !authorizeConfigurableResourceModel(c, resource) {
			return
		}
		if err := prepareAssetAccess(c, channel, profile, resource); err != nil {
			respondAssetAccessError(c, err)
			return
		}
		if err := prepareTgxMaasAssetRequest(c, channel, profile, resource); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		modelName := configurableResourceRequestModel(c, resource)
		service.UpdateCurrentRetryRouteTarget(c, channel, common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
		lastErr = relayConfigurableResourceAttempt(c, channel, profile, resource)
		if lastErr == nil {
			service.MarkRetryRouteFinal(c, c.Writer.Status() < 400, "completed")
			return
		}
		if configurableResourceHasIndependentHealth(channel, resource) {
			// Log the asset failure without disabling video keys or recording a
			// video routing-health failure. Asset operations are never replayed.
			processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", false), lastErr)
			break
		}
		service.MarkSmartRetryChannelFailure(c, channel, modelName, false)
		if !selectedChannelAllowsRetry(c) {
			service.SkipSmartRetryGroup(c, common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
		}
		if raw, ok := c.Get("relay_resource_attempt_info"); ok && shouldRecordPerfFailure(lastErr) {
			recordRelayAttemptFailure(c, raw.(*relaycommon.RelayInfo))
		}
		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), lastErr)
		if !smart || !configurableResourceAllowsReplay(c, resource) || !shouldRetryWithTokenGroupPlan(c, lastErr, 1, true) || !service.WaitBeforeSmartRetry(c) {
			break
		}
	}
	service.MarkRetryRouteFinal(c, false, "failed")
	if lastErr != nil && !c.Writer.Written() {
		c.JSON(lastErr.StatusCode, gin.H{"error": lastErr.ToOpenAIError()})
	}
}

func configurableResourceAllowsReplay(c *gin.Context, resource *configurable.ResourceConfig) bool {
	// Existing task/asset IDs belong to their original provider. Pre-requests may
	// create resources even when the main request is rejected, so never replay them.
	if resource.AssetLibrary || resource.DisableReplay || len(c.Params) > 0 || len(resource.PathParams) > 0 {
		return false
	}
	for _, pre := range resource.PreRequests {
		if pre.Upstream.Method != http.MethodGet && pre.Upstream.Method != http.MethodHead {
			return false
		}
	}
	return true
}

type smartResourceCandidate struct {
	channel  *model.Channel
	profile  *configurable.Profile
	resource *configurable.ResourceConfig
	group    string
	rank     int
}

// Resource endpoints can have no model name (asset listing, for example). Build
// their capability-filtered candidate list once, preserving the same smart group
// ordering, failed-key exclusion and group limits as the regular relay selector.
func selectSmartConfigurableResourceRoute(c *gin.Context, profileID, resourceID string) (*model.Channel, *configurable.Profile, *configurable.ResourceConfig, error) {
	if _, specific := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); specific {
		return selectConfigurableResourceRoute(c, profileID, resourceID)
	}
	const cacheKey = "relay_smart_resource_candidates"
	var candidates []smartResourceCandidate
	if cached, ok := c.Get(cacheKey); ok {
		candidates = cached.([]smartResourceCandidate)
	} else {
		var channels []model.Channel
		if err := model.DB.Where("type = ? AND status = ?", constant.ChannelTypeConfigurable, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
			return nil, nil, nil, err
		}
		for rank, group := range configurableResourceCandidateGroups(c) {
			for i := range channels {
				channel := &channels[i]
				var profile *configurable.Profile
				var resource *configurable.ResourceConfig
				var ok bool
				if profileID != "" && resourceID != "" {
					if !configurableResourceChannelMatches(channel, profileID, resourceID, group) {
						continue
					}
					profile, ok = configurable.GetProfile(profileID)
					if ok {
						resource, ok = profile.ResourceByID(resourceID)
					}
				} else {
					profile, resource, ok = configurableResourceForChannelEndpoint(channel, c.Request.Method, c.Request.URL.Path, group)
				}
				if !ok || !configurableResourceChannelAbilityEnabled(channel, group, configurableResourceRequestModel(c, resource)) {
					continue
				}
				candidates = append(candidates, smartResourceCandidate{channel: channel, profile: profile, resource: resource, group: group, rank: rank})
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].rank != candidates[j].rank {
				return candidates[i].rank < candidates[j].rank
			}
			if candidates[i].channel.GetPriority() != candidates[j].channel.GetPriority() {
				return candidates[i].channel.GetPriority() > candidates[j].channel.GetPriority()
			}
			return candidates[i].channel.Id < candidates[j].channel.Id
		})
		c.Set(cacheKey, candidates)
	}
	if len(candidates) == 0 {
		_, _, _, err := selectConfigurableResourceRoute(c, profileID, resourceID)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	for _, candidate := range candidates {
		if !service.SmartRetryGroupAvailable(c, candidate.group) {
			continue
		}
		if !configurableResourceHasIndependentCredentials(candidate.channel, candidate.resource) {
			filter := service.SmartRetryChannelFilter(c, configurableResourceRequestModel(c, candidate.resource))
			if filter != nil && !filter(candidate.channel) {
				continue
			}
		}
		common.SetContextKey(c, constant.ContextKeyAutoGroup, candidate.group)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, candidate.group)
		return candidate.channel, candidate.profile, candidate.resource, nil
	}
	return nil, nil, nil, fmt.Errorf("no untried configurable resource channel for %s %s", c.Request.Method, c.Request.URL.Path)
}

// Asset management is authorized by user ownership. Other resources still
// require an allowed model, including fixed/default models from their profile.
func authorizeConfigurableResourceModel(c *gin.Context, resource *configurable.ResourceConfig) bool {
	if resource.AssetLibrary {
		return true
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return true
	}
	name := configurableResourceRequestModel(c, resource)
	limits, _ := c.Get("token_model_limit")
	allowed, _ := limits.(map[string]bool)
	if name != "" {
		if _, ok := allowed[ratio_setting.FormatMatchingModelName(name)]; ok {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "model_not_allowed", "message": "resource access requires an explicitly authorized model"}})
	return false
}
