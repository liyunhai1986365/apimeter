package service

import (
	"context"
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

func TestBuildNewAPISupplierChannelPlans(t *testing.T) {
	weight := uint(7)
	priority := int64(12)
	supplier := &model.NewAPISupplier{
		Id:          42,
		Name:        "upstream-a",
		BaseURL:     "https://upstream.example.com/",
		AccessToken: "access-token",
		APIKey:      "sk-upstream",
		Tag:         "team-a",
		Weight:      &weight,
		Priority:    &priority,
		AutoBan:     1,
		ModelSource: "pricing",
		GroupModelsJSON: mustJSON([]NewAPISupplierGroupSnapshot{
			{Group: "default", Models: []string{"gpt-4o", "gpt-4o-mini"}, Source: "pricing"},
			{Group: "vip", Models: []string{"gpt-5"}, Source: "pricing"},
		}),
	}
	selection := NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{
			{
				UpstreamGroup: "default",
				LocalGroup:    "default",
				Models:        []string{"gpt-4o", "gpt-4o-mini"},
			},
			{
				UpstreamGroup: "vip",
				LocalGroup:    "premium",
				Models:        []string{"gpt-5"},
			},
		},
	}

	plans, err := BuildNewAPISupplierChannelPlans(supplier, selection)

	require.NoError(t, err)
	require.Len(t, plans, 2)
	assert.Equal(t, "upstream-a / default", plans[0].Name)
	require.NotNil(t, plans[0].BaseURL)
	assert.Equal(t, "https://upstream.example.com", *plans[0].BaseURL)
	assert.Equal(t, "sk-upstream", plans[0].Key)
	assert.Equal(t, "default", plans[0].Group)
	assert.Equal(t, "gpt-4o,gpt-4o-mini", plans[0].Models)
	require.NotNil(t, plans[0].Tag)
	assert.Equal(t, "team-a", *plans[0].Tag)
	assert.Equal(t, "newapi-supplier:42;upstream_group:default", plans[0].OtherInfo)
	assert.Equal(t, weight, *plans[0].Weight)
	assert.Equal(t, priority, *plans[0].Priority)

	assert.Equal(t, "upstream-a / vip", plans[1].Name)
	assert.Equal(t, "premium", plans[1].Group)
	assert.Equal(t, "gpt-5", plans[1].Models)
	assert.Equal(t, "newapi-supplier:42;upstream_group:vip", plans[1].OtherInfo)
}

func TestBuildNewAPISupplierChannelPlansUsesRequestedChannelType(t *testing.T) {
	supplier := &model.NewAPISupplier{
		Id:          42,
		Name:        "upstream-a",
		BaseURL:     "https://upstream.example.com/",
		APIKey:      "sk-upstream",
		ChannelType: 1,
		ModelSource: "pricing",
		GroupModelsJSON: mustJSON([]NewAPISupplierGroupSnapshot{
			{Group: "vip", Models: []string{"claude-sonnet-4"}, Source: "pricing"},
		}),
	}

	plans, err := BuildNewAPISupplierChannelPlans(supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "vip",
			LocalGroup:    "premium",
			ChannelType:   14,
			Models:        []string{"claude-sonnet-4"},
		}},
	})

	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, 14, plans[0].Type)
}

func TestNewAPISupplierNormalizePreservesDisabledStatus(t *testing.T) {
	supplier := &model.NewAPISupplier{
		Name:    " upstream ",
		BaseURL: "https://upstream.example.com/",
		Status:  model.NewAPISupplierStatusDisabled,
	}

	supplier.Normalize()

	assert.Equal(t, "upstream", supplier.Name)
	assert.Equal(t, "https://upstream.example.com", supplier.BaseURL)
	assert.Equal(t, model.NewAPISupplierStatusDisabled, supplier.Status)
}

func TestParseNewAPIGroupInfoWithRatio(t *testing.T) {
	info := parseNewAPIGroupInfo(" vip ", map[string]any{
		"ratio":       1.25,
		"description": "VIP group",
	})

	assert.Equal(t, "vip", info.Name)
	assert.Equal(t, "1.25", info.Ratio)
	assert.Equal(t, "VIP group", info.Desc)
}

func TestParseNewAPIGroupInfoStringValueAsDesc(t *testing.T) {
	info := parseNewAPIGroupInfo("vip", "VIP group")

	assert.Equal(t, "vip", info.Name)
	assert.Empty(t, info.Ratio)
	assert.Equal(t, "VIP group", info.Desc)
}

func TestFetchNewAPIModelPlazaGroups(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/pricing", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"group_ratio": {"default": 1, "vip": 1.25},
			"usable_group": {"vip": {"desc": "VIP group", "ratio": 1.2}},
			"data": [
				{"model_name": "gpt-4o", "enable_groups": ["default", "vip"]},
				{"model_name": "gpt-5", "enable_groups": ["vip"]},
				{"model_name": "ignored-auto", "enable_groups": ["auto"]},
				{"model_name": "ignored-all", "enable_groups": ["all"]}
			]
		}`))
	}))
	defer server.Close()

	groups, groupModels, source := fetchNewAPIModelPlazaGroups(context.Background(), server.Client(), server.URL, "access", 7)

	require.Equal(t, "pricing", source)
	require.Len(t, groups, 2)
	assert.Equal(t, "default", groups[0].Name)
	assert.Equal(t, "1", groups[0].Ratio)
	assert.Equal(t, "vip", groups[1].Name)
	assert.Equal(t, "1.2", groups[1].Ratio)
	assert.Equal(t, "VIP group", groups[1].Desc)
	assert.Equal(t, []string{"gpt-4o"}, groupModels["default"])
	assert.Equal(t, []string{"gpt-4o", "gpt-5"}, groupModels["vip"])
}

func TestFetchNewAPIModelPlazaGroupsKeepsGroupRatioWhenUsableGroupIsName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/pricing", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"group_ratio": {"default": 1, "vip": 1.25},
			"usable_group": {"default": "默认分组", "vip": "VIP 分组"},
			"data": [
				{"model_name": "gpt-4o-mini", "enable_groups": ["default"]},
				{"model_name": "gpt-4o", "enable_groups": ["vip"]}
			]
		}`))
	}))
	defer server.Close()

	groups, groupModels, source := fetchNewAPIModelPlazaGroups(context.Background(), server.Client(), server.URL, "access", 7)

	require.Equal(t, "pricing", source)
	require.Len(t, groups, 2)
	assert.Equal(t, "default", groups[0].Name)
	assert.Equal(t, "1", groups[0].Ratio)
	assert.Equal(t, "默认分组", groups[0].Desc)
	assert.Equal(t, "vip", groups[1].Name)
	assert.Equal(t, "1.25", groups[1].Ratio)
	assert.Equal(t, "VIP 分组", groups[1].Desc)
	assert.Equal(t, []string{"gpt-4o-mini"}, groupModels["default"])
	assert.Equal(t, []string{"gpt-4o"}, groupModels["vip"])
}

func TestFetchNewAPIModelPlazaGroupsKeepsGroupSpecificModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/pricing", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"group_ratio": {"default": 1, "vip": 1.25, "svip": 1.5},
			"usable_group": {
				"default": {"desc": "Default group", "ratio": 1},
				"vip": {"desc": "VIP group", "ratio": 1.25},
				"svip": {"desc": "SVIP group", "ratio": 1.5}
			},
			"data": [
				{"model_name": "default-only", "enable_groups": ["default"]},
				{"model_name": "vip-only", "enable_groups": ["vip"]},
				{"model_name": "shared-default-vip", "enable_groups": ["default", "vip"]},
				{"model_name": "global-for-empty-groups", "enable_groups": ["all"]}
			]
		}`))
	}))
	defer server.Close()

	groups, groupModels, source := fetchNewAPIModelPlazaGroups(context.Background(), server.Client(), server.URL, "access", 7)

	require.Equal(t, "pricing", source)
	require.Len(t, groups, 2)
	assert.Equal(t, []string{"default-only", "shared-default-vip"}, groupModels["default"])
	assert.Equal(t, []string{"shared-default-vip", "vip-only"}, groupModels["vip"])
	assert.Nil(t, groupModels["svip"])
}

func TestBuildNewAPISupplierChannelPlansRejectsStaleUserModelsSnapshot(t *testing.T) {
	supplier := &model.NewAPISupplier{
		Id:              42,
		Name:            "upstream-a",
		BaseURL:         "https://upstream.example.com/",
		APIKey:          "sk-upstream",
		ModelSource:     "user_models",
		GroupModelsJSON: `[{"group":"default","models":["gpt-4o"],"source":"user_models"}]`,
	}

	_, err := BuildNewAPISupplierChannelPlans(supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "default",
			LocalGroup:    "default",
			Models:        []string{"gpt-4o"},
		}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "模型广场")
}

func TestBuildNewAPISupplierChannelPlansRejectsModelOutsideSelectedGroup(t *testing.T) {
	supplier := &model.NewAPISupplier{
		Id:              42,
		Name:            "upstream-a",
		BaseURL:         "https://upstream.example.com/",
		APIKey:          "sk-upstream",
		ModelSource:     "pricing",
		GroupModelsJSON: `[{"group":"default","models":["default-only"],"source":"pricing"},{"group":"vip","models":["vip-only"],"source":"pricing"}]`,
	}

	_, err := BuildNewAPISupplierChannelPlans(supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "default",
			LocalGroup:    "default",
			Models:        []string{"vip-only"},
		}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "不属于供应商分组")
}

func TestCheckNewAPISupplierDoesNotCreateUpstreamTokens(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var tokenRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/pricing":
			_, _ = w.Write([]byte(`{
				"success": true,
				"group_ratio": {"default": 1, "vip": 1.25},
				"data": [
					{"model_name": "gpt-4o-mini", "enable_groups": ["default"]},
					{"model_name": "gpt-4o", "enable_groups": ["vip"]}
				]
			}`))
		case "/api/token/":
			tokenRequests++
			http.Error(w, "token creation should not be called while checking", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	result, err := CheckNewAPISupplier(context.Background(), supplier)

	require.NoError(t, err)
	assert.Equal(t, 0, tokenRequests)
	assert.Equal(t, "pricing", result.ModelSource)
	require.Len(t, result.GroupModels, 2)
	assert.Empty(t, result.GroupKeys)
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Empty(t, refreshed.GroupKeysJSON)
	assert.Contains(t, refreshed.GroupModelsJSON, "gpt-4o-mini")
	assert.Contains(t, refreshed.GroupModelsJSON, "gpt-4o")
}

func TestCheckNewAPISupplierRequiresPricingGroupModels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var fallbackRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/pricing":
			_, _ = w.Write([]byte(`{"success": true, "data": []}`))
		case "/api/user/self/groups", "/api/user/models", "/v1/models", "/api/token/":
			fallbackRequests++
			http.Error(w, "check must rely on model plaza group models only", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	result, err := CheckNewAPISupplier(context.Background(), supplier)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, fallbackRequests)
	assert.Contains(t, err.Error(), "模型广场")
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Empty(t, refreshed.GroupModelsJSON)
	assert.Empty(t, refreshed.GroupKeysJSON)
}

func TestEnsureNewAPISupplierConfigureKeysCreatesTokenForSelectedGroupOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var createdTokens int
	var createdPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "7", r.Header.Get("New-Api-User"))
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/token/":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success": true, "data": {"items": []}}`))
				return
			}
			require.Equal(t, http.MethodPost, r.Method)
			createdTokens++
			require.NoError(t, common.DecodeJson(r.Body, &createdPayload))
			_, _ = w.Write([]byte(`{"success": true, "data": {"id": 123, "name": "vip", "key": "sk-vip"}}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	err = ensureNewAPISupplierConfigureKeys(context.Background(), supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{
			{
				UpstreamGroup: "vip",
				LocalGroup:    "premium",
				Models:        []string{"gpt-4o-mini", "gpt-4o"},
			},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, createdTokens)
	assert.Equal(t, "vip", createdPayload["name"])
	assert.Equal(t, "vip", createdPayload["group"])
	assert.Equal(t, true, createdPayload["model_limits_enabled"])
	assert.Equal(t, "gpt-4o,gpt-4o-mini", createdPayload["model_limits"])
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Contains(t, refreshed.GroupKeysJSON, `"vip":"sk-vip"`)
	assert.Equal(t, "sk-vip", refreshed.APIKey)
}

func TestEnsureNewAPISupplierConfigureKeysAcceptsArrayTokenList(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var createdTokens int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/token/":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success": true, "data": []}`))
				return
			}
			createdTokens++
			_, _ = w.Write([]byte(`{"success": true, "data": {"id": 123, "name": "vip", "key": "sk-vip"}}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	err = ensureNewAPISupplierConfigureKeys(context.Background(), supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "vip",
			LocalGroup:    "premium",
			Models:        []string{"gpt-4o"},
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, createdTokens)
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Contains(t, refreshed.GroupKeysJSON, `"vip":"sk-vip"`)
}

func TestEnsureNewAPISupplierConfigureKeysUsesCreatedTokenKeyResponse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var createdTokens int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/token/":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success": true, "data": {"items": []}}`))
				return
			}
			require.Equal(t, http.MethodPost, r.Method)
			createdTokens++
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 456, "name": "vip", "key": "created-vip"}
			}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	err = ensureNewAPISupplierConfigureKeys(context.Background(), supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "vip",
			LocalGroup:    "premium",
			Models:        []string{"gpt-4o"},
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, createdTokens)
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Contains(t, refreshed.GroupKeysJSON, `"vip":"sk-created-vip"`)
}

func TestEnsureNewAPISupplierConfigureKeysFallsBackToSearchAndKeyForLegacyCreate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var createdTokens int
	var keyRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/token/":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success": true, "data": {"items": []}}`))
				return
			}
			require.Equal(t, http.MethodPost, r.Method)
			createdTokens++
			_, _ = w.Write([]byte(`{"success": true, "data": {}}`))
		case "/api/token/search":
			require.Equal(t, "vip", r.URL.Query().Get("keyword"))
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"items": [{"id": 789, "name": "vip"}]}
			}`))
		case "/api/token/789/key":
			keyRequests++
			_, _ = w.Write([]byte(`{"success": true, "data": {"key": "legacy-vip"}}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	err = ensureNewAPISupplierConfigureKeys(context.Background(), supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "vip",
			LocalGroup:    "premium",
			Models:        []string{"gpt-4o"},
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, createdTokens)
	assert.Equal(t, 1, keyRequests)
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Contains(t, refreshed.GroupKeysJSON, `"vip":"sk-legacy-vip"`)
}

func TestEnsureNewAPISupplierConfigureKeysUsesLegacySearchKeyWhenKeyEndpointMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var keyEndpointRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/token/":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success": true, "data": {"items": []}}`))
				return
			}
			require.Equal(t, http.MethodPost, r.Method)
			_, _ = w.Write([]byte(`{"success": true, "data": {}}`))
		case "/api/token/search":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"items": [{"id": 842, "name": "vip", "key": "legacy-search-vip"}]}
			}`))
		case "/api/token/842/key":
			keyEndpointRequests++
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	err = ensureNewAPISupplierConfigureKeys(context.Background(), supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "vip",
			LocalGroup:    "premium",
			Models:        []string{"gpt-4o"},
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, 0, keyEndpointRequests)
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Contains(t, refreshed.GroupKeysJSON, `"vip":"sk-legacy-search-vip"`)
}

func TestEnsureNewAPISupplierConfigureKeysFallsBackToDetailWhenKeyEndpointMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var keyEndpointRequests int
	var detailRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/token/":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success": true, "data": {"items": []}}`))
				return
			}
			require.Equal(t, http.MethodPost, r.Method)
			_, _ = w.Write([]byte(`{"success": true, "data": {}}`))
		case "/api/token/search":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"items": [{"id": 842, "name": "vip", "key": "sk****tail"}]}
			}`))
		case "/api/token/842/key":
			keyEndpointRequests++
			http.NotFound(w, r)
		case "/api/token/842":
			detailRequests++
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 842, "name": "vip", "key": "detail-vip"}
			}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	err = ensureNewAPISupplierConfigureKeys(context.Background(), supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "vip",
			LocalGroup:    "premium",
			Models:        []string{"gpt-4o"},
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, keyEndpointRequests)
	assert.Equal(t, 1, detailRequests)
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Contains(t, refreshed.GroupKeysJSON, `"vip":"sk-detail-vip"`)
}

func TestEnsureNewAPISupplierConfigureKeysFallsBackToCreatedTokenID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
	}
	require.NoError(t, supplier.Insert())

	var keyRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/token/":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success": true, "data": {"items": []}}`))
				return
			}
			require.Equal(t, http.MethodPost, r.Method)
			_, _ = w.Write([]byte(`{"success": true, "data": {"id": 321}}`))
		case "/api/token/321/key":
			keyRequests++
			_, _ = w.Write([]byte(`{"success": true, "data": {"key": "id-vip"}}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	err = ensureNewAPISupplierConfigureKeys(context.Background(), supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "vip",
			LocalGroup:    "premium",
			Models:        []string{"gpt-4o"},
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, keyRequests)
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Contains(t, refreshed.GroupKeysJSON, `"vip":"sk-id-vip"`)
}

func TestPrepareNewAPISupplierGroupTestChannelCreatesGroupToken(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NewAPISupplier{}, &model.NewAPISupplierChannel{}))
	originDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originDB })

	supplier := &model.NewAPISupplier{
		Name:           "upstream-a",
		BaseURL:        "placeholder",
		AccessToken:    "access-token",
		UpstreamUserID: 7,
		ChannelType:    1,
		Tag:            "team-a",
		ModelSource:    "pricing",
		GroupModelsJSON: mustJSON([]NewAPISupplierGroupSnapshot{
			{Group: "vip", Models: []string{"gpt-4o-mini", "gpt-4o"}, Source: "pricing"},
		}),
	}
	require.NoError(t, supplier.Insert())

	var createdPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			assert.Equal(t, "access-token", r.Header.Get("Authorization"))
			assert.Equal(t, "7", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {"id": 7, "username": "upstream", "quota": 100, "used_quota": 9}
			}`))
		case "/api/token/":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success": true, "data": {"items": []}}`))
				return
			}
			require.Equal(t, http.MethodPost, r.Method)
			require.NoError(t, common.DecodeJson(r.Body, &createdPayload))
			_, _ = w.Write([]byte(`{"success": true, "data": {"id": 123, "name": "vip", "key": "vip-test-key"}}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	supplier.BaseURL = server.URL
	require.NoError(t, supplier.Update())

	result, err := PrepareNewAPISupplierGroupTestChannel(context.Background(), supplier, NewAPISupplierTestModelRequest{
		UpstreamGroup: "vip",
		Model:         "gpt-4o",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Channel.BaseURL)
	assert.Equal(t, server.URL, *result.Channel.BaseURL)
	assert.Equal(t, "sk-vip-test-key", result.Channel.Key)
	assert.Equal(t, "gpt-4o", result.Model)
	assert.Equal(t, "gpt-4o", result.Channel.Models)
	assert.Equal(t, "vip", result.UpstreamGroup)
	assert.Equal(t, "vip", result.Channel.Group)
	assert.Equal(t, "upstream-a / vip test", result.Channel.Name)
	require.NotNil(t, result.Channel.Tag)
	assert.Equal(t, "team-a", *result.Channel.Tag)
	assert.Equal(t, "vip", createdPayload["group"])
	assert.Equal(t, "gpt-4o,gpt-4o-mini", createdPayload["model_limits"])
	refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
	require.NoError(t, err)
	assert.Contains(t, refreshed.GroupKeysJSON, `"vip":"sk-vip-test-key"`)
}

func TestPrepareNewAPISupplierGroupTestChannelRejectsModelOutsideGroup(t *testing.T) {
	supplier := &model.NewAPISupplier{
		Id:              42,
		Name:            "upstream-a",
		BaseURL:         "https://upstream.example.com",
		AccessToken:     "access-token",
		UpstreamUserID:  7,
		ModelSource:     "pricing",
		GroupModelsJSON: `[{"group":"vip","models":["gpt-4o"],"source":"pricing"}]`,
	}

	result, err := PrepareNewAPISupplierGroupTestChannel(context.Background(), supplier, NewAPISupplierTestModelRequest{
		UpstreamGroup: "vip",
		Model:         "default-only",
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "不属于供应商分组")
}

func TestConfigureNewAPISupplierChannelsRejectsStaleSnapshotBeforeTokenCreation(t *testing.T) {
	supplier := &model.NewAPISupplier{
		Id:              42,
		Name:            "upstream-a",
		BaseURL:         "https://upstream.example.com/",
		APIKey:          "sk-upstream",
		ModelSource:     "user_models",
		GroupModelsJSON: `[{"group":"default","models":["gpt-4o"],"source":"user_models"}]`,
	}

	_, err := ConfigureNewAPISupplierChannels(context.Background(), supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: "default",
			LocalGroup:    "default",
			Models:        []string{"gpt-4o"},
		}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "模型广场")
}
