package controller

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const assetAccessContextKey = "asset_access_request"

var errAssetStateUnavailable = errors.New("asset ownership state is unavailable; contact the administrator")

type assetOperation struct{ kind, action string }
type assetReference struct{ kind, id string }
type assetAccessRequest struct {
	op       assetOperation
	refs     []assetReference
	bindings []model.AssetBinding
	channel  *model.Channel
	profile  *configurable.Profile
	resource *configurable.ResourceConfig
	scope    string
	project  string
	groupID  string
	input    gjson.Result
	list     *assetListPagination
}

// Every asset resource, including inherited profiles, uses this shared policy.
// Unrecognized future asset operations fail closed until their semantics are
// specified here; provider transport/response mappings stay in the profiles.
func assetResourceOperation(id string) (assetOperation, bool) {
	operations := map[string]assetOperation{
		"asset_groups_create": {"group", "create"}, "asset_groups_list": {"group", "list"},
		"asset_groups_get": {"group", "get"}, "asset_group_detail": {"group", "get"},
		"asset_groups_update": {"group", "update"}, "asset_groups_delete": {"group", "delete"},
		"assets_create": {"asset", "create"}, "assets_upload": {"asset", "create"},
		"material_assets": {"asset", "create"}, "max_assets_create": {"asset", "create"},
		"assets_list": {"asset", "list"}, "material_assets_list": {"asset", "list"},
		"assets_get": {"asset", "get"}, "asset_detail": {"asset", "get"},
		"asset_query": {"asset", "get"}, "material_asset_detail": {"asset", "get"}, "max_assets_get": {"asset", "get"},
		"assets_update": {"asset", "update"}, "assets_delete": {"asset", "delete"},
		"asset_delete": {"asset", "delete"}, "material_asset_delete": {"asset", "delete"},
		"liveness_session_create": {"session", "create"}, "liveness_group_exchange": {"session", "exchange"},
	}
	op, ok := operations[id]
	return op, ok
}

func assetAccessError(c *gin.Context, status int, code string, err error) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": err.Error()}})
}

// Reject ambiguous objects before different JSON parsers can disagree about
// which occurrence of an ID/filter is being authorized and forwarded.
func validateAssetJSON(value gjson.Result, depth int) error {
	if depth > 128 {
		return fmt.Errorf("asset JSON nesting exceeds 128 levels")
	}
	if !value.IsObject() && !value.IsArray() {
		return nil
	}
	keys := map[string]bool{}
	var err error
	value.ForEach(func(key, item gjson.Result) bool {
		if value.IsObject() {
			name := strings.ToLower(key.String())
			if keys[name] {
				err = fmt.Errorf("duplicate asset JSON field")
				return false
			}
			keys[name] = true
		}
		err = validateAssetJSON(item, depth+1)
		return err == nil
	})
	return err
}

func assetRequestInput(c *gin.Context) (gjson.Result, error) {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return gjson.Parse("{}"), nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return gjson.Result{}, err
	}
	body, err := storage.Bytes()
	if err != nil {
		return gjson.Result{}, err
	}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return gjson.Result{}, fmt.Errorf("asset body must be a JSON object")
	}
	input := gjson.ParseBytes(body)
	return input, validateAssetJSON(input, 0)
}

func assetProjectField(name string) bool {
	return strings.EqualFold(name, "ProjectName") || strings.EqualFold(name, "project_name")
}

// Check every supported spelling before forwarding a single canonical field.
// Blank projects inherit the resource's project, then the channel default.
func assetRequestProject(query url.Values, inputs ...gjson.Result) (string, error) {
	project, seen := "", false
	add := func(value string) error {
		if seen {
			return fmt.Errorf("ProjectName must be supplied only once across body and query")
		}
		seen, project = true, strings.TrimSpace(value)
		return nil
	}
	for _, input := range inputs {
		var err error
		input.ForEach(func(key, value gjson.Result) bool {
			if !assetProjectField(key.String()) {
				return true
			}
			if value.Type != gjson.String {
				err = fmt.Errorf("ProjectName must be a string")
			} else {
				err = add(value.String())
			}
			return err == nil
		})
		if err != nil {
			return "", err
		}
	}
	for key, values := range query {
		if assetProjectField(key) {
			for _, value := range values {
				if err := add(value); err != nil {
					return "", err
				}
			}
		}
	}
	return project, nil
}

func newAssetAccessRequest(c *gin.Context, resource *configurable.ResourceConfig) (*assetAccessRequest, error) {
	op, ok := assetResourceOperation(resource.ID)
	if !ok {
		return nil, errUnsupportedAssetOperation
	}
	input, err := assetRequestInput(c)
	if err != nil {
		return nil, err
	}
	a := &assetAccessRequest{op: op, input: input}
	add := func(kind, id string) error {
		if id == "" {
			return nil
		}
		if kind != "session" && !validAssetHandle(id) {
			return fmt.Errorf("invalid asset resource handle")
		}
		for _, ref := range a.refs {
			if ref.kind == kind && ref.id == id {
				return nil
			}
		}
		a.refs = append(a.refs, assetReference{kind, id})
		return nil
	}
	read := func(kind string, keys ...string) error {
		for _, key := range keys {
			for name, value := range input.Map() {
				if !strings.EqualFold(name, key) {
					continue
				}
				if value.Type != gjson.String {
					return fmt.Errorf("%s must be a string", key)
				}
				if err := add(kind, value.String()); err != nil {
					return err
				}
			}
			for name, values := range c.Request.URL.Query() {
				if strings.EqualFold(name, key) {
					for _, v := range values {
						if err := add(kind, v); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}
	for _, param := range c.Params {
		if err := add(op.kind, param.Value); err != nil {
			return nil, err
		}
	}
	if err := read("group", "GroupId", "group_id"); err != nil {
		return nil, err
	}
	if err := read("asset", "asset_id", "AssetId"); err != nil {
		return nil, err
	}
	if err := read("task", "task_id", "TaskId"); err != nil {
		return nil, err
	}
	if err := read(op.kind, "Id", "id"); err != nil {
		return nil, err
	}
	if op.kind == "session" && op.action == "exchange" {
		if err := read("session", "BytedToken", "bytedToken", "byted_token"); err != nil {
			return nil, err
		}
	}
	for _, filterName := range []string{"Filter", "filter"} {
		filters := []gjson.Result{input.Get(filterName)}
		for _, raw := range c.QueryArray(filterName) {
			if !gjson.Valid(raw) {
				return nil, fmt.Errorf("Filter must contain valid JSON")
			}
			filters = append(filters, gjson.Parse(raw))
		}
		for _, filter := range filters {
			if !filter.Exists() {
				continue
			}
			if !filter.IsObject() {
				return nil, fmt.Errorf("Filter must be an object")
			}
			if err := validateAssetJSON(filter, 0); err != nil {
				return nil, err
			}
			for field, kind := range map[string]string{"GroupIds": "group", "Ids": op.kind, "AssetIds": "asset"} {
				ids := filter.Get(field)
				if ids.Exists() && !ids.IsArray() {
					return nil, fmt.Errorf("Filter.%s must be an array", field)
				}
				for _, id := range ids.Array() {
					if id.Type != gjson.String || id.String() == "" {
						return nil, fmt.Errorf("Filter.%s must contain resource handles", field)
					}
					if err := add(kind, id.String()); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	a.project, err = assetRequestProject(c.Request.URL.Query(), input)
	if err != nil {
		return nil, err
	}
	if op.action == "get" || op.action == "update" || op.action == "delete" || op.action == "exchange" {
		found := false
		primary := ""
		for _, ref := range a.refs {
			found = found || ref.kind == op.kind || (op.kind == "asset" && ref.kind == "task")
			if ref.kind == op.kind {
				if primary != "" && primary != ref.id {
					return nil, fmt.Errorf("conflicting resource handles")
				}
				primary = ref.id
			}
		}
		if !found {
			return nil, fmt.Errorf("resource handle is required")
		}
	}
	return a, nil
}

func assetAccountScope(ch *model.Channel, profile *configurable.Profile) (string, error) {
	credential, auth := ch.Key, "channel_key"
	if cfg := assetLibrary(ch); cfg != nil && cfg.Backend != "" && cfg.Backend != "inherit" && cfg.Backend != "disabled" {
		auth = cfg.AuthMode
		if auth == "api_key" || auth == "aksk" {
			credential = ch.AssetSecret
		}
	}
	if auth == "channel_key" && ch.ChannelInfo.IsMultiKey {
		return "", fmt.Errorf("asset libraries require dedicated credentials for multi-key channels")
	}
	endpoint := profile.ID + "\x00" + strings.TrimRight(assetBaseURL(ch), "/") + "\x00" + auth
	endpointKey := fmt.Sprintf("%x", sha256.Sum256([]byte(endpoint)))
	// Seed with the legacy scope only on first use. Credentials no longer
	// participate in matching once this endpoint has a persistent identity.
	initialScope := fmt.Sprintf("%x", sha256.Sum256([]byte(endpoint+"\x00"+credential)))
	scope, err := model.ResolveAssetLibraryScope(ch.Id, endpointKey, initialScope)
	if err != nil {
		common.SysError("resolve asset library scope: " + err.Error())
		return "", errAssetStateUnavailable
	}
	return scope, nil
}

// Resolve every supplied ID before selecting an upstream. A missing/revoked
// binding never falls back to the currently highest-priority supplier account.
func resolveAssetAccessRoute(c *gin.Context, profileID, resourceID string) (*assetAccessRequest, error) {
	var resource *configurable.ResourceConfig
	if profileID != "" && resourceID != "" {
		if profile, ok := configurable.GetProfile(profileID); ok {
			resource, _ = profile.ResourceByID(resourceID)
		}
	} else {
		_, resource, _ = configurable.MatchResource(c.Request.Method, c.Request.URL.Path)
	}
	if resource == nil || !resource.AssetLibrary {
		return nil, nil
	}
	a, err := newAssetAccessRequest(c, resource)
	if err != nil {
		return nil, err
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if userID <= 0 {
		return nil, model.ErrAssetNotOwned
	}
	for _, ref := range a.refs {
		bindings, err := model.FindAssetBindings(userID, ref.kind, ref.id)
		if err != nil {
			common.SysError("read asset binding: " + err.Error())
			return nil, errAssetStateUnavailable
		}
		// Some async upload APIs initially return only a task handle. It has
		// the same ownership requirements and may be used to resolve the asset.
		if len(bindings) == 0 && ref.kind == "asset" && a.op.action == "get" {
			bindings, err = model.FindAssetBindings(userID, "task", ref.id)
			if err != nil {
				common.SysError("read asset task binding: " + err.Error())
				return nil, errAssetStateUnavailable
			}
		}
		var matches []model.AssetBinding
		for _, binding := range bindings {
			if a.channel != nil && binding.ChannelID != a.channel.Id {
				continue
			}
			ch, err := model.GetChannelById(binding.ChannelID, true)
			if err != nil {
				continue
			}
			profile, candidate, ok := assetResourceChannel(c, ch)
			if !ok {
				continue
			}
			scope, err := assetAccountScope(ch, profile)
			if errors.Is(err, errAssetStateUnavailable) {
				return nil, err
			}
			if err != nil || scope != binding.Scope || (a.project != "" && binding.Project != "" && a.project != binding.Project) {
				continue
			}
			matches = append(matches, binding)
			a.profile, a.resource = profile, candidate
		}
		if len(matches) != 1 {
			return nil, model.ErrAssetNotOwned
		}
		binding := matches[0]
		a.channel, err = model.GetChannelById(binding.ChannelID, true)
		if err != nil {
			common.SysError("read bound asset channel: " + err.Error())
			return nil, errAssetStateUnavailable
		}
		a.scope = binding.Scope
		if a.project == "" {
			a.project = binding.Project
		}
		if ref.kind == "group" {
			a.groupID = binding.CanonicalID
		}
		a.bindings = append(a.bindings, binding)
	}
	c.Set(assetAccessContextKey, a)
	return a, nil
}

// Ownership is user-scoped. An existing handle selects its original endpoint;
// token model/group policies are not an additional asset permission boundary.
func assetResourceChannel(c *gin.Context, ch *model.Channel) (*configurable.Profile, *configurable.ResourceConfig, bool) {
	if raw, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
		id, err := parseSpecificChannelID(raw)
		if err != nil || id != ch.Id {
			return nil, nil, false
		}
	}
	profile, resource, ok := configurableResourceForChannelEndpoint(ch, c.Request.Method, c.Request.URL.Path, "")
	return profile, resource, ok && resource.AssetLibrary
}

func prepareAssetAccess(c *gin.Context, ch *model.Channel, profile *configurable.Profile, resource *configurable.ResourceConfig) error {
	if !resource.AssetLibrary {
		return nil
	}
	raw, ok := c.Get(assetAccessContextKey)
	if !ok {
		return fmt.Errorf("asset access policy was not initialized")
	}
	a := raw.(*assetAccessRequest)
	scope, err := assetAccountScope(ch, profile)
	if err != nil {
		return err
	}
	if a.channel != nil && (a.channel.Id != ch.Id || a.scope != scope) {
		return model.ErrAssetNotOwned
	}
	a.channel, a.profile, a.resource, a.scope = ch, profile, resource, scope
	if a.project == "" && ch.GetSetting().Protocol != nil {
		a.project = strings.TrimSpace(ch.GetSetting().Protocol.ProjectName)
	}
	if a.op.action == "list" {
		a.list, err = newAssetListPagination(c, a)
	}
	return err
}

func assetResponseSuccessful(body []byte) bool {
	if !gjson.ValidBytes(body) {
		return false
	}
	for _, path := range []string{"error", "ResponseMetadata.Error"} {
		if value := gjson.GetBytes(body, path); value.Exists() && value.Type != gjson.Null && value.Raw != "{}" && value.String() != "" {
			return false
		}
	}
	if value := gjson.GetBytes(body, "success"); value.Exists() && value.Type == gjson.False {
		return false
	}
	if value := gjson.GetBytes(body, "code"); value.Exists() && value.String() != "0" && value.String() != "200" {
		return false
	}
	return true
}

func assetResponseStrings(body []byte, fields ...string) []string {
	var result []string
	for _, root := range []string{"", "Result.", "data.", "result."} {
		for _, field := range fields {
			if value := gjson.GetBytes(body, root+field); value.Type == gjson.String && value.String() != "" {
				result = append(result, value.String())
			}
		}
	}
	return result
}

func rememberAssetResponse(c *gin.Context, resource *configurable.ResourceConfig, body []byte) error {
	if !resource.AssetLibrary {
		return nil
	}
	raw, ok := c.Get(assetAccessContextKey)
	if !ok {
		return fmt.Errorf("asset access policy was not initialized")
	}
	a := raw.(*assetAccessRequest)
	// REST delete endpoints may return 204 with no JSON body.
	if !assetResponseSuccessful(body) && !(a.op.action == "delete" && strings.TrimSpace(string(body)) == "") {
		return nil
	}
	if a.op.action == "delete" {
		for _, binding := range a.bindings {
			if binding.Kind == a.op.kind {
				if err := model.InvalidateAssetBindings(binding, a.op.kind == "group"); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if a.op.action == "list" || a.op.action == "update" {
		return nil
	}
	kind := a.op.kind
	if a.op.action == "exchange" {
		kind = "group"
	}
	fields := []string{"Id", "id"}
	switch kind {
	case "session":
		fields = []string{"BytedToken", "bytedToken", "byted_token"}
	case "group":
		fields = append(fields, "GroupId", "group_id", "upstream_group_id")
	case "asset":
		fields = append(fields, "asset_id", "AssetId", "upstream_asset_id")
	}
	ids := assetResponseStrings(body, fields...)
	binding := model.AssetBinding{ChannelID: a.channel.Id, UserID: common.GetContextKeyInt(c, constant.ContextKeyUserId), Backend: a.profile.ID, Scope: a.scope, Kind: kind, Project: a.project, GroupID: a.groupID}
	if kind == "session" {
		binding.ExpiresAt = common.GetTimestamp() + 30*60
	}
	if len(ids) > 0 {
		binding.CanonicalID = ids[0]
	}
	if a.op.action == "get" {
		for _, existing := range a.bindings {
			if existing.Kind == kind || (kind == "asset" && existing.Kind == "task") {
				binding.CanonicalID, binding.GroupID = existing.CanonicalID, existing.GroupID
				break
			}
		}
	}
	for _, id := range ids {
		if err := model.SaveAssetBinding(binding, id); err != nil {
			return err
		}
	}
	if kind == "asset" {
		binding.Kind = "task"
		for _, id := range assetResponseStrings(body, "task_id", "TaskId") {
			if err := model.SaveAssetBinding(binding, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func respondAssetAccessError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrAssetNotOwned):
		assetAccessError(c, http.StatusNotFound, "asset_not_found", model.ErrAssetNotOwned)
	case errors.Is(err, errUnsupportedAssetOperation):
		assetAccessError(c, http.StatusNotImplemented, "unsupported_asset_operation", err)
	case errors.Is(err, errAssetStateUnavailable):
		assetAccessError(c, http.StatusInternalServerError, "asset_state_unavailable", errAssetStateUnavailable)
	default:
		assetAccessError(c, http.StatusBadRequest, "invalid_asset_request", err)
	}
}

func rememberAssetPreGroup(c *gin.Context, resource *configurable.ResourceConfig, preID string, result map[string]any) error {
	if !resource.AssetLibrary || preID != "asset_group" {
		return nil
	}
	raw, ok := c.Get(assetAccessContextKey)
	if !ok {
		return fmt.Errorf("asset access policy was not initialized")
	}
	a := raw.(*assetAccessRequest)
	id, _ := result["id"].(string)
	if id == "" {
		return fmt.Errorf("automatic asset group did not return an ID")
	}
	binding := model.AssetBinding{ChannelID: a.channel.Id, UserID: common.GetContextKeyInt(c, constant.ContextKeyUserId), Backend: a.profile.ID, Scope: a.scope, Kind: "group", Project: a.project}
	if err := model.SaveAssetBinding(binding, id); err != nil {
		return err
	}
	a.groupID = id
	return nil
}
