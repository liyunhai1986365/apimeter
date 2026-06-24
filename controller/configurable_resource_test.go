package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openConfigurableResourceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originDB := model.DB
	originLogDB := model.LOG_DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originRedisEnabled := common.RedisEnabled
	originMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}))
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	model.InitColForTest()
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		common.RedisEnabled = originRedisEnabled
		common.MemoryCacheEnabled = originMemoryCacheEnabled
		model.InitColForTest()
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`)
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	})
	return db
}

func TestRelayConfigurableResourceProxiesMaterialCreateWithoutModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/material/assets", r.URL.Path)
		require.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		upstreamBody = string(bodyBytes)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"Id":"asset-123"}}`))
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "resource-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         10,
		Name:           "resource-token",
		Key:            "resourcetokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	channel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "upstream-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "jimu-seedance",
		Group:       "default",
		Models:      "any-placeholder-model",
		BaseURL:     &upstream.URL,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2"},
	})
	require.NoError(t, db.Create(channel).Error)

	router := gin.New()
	router.POST("/material/assets",
		middleware.ConfigurableResource("doubao-seedance-2", "material_assets"),
		middleware.TokenAuth(),
		RelayConfigurableResource,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/material/assets", strings.NewReader(`{"url":"https://cdn.example.com/char.jpg","asset_type":"Image","name":"角色立绘01"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-123"}}`, recorder.Body.String())
	require.JSONEq(t, `{"url":"https://cdn.example.com/char.jpg","asset_type":"Image","name":"角色立绘01"}`, upstreamBody)
}

func TestRelayConfigurableResourceProxiesAssetsUploadAliasToMaterialCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/material/assets", r.URL.Path)
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		upstreamBody = string(bodyBytes)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"Id":"asset-kkidc"}}`))
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "resource-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         10,
		Name:           "resource-token",
		Key:            "resourcetokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	channel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "upstream-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "jimu-seedance",
		Group:       "default",
		Models:      "user-filled-any-model",
		BaseURL:     &upstream.URL,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2"},
	})
	require.NoError(t, db.Create(channel).Error)

	router := gin.New()
	router.POST("/api/assets/upload",
		middleware.ConfigurableResource("doubao-seedance-2", "material_assets"),
		middleware.TokenAuth(),
		RelayConfigurableResource,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload", strings.NewReader(`{"url":"https://cdn.example.com/image.png","asset_type":"Image","name":"示例图片"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-kkidc"}}`, recorder.Body.String())
	require.JSONEq(t, `{"url":"https://cdn.example.com/image.png","asset_type":"Image","name":"示例图片"}`, upstreamBody)
}

func TestRelayConfigurableResourceNormalizesMaterialUploadResponseForAssetsUploadAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/material/assets", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"asset_id":"asset-from-material","ignored":"upstream-extra"}`))
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "resource-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         10,
		Name:           "resource-token",
		Key:            "resourcetokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	channel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "upstream-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "jimu-seedance",
		Group:       "default",
		Models:      "user-filled-any-model",
		BaseURL:     &upstream.URL,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2"},
	})
	require.NoError(t, db.Create(channel).Error)

	profile, ok := configurable.GetProfile("doubao-seedance-2")
	require.True(t, ok)
	resource, ok := profile.ResourceByID("material_assets")
	require.True(t, ok)

	router := gin.New()
	router.POST("/api/assets/upload",
		middleware.ConfigurableResource("doubao-seedance-2", "material_assets"),
		middleware.TokenAuth(),
		func(c *gin.Context) {
			upstreamReq, err := buildConfigurableResourceRequest(c, channel, resource)
			require.NoError(t, err)
			require.Equal(t, upstream.URL+"/material/assets", upstreamReq.URL.String())
			RelayConfigurableResource(c)
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload", strings.NewReader(`{"url":"https://cdn.example.com/image.png","asset_type":"Image"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-from-material"}}`, recorder.Body.String())
}

func TestRelayConfigurableResourceSelectsAPIAssetsProfileForSharedAssetsUploadRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/assets/upload", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"Id":"asset-direct"}}`))
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "resource-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         10,
		Name:           "resource-token",
		Key:            "resourcetokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	channel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "upstream-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "seedance-api-assets",
		Group:       "default",
		Models:      "user-filled-any-model",
		BaseURL:     &upstream.URL,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2-api-assets"},
	})
	require.NoError(t, db.Create(channel).Error)

	router := gin.New()
	router.POST("/api/assets/upload",
		middleware.ConfigurableResource("", ""),
		middleware.TokenAuth(),
		RelayConfigurableResource,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload", strings.NewReader(`{"url":"https://cdn.example.com/image.png","asset_type":"Image"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-direct"}}`, recorder.Body.String())
}

func TestRelayConfigurableResourceProxiesServiceInferenceAssetLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	var assetGroupBody string
	var assetCreateBody string
	var assetGetBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/asset-groups":
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assetGroupBody = string(bodyBytes)
			_, _ = w.Write([]byte(`{"id":"group-service","name":"characters","description":"test group"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/asset-groups/group-service":
			_, _ = w.Write([]byte(`{"id":"group-service","name":"characters","description":"test group"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/assets":
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assetCreateBody = string(bodyBytes)
			_, _ = w.Write([]byte(`{"id":"asset-service","task_id":"task-service","status":"processing"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/assets/get":
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assetGetBody = string(bodyBytes)
			_, _ = w.Write([]byte(`{"id":"asset-service","name":"child","url":"asset-url","asset_type":"Image","group_id":"group-service","status":"completed","error":null}`))
		default:
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "resource-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         10,
		Name:           "resource-token",
		Key:            "resourcetokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	channel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "upstream-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "seedance-service-inference",
		Group:       "default",
		Models:      "dreamina-seedance-2-0-fast-260128",
		BaseURL:     &upstream.URL,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"},
	})
	require.NoError(t, db.Create(channel).Error)

	router := gin.New()
	router.POST("/v1/asset-groups", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	router.GET("/v1/asset-groups/:group_id", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	router.POST("/v1/assets", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	router.POST("/v1/assets/get", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/asset-groups", strings.NewReader(`{"name":"characters","description":"test group"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"id":"group-service","name":"characters","description":"test group"}`, recorder.Body.String())
	require.JSONEq(t, `{"name":"characters","description":"test group"}`, assetGroupBody)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/asset-groups/group-service", nil)
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"id":"group-service","name":"characters","description":"test group"}`, recorder.Body.String())

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/assets", strings.NewReader(`{"group_id":"group-service","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"id":"asset-service","task_id":"task-service","status":"processing"}`, recorder.Body.String())
	require.JSONEq(t, `{"group_id":"group-service","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`, assetCreateBody)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/assets/get", strings.NewReader(`{"asset_id":"asset-service","task_id":"task-service"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"id":"asset-service","name":"child","url":"asset-url","asset_type":"Image","group_id":"group-service","status":"completed","error":null}`, recorder.Body.String())
	require.JSONEq(t, `{"asset_id":"asset-service","task_id":"task-service"}`, assetGetBody)
}

func TestRelayConfigurableResourceProxiesMaterialDetailPathParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/material/assets/asset-123", r.URL.Path)
		require.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"Id":"asset-123","Status":"Active"}}`))
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "resource-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         10,
		Name:           "resource-token",
		Key:            "resourcetokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	channel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "upstream-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "jimu-seedance",
		Group:       "default",
		Models:      "any-placeholder-model",
		BaseURL:     &upstream.URL,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2"},
	})
	require.NoError(t, db.Create(channel).Error)

	router := gin.New()
	router.GET("/material/assets/:asset_id",
		middleware.ConfigurableResource("doubao-seedance-2", "material_asset_detail"),
		middleware.TokenAuth(),
		RelayConfigurableResource,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/material/assets/asset-123", nil)
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"data":{"Id":"asset-123","Status":"Active"}}`, recorder.Body.String())
}

func TestRelayConfigurableResourceProxiesAssetsDetailAliasIDToMaterialAssetID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/material/assets/asset-kkidc", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"Id":"asset-kkidc","Status":"Active"}}`))
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "resource-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         10,
		Name:           "resource-token",
		Key:            "resourcetokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	channel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "upstream-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "jimu-seedance",
		Group:       "default",
		Models:      "user-filled-any-model",
		BaseURL:     &upstream.URL,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2"},
	})
	require.NoError(t, db.Create(channel).Error)

	router := gin.New()
	router.GET("/api/assets/:id",
		middleware.ConfigurableResource("doubao-seedance-2", "material_asset_detail"),
		middleware.TokenAuth(),
		RelayConfigurableResource,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/assets/asset-kkidc", nil)
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"data":{"Id":"asset-kkidc","Status":"Active"}}`, recorder.Body.String())
}

func TestBuildConfigurableResourceRequestMapsAssetListQueryFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/assets?page=2&page_size=20&name=logo&statuses=Active&statuses=Processing&sort_order=Desc", nil)

	channel := &model.Channel{
		BaseURL: common.GetPointer("https://upstream.example.com"),
	}
	resource := &configurable.ResourceConfig{
		Upstream: configurable.EndpointConfig{
			Method: http.MethodGet,
			Path:   "/material/assets",
		},
		Request: configurable.BodyConfig{
			Fields: []configurable.FieldMapping{
				{To: "page", From: "query.page", OmitEmpty: true},
				{To: "page_size", From: "query.page_size", OmitEmpty: true},
				{To: "name", From: "query.name", OmitEmpty: true},
				{To: "statuses", From: "query.statuses", OmitEmpty: true},
				{To: "sort_by", From: "query.sort_by", OmitEmpty: true},
				{To: "sort_order", From: "query.sort_order", OmitEmpty: true},
			},
		},
	}

	req, err := buildConfigurableResourceRequest(c, channel, resource)
	require.NoError(t, err)
	require.Equal(t, "2", req.URL.Query().Get("page"))
	require.Equal(t, "20", req.URL.Query().Get("page_size"))
	require.Equal(t, "logo", req.URL.Query().Get("name"))
	require.Equal(t, []string{"Active", "Processing"}, req.URL.Query()["statuses"])
	require.Equal(t, "", req.URL.Query().Get("sort_by"))
	require.Equal(t, "Desc", req.URL.Query().Get("sort_order"))
}

func TestBuildConfigurableResourceRequestMapsQueryFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/resource/search?name=avatar&type=Image", nil)

	channel := &model.Channel{
		BaseURL: common.GetPointer("https://upstream.example.com"),
	}
	resource := &configurable.ResourceConfig{
		Upstream: configurable.EndpointConfig{
			Method: http.MethodGet,
			Path:   "/material/assets",
		},
		Request: configurable.BodyConfig{
			Fields: []configurable.FieldMapping{
				{To: "name", From: "query.name"},
				{To: "asset_type", From: "query.type"},
			},
		},
	}

	req, err := buildConfigurableResourceRequest(c, channel, resource)
	require.NoError(t, err)
	require.Equal(t, "https://upstream.example.com/material/assets?asset_type=Image&name=avatar", req.URL.String())
}
