package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	"github.com/tidwall/sjson"
)

const assetListBatchSize = 100
const assetListScanTimeout = time.Minute

type assetListPagination struct {
	page, size, offset int
	cursorMode         bool
	upstreamCursor     bool
	cursorPrefix       string
	owned              map[string]string // handle -> canonical resource ID
	ownedCount         int
}

func newAssetListPagination(c *gin.Context, a *assetAccessRequest) (*assetListPagination, error) {
	p := &assetListPagination{page: 1, size: 20, owned: map[string]string{}}
	value := func(keys ...string) string {
		for _, key := range keys {
			if v := a.input.Get(key); v.Exists() {
				return v.String()
			}
			if v := c.Query(key); v != "" {
				return v
			}
		}
		return ""
	}
	for target, raw := range map[*int]string{&p.page: value("PageNumber", "page_number", "page"), &p.size: value("PageSize", "page_size", "MaxResults", "max_results")} {
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("asset pagination value is out of range")
		}
		*target = n
	}
	if p.size > assetListBatchSize {
		return nil, fmt.Errorf("asset pagination supports at most 100 items per page")
	}
	if p.page-1 > (math.MaxInt-p.size)/p.size {
		return nil, fmt.Errorf("asset pagination offset is out of range")
	}
	p.offset = (p.page - 1) * p.size
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	p.cursorPrefix = fmt.Sprintf("asset-v1-%x-", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", userID, a.scope, a.project, a.op.kind))))
	cursor := value("NextToken", "next_token")
	p.cursorMode = cursor != "" || value("MaxResults", "max_results") != ""
	p.upstreamCursor = p.cursorMode && a.resource.Upstream.Method != http.MethodGet
	if cursor != "" {
		if !strings.HasPrefix(cursor, p.cursorPrefix) {
			return nil, fmt.Errorf("asset pagination token does not belong to this user/account; restart listing")
		}
		offset, err := strconv.Atoi(strings.TrimPrefix(cursor, p.cursorPrefix))
		if err != nil || offset < 0 || offset > math.MaxInt-p.size {
			return nil, fmt.Errorf("invalid asset pagination token")
		}
		p.offset = offset
	}
	bindings, err := model.ListAssetBindings(a.channel.Id, userID, a.scope, a.op.kind)
	if err != nil {
		common.SysError("list asset bindings: " + err.Error())
		return nil, errAssetStateUnavailable
	}
	canonicalIDs := map[string]bool{}
	for _, binding := range bindings {
		if a.project == "" || binding.Project == "" || binding.Project == a.project {
			canonical := binding.CanonicalID
			if canonical == "" {
				canonical = binding.ID
			}
			if canonical == "" {
				continue
			}
			canonicalIDs[canonical] = true
			p.owned[canonical] = canonical
			if binding.ID != "" {
				p.owned[binding.ID] = canonical
			}
		}
	}
	p.ownedCount = len(canonicalIDs)
	return p, nil
}

// Apply pagination before authorization/signing. Filtering just one upstream
// page would leak account-wide totals and incorrectly return empty pages while
// this user still has matching assets later in the supplier's list.
func rewriteAssetListRequest(c *gin.Context, resource *configurable.ResourceConfig, req *http.Request, page int, cursor string) error {
	raw, exists := c.Get(assetAccessContextKey)
	if !resource.AssetLibrary || !exists || raw.(*assetAccessRequest).list == nil {
		return nil
	}
	p := raw.(*assetAccessRequest).list
	if req.Method == http.MethodGet {
		query := req.URL.Query()
		query.Set("page", strconv.Itoa(page))
		query.Set("page_size", strconv.Itoa(assetListBatchSize))
		req.URL.RawQuery = query.Encode()
		return nil
	}
	body := map[string]json.RawMessage{}
	if req.GetBody != nil {
		reader, err := req.GetBody()
		if err != nil {
			return err
		}
		err = common.DecodeJson(reader, &body)
		_ = reader.Close()
		if err != nil {
			return err
		}
	}
	for _, key := range []string{"PageNumber", "PageSize", "NextToken", "MaxResults", "page", "page_number", "page_size", "next_token", "max_results"} {
		delete(body, key)
	}
	if p.cursorMode {
		body["MaxResults"] = json.RawMessage(strconv.Itoa(assetListBatchSize))
		if cursor != "" {
			body["NextToken"], _ = common.Marshal(cursor)
		}
	} else {
		body["PageNumber"] = json.RawMessage(strconv.Itoa(page))
		body["PageSize"] = json.RawMessage(strconv.Itoa(assetListBatchSize))
	}
	data, err := common.Marshal(body)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(data))
	req.ContentLength = int64(len(data))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
	return nil
}

func assetListItems(body []byte) (string, gjson.Result, error) {
	if root := gjson.ParseBytes(body); root.IsArray() {
		return "", root, nil
	}
	for _, path := range []string{"Result.Items", "data.items", "data.list", "data.assets", "data", "items", "list", "assets", "result.items"} {
		items := gjson.GetBytes(body, path)
		if items.IsArray() || (items.Exists() && items.Type == gjson.Null) {
			return path, items, nil
		}
	}
	return "", gjson.Result{}, fmt.Errorf("unrecognized asset list response; refusing to expose an unfiltered account list")
}

func filterAssetListResponse(c *gin.Context, client *http.Client, ch *model.Channel, resource *configurable.ResourceConfig, req *http.Request, resp *http.Response, body []byte) (*http.Response, []byte, error) {
	raw, exists := c.Get(assetAccessContextKey)
	if !resource.AssetLibrary || !exists || raw.(*assetAccessRequest).list == nil || resp.StatusCode >= 400 || !assetResponseSuccessful(body) {
		return resp, body, nil
	}
	p := raw.(*assetAccessRequest).list
	// Limit elapsed work, not account size. A user's matching resource may be
	// beyond page 100; cancellation and a scan deadline still bound the request.
	ctx, cancel := context.WithTimeout(c.Request.Context(), assetListScanTimeout)
	defer cancel()
	template := append([]byte(nil), body...)
	listPath, _, err := assetListItems(body)
	if err != nil {
		return resp, nil, err
	}
	var owned []json.RawMessage
	seenPages := map[[sha256.Size]byte]bool{}
	seenIDs := map[string]bool{}
	var scanned int64
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return resp, nil, fmt.Errorf("asset list scan interrupted: %w", err)
		}
		_, items, err := assetListItems(body)
		if err != nil {
			return resp, nil, err
		}
		rows := items.Array()
		for _, row := range rows {
			ids := assetResponseStrings([]byte(row.Raw), "Id", "id", "asset_id", "AssetId", "GroupId", "group_id")
			// An asset's parent GroupId is not proof of ownership of the asset.
			if raw.(*assetAccessRequest).op.kind == "asset" {
				ids = assetResponseStrings([]byte(row.Raw), "Id", "id", "asset_id", "AssetId")
			}
			if len(ids) == 0 {
				continue
			}
			canonical, ownedByUser := p.owned[ids[0]]
			if !ownedByUser || seenIDs[canonical] {
				continue
			}
			seenIDs[canonical] = true
			owned = append(owned, json.RawMessage(row.Raw))
		}
		scanned += int64(len(rows))
		next := gjson.GetBytes(body, "Result.NextToken").String()
		if next == "" {
			next = gjson.GetBytes(body, "NextToken").String()
		}
		// Aliases are one resource. Never use a local binding count as the
		// response total: upstream filters or external deletion may exclude IDs.
		done := len(seenIDs) == p.ownedCount
		if p.upstreamCursor {
			done = done || next == ""
		} else {
			done = done || len(rows) == 0
			total := gjson.Result{}
			for _, path := range []string{"Result.TotalCount", "data.total", "data.total_count", "total", "total_count", "TotalCount"} {
				if value := gjson.GetBytes(body, path); value.Exists() {
					total = value
					break
				}
			}
			done = done || (total.Exists() && scanned >= total.Int()) || (!total.Exists() && len(rows) < assetListBatchSize)
		}
		if done {
			break
		}
		// Store only a digest per page instead of retaining every account-wide
		// response. Repeated cursors are loops even if their page data changes.
		pageKey := items.Raw
		if p.upstreamCursor {
			pageKey = next
		}
		fingerprint := sha256.Sum256([]byte(pageKey))
		if seenPages[fingerprint] {
			return resp, nil, fmt.Errorf("asset list pagination repeated a page or cursor")
		}
		seenPages[fingerprint] = true
		nextReq := req.Clone(ctx)
		if err := rewriteAssetListRequest(c, resource, nextReq, page+1, next); err != nil {
			return resp, nil, err
		}
		if err := authorizeConfigurableResourceRequest(c, ch, resource, nextReq); err != nil {
			return resp, nil, err
		}
		nextResp, err := client.Do(nextReq)
		if err != nil {
			return resp, nil, err
		}
		body, err = io.ReadAll(io.LimitReader(nextResp.Body, 16<<20+1))
		_ = nextResp.Body.Close()
		if err != nil || len(body) > 16<<20 {
			return resp, nil, fmt.Errorf("cannot read asset list page within size limit")
		}
		if nextResp.StatusCode >= 400 || !assetResponseSuccessful(body) {
			return nextResp, body, nil
		}
	}
	start := min(p.offset, len(owned))
	end := min(start+p.size, len(owned))
	pageItems := append([]json.RawMessage{}, owned[start:end]...)
	itemsJSON, err := common.Marshal(pageItems)
	if err != nil {
		return resp, nil, err
	}
	if listPath == "" {
		return resp, itemsJSON, nil
	}
	output, err := sjson.SetRawBytes(template, listPath, itemsJSON)
	if err != nil {
		return resp, nil, err
	}
	for _, root := range []string{"", "Result.", "data.", "result."} {
		for _, field := range []string{"TotalCount", "total", "total_count", "count"} {
			if gjson.GetBytes(output, root+field).Exists() {
				output, _ = sjson.SetBytes(output, root+field, len(owned))
			}
		}
		for field, value := range map[string]int{"PageNumber": p.page, "page": p.page, "PageSize": p.size, "page_size": p.size} {
			if gjson.GetBytes(output, root+field).Exists() {
				output, _ = sjson.SetBytes(output, root+field, value)
			}
		}
		for _, field := range []string{"NextToken", "next_token"} {
			if gjson.GetBytes(output, root+field).Exists() {
				output, _ = sjson.SetBytes(output, root+field, "")
			}
		}
	}
	if p.cursorMode {
		next := ""
		if end < len(owned) {
			next = p.cursorPrefix + strconv.Itoa(end)
		}
		path := "NextToken"
		if strings.HasPrefix(listPath, "Result.") {
			path = "Result.NextToken"
		}
		output, _ = sjson.SetBytes(output, path, next)
	}
	return resp, output, nil
}
