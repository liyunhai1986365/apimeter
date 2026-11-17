package relay

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type googleImageReservation struct {
	imageAsyncBillingStub
	reserved int
	err      error
}

func (s *googleImageReservation) Reserve(n int) error { s.reserved = n; s.quota = n; return s.err }
func TestGoogleImagenFinalCountReservation(t *testing.T) {
	for _, tc := range []struct {
		body string
		want uint
	}{
		{`{"instances":[{}],"parameters":{"sampleCount":3}}`, 3},
		{`{"instances":[{}],"parameters":{}}`, 4},
	} {
		billing := &googleImageReservation{}
		original := &dto.ImageRequest{N: common.GetPointer(uint(1))}
		req := *original
		info := &relaycommon.RelayInfo{Request: original, Billing: billing, PriceData: types.PriceData{UsePrice: true, ModelPrice: .02, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}}}
		require.Nil(t, prepareOutboundImageCount(nil, info, &req, &gemini.Adaptor{}, []byte(tc.body)))
		require.Equal(t, tc.want, *req.N)
		require.Equal(t, uint(1), *original.N)
		require.Equal(t, int(tc.want), info.RequestedImageCount())
		require.Equal(t, common.QuotaRound(.02*common.QuotaPerUnit*2*float64(tc.want)), billing.reserved)
		// Imagen may filter one of the requested images. Exercise the real
		// response handler so settlement uses the two completed outputs.
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		usage, apiErr := gemini.GeminiImageHandler(ctx, info, &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{"predictions":[
				{"bytesBase64Encoded":"aW1hZ2Ux"},
				{"raiFilteredReason":"filtered"},
				{"bytesBase64Encoded":"aW1hZ2Uy"}
			]}`)),
		})
		require.Nil(t, apiErr)
		require.NotNil(t, usage)
		var response dto.ImageResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.Len(t, response.Data, 2)
		require.NotNil(t, info.BillingImageCount)
		require.Equal(t, 2, *info.BillingImageCount)
		require.Equal(t, 2.0, info.PriceData.OtherRatios["n"])
	}
}
func TestGoogleImagenCountRejectsInvalidAndKeepsTokenPricing(t *testing.T) {
	billing := &googleImageReservation{}
	info := &relaycommon.RelayInfo{Billing: billing}
	req := &dto.ImageRequest{}
	for _, body := range []string{`{"instances":[{}],"parameters":{"sampleCount":0}}`, `{"instances":[{}],"parameters":{"sampleCount":-1}}`, `{"instances":[{}],"parameters":{"sampleCount":1.5}}`} {
		err := prepareOutboundImageCount(nil, info, req, &gemini.Adaptor{}, []byte(body))
		require.NotNil(t, err)
		require.Equal(t, 400, err.StatusCode)
	}
	require.Nil(t, prepareOutboundImageCount(nil, info, req, &gemini.Adaptor{}, []byte(`{"contents":[]}`)))
	require.Nil(t, req.N)
	require.Nil(t, prepareOutboundImageCount(nil, info, req, &gemini.Adaptor{}, []byte(`{"instances":[{}],"parameters":{"sampleCount":3}}`)))
	require.Zero(t, billing.reserved)
	info.PriceData = types.PriceData{UsePrice: true, ModelPrice: .02, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{}
	require.Nil(t, prepareOutboundImageCount(nil, info, req, &gemini.Adaptor{}, []byte(`{"instances":[{}],"parameters":{"sampleCount":3}}`)))
	require.Zero(t, billing.reserved)
	info.TieredBillingSnapshot = nil
	billing.err = errors.New("insufficient quota")
	require.NotNil(t, prepareOutboundImageCount(nil, info, req, &gemini.Adaptor{}, []byte(`{"instances":[{}],"parameters":{"sampleCount":3}}`)))
}
