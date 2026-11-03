package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
)

func forwardContentRange(c *gin.Context, req *http.Request) {
	for _, key := range []string{"Range", "If-Range"} {
		if value := c.GetHeader(key); value != "" {
			req.Header.Set(key, value)
		}
	}
	// Keep Content-Length and byte ranges consistent with the transmitted body.
	req.Header.Set("Accept-Encoding", "identity")
}

func writeProtectedContent(c *gin.Context, resp *http.Response) error {
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Content-Disposition", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := resp.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(resp.StatusCode)
	_, err := io.Copy(c.Writer, resp.Body)
	return err
}

// Shared access policy has already pinned the owned resource and its account.
// Fetch only the configured supplier endpoint, never a URL supplied by a user.
func relayYHAssetContent(c *gin.Context, ch *model.Channel, resource *configurable.ResourceConfig, client *http.Client) {
	req, err := buildConfigurableResourceRequest(c, ch, resource)
	if err != nil {
		assetInternalError(c, http.StatusBadRequest, "asset_prepare_failed", err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	req = req.Clone(ctx)
	forwardContentRange(c, req)
	downloadClient := *client
	downloadClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := downloadClient.Do(req)
	if err != nil {
		assetInternalError(c, http.StatusBadGateway, "asset_download_failed", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		status := resp.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		assetAccessError(c, status, "asset_download_failed", fmt.Errorf("upstream download returned status %d", resp.StatusCode))
		return
	}
	_ = writeProtectedContent(c, resp)
}
