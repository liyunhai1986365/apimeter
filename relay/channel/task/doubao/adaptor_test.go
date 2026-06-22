package doubao

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func TestTaskAdaptorAcceptsSeedanceNativeContentRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"doubao-seedance-2-0-fast-260128",
		"content":[{"type":"text","text":"一只金色柴犬在樱花树下奔跑"}],
		"resolution":"720p",
		"ratio":"16:9",
		"duration":5,
		"watermark":false
	}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if _, err := common.GetBodyStorage(c); err != nil {
		t.Fatalf("cache body: %v", err)
	}

	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-fast-260128",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeVolcEngine,
			ChannelBaseUrl:    "https://ark.cn-beijing.volces.com",
			ApiKey:            "sk-test",
			UpstreamModelName: "doubao-seedance-2-0-fast-260128",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("validate native seedance request: %v", taskErr)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	upstreamBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := gjson.GetBytes(upstreamBody, "content.0.type").String(); got != "text" {
		t.Fatalf("unexpected content type: %s body=%s", got, upstreamBody)
	}
	if got := gjson.GetBytes(upstreamBody, "content.0.text").String(); got != "一只金色柴犬在樱花树下奔跑" {
		t.Fatalf("unexpected text content: %s body=%s", got, upstreamBody)
	}
	if got := gjson.GetBytes(upstreamBody, "duration").Int(); got != 5 {
		t.Fatalf("unexpected duration: %d body=%s", got, upstreamBody)
	}

	ratios := adaptor.EstimateBilling(c, info)
	if got := ratios["seconds"]; got != 5 {
		t.Fatalf("unexpected seconds ratio: %v", got)
	}
}
