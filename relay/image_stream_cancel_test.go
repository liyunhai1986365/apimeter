package relay

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

// Exercise the real SSE -> normalize -> HTTP download/upload path. Every
// blocking server is released during cleanup, even if a regression is detected.
func TestImageStreamNormalizationCancellation(t *testing.T) {
	for _, target := range []string{"origin_headers", "origin_body", "worker_headers", "worker_body", "oss", "r2"} {
		for _, end := range []string{"client_cancel", "caller_deadline", "idle_timeout", "processing_timeout", "eof_then_cancel", "eof_then_timeout", "eof_success"} {
			t.Run(target+"/"+end, func(t *testing.T) {
				fetch := system_setting.GetFetchSetting()
				oldFetch := *fetch
				fetch.EnableSSRFProtection = false
				oldRelayTimeout := common.RelayTimeout
				common.RelayTimeout = 0
				oldDownloadSize := constant.MaxFileDownloadMB
				constant.MaxFileDownloadMB = 20
				oldWorker, oldAllow := system_setting.WorkerUrl, system_setting.WorkerAllowHttpImageRequestEnabled
				system_setting.WorkerUrl = ""
				t.Cleanup(func() {
					*fetch = oldFetch
					common.RelayTimeout = oldRelayTimeout
					constant.MaxFileDownloadMB = oldDownloadSize
					system_setting.WorkerUrl = oldWorker
					system_setting.WorkerAllowHttpImageRequestEnabled = oldAllow
				})
				started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				cloud := target == "oss" || target == "r2"
				imageBytes, err := base64.StdEncoding.DecodeString(imageTaskTestPNGBase64)
				require.NoError(t, err)
				imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					if target == "origin_body" || target == "worker_body" {
						w.Header().Set("Content-Type", "image/png")
						w.WriteHeader(http.StatusOK)
						w.(http.Flusher).Flush()
					}
					close(started)
					select {
					case <-r.Context().Done():
						close(canceled)
						return
					case <-release:
					}
					if cloud {
						w.WriteHeader(http.StatusOK)
						return
					}
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write(imageBytes)
				}))
				defer imageServer.Close()
				defer releaseOnce.Do(func() { close(release) })
				if target == "worker_headers" || target == "worker_body" {
					system_setting.WorkerUrl = imageServer.URL
					system_setting.WorkerAllowHttpImageRequestEnabled = true
				}
				eof := end == "eof_success" || end == "eof_then_cancel" || end == "eof_then_timeout"
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					image := fmt.Sprintf(`"url":%q`, imageServer.URL)
					if cloud {
						image = fmt.Sprintf(`"b64_json":%q`, imageTaskTestPNGBase64)
					}
					fmt.Fprintf(w, "data: {\"type\":\"image_generation.completed\",%s}\n\n", image)
					w.(http.Flusher).Flush()
					if !eof {
						<-r.Context().Done()
					}
				}))
				defer upstream.Close()
				request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", Stream: common.GetPointer(true)}
				c, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
				if cloud {
					storageType := dto.UserImageStorageTypeAliyunOSS
					if target == "r2" {
						storageType = dto.UserImageStorageTypeCloudflareR2
					}
					info.UserSetting.ImageStorage = dto.UserImageStorageSetting{Enabled: true, Type: storageType, Endpoint: imageServer.URL, Bucket: "test-bucket", AccessKeyID: "test-key", AccessKeySecret: "test-secret", PublicBaseURL: "https://cdn.example.test"}
					info.TokenImageSettings = model.TokenImageSettings{Format: "url", Store: dto.TokenImageStoreForceStoreURLAndBase64}
				}
				if end == "idle_timeout" || end == "eof_then_timeout" {
					constant.StreamingTimeout = 1
				}
				if end == "processing_timeout" {
					t.Setenv("IMAGE_RESULT_PROCESSING_TIMEOUT_SECONDS", "1")
				}
				ctx, cancel := context.WithCancel(c.Request.Context())
				if end == "caller_deadline" {
					cancel()
					ctx, cancel = context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
				}
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				done := make(chan *types.NewAPIError, 1)
				go func() { done <- ImageHelper(c, info) }()
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("image transfer did not start")
				}
				switch end {
				case "client_cancel", "eof_then_cancel":
					cancel()
				case "eof_success":
					// Let EOF reach the scanner while the image callback is still blocked.
					select {
					case err := <-done:
						t.Fatalf("normal EOF canceled the final image: %v", err)
					case <-time.After(50 * time.Millisecond):
					}
					releaseOnce.Do(func() { close(release) })
				}
				select {
				case apiErr := <-done:
					if end == "eof_success" {
						require.Nil(t, apiErr)
						require.Contains(t, recorder.Body.String(), "event: image_generation.completed")
						require.Equal(t, 1, info.Billing.(*imageAsyncBillingStub).settleCalls)
					} else {
						require.NotNil(t, apiErr)
						require.Zero(t, info.Billing.(*imageAsyncBillingStub).settleCalls)
						require.NotContains(t, recorder.Body.String(), "event: image_generation.completed")
						select {
						case <-canceled:
						case <-time.After(time.Second):
							t.Fatal("image HTTP request was not canceled")
						}
					}
				case <-time.After(3 * time.Second):
					t.Fatal("image processing did not stop while the remote server remained blocked")
				}
			})
		}
	}
}
