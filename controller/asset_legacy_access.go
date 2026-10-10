package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type assetLookupError struct {
	status int
	header http.Header
	body   []byte
}

var errAssetLookupFailed = errors.New("asset verification is temporarily unavailable")

func assetLookupFailure(err error) error {
	common.SysError("verify historical asset: " + err.Error())
	return errAssetLookupFailed
}

func (e *assetLookupError) Error() string { return "upstream asset lookup failed" }

func respondAssetLookupError(c *gin.Context, err error) bool {
	if errors.Is(err, errAssetLookupFailed) {
		assetInternalError(c, http.StatusBadGateway, "asset_lookup_failed", err)
		return true
	}
	var lookup *assetLookupError
	if !errors.As(err, &lookup) {
		return false
	}
	for _, name := range []string{"X-Request-Id", "Retry-After"} {
		if value := lookup.header.Get(name); value != "" {
			c.Header(name, value)
		}
	}
	contentType := lookup.header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	body, err := publicAssetResponse(lookup.body)
	if err != nil {
		assetInternalError(c, http.StatusBadGateway, "asset_response_failed", err)
		return true
	}
	c.Data(lookup.status, contentType, body)
	return true
}

func canClaimLegacyAsset(ref assetReference, registered bool) error {
	if ref.kind != "asset" && ref.kind != "group" && ref.kind != "task" {
		return model.ErrAssetNotOwned // Unknown/expired bearer sessions cannot be recovered.
	}
	if !validAssetHandle(ref.id) {
		return model.ErrAssetNotOwned
	}
	if registered {
		return model.ErrAssetNotOwned
	}
	return nil
}

func legacyAssetDetailResource(c *gin.Context, profile *configurable.Profile, ref assetReference, preferred *configurable.ResourceConfig) *configurable.ResourceConfig {
	kind := ref.kind
	if kind == "task" {
		kind = "asset"
	}
	if preferred != nil {
		if op, ok := assetResourceOperation(preferred.ID); ok && op.kind == kind && op.action == "get" {
			return preferred
		}
	}
	// Service-inference has separate v1 and v2 libraries. Keep an explicitly
	// requested v2 API family when looking up a video reference.
	for _, prefix := range []string{"/v2/sd-max/", "/v2/db-sd-max/"} {
		if kind == "asset" && strings.HasPrefix(c.Request.URL.Path, prefix) {
			if resource, ok := profile.ResourceForEndpoint(http.MethodGet, prefix+"assets/"+ref.id); ok {
				return resource
			}
		}
	}
	for i := range profile.Resources {
		resource := &profile.Resources[i]
		if op, ok := assetResourceOperation(resource.ID); resource.AssetLibrary && ok && op.kind == kind && op.action == "get" {
			return resource
		}
	}
	return nil
}

// Only a detail response can claim a historical handle. Never claim everything
// returned by a shared account's list endpoint, or fall through other suppliers.
func claimLegacyAsset(c *gin.Context, ch *model.Channel, profile *configurable.Profile, preferred *configurable.ResourceConfig, ref assetReference, project string) (model.AssetBinding, error) {
	var empty model.AssetBinding
	resource := legacyAssetDetailResource(c, profile, ref, preferred)
	if resource == nil {
		return empty, errUnsupportedAssetOperation
	}
	scope, err := assetAccountScopeForRequest(c, ch, profile)
	if err != nil {
		return empty, err
	}
	if project == "" {
		project = assetDefaultProject(ch, profile)
	}
	input := map[string]any{"Id": ref.id}
	input[ref.kind+"_id"] = ref.id
	if project != "" {
		input["ProjectName"] = project
	}
	modelName := configurableResourceRequestModel(c, nil)
	if modelName != "" && resource.Public.Method != http.MethodGet {
		input["model"] = modelName
	}
	body, err := common.Marshal(input)
	if err != nil {
		return empty, err
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	path := resource.Public.Path
	params := gin.Params{{Key: "id", Value: ref.id}, {Key: "asset_id", Value: ref.id}, {Key: "group_id", Value: ref.id}}
	if ref.kind == "task" && !strings.Contains(path, "{") {
		// Body-based task queries must not also send the task as an asset ID.
		params = nil
	}
	for _, param := range params {
		path = strings.ReplaceAll(path, "{"+param.Key+"}", param.Value)
	}
	request, err := http.NewRequestWithContext(ctx, resource.Public.Method, path, bytes.NewReader(body))
	if err != nil {
		return empty, err
	}
	request.Header.Set("Content-Type", "application/json")
	query := request.URL.Query()
	if modelName != "" {
		query.Set("model", modelName)
	}
	request.URL.RawQuery = query.Encode()
	// Use a fresh context so probe body/path conversions cannot mutate the
	// caller's operation, reusable body, credentials or video billing state.
	probe := &gin.Context{Request: request, Params: params}
	defer common.CleanupBodyStorage(probe)
	common.SetContextKey(probe, constant.ContextKeyChannelKey, ch.Key)
	probe.Set(middleware.ContextKeyConfigurableResourceProfileID, profile.ID)
	probe.Set(middleware.ContextKeyConfigurableResourceID, resource.ID)
	probe.Set(assetAccessContextKey, &assetAccessRequest{project: project})
	if err := prepareTgxMaasAssetRequest(probe, ch, profile, resource); err != nil {
		return empty, err
	}
	upstream, err := buildConfigurableResourceRequest(probe, ch, resource)
	if err != nil {
		return empty, err
	}
	client, err := service.GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil {
		return empty, assetLookupFailure(err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(upstream)
	if err != nil {
		return empty, assetLookupFailure(err)
	}
	defer response.Body.Close()
	body, err = io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil {
		return empty, assetLookupFailure(err)
	}
	if len(body) > 16<<20 {
		return empty, fmt.Errorf("upstream asset response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !assetResponseSuccessful(body) {
		return empty, &assetLookupError{status: response.StatusCode, header: response.Header, body: body}
	}
	fields := []string{"Id", "id", "asset_id", "AssetId", "upstream_asset_id"}
	if ref.kind == "group" {
		fields = []string{"Id", "id", "GroupId", "group_id", "upstream_group_id"}
	}
	ids := assetResponseStrings(body, fields...)
	if len(ids) == 0 {
		return empty, model.ErrAssetNotOwned // A generic 200/{} is not proof of existence.
	}
	kind := ref.kind
	if kind == "task" {
		kind = "asset"
	}
	binding := model.AssetBinding{ChannelID: ch.Id, UserID: common.GetContextKeyInt(c, constant.ContextKeyUserId), Backend: profile.ID, Scope: scope, Kind: kind, CanonicalID: ids[0], Project: project}
	if err := applyAssetResponseMetadata(ctx, &binding, body); err != nil {
		return empty, err
	}
	var bindings []model.AssetBinding
	add := func(kind, id string) error {
		if !validAssetHandle(id) {
			return model.ErrAssetNotOwned
		}
		item := binding
		item.Kind, item.ID = kind, id
		bindings = append(bindings, item)
		return nil
	}
	for _, id := range ids {
		if err := add(kind, id); err != nil {
			return empty, err
		}
	}
	if kind == "asset" {
		for _, id := range assetResponseStrings(body, "task_id", "TaskId") {
			if err := add("task", id); err != nil {
				return empty, err
			}
		}
	}
	if err := add(ref.kind, ref.id); err != nil {
		return empty, err
	}
	if err := model.ClaimLegacyAssetBindingsContext(ctx, bindings); err != nil {
		if errors.Is(err, model.ErrAssetNotOwned) {
			return empty, err
		}
		common.SysError("claim legacy asset: " + err.Error())
		return empty, errAssetStateUnavailable
	}
	binding.Kind, binding.ID = ref.kind, ref.id
	return binding, nil
}
