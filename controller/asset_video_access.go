package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Asset URIs must use their owner's original channel even when video model
// routing would otherwise select a different provider or retry another account.
func lockAssetVideoChannel(c *gin.Context, info *relaycommon.RelayInfo) error {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	body, err := storage.Bytes()
	if err != nil {
		return err
	}
	if !gjson.ValidBytes(body) {
		// Match the relay's reusable form parser so multipart and URL-encoded
		// requests cannot bypass the same ownership check applied to JSON.
		var fields map[string]any
		if err := common.UnmarshalBodyReusable(c, &fields); err != nil {
			return err
		}
		body, err = common.Marshal(fields)
		if err != nil {
			return err
		}
	}
	var ids []string
	var visit func(gjson.Result, int, bool)
	tooDeep := false
	visit = func(value gjson.Result, depth int, media bool) {
		if depth > 128 {
			tooDeep = true
			return
		}
		if value.Type == gjson.String {
			if !media {
				return
			}
			uri := strings.TrimSpace(value.String())
			if strings.HasPrefix(strings.ToLower(uri), "asset://") {
				ids = append(ids, uri[len("asset://"):])
			} else if nested := gjson.Parse(value.String()); nested.IsObject() || nested.IsArray() {
				// Form metadata/images may themselves be JSON-encoded strings.
				visit(nested, depth+1, media)
			}
			return
		}
		if !value.IsArray() && !value.IsObject() {
			return
		}
		value.ForEach(func(key, item gjson.Result) bool {
			if value.IsArray() {
				visit(item, depth+1, media)
				return true
			}
			// Inspect media fields, including nested/form metadata. Ordinary
			// labels and negative prompts can legitimately contain URI text.
			childMedia := false
			switch strings.ToLower(key.String()) {
			case "image", "images", "image_url", "image_urls", "video", "videos", "video_url", "video_urls", "audio", "audio_url", "audio_urls", "url", "input_reference", "reference_images", "content", "metadata":
				childMedia = true
			}
			if key.String() != "prompt" && key.String() != "text" {
				visit(item, depth+1, childMedia)
			}
			return true
		})
	}
	visit(gjson.ParseBytes(body), 0, false)
	if tooDeep {
		return fmt.Errorf("asset reference nesting exceeds 128 levels")
	}
	if len(ids) == 0 {
		return nil
	}
	input := gjson.ParseBytes(body)
	projects := []gjson.Result{input}
	input.ForEach(func(key, value gjson.Result) bool {
		if strings.EqualFold(key.String(), "metadata") {
			if value.Type == gjson.String {
				value = gjson.Parse(value.String())
			}
			projects = append(projects, value)
		}
		return true
	})
	requestedProject, err := assetRequestProject(nil, projects...)
	if err != nil {
		return err
	}
	var selected *model.Channel
	selectedGroup := ""
	project := requestedProject
	aliases := map[string]string{}
	protocolFilter := service.BuildProtocolChannelFilter(&service.RetryParam{Ctx: c, ModelName: info.OriginModelName})
	// Video authentication already ran in Distribute. Asset access only adds
	// user ownership; the original channel still needs an enabled video model.
	// Prefer the selected billing group when that channel also serves it.
	preferredGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	if preferredGroup == "" {
		preferredGroup = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	for _, id := range ids {
		bindings, err := model.FindAssetBindings(info.UserId, "asset", id)
		if err != nil {
			common.SysError("read video asset binding: " + err.Error())
			return errAssetStateUnavailable
		}
		matches := 0
		var candidate *model.Channel
		candidateGroup := ""
		candidateProject := ""
		canonicalID := ""
		for _, binding := range bindings {
			if selected != nil && binding.ChannelID != selected.Id {
				continue
			}
			if raw, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
				requested, err := parseSpecificChannelID(raw)
				if err != nil || requested != binding.ChannelID {
					continue
				}
			}
			ch, err := model.GetChannelById(binding.ChannelID, true)
			if err != nil || ch.Status != common.ChannelStatusEnabled {
				continue
			}
			if protocolFilter != nil && !protocolFilter(ch) {
				continue
			}
			profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
			if !ok {
				continue
			}
			scope, err := assetAccountScope(ch, profile)
			if errors.Is(err, errAssetStateUnavailable) {
				return err
			}
			if err != nil || scope != binding.Scope {
				continue
			}
			if binding.Project != "" && ((requestedProject != "" && requestedProject != binding.Project) || (project != "" && project != binding.Project)) {
				continue
			}
			channelGroup := ""
			for _, group := range ch.GetGroups() {
				if configurableResourceChannelAbilityEnabled(ch, group, info.OriginModelName) {
					if channelGroup == "" || group == preferredGroup {
						channelGroup = group
					}
				}
			}
			if channelGroup == "" {
				continue
			}
			matches++
			candidate, candidateGroup = ch, channelGroup
			candidateProject = binding.Project
			if profile.ID == "seedance-tgxmaas" {
				canonicalID = binding.CanonicalID
			}
		}
		if matches != 1 {
			return model.ErrAssetNotOwned
		}
		selected, selectedGroup = candidate, candidateGroup
		if canonicalID != "" {
			aliases[id] = canonicalID
		}
		if candidateProject != "" {
			project = candidateProject
		}
	}
	if selected == nil {
		return nil
	}
	if locked, ok := info.LockedChannel.(*model.Channel); ok && locked != nil && locked.Id != selected.Id {
		return fmt.Errorf("task and asset belong to different channels")
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, selectedGroup)
	common.SetContextKey(c, constant.ContextKeyAutoGroup, selectedGroup)
	if apiErr := middleware.SetupContextForSelectedChannel(c, selected, info.OriginModelName); apiErr != nil {
		return apiErr.Err
	}
	info.InitChannelMeta(c)
	info.UsingGroup = selectedGroup
	info.LockedChannel = selected
	info.AssetAliases = aliases
	if project != "" {
		info.AssetProject = common.GetPointer(project)
	}
	return nil
}
