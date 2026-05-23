package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelSyncControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	initModelListColumnNames(t)

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Vendor{}, &model.Option{}, &model.Ability{}))
	common.OptionMap = make(map[string]string)

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestFetchAIHubMixModelsContinuesWhenUpstreamIgnoresPageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("p")
		switch page {
		case "1":
			_, _ = w.Write([]byte(`{"success":true,"total":3,"data":[{"model":"model-a","developer":"Vendor"}]}`))
		case "2":
			_, _ = w.Write([]byte(`{"success":true,"total":3,"data":[{"model":"model-b","developer":"Vendor"}]}`))
		case "3":
			_, _ = w.Write([]byte(`{"success":true,"total":3,"data":[{"model":"model-c","developer":"Vendor"}]}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"total":3,"data":[]}`))
		}
	}))
	defer upstream.Close()
	t.Setenv("SYNC_AIHUBMIX_MODELS_URL", upstream.URL)
	t.Setenv("SYNC_AIHUBMIX_PAGE_SIZE", "200")
	t.Setenv("SYNC_AIHUBMIX_MAX_PAGES", "10")

	items, _, err := fetchAIHubMixModels(t.Context())

	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, "model-a", items[0].Model)
	require.Equal(t, "model-c", items[2].Model)
}

func TestSyncUpstreamModelsOverwriteAllUpdatesExistingOfficialModels(t *testing.T) {
	setupModelSyncControllerTestDB(t)
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/newapi/models.json":
			_, _ = w.Write([]byte(`[
				{
					"model_name":"gpt-test",
					"description":"new description",
					"icon":"new-icon",
					"tags":"chat,reasoning",
					"category":"text",
					"vendor_name":"NewVendor",
					"name_rule":1,
					"status":2,
					"endpoints":{"chat_completions":true,"responses":true}
				}
			]`))
		case "/api/newapi/vendors.json":
			_, _ = w.Write([]byte(`[
				{"name":"NewVendor","description":"new vendor description","icon":"new-vendor-icon","status":1}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	t.Setenv("SYNC_UPSTREAM_BASE", upstream.URL)
	t.Setenv("SYNC_HTTP_RETRY", "1")

	oldVendor := &model.Vendor{Name: "OldVendor", Description: "old vendor", Status: 1}
	require.NoError(t, oldVendor.Insert())
	require.NoError(t, (&model.Model{
		ModelName:    "gpt-test",
		Description:  "old description",
		Icon:         "old-icon",
		Tags:         "old",
		Category:     "legacy",
		VendorID:     oldVendor.Id,
		Endpoints:    `{"old":true}`,
		Status:       1,
		SyncOfficial: 1,
		NameRule:     0,
	}).Insert())

	body, err := common.Marshal(gin.H{"source": "official", "overwrite_all": true})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_upstream", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	SyncUpstreamModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var local model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "gpt-test").First(&local).Error)
	require.Equal(t, "new description", local.Description)
	require.Equal(t, "new-icon", local.Icon)
	require.Equal(t, "chat,reasoning", local.Tags)
	require.Equal(t, "text", local.Category)
	require.JSONEq(t, `{"chat_completions":true,"responses":true}`, local.Endpoints)
	require.Equal(t, 2, local.Status)
	require.Equal(t, 1, local.NameRule)

	var vendor model.Vendor
	require.NoError(t, model.DB.First(&vendor, local.VendorID).Error)
	require.Equal(t, "NewVendor", vendor.Name)
}

func TestSyncAIHubMixModelsOverwriteAllUpdatesLocalModelsDisabledForOfficialSync(t *testing.T) {
	setupModelSyncControllerTestDB(t)
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/call/mdl_info_pagination", r.URL.Path)
		_, _ = w.Write([]byte(`{
			"success": true,
			"total": 1,
			"data": [
				{
					"model":"gpt-5.5",
					"developer":"OpenAI",
					"desc":"new description",
					"desc_en":"new English description",
					"features":"thinking,function_calling",
					"modalities":"text,image",
					"flag":1,
					"model_ratio":2.5,
					"completion_ratio":6,
					"cache_ratio":0.1,
					"endpoints":"chat_completions,responses",
					"context_window":1050000
				}
			]
		}`))
	}))
	defer upstream.Close()
	t.Setenv("SYNC_AIHUBMIX_MODELS_URL", upstream.URL+"/call/mdl_info_pagination")

	oldVendor := &model.Vendor{Name: "OldVendor", Description: "old vendor", Status: 1}
	require.NoError(t, oldVendor.Insert())
	require.NoError(t, (&model.Model{
		ModelName:    "gpt-5.5",
		Description:  "old description",
		Icon:         "old-icon",
		Tags:         "old",
		Category:     "legacy",
		VendorID:     oldVendor.Id,
		Endpoints:    `{"old":true}`,
		Status:       2,
		SyncOfficial: 0,
		NameRule:     0,
	}).Insert())

	body, err := common.Marshal(gin.H{"source": "aihubmix", "overwrite_all": true, "locale": "zh"})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_upstream", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	SyncUpstreamModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var local model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "gpt-5.5").First(&local).Error)
	require.Equal(t, "new description", local.Description)
	require.Equal(t, "thinking,function_calling,text,image,context:1050000", local.Tags)
	require.Equal(t, "image", local.Category)
	require.JSONEq(t, `{"chat_completions":true,"responses":true}`, local.Endpoints)
	require.Equal(t, 1, local.Status)

	var vendor model.Vendor
	require.NoError(t, model.DB.First(&vendor, local.VendorID).Error)
	require.Equal(t, "OpenAI", vendor.Name)
}

func TestSyncAIHubMixModelsWithoutOverwriteAllKeepsExistingLocalModels(t *testing.T) {
	setupModelSyncControllerTestDB(t)
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"success": true,
			"total": 1,
			"data": [
				{
					"model":"gpt-5.5",
					"developer":"OpenAI",
					"desc":"new description",
					"features":"thinking",
					"modalities":"text",
					"flag":1,
					"model_ratio":2.5
				}
			]
		}`))
	}))
	defer upstream.Close()
	t.Setenv("SYNC_AIHUBMIX_MODELS_URL", upstream.URL)

	require.NoError(t, (&model.Model{
		ModelName:    "gpt-5.5",
		Description:  "old description",
		Tags:         "old",
		Category:     "legacy",
		Status:       2,
		SyncOfficial: 1,
	}).Insert())

	body, err := common.Marshal(gin.H{"source": "aihubmix", "locale": "zh"})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_upstream", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	SyncUpstreamModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var local model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "gpt-5.5").First(&local).Error)
	require.Equal(t, "old description", local.Description)
	require.Equal(t, "old", local.Tags)
	require.Equal(t, "legacy", local.Category)
	require.Equal(t, 2, local.Status)
}

func TestBuildAIHubMixUpstreamModelUsesLocaleDescriptionAndMetadata(t *testing.T) {
	item := aihubmixModel{
		Model:          "gpt-5.5",
		Developer:      "OpenAI",
		Desc:           "中文描述",
		DescEn:         "English description",
		Features:       "thinking,tools",
		Modalities:     "text,image",
		Endpoints:      "chat_completions,responses",
		Flag:           1,
		ContextWindow:  1050000,
		ModelRatio:     2.5,
		CompletionRate: 6,
		CacheRatio:     0.1,
	}

	up := buildAIHubMixUpstreamModel(item, "en")

	require.Equal(t, "gpt-5.5", up.ModelName)
	require.Equal(t, "English description", up.Description)
	require.Equal(t, "OpenAI", up.VendorName)
	require.Equal(t, "thinking,tools,text,image,context:1050000", up.Tags)
	require.Equal(t, "image", up.Category)
	require.Equal(t, 1, up.Status)
	require.JSONEq(t, `{"chat_completions":true,"responses":true}`, string(up.Endpoints))
}

func TestSyncAIHubMixPricingMergesBasicRatioSettings(t *testing.T) {
	setupModelSyncControllerTestDB(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"existing-model":1.25}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"existing-model":2}`))
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"existing-model":0.5}`))

	items := []aihubmixModel{
		{
			Model:          "gpt-5.5",
			ModelRatio:     2.5,
			CompletionRate: 6,
			CacheRatio:     0.1,
		},
		{
			Model:      "ignored-no-ratio",
			ModelRatio: 0,
		},
	}

	updated, err := syncAIHubMixPricing(items)
	require.NoError(t, err)
	require.Equal(t, 1, updated)

	modelRatios := ratio_setting.GetModelRatioCopy()
	require.Equal(t, 1.25, modelRatios["existing-model"])
	require.Equal(t, 2.5, modelRatios["gpt-5.5"])

	completionRatios := ratio_setting.GetCompletionRatioCopy()
	require.Equal(t, 2.0, completionRatios["existing-model"])
	require.Equal(t, 6.0, completionRatios["gpt-5.5"])

	cacheRatios := ratio_setting.GetCacheRatioCopy()
	require.Equal(t, 0.5, cacheRatios["existing-model"])
	require.Equal(t, 0.1, cacheRatios["gpt-5.5"])
}
