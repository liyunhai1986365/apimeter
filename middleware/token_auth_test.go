package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openTokenAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalSQLitePath := common.SQLitePath
	originalIsMasterNode := common.IsMasterNode
	t.Setenv("SQL_DSN", "local")
	common.SQLitePath = "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true
	common.IsMasterNode = false

	require.NoError(t, model.InitDB())
	db := model.DB
	require.NotNil(t, db)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}))
	model.LOG_DB = db

	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
		common.SQLitePath = originalSQLitePath
		common.IsMasterNode = originalIsMasterNode
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func seedTokenAuthAutoGroupFixture(t *testing.T, db *gorm.DB) {
	t.Helper()

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	require.NoError(t, db.Create(&model.User{
		Id:       1,
		Username: "auto-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         1,
		Name:           "auto-token",
		Key:            "autotokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "auto",
	}).Error)

	weight := uint(100)
	priority := int64(10)
	autoBan := 1
	require.NoError(t, db.Create(&model.Channel{
		Id:       1001,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "upstream-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "default-channel",
		Group:    "default",
		Models:   "gpt-test",
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-test",
		ChannelId: 1001,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()
}

func TestTokenAuthAllowsAutoTokenGroupAndDistributorSelectsDefaultAutoGroup(t *testing.T) {
	db := openTokenAuthTestDB(t)
	seedTokenAuthAutoGroupFixture(t, db)

	router := gin.New()
	router.POST("/v1/chat/completions", TokenAuth(), Distribute(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"using_group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
			"auto_group":  common.GetContextKeyString(c, constant.ContextKeyAutoGroup),
			"channel_id":  common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer autotokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		UsingGroup string `json:"using_group"`
		AutoGroup  string `json:"auto_group"`
		ChannelId  int    `json:"channel_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "auto", response.UsingGroup)
	require.Equal(t, "default", response.AutoGroup)
	require.Equal(t, 1001, response.ChannelId)
}
