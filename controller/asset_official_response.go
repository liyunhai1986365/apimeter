package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// ArkAssetResponse is installed before authentication on the official Action
// endpoint. Buffering this non-streaming API also covers early gateway errors
// and historical-asset lookup failures, without changing the supplier routes.
func ArkAssetResponse(c *gin.Context) {
	w := &arkAssetResponseWriter{ResponseWriter: c.Writer, status: http.StatusOK, size: -1}
	c.Writer = w
	defer func() { c.Writer = w.ResponseWriter }()
	defer func() {
		if recovered := recover(); recovered != nil {
			if recovered == http.ErrAbortHandler {
				panic(recovered)
			}
			common.SysError(fmt.Sprintf("official asset request panic: %v\n%s", recovered, debug.Stack()))
			c.Abort()
			w.status, w.size, w.tooLarge = http.StatusInternalServerError, -1, false
			w.body.Reset()
			_, _ = w.Write([]byte(`{"error":{"code":"asset_internal_error","message":"asset service is temporarily unavailable; contact the administrator with the request ID"}}`))
		}
		writeOfficialAssetResponse(c, w)
	}()
	c.Next()
}

func writeOfficialAssetResponse(c *gin.Context, w *arkAssetResponseWriter) {
	status, body, err := officialAssetResponse(c, w.Status(), w.body.Bytes())
	if w.tooLarge {
		err = fmt.Errorf("official asset response exceeds size limit")
	}
	if err != nil {
		common.SysError("convert official asset response: " + err.Error())
		failure := gin.H{"error": gin.H{
			"code":    "asset_response_failed",
			"message": "invalid asset response; the operation may have been accepted; do not resubmit creation automatically; contact the administrator with the request ID",
		}}
		if ids := assetResponseStrings(w.body.Bytes(), "ResponseMetadata.RequestId", "RequestId", "request_id", "requestId"); len(ids) > 0 {
			failure["ResponseMetadata"] = gin.H{"RequestId": ids[0]}
		}
		fallback, _ := common.Marshal(failure)
		status, body, _ = officialAssetResponse(c, http.StatusBadGateway, fallback)
	}
	c.Writer = w.ResponseWriter
	c.Header("Content-Length", "")
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Data(status, "application/json; charset=utf-8", body)
}

type arkAssetResponseWriter struct {
	gin.ResponseWriter
	body     bytes.Buffer
	status   int
	size     int
	tooLarge bool
}

func (w *arkAssetResponseWriter) WriteHeader(status int) {
	if status > 0 && !w.Written() {
		w.status = status
	}
}

func (w *arkAssetResponseWriter) WriteHeaderNow() {
	if !w.Written() {
		w.size = 0
	}
}

func (w *arkAssetResponseWriter) Write(body []byte) (int, error) {
	w.WriteHeaderNow()
	w.size += len(body)
	// Asset upstream responses already have the same limit. Bound this final
	// buffer too, including errors written before the relay handler runs.
	if w.body.Len()+len(body) > 16<<20 {
		w.tooLarge = true
		return 0, fmt.Errorf("official asset response exceeds size limit")
	}
	return w.body.Write(body)
}

func (w *arkAssetResponseWriter) WriteString(body string) (int, error) {
	return w.Write([]byte(body))
}

func (w *arkAssetResponseWriter) Status() int   { return w.status }
func (w *arkAssetResponseWriter) Size() int     { return w.size }
func (w *arkAssetResponseWriter) Written() bool { return w.size >= 0 }
func (w *arkAssetResponseWriter) Flush()        { w.WriteHeaderNow() }

// RawMessage keeps IDs, timestamps, explicit zero/false, and large extension
// numbers intact. Only the public envelope and known field aliases change.
func officialAssetResponse(c *gin.Context, status int, body []byte) (int, []byte, error) {
	filtered, err := publicAssetResponse(body)
	if err != nil {
		return status, nil, err
	}
	root := map[string]json.RawMessage{}
	parseErr := common.Unmarshal(filtered, &root)
	action := c.Query("Action")
	if parseErr != nil && strings.HasPrefix(action, "List") && common.GetJsonType(filtered) == "array" {
		root = map[string]json.RawMessage{"Items": filtered}
		parseErr = nil
	}
	if root == nil {
		parseErr = fmt.Errorf("asset response is null")
	}
	metadata := map[string]json.RawMessage{}
	if raw := root["ResponseMetadata"]; len(raw) > 0 {
		_ = common.Unmarshal(raw, &metadata)
	}
	if metadata == nil {
		metadata = map[string]json.RawMessage{}
	}
	setString := func(key, value string) { metadata[key], _ = common.Marshal(value) }
	requestID := rawAssetString(metadata, "RequestId", "request_id")
	if requestID == "" {
		requestID = rawAssetString(root, "RequestId", "request_id", "requestId")
	}
	if requestID == "" {
		requestID = c.Writer.Header().Get("X-Request-Id")
	}
	if requestID == "" {
		requestID = c.GetString(common.RequestIdKey)
	}
	if requestID == "" {
		requestID = common.NewRequestId()
	}
	setString("RequestId", requestID)
	setString("Action", action)
	setString("Version", c.Query("Version"))
	if rawAssetString(metadata, "Service") == "" {
		setString("Service", "ark")
	}
	if rawAssetString(metadata, "Region") == "" {
		region := "cn-beijing"
		if raw, ok := c.Get(assetAccessContextKey); ok {
			if a, ok := raw.(*assetAccessRequest); ok && a.channel != nil {
				if cfg := assetLibrary(a.channel); cfg != nil && cfg.Region != "" {
					region = cfg.Region
				}
			}
		}
		setString("Region", region)
	}
	response := map[string]any{"ResponseMetadata": metadata}
	emptySuccess := len(bytes.TrimSpace(body)) == 0 && (strings.HasPrefix(action, "Delete") || strings.HasPrefix(action, "Update"))
	// Removing internal fields must not turn an upstream error into success,
	// even when the error object contains nothing but redacted diagnostics.
	failed := status >= http.StatusBadRequest || (parseErr == nil && !assetResponseSuccessful(body))
	if failed {
		metadata["Error"] = officialAssetError(metadata, root, status)
	} else {
		if parseErr != nil && !emptySuccess {
			return status, nil, fmt.Errorf("asset response is not a JSON object")
		}
		result, err := officialAssetResult(action, root)
		if err != nil {
			return status, nil, err
		}
		if err := validateOfficialAssetResult(action, result); err != nil {
			return status, nil, err
		}
		if action == "ListAssets" || action == "ListAssetGroups" {
			applyOfficialAssetPagination(c, result)
		}
		delete(metadata, "Error")
		response["Result"] = result
		if status == http.StatusNoContent {
			status = http.StatusOK // The official API returns a JSON Result on delete.
		}
	}
	encoded, err := common.Marshal(response)
	return status, encoded, err
}

func officialAssetError(metadata, root map[string]json.RawMessage, status int) json.RawMessage {
	fields := map[string]json.RawMessage{}
	for _, candidate := range []json.RawMessage{metadata["Error"], root["error"], root["Error"]} {
		if len(candidate) == 0 || string(candidate) == "null" || string(candidate) == "{}" {
			continue
		}
		if common.Unmarshal(candidate, &fields) != nil || fields == nil {
			fields = map[string]json.RawMessage{}
			var message string
			if common.Unmarshal(candidate, &message) == nil {
				fields["Message"], _ = common.Marshal(message)
			}
		}
		break
	}
	code := rawAssetString(fields, "Code", "code", "type")
	if code == "" {
		code = rawAssetString(root, "code", "Code")
		if code == "0" || code == "200" {
			code = ""
		}
	}
	if code == "" {
		code = "asset_request_failed"
	}
	message := rawAssetString(fields, "Message", "message")
	if message == "" {
		message = rawAssetString(root, "message", "Message", "msg")
	}
	if message == "" {
		message = http.StatusText(status)
		if status < 400 || message == "" {
			message = "asset request failed"
		}
	}
	delete(fields, "code")
	delete(fields, "message")
	delete(fields, "type")
	fields["Code"], _ = common.Marshal(code)
	fields["Message"], _ = common.Marshal(message)
	encoded, _ := common.Marshal(fields)
	return encoded
}

func rawAssetString(fields map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		value := fields[name]
		if len(value) == 0 || string(value) == "null" {
			continue
		}
		var text string
		if common.Unmarshal(value, &text) == nil {
			if text != "" {
				return text
			}
		} else if common.GetJsonType(value) == "number" {
			return string(value)
		}
	}
	return ""
}

func officialAssetResult(action string, root map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if action == "DeleteAsset" || action == "DeleteAssetGroup" {
		return map[string]json.RawMessage{}, nil
	}
	result := root
	bareResource := rawAssetString(root, "Id", "id", "asset_id", "AssetId", "GroupId", "group_id", "BytedToken", "bytedToken", "byted_token") != ""
	for _, name := range []string{"Result", "data", "result"} {
		if name != "Result" && bareResource {
			continue
		}
		if raw, ok := root[name]; ok {
			result = nil
			if strings.HasPrefix(action, "List") && common.GetJsonType(raw) == "array" {
				result = map[string]json.RawMessage{"Items": raw}
			} else if err := common.Unmarshal(raw, &result); err != nil {
				return nil, fmt.Errorf("invalid asset result object: %w", err)
			}
			break
		}
	}
	if result == nil {
		result = map[string]json.RawMessage{}
	}
	// Some REST suppliers put pagination beside data rather than inside it.
	if strings.HasPrefix(action, "List") {
		for _, field := range []string{"TotalCount", "total", "total_count", "count", "PageNumber", "page", "page_number", "PageSize", "page_size", "NextToken", "next_token", "MaxResults", "max_results"} {
			if _, exists := result[field]; !exists && root[field] != nil {
				result[field] = root[field]
			}
		}
	}
	for _, field := range []string{"ResponseMetadata", "code", "message", "msg", "success", "request_id", "requestId", "RequestId"} {
		delete(result, field)
	}
	return normalizeOfficialAssetFields(action, result)
}

// Items are resource objects, not response envelopes. In particular, a data
// extension inside an item must never replace the item and discard its ID.
func normalizeOfficialAssetFields(action string, result map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if result == nil {
		return nil, fmt.Errorf("asset item is null")
	}
	aliases := map[string][]string{
		"Id": {"id", "asset_id", "AssetId"}, "Name": {"name"}, "Description": {"description"},
		"URL": {"url"}, "AssetType": {"asset_type", "assetType"}, "GroupId": {"group_id", "groupId"},
		"GroupType": {"group_type", "groupType"}, "Status": {"status"}, "ProjectName": {"project_name", "projectName"},
		"CreateTime": {"create_time", "createTime", "created_at"}, "UpdateTime": {"update_time", "updateTime", "updated_at"},
		"LastInferenceTime": {"last_inference_time", "lastInferenceTime"}, "ExpireTime": {"expire_time", "expireTime"},
		"BytedToken": {"bytedToken", "byted_token"}, "H5Link": {"h5_link", "h5Link"}, "CallbackURL": {"callback_url", "callbackUrl"},
		"TotalCount": {"total", "total_count", "count"}, "PageNumber": {"page", "page_number"}, "PageSize": {"page_size"},
		"NextToken": {"next_token"}, "MaxResults": {"max_results"}, "Items": {"items", "list", "assets"}, "Error": {"error"},
	}
	for canonical, names := range aliases {
		for _, name := range names {
			if value, ok := result[name]; ok {
				if _, exists := result[canonical]; !exists {
					result[canonical] = value
				}
				delete(result, name)
			}
		}
	}
	if strings.Contains(action, "AssetGroup") {
		if _, exists := result["Id"]; !exists && result["GroupId"] != nil {
			result["Id"] = result["GroupId"]
			delete(result, "GroupId")
		}
	}
	if raw := result["Items"]; len(raw) > 0 {
		var items []map[string]json.RawMessage
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("invalid asset list items: %w", err)
		}
		if items == nil {
			items = []map[string]json.RawMessage{}
		}
		for i, item := range items {
			converted, err := normalizeOfficialAssetFields(action, item)
			if err != nil {
				return nil, err
			}
			items[i] = converted
		}
		result["Items"], _ = common.Marshal(items)
	}
	if raw := result["Error"]; len(raw) > 0 && string(raw) != "null" && string(raw) != "{}" {
		result["Error"] = officialAssetError(map[string]json.RawMessage{"Error": raw}, nil, http.StatusOK)
	}
	if value := rawAssetString(result, "Status"); value != "" {
		for _, status := range []string{"Processing", "Active", "Failed", "Expired"} {
			if strings.EqualFold(value, status) {
				result["Status"], _ = common.Marshal(status)
				break
			}
		}
	}
	return result, nil
}

func validateOfficialAssetResult(action string, result map[string]json.RawMessage) error {
	field := ""
	switch action {
	case "CreateAsset", "GetAsset", "CreateAssetGroup", "GetAssetGroup":
		field = "Id"
	case "CreateVisualValidateSession":
		field = "BytedToken"
	case "ListAssets", "ListAssetGroups":
		if common.GetJsonType(result["Items"]) != "array" {
			return fmt.Errorf("asset list response has no Items array")
		}
	}
	if field != "" {
		var value string
		if common.Unmarshal(result[field], &value) != nil || strings.TrimSpace(value) == "" {
			return fmt.Errorf("asset response has no valid %s", field)
		}
		if field == "Id" && !validAssetHandle(value) {
			return fmt.Errorf("asset response contains an invalid resource handle")
		}
	}
	return nil
}

func applyOfficialAssetPagination(c *gin.Context, result map[string]json.RawMessage) {
	raw, ok := c.Get(assetAccessContextKey)
	if !ok {
		return
	}
	a, ok := raw.(*assetAccessRequest)
	if !ok || a.list == nil || a.list.filteredTotal == nil {
		return
	}
	p := a.list
	// Bare supplier arrays have nowhere to carry totals or cursors. Use the
	// completed ownership scan, never local binding counts or supplier totals.
	result["TotalCount"], _ = common.Marshal(*p.filteredTotal)
	if p.cursorMode {
		next := ""
		if p.offset+p.size < *p.filteredTotal {
			next = fmt.Sprintf("%s%d", p.cursorPrefix, p.offset+p.size)
		}
		result["NextToken"], _ = common.Marshal(next)
	} else {
		result["PageNumber"], _ = common.Marshal(p.page)
		result["PageSize"], _ = common.Marshal(p.size)
	}
}
