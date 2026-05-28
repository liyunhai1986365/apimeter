package common_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/conversion"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGenRelayInfoUsesImageSemanticsForCompatibleImageChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-image-2","messages":[{"role":"user","content":"draw a river"}]}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("original_model", "gpt-image-2")

	request, err := helper.GetAndValidateRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	_, ok := request.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	request, plan, err := conversion.ApplyRequest(c, types.RelayFormatOpenAI, constant.RelayModeChatCompletions, request)
	require.NoError(t, err)
	require.NotNil(t, plan)
	plan.Store(c)
	_, ok = request.(*dto.ImageRequest)
	require.True(t, ok)

	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, request, nil)
	require.NoError(t, err)

	require.EqualValues(t, types.RelayFormatOpenAIImage, info.RelayFormat)
	require.Equal(t, constant.RelayModeImagesGenerations, info.RelayMode)
	require.Equal(t, "/v1/images/generations", info.RequestURLPath)
	require.Equal(t, []types.RelayFormat{types.RelayFormatOpenAIImage}, info.RequestConversionChain)
	require.Equal(t, string(conversion.ConversionOpenAIChatToImageGenerations), info.ConversionID)
	require.Equal(t, string(conversion.RequestModeOpenAIChat), info.SourceRequestMode)
	require.Equal(t, string(conversion.RequestModeOpenAIImageGenerations), info.TargetRequestMode)
	require.True(t, info.PreserveResponseMode)
}
