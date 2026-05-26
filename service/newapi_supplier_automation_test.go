package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupNewAPISupplierAutomationTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.NewAPISupplier{},
		&model.NewAPISupplierChannel{},
		&model.NewAPISupplierChannelProfile{},
		&model.NewAPISupplierChannelProfileModel{},
	))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })
}

func TestSyncNewAPISupplierChannelProfilesStoresGroupModelsAndRatios(t *testing.T) {
	setupNewAPISupplierAutomationTestDB(t)
	weight := uint(3)
	priority := int64(8)
	supplier := &model.NewAPISupplier{
		Name:              "upstream-a",
		BaseURL:           "https://upstream.example.com/",
		Tag:               "supplier-a",
		DefaultLocalGroup: "auto",
		ChannelType:       1,
		Weight:            &weight,
		Priority:          &priority,
		AutoBan:           1,
		ModelSource:       "pricing",
	}
	require.NoError(t, supplier.Insert())

	result, err := SyncNewAPISupplierChannelProfiles(supplier, []NewAPISupplierGroupSnapshot{{
		Group:  "vip",
		Models: []string{"gpt-4o", "gpt-5"},
		Source: "pricing",
		Ratio:  "0.8",
		Desc:   "VIP group",
		ModelProviders: map[string][]string{
			"gpt-4o": {"OpenAI"},
			"gpt-5":  {"OpenAI"},
		},
	}})

	require.NoError(t, err)
	require.Len(t, result.Profiles, 1)
	assert.Equal(t, "vip", result.Profiles[0].UpstreamGroup)
	assert.Equal(t, "0.8", result.Profiles[0].UpstreamGroupRatio)
	assert.Equal(t, "auto", result.Profiles[0].LocalGroup)
	assert.Equal(t, "openai", result.Profiles[0].EndpointType)
	assert.Equal(t, model.NewAPISupplierChannelSyncStatusPending, result.Profiles[0].SyncStatus)
	require.NotNil(t, result.Profiles[0].ChannelRatio)
	assert.Equal(t, 0.8, *result.Profiles[0].ChannelRatio)

	var profileModels []model.NewAPISupplierChannelProfileModel
	require.NoError(t, model.DB.Order("model_name asc").Find(&profileModels).Error)
	require.Len(t, profileModels, 2)
	assert.Equal(t, "gpt-4o", profileModels[0].ModelName)
	assert.Equal(t, "OpenAI", profileModels[0].ModelProvider)
	assert.Equal(t, model.NewAPISupplierProfileModelStatusAvailable, profileModels[0].AvailableStatus)
}

func TestUpdateNewAPISupplierChannelProfileKeepsSyncAndChannelStatusSeparate(t *testing.T) {
	setupNewAPISupplierAutomationTestDB(t)
	supplier := createAutomationSupplierFixture(t)
	profile := createAutomationProfileFixture(t, supplier, "vip", "0.8", "gpt-4o")

	updated, err := UpdateNewAPISupplierChannelProfile(profile.Id, NewAPISupplierChannelProfileUpdateRequest{
		LocalGroup:          "auto",
		ChannelType:         1,
		ChannelNameTemplate: "{group} - {ratio}",
		Tag:                 "supplier-a",
		AutoBan:             1,
		SyncMode:            model.NewAPISupplierSyncModeManaged,
		SyncStatus:          model.NewAPISupplierChannelSyncStatusPending,
		ChannelStatus:       model.NewAPISupplierChannelStatusUnavailable,
	})

	require.NoError(t, err)
	assert.Equal(t, model.NewAPISupplierChannelSyncStatusPending, updated.SyncStatus)
	assert.Equal(t, model.NewAPISupplierChannelStatusUnavailable, updated.ChannelStatus)
}

func TestUpdateNewAPISupplierChannelProfileManualConfigPersistsZeroValues(t *testing.T) {
	setupNewAPISupplierAutomationTestDB(t)
	supplier := createAutomationSupplierFixture(t)
	profile := createAutomationProfileFixture(t, supplier, "vip", "0.8", "gpt-4o")
	weight := uint(0)
	priority := int64(0)
	channelRatio := 0.75

	updated, err := UpdateNewAPISupplierChannelProfile(profile.Id, NewAPISupplierChannelProfileUpdateRequest{
		LocalGroup:          "paid",
		ChannelType:         14,
		ChannelNameTemplate: "{supplier}/{group}/{model}",
		Tag:                 "edited-tag",
		Weight:              &weight,
		Priority:            &priority,
		AutoBan:             0,
		BalanceThreshold:    12,
		ChannelRatio:        &channelRatio,
		SyncMode:            model.NewAPISupplierSyncModeManual,
		SyncStatus:          model.NewAPISupplierChannelSyncStatusPending,
		ChannelStatus:       model.NewAPISupplierChannelStatusUnavailable,
	})

	require.NoError(t, err)
	assert.Equal(t, profile.Id, updated.Id)
	assert.Equal(t, supplier.Id, updated.SupplierID)
	assert.Equal(t, "vip", updated.UpstreamGroup)
	assert.Equal(t, "paid", updated.LocalGroup)
	assert.Equal(t, 14, updated.ChannelType)
	assert.Equal(t, "openai", updated.EndpointType)
	assert.Equal(t, "{supplier}/{group}/{model}", updated.ChannelNameTemplate)
	assert.Equal(t, "edited-tag", updated.Tag)
	require.NotNil(t, updated.Weight)
	assert.Equal(t, uint(0), *updated.Weight)
	require.NotNil(t, updated.Priority)
	assert.Equal(t, int64(0), *updated.Priority)
	assert.Equal(t, 0, updated.AutoBan)
	assert.Equal(t, 12, updated.BalanceThreshold)
	require.NotNil(t, updated.ChannelRatio)
	assert.Equal(t, 0.75, *updated.ChannelRatio)
	assert.Equal(t, model.NewAPISupplierSyncModeManual, updated.SyncMode)
	assert.Equal(t, model.NewAPISupplierChannelSyncStatusPending, updated.SyncStatus)
	assert.Equal(t, model.NewAPISupplierChannelStatusUnavailable, updated.ChannelStatus)
	assert.NotEmpty(t, updated.ConfigHash)
}

func TestSyncNewAPISupplierChannelProfileCreatesSystemChannel(t *testing.T) {
	setupNewAPISupplierAutomationTestDB(t)
	supplier := createAutomationSupplierFixture(t)
	profile := createAutomationProfileFixture(t, supplier, "vip", "0.8", "gpt-4o")
	createAutomationProfileModel(t, supplier.Id, profile.Id, "vip", "gpt-4o-mini", model.NewAPISupplierProfileModelStatusAvailable)
	server := newAutomationSupplierServer(t)
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	updated, err := SyncNewAPISupplierChannelProfile(t.Context(), profile.Id)

	require.NoError(t, err)
	require.NotNil(t, updated.ChannelID)
	assert.Equal(t, model.NewAPISupplierChannelSyncStatusCreated, updated.SyncStatus)

	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, *updated.ChannelID).Error)
	assert.Equal(t, "gpt-4o,gpt-4o-mini", channel.Models)
	assert.Equal(t, "auto", channel.Group)
	assert.Equal(t, "sk-vip", channel.Key)
	require.NotNil(t, channel.BaseURL)
	assert.Equal(t, profile.BaseURLSnapshot, *channel.BaseURL)
	require.NotNil(t, channel.ChannelRatio)
	assert.Equal(t, 0.8, *channel.ChannelRatio)

	var binding model.NewAPISupplierChannel
	require.NoError(t, model.DB.First(&binding, "supplier_id = ? AND channel_id = ?", supplier.Id, channel.Id).Error)
	assert.Equal(t, "vip", binding.UpstreamGroup)
	assert.Equal(t, "auto", binding.LocalGroup)
	assert.Equal(t, "gpt-4o,gpt-4o-mini", binding.Models)

	var ability model.Ability
	require.NoError(t, model.DB.First(&ability, "channel_id = ? AND model = ? AND "+model.CommonGroupCol()+" = ?", channel.Id, "gpt-4o", "auto").Error)
	assert.True(t, ability.Enabled)
}

func TestSyncNewAPISupplierChannelProfileUpdatesExistingSystemChannel(t *testing.T) {
	setupNewAPISupplierAutomationTestDB(t)
	supplier := createAutomationSupplierFixture(t)
	profile := createAutomationProfileFixture(t, supplier, "vip", "0.8", "gpt-4o")
	server := newAutomationSupplierServer(t)
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())
	existing := model.Channel{
		Type:        1,
		Key:         "old-key",
		Status:      common.ChannelStatusEnabled,
		Name:        "existing",
		BaseURL:     common.GetPointer("https://old.example.com"),
		Models:      "old-model",
		Group:       "old",
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(&existing).Error)
	require.NoError(t, model.DB.Model(&model.NewAPISupplierChannelProfile{}).Where("id = ?", profile.Id).Update("channel_id", existing.Id).Error)

	updated, err := SyncNewAPISupplierChannelProfile(t.Context(), profile.Id)

	require.NoError(t, err)
	require.NotNil(t, updated.ChannelID)
	assert.Equal(t, existing.Id, *updated.ChannelID)
	assert.Equal(t, model.NewAPISupplierChannelSyncStatusSynced, updated.SyncStatus)

	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, existing.Id).Error)
	assert.Equal(t, "gpt-4o", channel.Models)
	assert.Equal(t, "auto", channel.Group)
	assert.Equal(t, "sk-vip", channel.Key)
}

func newAutomationSupplierServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success": true, "data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
}

func TestUpdateNewAPISupplierProfileModelTestResult(t *testing.T) {
	setupNewAPISupplierAutomationTestDB(t)
	supplier := createAutomationSupplierFixture(t)
	profile := createAutomationProfileFixture(t, supplier, "vip", "0.8", "gpt-4o")

	available, err := UpdateNewAPISupplierProfileModelTestResult(profile.Id, NewAPISupplierProfileModelTestResultRequest{
		ModelName:      "gpt-4o",
		Success:        true,
		ResponseTimeMS: 1234,
		Message:        "",
	})

	require.NoError(t, err)
	assert.Equal(t, model.NewAPISupplierProfileModelStatusAvailable, available.AvailableStatus)
	assert.True(t, available.LastTestSuccess)
	assert.Equal(t, 1234, available.LastResponseTime)
	assert.Empty(t, available.LastError)
	assert.NotZero(t, available.LastTestTime)

	unavailable, err := UpdateNewAPISupplierProfileModelTestResult(profile.Id, NewAPISupplierProfileModelTestResultRequest{
		ModelName:      "gpt-4o-mini",
		Success:        false,
		ResponseTimeMS: 78,
		Message:        "model failed",
	})

	require.NoError(t, err)
	assert.Equal(t, model.NewAPISupplierProfileModelStatusUnavailable, unavailable.AvailableStatus)
	assert.False(t, unavailable.LastTestSuccess)
	assert.Equal(t, 78, unavailable.LastResponseTime)
	assert.Equal(t, "model failed", unavailable.LastError)
	assert.Equal(t, profile.Id, unavailable.ProfileID)
	assert.Equal(t, supplier.Id, unavailable.SupplierID)
	assert.Equal(t, "vip", unavailable.UpstreamGroup)
}

func TestUpdateNewAPISupplierProfileModelTestResultRefreshesChannelStatus(t *testing.T) {
	setupNewAPISupplierAutomationTestDB(t)
	supplier := createAutomationSupplierFixture(t)
	profile := createAutomationProfileFixture(t, supplier, "vip", "0.8", "gpt-4o")
	createAutomationProfileModel(t, supplier.Id, profile.Id, "vip", "gpt-4o-mini", model.NewAPISupplierProfileModelStatusAvailable)

	_, err := UpdateNewAPISupplierProfileModelTestResult(profile.Id, NewAPISupplierProfileModelTestResultRequest{
		ModelName:      "gpt-4o",
		Success:        false,
		ResponseTimeMS: 88,
		Message:        "failed",
	})
	require.NoError(t, err)
	var refreshed model.NewAPISupplierChannelProfile
	require.NoError(t, model.DB.First(&refreshed, profile.Id).Error)
	assert.Equal(t, model.NewAPISupplierChannelStatusUnavailable, refreshed.ChannelStatus)

	_, err = UpdateNewAPISupplierProfileModelTestResult(profile.Id, NewAPISupplierProfileModelTestResultRequest{
		ModelName:      "gpt-4o",
		Success:        true,
		ResponseTimeMS: 66,
		Message:        "",
	})
	require.NoError(t, err)
	_, err = UpdateNewAPISupplierProfileModelTestResult(profile.Id, NewAPISupplierProfileModelTestResultRequest{
		ModelName:      "gpt-4o-mini",
		Success:        true,
		ResponseTimeMS: 55,
		Message:        "",
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&refreshed, profile.Id).Error)
	assert.Equal(t, model.NewAPISupplierChannelStatusAvailable, refreshed.ChannelStatus)
}

func createAutomationSupplierFixture(t *testing.T) *model.NewAPISupplier {
	t.Helper()
	supplier := &model.NewAPISupplier{
		Name:              "upstream-a",
		BaseURL:           "https://upstream.example.com",
		AccessToken:       "access-token",
		UpstreamUserID:    7,
		APIKey:            "sk-vip",
		GroupKeysJSON:     mustJSON(map[string]string{"vip": "sk-vip"}),
		DefaultLocalGroup: "auto",
		Tag:               "supplier-a",
		ChannelType:       1,
		AutoBan:           1,
		ModelSource:       "pricing",
		GroupModelsJSON: mustJSON([]NewAPISupplierGroupSnapshot{{
			Group:  "vip",
			Models: []string{"gpt-4o", "gpt-4o-mini"},
			Source: "pricing",
			Ratio:  "0.8",
		}}),
	}
	require.NoError(t, supplier.Insert())
	return supplier
}

func createAutomationProfileFixture(t *testing.T, supplier *model.NewAPISupplier, group string, ratio string, modelName string) *model.NewAPISupplierChannelProfile {
	t.Helper()
	channelRatio := 0.8
	if ratio != "0.8" {
		channelRatio = 1.2
	}
	profile := &model.NewAPISupplierChannelProfile{
		SupplierID:           supplier.Id,
		SupplierNameSnapshot: supplier.Name,
		BaseURLSnapshot:      supplier.BaseURL,
		UpstreamGroup:        group,
		UpstreamGroupRatio:   ratio,
		ModelSource:          "pricing",
		ChannelType:          1,
		EndpointType:         "openai",
		LocalGroup:           "auto",
		ChannelNameTemplate:  "{group} - {ratio} - {channel_type} - {supplier}",
		Tag:                  "supplier-a",
		AutoBan:              1,
		ChannelRatio:         &channelRatio,
		SyncMode:             model.NewAPISupplierSyncModeManaged,
		SyncStatus:           model.NewAPISupplierChannelSyncStatusPending,
		ChannelStatus:        model.NewAPISupplierChannelStatusAvailable,
		CreatedTime:          common.GetTimestamp(),
		UpdatedTime:          common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(profile).Error)
	createAutomationProfileModel(t, supplier.Id, profile.Id, group, modelName, model.NewAPISupplierProfileModelStatusAvailable)
	return profile
}

func createAutomationProfileModel(t *testing.T, supplierID int, profileID int, group string, modelName string, status string) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.NewAPISupplierChannelProfileModel{
		ProfileID:       profileID,
		SupplierID:      supplierID,
		UpstreamGroup:   group,
		ModelName:       modelName,
		ModelProvider:   "OpenAI",
		AvailableStatus: status,
		LastTestSuccess: status == model.NewAPISupplierProfileModelStatusAvailable,
		CreatedTime:     common.GetTimestamp(),
		UpdatedTime:     common.GetTimestamp(),
	}).Error)
}
