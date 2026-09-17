package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestChannelManagementPreservesConcurrentAssetCredentialRotation(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "channel-management-test-only")
	for _, credentials := range []struct {
		name, backend, auth string
		original, rotated   model.AssetCredentials
	}{
		{"api_key", "tgxmaas", "api_key", model.AssetCredentials{APIKey: "old-asset-key"}, model.AssetCredentials{APIKey: "new-asset-key"}},
		{"aksk", "volcengine-assets", "aksk", model.AssetCredentials{AccessKeyID: "old-ak", SecretAccessKey: "old-sk"}, model.AssetCredentials{AccessKeyID: "new-ak", SecretAccessKey: "new-sk"}},
	} {
		for _, operation := range []string{"append_retry_rule", "disable_key", "enable_key", "disable_all_keys", "enable_all_keys", "delete_key", "delete_disabled_keys"} {
			t.Run(credentials.name+"/"+operation, func(t *testing.T) {
				openConfigurableResourceTestDB(t)
				ch := &model.Channel{
					Name: "management-rotation", Type: constant.ChannelTypeConfigurable,
					Key: "video-a\nvideo-b\nvideo-c", Models: "model", Group: "default",
					Status: common.ChannelStatusEnabled, UsedQuota: 100,
					ChannelInfo: model.ChannelInfo{
						IsMultiKey: true, MultiKeySize: 3, MultiKeyMode: constant.MultiKeyModePolling,
						MultiKeyStatusList:     map[int]int{1: common.ChannelStatusAutoDisabled},
						MultiKeyDisabledReason: map[int]string{1: "upstream failure"},
						MultiKeyDisabledTime:   map[int]int64{1: 123},
					},
				}
				ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{
					ProfileID: "seedance-tgxmaas", ProjectName: "mock-project",
					AssetLibrary: &relaydto.AssetLibrarySettings{Backend: credentials.backend, AuthMode: credentials.auth},
				}})
				require.NoError(t, ch.SetAssetCredentials(&credentials.original))
				require.NoError(t, ch.Insert())
				rotated := *ch
				require.NoError(t, rotated.SetAssetCredentials(&credentials.rotated))

				// Commit a credential rotation after the handler reads the channel,
				// but before its update starts. No timing-dependent goroutines.
				fired := false
				const callback = "test:rotate_assets_before_channel_management_write"
				reads := 0
				rotate := func(tx *gorm.DB) {
					if fired || tx.Statement.Table != "channels" {
						return
					}
					if operation == "append_retry_rule" {
						reads++
						if reads != 2 {
							return
						}
					}
					fired = true
					require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Updates(map[string]any{
						"asset_secret": rotated.AssetSecret,
						"name":         "concurrent-name",
						"used_quota":   gorm.Expr("used_quota + ?", 250),
					}).Error)
				}
				if operation == "append_retry_rule" {
					require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callback, rotate))
					t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(callback) })
				} else {
					require.NoError(t, model.DB.Callback().Update().Before("gorm:begin_transaction").Register(callback, rotate))
					t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callback) })
				}

				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Set("role", common.RoleRootUser)
				c.Set("id", 1)
				if operation == "append_retry_rule" {
					c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(ch.Id)}}
					c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/1/retry_policy_rules", strings.NewReader(`{"rule":{"name":"probe","action":"skip_retry","status_codes":"400"}}`))
					c.Request.Header.Set("Content-Type", "application/json")
					AppendChannelRetryPolicyRule(c)
				} else {
					c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key/manage", strings.NewReader(fmt.Sprintf(`{"channel_id":%d,"action":%q,"key_index":1}`, ch.Id, operation)))
					c.Request.Header.Set("Content-Type", "application/json")
					ManageMultiKeys(c)
				}
				require.Equal(t, http.StatusOK, w.Code)
				require.True(t, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
				require.True(t, fired)
				stored, err := model.GetChannelById(ch.Id, true)
				require.NoError(t, err)
				actual, err := stored.GetAssetCredentials()
				require.NoError(t, err)
				require.Equal(t, credentials.rotated, *actual)
				require.Equal(t, "concurrent-name", stored.Name)
				require.Equal(t, int64(350), stored.UsedQuota)

				if operation == "delete_key" || operation == "delete_disabled_keys" {
					require.Equal(t, "video-a\nvideo-c", stored.Key)
					require.Equal(t, 2, stored.ChannelInfo.MultiKeySize)
				} else {
					require.Equal(t, ch.Key, stored.Key)
					require.Equal(t, 3, stored.ChannelInfo.MultiKeySize)
				}
				switch operation {
				case "append_retry_rule":
					require.Equal(t, ch.ChannelInfo, stored.ChannelInfo)
					rules := stored.GetSetting().RetryPolicyRules
					require.Len(t, rules, 1)
					require.Equal(t, "probe", rules[0].Name)
				case "disable_key":
					require.Equal(t, map[int]int{1: common.ChannelStatusManuallyDisabled}, stored.ChannelInfo.MultiKeyStatusList)
				case "disable_all_keys":
					require.Equal(t, map[int]int{0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusAutoDisabled, 2: common.ChannelStatusManuallyDisabled}, stored.ChannelInfo.MultiKeyStatusList)
				default:
					require.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
					require.Empty(t, stored.ChannelInfo.MultiKeyDisabledReason)
					require.Empty(t, stored.ChannelInfo.MultiKeyDisabledTime)
				}
				if operation != "append_retry_rule" {
					require.Equal(t, ch.Setting, stored.Setting)
				}
			})
		}
	}
}
