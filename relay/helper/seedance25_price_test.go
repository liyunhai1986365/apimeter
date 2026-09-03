package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedance25TieredPreConsumeKeepsAutomaticDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	for _, modelName := range []string{"doubao-seedance-2.5", "dreamina-seedance-2-5-260628-max"} {
		mode, err := common.Marshal(map[string]string{modelName: "tiered_expr"})
		require.NoError(t, err)
		expr, err := common.Marshal(map[string]string{modelName: `tier("base", c * 7.7)`})
		require.NoError(t, err)
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": string(mode), "billing_setting.billing_expr": string(expr)}))
		for _, path := range []string{"/api/v3/contents/generations/tasks", "/v1/video/generations", "/v1/video"} {
			t.Run(modelName+path, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, nil)
				c.Set("group", "default")
				body := []byte(`{"omni_reference_task_type":"edit","duration":-1}`)
				info := &relaycommon.RelayInfo{OriginModelName: modelName, UserGroup: "default", UsingGroup: "default", BillingRequestInput: &billingexpr.RequestInput{Body: body}}
				price, err := ModelPriceHelper(c, info, 0, &types.TokenCountMeta{MaxTokens: 250000})
				require.NoError(t, err)
				require.Equal(t, int(0.5*common.QuotaPerUnit), price.QuotaToPreConsume)
				require.Equal(t, body, info.BillingRequestInput.Body)
				require.Equal(t, 962500, info.TieredBillingSnapshot.EstimatedQuotaAfterGroup)
			})
		}
	}
}
