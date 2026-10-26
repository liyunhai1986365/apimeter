package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/inflight"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
)

type assetListScanResult struct {
	response *http.Response
	body     []byte
	total    *int
	proofs   map[string]string
}

var assetListScans inflight.Group[assetListScanResult]

type assetListPage struct {
	response *http.Response
	body     []byte
}

var assetListPages inflight.Group[assetListPage]

// Coalesce only an identical, already-authorized upstream page. Every user
// continues its own filtering, total calculation and final authorization. A
// user that has found all its assets never waits for another user's next page.
// Canceling one caller cannot abort the other readers of this page.
func readAssetListPage(client *http.Client, req *http.Request) (*http.Response, []byte, error) {
	return readAssetListPageInScope(client, req, "")
}

func readAssetListPageInScope(client *http.Client, req *http.Request, scope string) (*http.Response, []byte, error) {
	var requestBody []byte
	if req.GetBody != nil {
		reader, err := req.GetBody()
		if err != nil {
			return nil, nil, err
		}
		requestBody, err = io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return nil, nil, err
		}
	}
	read := func(ctx context.Context) (assetListPage, error) {
		request := req.Clone(ctx)
		if req.GetBody != nil {
			request.Body = io.NopCloser(bytes.NewReader(requestBody))
		}
		resp, err := client.Do(request)
		if err != nil {
			return assetListPage{}, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
		if err != nil || len(body) > 16<<20 {
			return assetListPage{}, fmt.Errorf("cannot read asset list page within size limit")
		}
		// A retained raw page must not keep request contexts or closed response
		// bodies alive. The caller only needs status/headers and the read bytes.
		response := *resp
		response.Body, response.Request = http.NoBody, nil
		return assetListPage{&response, body}, nil
	}
	// A cookie jar or a non-replayable body can change the outgoing request
	// independently of its headers, so it cannot use the shared path.
	var page assetListPage
	var err error
	if client.Jar != nil || (req.Body != nil && req.GetBody == nil) {
		page, err = read(req.Context())
	} else {
		key, marshalErr := common.Marshal([]any{scope, fmt.Sprintf("%p", client), req.Method, req.URL.String(), req.Host, req.Header, string(requestBody)})
		if marshalErr != nil {
			return nil, nil, marshalErr
		}
		page, err = assetListPages.Do(req.Context(), fmt.Sprintf("%x", sha256.Sum256(key)), read)
	}
	return page.response, page.body, err
}

// Share only overlapping identical list scans, after each caller's own routing
// and filter authorization. Revalidate the returned page against current durable
// ownership for every caller. Results are discarded as soon as a scan finishes.
func filterAssetListResponse(c *gin.Context, client *http.Client, ch *model.Channel, resource *configurable.ResourceConfig, req *http.Request, resp *http.Response, body []byte) (*http.Response, []byte, error) {
	raw, exists := c.Get(assetAccessContextKey)
	if !resource.AssetLibrary || !exists || raw.(*assetAccessRequest).list == nil || resp.StatusCode >= 400 || !assetResponseSuccessful(body) {
		return resp, body, nil
	}
	a := raw.(*assetAccessRequest)
	p := a.list
	firstPage, err := assetListFirstPageFingerprint(body)
	if err != nil {
		return resp, nil, err
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	var requestBody []byte
	if req.GetBody != nil {
		reader, err := req.GetBody()
		if err != nil {
			return resp, nil, err
		}
		requestBody, err = io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return resp, nil, err
		}
	}
	keyBytes, err := common.Marshal([]any{
		fmt.Sprintf("%p", model.DB.Statement.ConnPool), userID, ch.Id, a.scope, a.project, a.op.kind,
		resource.ID, req.Method, req.URL.String(), req.Header, string(requestBody), ch.GetSetting().Proxy,
		p.page, p.size, p.offset, p.cursorMode, p.upstreamCursor,
		firstPage,
	})
	if err != nil {
		return resp, nil, err
	}
	key := fmt.Sprintf("%x", sha256.Sum256(keyBytes))
	// The worker must not retain the pooled Gin context or the caller's mutable
	// pagination state after that caller disconnects.
	workContext := c.Copy()
	accessCopy, paginationCopy := *a, *p
	accessCopy.list = &paginationCopy
	workContext.Set(assetAccessContextKey, &accessCopy)
	result, err := assetListScans.Do(c.Request.Context(), key, func(ctx context.Context) (assetListScanResult, error) {
		ctx, cancel := context.WithTimeout(ctx, assetListScanTimeout)
		defer cancel()
		workContext.Request = workContext.Request.Clone(ctx)
		pageKey, err := common.Marshal([]any{fmt.Sprintf("%p", client), ch.Id, accessCopy.scope, accessCopy.project, accessCopy.op.kind,
			resource.ID, req.Method, req.URL.String(), req.Host, req.Header, string(requestBody), ch.GetSetting().Proxy, paginationCopy.upstreamCursor, firstPage})
		if err != nil {
			return assetListScanResult{}, err
		}
		paginationCopy.pages = acquireAssetListPages(fmt.Sprintf("%x", sha256.Sum256(pageKey)))
		defer paginationCopy.pages.release()
		if err := loadAssetListOwnership(ctx, &accessCopy, userID); err != nil {
			return assetListScanResult{}, err
		}
		response, data, err := scanAssetListResponse(workContext, client, ch, resource, req.Clone(ctx), resp, body)
		return assetListScanResult{response, data, paginationCopy.filteredTotal, paginationCopy.pageProofs}, err
	})
	if err != nil {
		return resp, nil, err
	}
	if result.response.StatusCode < 400 && assetResponseSuccessful(result.body) {
		if err := revalidateAssetListPage(c.Request.Context(), a, userID, result.proofs); err != nil {
			return resp, nil, err
		}
	}
	if result.total != nil {
		p.filteredTotal = common.GetPointer(*result.total)
	}
	response := *result.response
	response.Header = result.response.Header.Clone()
	return &response, append([]byte(nil), result.body...), nil
}

func revalidateAssetListPage(ctx context.Context, a *assetAccessRequest, userID int, proofs map[string]string) error {
	handles := make([]string, 0, len(proofs))
	for id := range proofs {
		handles = append(handles, id)
	}
	bindings, err := model.FindScopedAssetBindingsBatchContext(ctx, a.channel.Id, userID, a.scope, a.op.kind, handles)
	if err != nil {
		return err
	}
	valid := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		canonical := binding.CanonicalID
		if canonical == "" {
			canonical = binding.ID
		}
		if binding.Kind == a.op.kind && canonical == proofs[binding.ID] && (a.project == "" || binding.Project == "" || binding.Project == a.project) {
			valid[binding.ID] = true
		}
	}
	if len(valid) != len(proofs) {
		return model.ErrAssetRevoked
	}
	return nil
}
