package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSandGenerationGateway(t *testing.T) {
	const v2, ark = relaycommon.SandGenerationsPath, relaycommon.SandArkTasksPath
	for _, tc := range []struct {
		name, entry, upstream, input, wire, outcome string
		duration                                    int
	}{
		{"native", v2, v2, `{"parts":[{"type":"reference_video","video":{"url":"https://media.test/v"}},{"type":"text","text":"scene"}],"aspectRatio":"16:9","duration":6,"seed":0,"generateAudio":false}`, "", "succeeded", 6},
		{"ark-suffix", ark, ark, `{"content":[{"type":"text","text":"scene --rt 16:9 --dur 7 --rs 1080p"}],"generate_audio":false,"seed":-1}`, "", "succeeded", 7},
		{"ark-refund", ark, ark, `{"content":[{"type":"audio_url","audio_url":{"url":"https://media.test/a"},"role":"reference_audio"}],"ratio":"1:1","duration":8}`, "", "failed", 8},
		{"generic-frames", "/v1/videos", ark, `{"image_url":"https://media.test/i","end_image_url":"https://media.test/e","generate_audio":false}`, `{"model":"magi-2","duration":5,"ratio":"16:9","content":[{"type":"image_url","role":"first_frame","image_url":{"url":"https://media.test/i"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://media.test/e"}}],"generate_audio":false}`, "succeeded", 5},
		{"generic-references", "/v1/videos", v2, `{"prompt":"scene","image_urls":["https://media.test/i"],"audio_urls":["https://media.test/a"],"seconds":"9","seed":0}`, `{"model":"magi-2","duration":9,"aspectRatio":"16:9","parts":[{"type":"text","text":"scene"},{"type":"reference_image","image":{"url":"https://media.test/i"}},{"type":"reference_audio","audio":{"url":"https://media.test/a"}}],"seed":0}`, "succeeded", 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := `{"model":"magi-2",` + tc.input[1:]
			const upstreamID = "987654321"
			var polls atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer sand-test-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					require.Equal(t, tc.upstream, r.URL.Path)
					b, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					want := tc.wire
					if want == "" {
						want = input
						if tc.name == "ark-suffix" {
							want = strings.TrimSuffix(want, "}") + `,"duration":7}`
						}
					}
					require.JSONEq(t, want, string(b))
					if tc.upstream == v2 {
						w.WriteHeader(http.StatusAccepted)
					}
					fmt.Fprintf(w, `{"id":"%s"}`, upstreamID)
					return
				}
				require.Equal(t, tc.upstream+"/"+upstreamID, r.URL.Path)
				url := fmt.Sprintf("https://media.test/output?signature=%d", polls.Add(1))
				details := fmt.Sprintf(`"content":{"video_url":%q}`, url)
				if tc.upstream == v2 {
					details = fmt.Sprintf(`"createdAt":"2026-11-01T02:00:00.000Z","output":{"videoUrl":%q,"urlExpiresAt":"2026-11-01T03:00:00.000Z"},"error":null`, url)
				}
				if tc.outcome == "failed" {
					details = `"error":{"code":"test_error","message":"generation failed"}`
				}
				fmt.Fprintf(w, `{"id":"%s","status":"%s",%s}`, upstreamID, tc.outcome, details)
			}))
			defer api.Close()
			r := seedanceTestRouter(t, api.URL, "sand-test-key", relaycommon.MagiModel)
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 20).Updates(map[string]any{"type": constant.ChannelTypeConfigurable, "setting": `{"protocol":{"profile_id":"sandai-official-magi-2"}}`}).Error)
			oldMode := common.GetJsonString(billing_setting.GetBillingModeCopy())
			oldExpr := common.GetJsonString(billing_setting.GetBillingExprCopy())
			t.Cleanup(func() {
				billing_setting.UpdateBillingField("billing_mode", oldMode)
				billing_setting.UpdateBillingField("billing_expr", oldExpr)
			})
			billing_setting.UpdateBillingField("billing_mode", `{"magi-2":"tiered_expr"}`)
			billing_setting.UpdateBillingField("billing_expr", `{"magi-2":"tier(\"base\",param(\"duration\") * 0.1 * 1000000)"}`)
			for _, path := range []string{v2, ark} {
				r.POST(path, middleware.ConfigurableNativeProfile(relaycommon.OfficialMagiProfileID, relayconstant.RelayModeVideoSubmit), middleware.TokenAuth(), middleware.Distribute(), RelayTask)
				r.GET(path+"/:task_id", middleware.ConfigurableNativeProfile(relaycommon.OfficialMagiProfileID, relayconstant.RelayModeVideoFetchByID), middleware.TokenAuth(), middleware.DistributeVideoTaskFetch(), RelayTaskFetch)
			}
			r.POST("/v1/videos", middleware.TokenAuth(), middleware.Distribute(), RelayTask)
			r.GET("/v1/videos/:task_id", middleware.TokenAuth(), middleware.DistributeVideoTaskFetch(), RelayTaskFetch)
			submit := seedanceCall(r, "POST", tc.entry, input, 1)
			status := 200
			if tc.entry == v2 {
				status = 202
			}
			require.Equal(t, status, submit.Code, submit.Body.String())
			id := gjson.Get(submit.Body.String(), "id").String()
			require.NotEmpty(t, id)
			require.NotEqual(t, upstreamID, id)
			if tc.entry != "/v1/videos" {
				_, err := strconv.ParseInt(id, 10, 64)
				require.NoError(t, err)
				require.Equal(t, 404, seedanceCall(r, "GET", tc.entry+"/"+id, "", 2).Code)
			}
			if tc.entry == v2 {
				require.Equal(t, v2+"/"+id, submit.Header().Get("Location"))
			}
			task, exists, err := model.GetByTaskId(1, id)
			require.NoError(t, err)
			require.True(t, exists)
			require.Equal(t, tc.upstream == ark, task.PrivateData.SandArk)
			charge := int(float64(tc.duration) * common.QuotaPerUnit / 10)
			require.Equal(t, charge, task.Quota)
			for i := range 2 {
				result := seedanceCall(r, "GET", tc.entry+"/"+id, "", 1)
				require.Equal(t, 200, result.Code, result.Body.String())
				require.Equal(t, id, gjson.Get(result.Body.String(), "id").String())
				if tc.entry != "/v1/videos" {
					require.Equal(t, tc.outcome, gjson.Get(result.Body.String(), "status").String())
				}
				if tc.outcome == "succeeded" {
					path := "metadata.url"
					switch tc.entry {
					case v2:
						path = "output.videoUrl"
					case ark:
						path = "content.video_url"
					}
					require.Equal(t, fmt.Sprintf("https://media.test/output?signature=%d", i+1), gjson.Get(result.Body.String(), path).String())
				}
			}
			if tc.outcome == "failed" {
				charge = 0
			}
			var user model.User
			require.NoError(t, model.DB.First(&user, 1).Error)
			require.Equal(t, 10000000-charge, user.Quota)
		})
	}
}
