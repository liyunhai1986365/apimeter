package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAppendChannelRetryPolicyRule(t *testing.T) {
	setupChannelControllerBatchTestDB(t)

	existingSetting := `{"proxy":"socks5://127.0.0.1:1080","retry_policy_rules":[{"name":"existing","action":"skip_retry","status_codes":"400"}]}`
	channel := model.Channel{
		Type:    1,
		Key:     "sk-test",
		Name:    "retry-policy-channel",
		Models:  "gpt-image-2",
		Group:   "default",
		Setting: &existingSetting,
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/channel/1/retry_policy_rules",
		strings.NewReader(`{"rule":{"name":"private ip","action":"retry","models":["gpt-image-2"],"message_contains":["private ip"]}}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	AppendChannelRetryPolicyRule(c)

	require.Equal(t, http.StatusOK, w.Code)
	var updated model.Channel
	require.NoError(t, model.DB.First(&updated, channel.Id).Error)
	setting := updated.GetSetting()
	require.Equal(t, "socks5://127.0.0.1:1080", setting.Proxy)
	require.Len(t, setting.RetryPolicyRules, 2)
	require.Equal(t, "private ip", setting.RetryPolicyRules[1].Name)
	require.Equal(t, "retry", setting.RetryPolicyRules[1].Action)
}
