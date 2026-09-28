package configurable

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSeedanceIndependentAssetBackendConversion(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, scenario := range []string{"authorized-tgx", "plain-video", "official-assets"} {
			t.Run(map[bool]string{false: "generic", true: "native"}[native]+"/"+scenario, func(t *testing.T) {
				uri, expected, backend := "asset://custom-original", "asset://custom-local", "tgxmaas"
				aliases := map[string]string{"custom-original": "custom-local"}
				project := common.GetPointer("owned-project")
				switch scenario {
				case "plain-video":
					uri, expected, aliases, project = "https://example.com/image.png", "https://example.com/image.png", nil, nil
				case "official-assets":
					backend, expected, aliases = OfficialAssetBackend, uri, map[string]string{}
				}
				content := `[{"type":"image_url","image_url":{"url":"` + uri + `"},"role":"first_frame"}]`
				path, body := "/v1/videos", `{"model":"seedance-2","duration":4,"metadata":{"content":`+content+`}}`
				if native {
					path, body = "/api/v3/contents/generations/tasks", `{"model":"seedance-2","duration":4,"content":`+content+`,"watermark":false}`
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", path, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(c) })
				info := &relaycommon.RelayInfo{OriginModelName: "seedance-2", ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: constant.ChannelTypeConfigurable, UpstreamModelName: "seedance-upstream",
					ChannelSetting: dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-ark-task-assets", ProjectName: "default-project", AssetLibrary: &dto.AssetLibrarySettings{Backend: backend}}},
				}, TaskRelayInfo: &relaycommon.TaskRelayInfo{AssetAliases: aliases, AssetProject: project}}
				adaptor := &TaskAdaptor{}
				adaptor.Init(info)
				require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
				reader, err := adaptor.BuildRequestBody(c, info)
				require.NoError(t, err)
				result, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Equal(t, expected, gjson.GetBytes(result, "content.0.image_url.url").String())
				require.Equal(t, "first_frame", gjson.GetBytes(result, "content.0.role").String())
				if scenario == "authorized-tgx" {
					require.Equal(t, "owned-project", gjson.GetBytes(result, "ProjectName").String())
				} else {
					require.False(t, gjson.GetBytes(result, "ProjectName").Exists())
				}
				if native {
					require.Equal(t, gjson.False, gjson.GetBytes(result, "watermark").Type)
				}
			})
		}
	}
}
