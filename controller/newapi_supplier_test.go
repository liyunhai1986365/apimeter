package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupNewAPISupplierControllerTestDB(t *testing.T) {
	t.Helper()

	originDB := model.DB
	originLogDB := model.LOG_DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.NewAPISupplier{},
		&model.NewAPISupplierChannelProfile{},
		&model.NewAPISupplierChannelProfileModel{},
	))

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	model.InitColForTest()
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		model.InitColForTest()
	})
}

func TestListNewAPISupplierChannelProfilesFiltersAndSearchesModels(t *testing.T) {
	setupNewAPISupplierControllerTestDB(t)

	linkedChannelID := 88
	matching := model.NewAPISupplierChannelProfile{
		SupplierID:           1,
		SupplierNameSnapshot: "Supplier Alpha",
		BaseURLSnapshot:      "https://alpha.example.com",
		UpstreamGroup:        "vip",
		UpstreamGroupRatio:   "0.8",
		ChannelType:          1,
		EndpointType:         "openai",
		LocalGroup:           "auto",
		ChannelID:            &linkedChannelID,
		SyncStatus:           model.NewAPISupplierChannelSyncStatusSynced,
		ChannelStatus:        model.NewAPISupplierChannelStatusAvailable,
	}
	require.NoError(t, model.DB.Create(&matching).Error)
	require.NoError(t, model.DB.Create(&model.NewAPISupplierChannelProfileModel{
		ProfileID:       matching.Id,
		SupplierID:      matching.SupplierID,
		UpstreamGroup:   matching.UpstreamGroup,
		ModelName:       "gpt-4o",
		AvailableStatus: model.NewAPISupplierProfileModelStatusAvailable,
	}).Error)

	unmatched := model.NewAPISupplierChannelProfile{
		SupplierID:           2,
		SupplierNameSnapshot: "Supplier Beta",
		BaseURLSnapshot:      "https://beta.example.com",
		UpstreamGroup:        "standard",
		UpstreamGroupRatio:   "1.2",
		ChannelType:          1,
		EndpointType:         "openai",
		LocalGroup:           "default",
		SyncStatus:           model.NewAPISupplierChannelSyncStatusPending,
		ChannelStatus:        model.NewAPISupplierChannelStatusUnavailable,
	}
	require.NoError(t, model.DB.Create(&unmatched).Error)
	require.NoError(t, model.DB.Create(&model.NewAPISupplierChannelProfileModel{
		ProfileID:       unmatched.Id,
		SupplierID:      unmatched.SupplierID,
		UpstreamGroup:   unmatched.UpstreamGroup,
		ModelName:       "claude-sonnet",
		AvailableStatus: model.NewAPISupplierProfileModelStatusAvailable,
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/newapi_suppliers/channel_profiles?model=gpt&supplier=Alpha&upstream_group=vip&local_group=auto&managed_channel=linked&sync_status=synced&channel_status=available", nil)

	ListNewAPISupplierChannelProfiles(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Contains(t, recorder.Body.String(), "Supplier Alpha")
	require.Contains(t, recorder.Body.String(), `"total":1`)
	require.NotContains(t, recorder.Body.String(), "Supplier Beta")
}
