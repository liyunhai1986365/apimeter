package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/tidwall/gjson"
)

// Upload handles use a separate resource kind. A processing batch task ID is
// never a usable asset alias; only returned real asset IDs may be referenced.
func (s yhAssetRequest) normalizeUpload(ctx context.Context, body []byte) ([]byte, error) {
	data := gjson.GetBytes(body, "data")
	id := s.id
	if s.operation == "yh_uploads_create" {
		if s.binding.UploadBatch {
			id = data.Get("batch_id").String()
		} else {
			id = data.Get("id").String()
		}
	}
	if !validAssetHandle(id) {
		return nil, fmt.Errorf("missing YH upload handle")
	}
	if returned := data.Get("batch_id"); s.binding.UploadBatch && returned.Exists() && returned.String() != id {
		return nil, fmt.Errorf("YH returned another batch")
	}
	upload := s.binding
	upload.Kind, upload.ID, upload.CanonicalID = "batch", id, id
	bindings := []model.AssetBinding{upload}
	items := []gjson.Result{data}
	if s.binding.UploadBatch {
		list := data.Get("items")
		if !list.IsArray() {
			return nil, fmt.Errorf("invalid YH upload items")
		}
		items = list.Array()
	}
	rows := make([]map[string]any, 0, len(items))
	status, failed, processing := "Active", false, false
	for _, item := range items {
		row := map[string]any{}
		state := item.Get("status").String()
		if state == "" && s.operation == "yh_uploads_create" && !s.binding.UploadBatch && strings.HasPrefix(item.Get("id").String(), "asset-") {
			state = "Active"
		}
		switch state {
		case "Pending", "Processing":
			state = "Processing"
			processing = true
		case "Failed", "Deleted":
			failed = true
		case "Active":
		default:
			return nil, fmt.Errorf("invalid YH upload status")
		}
		row["Status"] = state
		assetID := item.Get("real_id").String()
		if assetID == "" && strings.HasPrefix(item.Get("id").String(), "asset-") {
			assetID = item.Get("id").String()
		}
		if assetID == "" && strings.HasPrefix(item.Get("asset_url").String(), "asset://asset-") {
			assetID = strings.TrimPrefix(item.Get("asset_url").String(), "asset://")
		}
		if assetID != "" {
			if !validAssetHandle(assetID) {
				return nil, fmt.Errorf("invalid YH asset ID")
			}
			if group := item.Get("group_id"); group.Exists() {
				resolved, err := yhGroupID(group)
				if err != nil || resolved != s.binding.GroupID {
					return nil, fmt.Errorf("YH upload group mismatch")
				}
			}
			asset := s.binding
			asset.Kind, asset.ID, asset.CanonicalID, asset.UploadBatch = "asset", assetID, assetID, false
			bindings = append(bindings, asset)
			row["Id"] = assetID
		}
		for from, to := range map[string]string{"source_url": "SourceURL", "url": "URL", "error": "Error", "name": "Name", "asset_type": "AssetType"} {
			if value := item.Get(from); value.Exists() {
				row[to] = json.RawMessage(value.Raw)
			}
		}
		rows = append(rows, row)
	}
	if processing {
		status = "Processing"
	} else if failed {
		status = "Failed"
	}
	if len(rows) == 0 {
		status = "Processing"
	}
	if err := model.SaveAssetBindingsContext(ctx, bindings); err != nil {
		return nil, err
	}
	return common.Marshal(map[string]any{"Id": id, "Status": status, "Items": rows})
}
