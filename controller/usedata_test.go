package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type usageDimensionTrendAPIResponse struct {
	Success bool                            `json:"success"`
	Data    []model.UsageDimensionTrendData `json:"data"`
}

type quotaDataAPIResponse struct {
	Success bool              `json:"success"`
	Data    []model.QuotaData `json:"data"`
}

func setupUseDataControllerTestDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.AutoMigrate(&model.Workspace{}, &model.Token{}, &model.Log{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestGetAllUsageDimensionTrendsUsesCurrentAdminAccount(t *testing.T) {
	db := setupUseDataControllerTestDB(t)

	require.NoError(t, db.Create(&[]model.Workspace{
		{Id: 301, UserId: 1001, Name: "Admin Workspace", Status: model.WorkspaceStatusEnabled},
		{Id: 302, UserId: 2002, Name: "Other Workspace", Status: model.WorkspaceStatusEnabled},
	}).Error)
	require.NoError(t, db.Create(&[]model.Token{
		{Id: 401, UserId: 1001, WorkspaceId: 301, Name: "admin-key", Key: "admin-key-value"},
		{Id: 402, UserId: 2002, WorkspaceId: 302, Name: "other-key", Key: "other-key-value"},
	}).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{Id: 501, UserId: 1001, Username: "admin", Type: model.LogTypeConsume, CreatedAt: 3600, TokenId: 401, TokenName: "admin-key", Quota: 100, PromptTokens: 10, CompletionTokens: 20},
		{Id: 502, UserId: 2002, Username: "other", Type: model.LogTypeConsume, CreatedAt: 3600, TokenId: 402, TokenName: "other-key", Quota: 900, PromptTokens: 30, CompletionTokens: 40},
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/data/dimensions?start_timestamp=0&end_timestamp=10000", nil, 1001)

	GetAllUsageDimensionTrends(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response usageDimensionTrendAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data, 1)
	require.Equal(t, "admin-key", response.Data[0].TokenName)
	require.Equal(t, "Admin Workspace", response.Data[0].WorkspaceName)
}

func TestGetUserQuotaDatesMergesTaskRefundWithPreConsume(t *testing.T) {
	db := setupUseDataControllerTestDB(t)

	require.NoError(t, db.Create(&[]model.Log{
		{
			Id:        600,
			UserId:    1001,
			Username:  "alice",
			Type:      model.LogTypeConsume,
			CreatedAt: 3600,
			ModelName: "seedance-2.0",
			Quota:     1000,
		},
		{
			Id:               601,
			UserId:           1001,
			Username:         "alice",
			Type:             model.LogTypeRefund,
			CreatedAt:        3600,
			ModelName:        "seedance-2.0",
			Quota:            700,
			PromptTokens:     10,
			CompletionTokens: 20,
			Other:            `{"pre_consumed_quota":1000,"actual_quota":300}`,
		},
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/data/self?start_timestamp=0&end_timestamp=10000", nil, 1001)

	GetUserQuotaDates(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response quotaDataAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	var totalQuota, totalCount, totalTokens int
	for _, item := range response.Data {
		totalQuota += item.Quota
		totalCount += item.Count
		totalTokens += item.TokenUsed
	}
	require.Equal(t, 300, totalQuota)
	require.Equal(t, 1, totalCount)
	require.Equal(t, 30, totalTokens)
}
