package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAgentGroupControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AgentUser{}, &model.AgentGroupRatio{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestGetUserGroupsUsesAgentConfiguredVisibleGroups(t *testing.T) {
	setupAgentGroupControllerTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`))
	require.NoError(t, model.DB.Create(&model.User{Id: 101, Username: "agent-user", Group: "default", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.AgentUser{AgentId: 1, UserId: 101, Status: model.AgentUserStatusEnabled, Group: "starter"}).Error)
	_, err := agentservice.UpsertGroupRatio(1, "starter", "default", 1, true, []string{"agent-vip"}, nil)
	require.NoError(t, err)
	_, err = agentservice.UpsertGroupRatio(1, "agent-vip", "vip", 1.5, false, nil, nil)
	require.NoError(t, err)
	groups, err := agentservice.EffectiveGroupMap(1)
	require.NoError(t, err)

	router := gin.New()
	router.GET("/api/user/self/groups", func(c *gin.Context) {
		c.Set("id", 101)
		common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
			AgentID: 1,
			Groups:  groups,
		})
		GetUserGroups(c)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/user/self/groups", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success bool `json:"success"`
		Data    map[string]struct {
			Ratio float64 `json:"ratio"`
			Desc  string  `json:"desc"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Contains(t, body.Data, "starter")
	require.Contains(t, body.Data, "agent-vip")
	require.Equal(t, 1.5, body.Data["agent-vip"].Ratio)
	require.Equal(t, "vip分组", body.Data["agent-vip"].Desc)
}
