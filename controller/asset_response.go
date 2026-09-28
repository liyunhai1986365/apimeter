package controller

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Keep provider asset fields (including IDs, state, expiry and pagination), but
// do not expose routing, credentials or administrator diagnostics to customers.
func publicAssetResponse(body []byte) ([]byte, error) {
	filtered, err := filterAssetResponseFields(body, 0)
	if err != nil || assetResponseSuccessful(body) || !assetResponseSuccessful(filtered) {
		return filtered, err
	}
	// If every field of an upstream error was internal, redaction leaves {}.
	// Keep a safe error marker so later response conversion cannot report success.
	for _, path := range []string{"error", "Error", "ResponseMetadata.Error"} {
		if assetResponseHasError(gjson.GetBytes(body, path)) {
			replacement := `{"Code":"asset_request_failed","Message":"asset request failed"}`
			if path == "error" {
				replacement = `{"code":"asset_request_failed","message":"asset request failed"}`
			}
			return sjson.SetRawBytes(filtered, path, []byte(replacement))
		}
	}
	return filtered, nil
}

func filterAssetResponseFields(body []byte, depth int) ([]byte, error) {
	if depth > 128 {
		return nil, fmt.Errorf("asset response nesting exceeds 128 levels")
	}
	switch common.GetJsonType(body) {
	case "object":
		var fields map[string]json.RawMessage
		if err := common.Unmarshal(body, &fields); err != nil {
			return body, nil // Preserve non-JSON upstream errors.
		}
		changed := false
		for key, value := range fields {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			switch normalized {
			case "channel", "channelid", "channelname", "channeltype", "admininfo", "auditinfo", "retryrouteeventids", "assetsecret", "assetcredentials", "apikey", "accesskeyid", "secretaccesskey":
				delete(fields, key)
				changed = true
				continue
			}
			filtered, err := filterAssetResponseFields(value, depth+1)
			if err != nil {
				return nil, err
			}
			if string(filtered) != string(value) {
				fields[key], changed = filtered, true
			}
		}
		if changed {
			return common.Marshal(fields)
		}
	case "array":
		var items []json.RawMessage
		if err := common.Unmarshal(body, &items); err != nil {
			return body, nil
		}
		changed := false
		for i, item := range items {
			filtered, err := filterAssetResponseFields(item, depth+1)
			if err != nil {
				return nil, err
			}
			if string(filtered) != string(item) {
				items[i], changed = filtered, true
			}
		}
		if changed {
			return common.Marshal(items)
		}
	}
	return body, nil
}

func assetInternalError(c *gin.Context, status int, code string, err error) {
	common.SysError(code + ": " + err.Error())
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": "asset service is temporarily unavailable; contact the administrator with the request ID"}})
}
