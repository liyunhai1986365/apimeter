package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const yhAssetContextKey = "yh_asset_request"

type yhAssetRequest struct {
	method, path, operation, id, name, groupType string
	filter                                       gjson.Result
	binding                                      model.AssetBinding
	session                                      model.AssetBinding
}

func prepareYHAssetRequest(c *gin.Context, ch *model.Channel, resource *configurable.ResourceConfig) error {
	a := c.MustGet(assetAccessContextKey).(*assetAccessRequest)
	if a.project != "" && a.project != "default" {
		return fmt.Errorf("YH assets only support the default project")
	}
	s := &yhAssetRequest{method: resource.Upstream.Method, path: resource.Upstream.Path, operation: resource.ID,
		binding: model.AssetBinding{ChannelID: ch.Id, UserID: common.GetContextKeyInt(c, constant.ContextKeyUserId), Backend: a.profile.ID, Scope: a.scope, Project: "default", GroupID: a.groupID}}
	input := a.input.Map()
	for key, entries := range c.Request.URL.Query() {
		if key == "Action" || key == "Version" || assetProjectField(key) {
			continue
		}
		if len(entries) != 1 {
			return fmt.Errorf("duplicate query parameter %s", key)
		}
		if _, exists := input[key]; exists {
			return fmt.Errorf("duplicate parameter %s", key)
		}
		if key == "Filter" || key == "filter" {
			input[key] = gjson.Parse(entries[0])
		} else {
			value, _ := common.Marshal(entries[0])
			input[key] = gjson.ParseBytes(value)
		}
	}
	aliases := map[string]string{"Name": "name", "Description": "description", "desc": "description", "URL": "url", "AssetType": "asset_type", "GroupId": "group_id", "GroupType": "group_type", "Filter": "filter", "Id": "id", "BytedToken": "byted_token", "CallbackURL": "callback_url"}
	values := map[string]json.RawMessage{}
	for key, value := range input {
		if assetProjectField(key) || key == "model" {
			continue
		}
		if mapped, ok := aliases[key]; ok {
			key = mapped
		}
		if key == "asset_id" || key == "AssetId" {
			key = "id"
		}
		if _, ok := values[key]; ok {
			return fmt.Errorf("duplicate %s", key)
		}
		values[key] = json.RawMessage(value.Raw)
	}
	for _, param := range c.Params {
		v, _ := common.Marshal(param.Value)
		if old, ok := values["id"]; ok && string(old) != string(v) {
			return fmt.Errorf("conflicting IDs")
		}
		values["id"] = v
	}
	readString := func(key string) (string, error) {
		v, ok := values[key]
		if !ok {
			return "", nil
		}
		var value string
		if common.Unmarshal(v, &value) != nil {
			return "", fmt.Errorf("%s must be a string", key)
		}
		return value, nil
	}
	var err error
	s.id, err = readString("id")
	if err != nil {
		return err
	}
	s.name, err = readString("name")
	if err != nil {
		return err
	}
	for _, b := range a.bindings {
		if b.Kind == "session" {
			s.session = b
			for _, ref := range a.refs {
				if ref.kind == "session" {
					s.session.ID = ref.id
				}
			}
		}
		if b.Kind == a.op.kind && b.ID == s.id && b.CanonicalID != "" {
			s.id = b.CanonicalID
			s.binding.UploadBatch = b.UploadBatch
			if b.GroupID != "" {
				s.binding.GroupID = b.GroupID
			}
		}
		if b.GroupType == "LivenessFace" {
			s.groupType = b.GroupType
		}
	}
	if s.groupType == "" && a.groupID != "" {
		groups, err := model.FindScopedAssetBindingsContext(c.Request.Context(), ch.Id, s.binding.UserID, a.scope, "group", a.groupID)
		if err != nil {
			return err
		}
		for _, b := range groups {
			if b.GroupType == "LivenessFace" {
				s.groupType = b.GroupType
			}
		}
	}
	if s.groupType == "" {
		s.groupType = "AIGC"
	}
	s.binding.GroupType = s.groupType
	body := map[string]json.RawMessage{}
	copyField := func(key string) {
		if v, ok := values[key]; ok {
			body[key] = v
		}
	}
	setString := func(key, value string) { body[key], _ = common.Marshal(value) }
	setID := func(key, id string) error {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n <= 0 || strconv.FormatInt(n, 10) != id {
			return fmt.Errorf("%s must be a positive decimal group ID", key)
		}
		body[key] = json.RawMessage(strconv.FormatInt(n, 10))
		return nil
	}
	// Reject unsupported business options instead of silently changing their meaning.
	fieldsByOperation := map[string]string{
		"asset_groups_create":     "name description group_type",
		"asset_groups_get":        "id",
		"asset_groups_update":     "id name description",
		"asset_groups_delete":     "id",
		"yh_uploads_create":       "group_id url urls asset_type name upload_mode callback_url callback_secret",
		"yh_uploads_get":          "id",
		"assets_create":           "group_id url asset_type name",
		"assets_get":              "id",
		"assets_content":          "id",
		"assets_update":           "id name",
		"assets_delete":           "id",
		"assets_list":             "filter name group_id PageNumber PageSize MaxResults NextToken page_number page_size max_results next_token",
		"asset_groups_list":       "filter PageNumber PageSize MaxResults NextToken page_number page_size max_results next_token",
		"liveness_session_create": "description callback_url",
		"liveness_group_exchange": "byted_token",
	}
	allowed := map[string]bool{}
	for _, key := range strings.Fields(fieldsByOperation[resource.ID]) {
		allowed[key] = true
	}
	for key := range values {
		if !allowed[key] {
			return fmt.Errorf("YH does not support asset parameter %s", key)
		}
	}
	if callback, err := readString("callback_url"); err != nil || callback != "" {
		return fmt.Errorf("YH callback relay is not supported; use polling")
	}
	if _, err := readString("description"); err != nil {
		return err
	}
	if _, ok := values["callback_secret"]; ok {
		return fmt.Errorf("YH upload callbacks are not supported; use polling")
	}
	switch resource.ID {
	case "asset_groups_create":
		groupType, err := readString("group_type")
		if err != nil || (groupType != "" && groupType != "AIGC") {
			return fmt.Errorf("use H5 authentication to create a LivenessFace group")
		}
		copyField("name")
		copyField("description")
	case "asset_groups_update", "asset_groups_delete":
		if err := setID("id", s.id); err != nil {
			return err
		}
		if resource.ID == "asset_groups_update" {
			if s.groupType == "LivenessFace" {
				return fmt.Errorf("YH does not support updating a real-person group")
			}
			copyField("name")
			copyField("description")
		} else if s.groupType == "LivenessFace" {
			s.path = "/v1/assets/real-person/groups/delete"
		}
	case "asset_groups_get":
		if s.id == "" {
			return fmt.Errorf("Id is required")
		}
		if s.groupType == "LivenessFace" {
			s.path = "/v1/assets/real-person/groups"
		}
	case "yh_uploads_get":
		if !validAssetHandle(s.id) {
			return fmt.Errorf("Id is required")
		}
		if s.binding.UploadBatch {
			s.path = "/v1/assets/batch/get"
			setString("batch_id", s.id)
		} else {
			s.path = "/v1/assets/get"
			setString("id", s.id)
		}
	case "yh_uploads_create":
		if a.groupID == "" {
			return fmt.Errorf("group_id is required; create an asset group first")
		}
		urls := gjson.ParseBytes(values["urls"])
		if urls.Exists() {
			if !urls.IsArray() {
				return fmt.Errorf("urls must be an array")
			}
			for _, url := range urls.Array() {
				if url.Type != gjson.String {
					return fmt.Errorf("urls must contain strings")
				}
			}
			if _, exists := values["url"]; exists {
				return fmt.Errorf("specify either url or urls")
			}
			s.binding.UploadBatch = true
			copyField("urls")
		} else {
			if _, err := readString("url"); err != nil {
				return err
			}
			copyField("url")
		}
		if _, err := readString("upload_mode"); err != nil {
			return err
		}
		if err := setID("group_id", a.groupID); err != nil {
			return err
		}
		for _, key := range []string{"asset_type", "name", "upload_mode"} {
			copyField(key)
		}
	case "assets_create":
		if a.groupID == "" {
			return fmt.Errorf("GroupId is required; create an asset group first")
		}
		if err := setID("group_id", a.groupID); err != nil {
			return err
		}
		for _, key := range []string{"url", "asset_type", "name"} {
			copyField(key)
		}
	case "assets_content":
		if !validAssetHandle(s.id) {
			return fmt.Errorf("Id is required")
		}
		s.path = "/v1/assets/" + s.id + "/content"
	case "assets_get", "assets_update", "assets_delete":
		if !validAssetHandle(s.id) {
			return fmt.Errorf("Id is required")
		}
		setString("id", s.id)
		if resource.ID == "assets_update" {
			copyField("name")
		}
		if resource.ID == "assets_delete" && s.groupType == "LivenessFace" {
			s.path = "/v1/assets/real-person/assets/delete"
		}
	case "assets_list", "asset_groups_list":
		copyField("name")
		if raw, ok := values["filter"]; ok {
			s.filter = gjson.ParseBytes(raw)
			if !s.filter.IsObject() {
				return fmt.Errorf("Filter must be an object")
			}
			for key := range s.filter.Map() {
				if key != "Name" && key != "GroupIds" && key != "GroupType" {
					return fmt.Errorf("YH does not support Filter.%s", key)
				}
			}
			if v := s.filter.Get("Name"); v.Exists() {
				if v.Type != gjson.String {
					return fmt.Errorf("Filter.Name must be a string")
				}
				body["name"] = json.RawMessage(v.Raw)
			}
			if kind := s.filter.Get("GroupType"); kind.Exists() {
				if resource.ID != "asset_groups_list" || kind.Type != gjson.String || (kind.String() != "AIGC" && kind.String() != "LivenessFace") {
					return fmt.Errorf("Filter.GroupType only supports AIGC or LivenessFace on group lists")
				}
			}
			if groups := s.filter.Get("GroupIds"); groups.Exists() {
				if !groups.IsArray() || len(groups.Array()) != 1 || resource.ID != "assets_list" {
					return fmt.Errorf("Filter.GroupIds must contain one group ID")
				}
				if err := setID("group_id", a.groupID); err != nil {
					return err
				}
			}
		}
		if a.groupID != "" && resource.ID == "assets_list" {
			if err := setID("group_id", a.groupID); err != nil {
				return err
			}
		}
	case "liveness_session_create":
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			return err
		}
		s.session = s.binding
		s.session.Kind = "session"
		s.session.GroupType = ""
		s.session.ID = "yh-" + base64.RawURLEncoding.EncodeToString(token[:])
		s.session.SessionName = base64.RawURLEncoding.EncodeToString(token[:6])
		s.session.SessionCreatedAt = common.GetTimestamp()
		s.session.ExpiresAt = s.session.SessionCreatedAt + 30*60
		setString("artist_name", s.session.SessionName)
		if raw, ok := values["description"]; ok {
			body["artist_desc"] = raw
		}
	case "liveness_group_exchange":
		if s.session.ID == "" || s.session.SessionName == "" {
			return model.ErrAssetNotOwned
		}
	default:
		return errUnsupportedAssetOperation
	}
	if raw, ok := body["asset_type"]; ok {
		v := gjson.ParseBytes(raw)
		if v.Type != gjson.String {
			return fmt.Errorf("AssetType must be a string")
		}
	}
	raw, err := common.Marshal(body)
	if err != nil {
		return err
	}
	c.Set(tgxMaasAssetBodyKey, raw)
	c.Set(yhAssetContextKey, s)
	return nil
}

type yhAssetTransport struct {
	client *http.Client
	state  yhAssetRequest
}

func newYHAssetClient(c *gin.Context, client *http.Client) *http.Client {
	s := *c.MustGet(yhAssetContextKey).(*yhAssetRequest)
	return &http.Client{Transport: &yhAssetTransport{client: client, state: s}, Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func yhHTTPResponse(req *http.Request, status int, body []byte, header http.Header) *http.Response {
	if header == nil {
		header = make(http.Header)
	} else {
		header = header.Clone()
	}
	header.Set("Content-Type", "application/json")
	header.Del("Content-Encoding")
	header.Del("Content-Length")
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req}
}

func yhError(req *http.Request, status int, code, message string) *http.Response {
	body, _ := common.Marshal(map[string]any{"error": map[string]string{"code": code, "message": message}})
	return yhHTTPResponse(req, status, body, nil)
}

func readYHResponse(client *http.Client, req *http.Request) (*http.Response, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > 16<<20 {
		return nil, nil, fmt.Errorf("YH asset response exceeds size limit")
	}
	if !gjson.ValidBytes(body) {
		return nil, nil, fmt.Errorf("invalid YH asset response")
	}
	return resp, body, nil
}

func yhFailure(resp *http.Response, body []byte) bool {
	code := gjson.GetBytes(body, "code")
	return resp.StatusCode >= 400 || (code.Exists() && code.String() != "success" && code.String() != "0" && code.String() != "200") || assetResponseHasError(gjson.GetBytes(body, "error"))
}

func yhFailureResponse(req *http.Request, resp *http.Response, body []byte) *http.Response {
	status := resp.StatusCode
	if status < 400 {
		status = http.StatusBadRequest
	}
	code := firstYHString(body, "error.code", "code")
	if code == "" {
		code = "upstream_error"
	}
	message := firstYHString(body, "error.message", "message")
	if message == "" {
		message = http.StatusText(status)
	}
	result := yhError(req, status, code, message)
	for _, key := range []string{"X-Request-Id", "Retry-After"} {
		if v := resp.Header.Get(key); v != "" {
			result.Header.Set(key, v)
		}
	}
	return result
}

func firstYHString(body []byte, paths ...string) string {
	for _, p := range paths {
		if v := gjson.GetBytes(body, p); v.Type == gjson.String && v.String() != "" {
			return v.String()
		}
	}
	return ""
}

func (t *yhAssetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s := t.state
	if s.operation == "liveness_group_exchange" && s.session.SessionGroupID != "" {
		return s.completedSession(req)
	}
	if s.operation == "liveness_session_create" {
		if err := model.ReserveYHSessionName(req.Context(), s.session); err != nil {
			return nil, err
		}
		if err := model.SaveAssetBindingsContext(req.Context(), []model.AssetBinding{s.session}); err != nil {
			return nil, err
		}
	}
	var resp *http.Response
	var body []byte
	var err error
	if s.operation == "liveness_group_exchange" {
		resp, body, err = readYHGroups(t.client, req)
	} else {
		resp, body, err = readYHResponse(t.client, req)
	}
	if err != nil {
		return nil, err
	}
	if yhFailure(resp, body) {
		return yhFailureResponse(req, resp, body), nil
	}
	if s.operation == "yh_uploads_create" || s.operation == "yh_uploads_get" {
		ctx := req.Context()
		if s.operation == "yh_uploads_create" {
			ctx = context.WithoutCancel(ctx)
		}
		normalized, err := s.normalizeUpload(ctx, body)
		if err != nil {
			return nil, err
		}
		return yhHTTPResponse(req, resp.StatusCode, normalized, resp.Header), nil
	}
	data := gjson.GetBytes(body, "data")
	var result any
	switch s.operation {
	case "liveness_session_create":
		link := data.Get("h5_url").String()
		if !strings.HasPrefix(link, "https://") {
			return nil, fmt.Errorf("missing YH H5 link")
		}
		result = map[string]any{"BytedToken": s.session.ID, "H5Link": link}
	case "liveness_group_exchange":
		return s.completeSession(req, data)
	case "asset_groups_list", "asset_groups_get":
		rows, err := yhGroupRows(data, strings.Contains(req.URL.Path, "real-person"))
		if err != nil {
			return nil, err
		}
		if s.operation == "asset_groups_list" {
			other := req.Clone(req.Context())
			other.URL.Path = strings.TrimSuffix(req.URL.Path, "/groups") + "/real-person/groups"
			other.URL.RawPath = ""
			otherResp, otherBody, err := readYHGroups(t.client, other)
			if err != nil {
				return nil, err
			}
			if yhFailure(otherResp, otherBody) {
				return yhFailureResponse(req, otherResp, otherBody), nil
			}
			humans, err := yhGroupRows(gjson.GetBytes(otherBody, "data"), true)
			if err != nil {
				return nil, err
			}
			rows = append(rows, humans...)
		}
		filtered := make([]map[string]any, 0, len(rows))
		seen := map[string]bool{}
		for _, row := range rows {
			id := row["Id"].(string)
			if s.operation == "asset_groups_get" {
				if id == s.id {
					result = row
					break
				}
				continue
			}
			if name := s.filter.Get("Name").String(); name != "" && !strings.Contains(fmt.Sprint(row["Name"]), name) {
				continue
			}
			if kind := s.filter.Get("GroupType").String(); kind != "" && kind != row["GroupType"] {
				continue
			}
			if !seen[id] {
				filtered = append(filtered, row)
				seen[id] = true
			}
		}
		if s.operation == "asset_groups_get" {
			if result == nil {
				return yhError(req, 404, "NotFound", "asset group not found"), nil
			}
		} else {
			result = map[string]any{"Items": filtered, "TotalCount": len(filtered)}
		}
	case "assets_list":
		items := data.Get("items")
		if !items.IsArray() {
			return nil, fmt.Errorf("invalid YH asset list")
		}
		rows := make([]map[string]any, 0, len(items.Array()))
		for _, item := range items.Array() {
			row, err := yhAssetRow(item)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
		count := data.Get("total_count")
		if count.Type != gjson.Number || count.Int() < 0 {
			return nil, fmt.Errorf("invalid YH asset list total")
		}
		result = map[string]any{"Items": rows, "TotalCount": json.RawMessage(count.Raw)}
	case "asset_groups_create":
		id, err := yhGroupID(data.Get("group_id"))
		if err != nil {
			return nil, err
		}
		result = map[string]any{"Id": id, "Name": s.name, "GroupType": "AIGC"}
	case "asset_groups_update":
		result = map[string]any{"Id": s.id, "Name": s.name, "GroupType": s.groupType}
	case "assets_create", "assets_get", "assets_update":
		row, err := yhAssetRow(data)
		if err != nil {
			return nil, err
		}
		if s.operation != "assets_create" && row["Id"] != s.id {
			return nil, fmt.Errorf("YH returned a different asset ID")
		}
		if s.binding.GroupID != "" {
			if group, ok := row["GroupId"]; ok && group != s.binding.GroupID {
				return nil, fmt.Errorf("YH returned a different asset group")
			}
			row["GroupId"] = s.binding.GroupID
		}
		row["GroupType"] = s.groupType
		result = row
	default:
		result = map[string]any{}
	}
	// A failed asset review is a successful query with a business Error. Keep
	// it inside Result so shared response validation does not mistake it for
	// a failed HTTP/API operation and discard the asset status and identity.
	if row, ok := result.(map[string]any); ok && row["Error"] != nil {
		result = map[string]any{"Result": row}
	}
	normalized, err := common.Marshal(result)
	if err != nil {
		return nil, err
	}
	return yhHTTPResponse(req, resp.StatusCode, normalized, resp.Header), nil
}

func yhGroupID(value gjson.Result) (string, error) {
	if value.Type != gjson.Number && value.Type != gjson.String {
		return "", fmt.Errorf("missing YH group ID")
	}
	id := value.String()
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != id {
		return "", fmt.Errorf("invalid YH group ID")
	}
	return id, nil
}

func yhGroupRows(data gjson.Result, human bool) ([]map[string]any, error) {
	if !data.IsArray() {
		data = data.Get("items")
	}
	if !data.IsArray() {
		return nil, fmt.Errorf("invalid YH group list")
	}
	rows := make([]map[string]any, 0, len(data.Array()))
	for _, item := range data.Array() {
		id, err := yhGroupID(item.Get("id"))
		if err != nil {
			return nil, err
		}
		row := map[string]any{"Id": id, "Name": item.Get("name").String(), "GroupType": "AIGC"}
		if human {
			row["Name"] = item.Get("artist_name").String()
			row["Description"] = item.Get("artist_desc").String()
			row["GroupType"] = "LivenessFace"
		}
		if count := item.Get("asset_count"); count.Exists() {
			row["AssetCount"] = json.RawMessage(count.Raw)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func yhAssetRow(data gjson.Result) (map[string]any, error) {
	id := data.Get("real_id").String()
	if id == "" {
		id = data.Get("id").String()
	}
	if !validAssetHandle(id) {
		return nil, fmt.Errorf("missing YH asset ID")
	}
	row := map[string]any{"Id": id}
	for source, target := range map[string]string{"name": "Name", "url": "URL", "asset_type": "AssetType", "error": "Error"} {
		if v := data.Get(source); v.Exists() {
			row[target] = json.RawMessage(v.Raw)
		}
	}
	if value := data.Get("group_id"); value.Exists() {
		group, err := yhGroupID(value)
		if err != nil {
			return nil, err
		}
		row["GroupId"] = group
	}
	if status := data.Get("status"); status.Exists() {
		switch status.String() {
		case "Pending", "Processing":
			row["Status"] = "Processing"
		case "Active", "Failed", "Deleted":
			row["Status"] = status.String()
		default:
			return nil, fmt.Errorf("unrecognized YH asset status")
		}
	}
	// The supplier also emits timestamps without a timezone. Do not invent one.
	if value := data.Get("create_time"); value.Type == gjson.String {
		if at, err := time.Parse(time.RFC3339, value.String()); err == nil {
			row["CreateTime"] = at.Format(time.RFC3339)
		}
	}
	return row, nil
}
