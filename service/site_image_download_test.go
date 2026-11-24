package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSiteImageLocalRoundTrip(t *testing.T) {
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	for _, tc := range []struct{ origin, base string }{
		{"http://localhost:3000", ""},
		{"http://gateway.lan:3000", ""},
		{"https://gateway.example.org", ""},
		{"http://gateway.lan", "http://assets.internal:8080/gateway"},
	} {
		t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", tc.base)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", tc.origin+"/", nil)
		raw, err := SaveTemporaryImageBytes(c, []byte("disk image"), "image/png", "img.png")
		require.NoError(t, err)
		resp, err := DoDownloadRequest(raw)
		require.NoError(t, err)
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, err)
		require.Equal(t, "disk image", string(data))
		_, matched, _ := downloadSiteImage(t.Context(), raw+"?extra=1")
		require.False(t, matched)
	}
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "")
	relative, err := SaveTemporaryImageBytes(nil, []byte("async image"), "image/png", "img.png")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(relative, relayTempImageRoutePrefix))
	resp, err := DoDownloadRequest(relative)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "async image", string(data))
}

func TestSiteImageCannotEscapeLocalDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("API_TEMP_IMAGE_DIR", dir)
	name := "84276077-349c-4ef8-8840-2e14c033e015.png"
	outside := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, name)))
	for _, id := range []string{name, "../" + name} {
		_, _, err := openSiteImage(id)
		require.Error(t, err)
	}
}

func TestSiteImagePrivateStorageRoundTrip(t *testing.T) {
	for _, kind := range []string{"oss", "r2"} {
		for _, signed := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/signed=%t", kind, signed), func(t *testing.T) {
				var uploads, reads, followed atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/other" {
						followed.Add(1)
						return
					}
					if r.Method == "PUT" {
						uploads.Add(1)
						return
					}
					if reads.Add(1) > 1 {
						http.Redirect(w, r, "/other", 302)
						return
					}
					fmt.Fprint(w, "stored image")
				}))
				defer srv.Close()
				t.Setenv("API_TEMP_IMAGE_STORAGE", kind)
				prefix := "API_TEMP_IMAGE_" + strings.ToUpper(kind) + "_"
				t.Setenv(prefix+"ENDPOINT", srv.URL)
				t.Setenv(prefix+"BUCKET", "site")
				t.Setenv(prefix+"ACCESS_KEY_ID", "test")
				t.Setenv(prefix+"ACCESS_KEY_SECRET", "test-secret")
				if signed {
					t.Setenv(prefix+"PUBLIC_BASE_URL", "")
				} else {
					t.Setenv(prefix+"PUBLIC_BASE_URL", srv.URL+"/cdn")
				}
				raw, err := SaveTemporaryImageBytes(nil, []byte("image"), "image/png", "test.png")
				require.NoError(t, err)
				resp, err := DoDownloadRequest(raw)
				require.NoError(t, err)
				data, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				require.NoError(t, err)
				require.Equal(t, "stored image", string(data))
				resp, err = DoDownloadRequest(raw)
				require.NoError(t, err)
				resp.Body.Close()
				require.Equal(t, 302, resp.StatusCode)
				_, err = DoDownloadRequest(raw + "&extra=1")
				require.Error(t, err)
				_, err = DoDownloadRequest(srv.URL + "/other")
				require.Error(t, err)
				if signed {
					u, _ := url.Parse(raw)
					q := u.Query()
					if kind == "oss" {
						q.Set("Signature", "invalid")
					} else {
						q.Set("X-Amz-Signature", "invalid")
					}
					u.RawQuery = q.Encode()
					_, err = DoDownloadRequest(u.String())
					require.Error(t, err)

					cfg, err := tempImageStorageConfigFromEnv(nil)
					require.NoError(t, err)
					// Correct signatures which are already expired must fail too.
					past := time.Now().UTC().Add(-2 * time.Hour)
					if kind == "oss" {
						exp := strconv.FormatInt(past.Unix(), 10)
						q.Set("Expires", exp)
						q.Set("Signature", ossSignature(cfg.OSSAccessKeySecret, "GET\n\n\n"+exp+"\n"+u.Path))
						u.RawQuery = q.Encode()
					} else {
						u.RawQuery = ""
						r, err := http.NewRequest(http.MethodGet, u.String(), nil)
						require.NoError(t, err)
						u, err = url.Parse(presignR2RequestAt(r, cfg, time.Hour, past))
						require.NoError(t, err)
					}
					_, err = DoDownloadRequest(u.String())
					require.Error(t, err)
				}
				require.EqualValues(t, 1, uploads.Load())
				require.EqualValues(t, 2, reads.Load())
				require.Zero(t, followed.Load())
			})
		}
	}
}

func TestPersonalNotificationNeverUsesWorkerOrPrivateDestination(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	old := system_setting.WorkerUrl
	system_setting.WorkerUrl = "http://worker.invalid"
	t.Cleanup(func() { system_setting.WorkerUrl = old })
	msg := dto.Notify{Title: "test"}
	require.Error(t, sendBarkNotify("http://127.0.0.1", msg))
	require.Error(t, sendGotifyNotify("http://127.0.0.1", "test", 1, msg))
	require.Error(t, SendWebhookNotify("http://127.0.0.1", "", msg, dto.WebhookFormatGeneric))
}
