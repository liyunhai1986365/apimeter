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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.ConfigurableResourceState{}))
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

func createConfigurableResourceAbility(t *testing.T, db *gorm.DB, channelID int, modelName string, enabled bool, priority int64) {
	t.Helper()
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     modelName,
		ChannelId: channelID,
		Enabled:   enabled,
		Priority:  &priority,
	}).Error)
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

	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/material/assets", r.URL.Path)
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		upstreamBody = string(bodyBytes)
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
	createConfigurableResourceAbility(t, db, channel.Id, "user-filled-any-model", true, 0)

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
	require.JSONEq(t, `{"url":"https://cdn.example.com/image.png","asset_type":"Image"}`, upstreamBody)
}

func TestRelayConfigurableResourceSelectsAPIAssetsProfileAndMapsDetailIDToMaterialAssetID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/material/assets/asset-direct", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"Id":"asset-direct","Status":"Active"}}`))
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
	createConfigurableResourceAbility(t, db, channel.Id, "user-filled-any-model", true, 0)

	router := gin.New()
	router.GET("/api/assets/:id",
		middleware.ConfigurableResource("", ""),
		middleware.TokenAuth(),
		RelayConfigurableResource,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/assets/asset-direct", nil)
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"data":{"Id":"asset-direct","Status":"Active"}}`, recorder.Body.String())
}

func TestRelayConfigurableResourceProxiesArkTaskAssetUploadAndQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	var uploadBody string
	var queryBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/task/submit", r.URL.Path)
		require.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.Contains(string(bodyBytes), `"action":"upload"`) {
			uploadBody = string(bodyBytes)
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"Id":"asset-20260701121457-xpd75"}}`))
			return
		}
		queryBody = string(bodyBytes)
		_, _ = w.Write([]byte(`{"code":"MissingParameter.Id","message":"The required parameter Id is missing.","data":{"ResponseMetadata":{"RequestId":"20260701122118A6D201C3CBA463E45365","Action":"GetAsset","Version":"2024-01-01","Service":"ark","Region":"cn-beijing","Error":{"Code":"MissingParameter.Id","Message":"The required parameter Id is missing.","Data":{"__Message.parameter":"Id"}}}}}`))
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
		Name:        "seedance2-ark-task-assets",
		Group:       "default",
		Models:      "doubao-asset",
		BaseURL:     &upstream.URL,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-ark-task-assets"},
	})
	require.NoError(t, db.Create(channel).Error)
	createConfigurableResourceAbility(t, db, channel.Id, "doubao-asset", true, 0)

	router := gin.New()
	router.POST("/api/assets/upload", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	router.GET("/api/assets/:id", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload", strings.NewReader(`{"name":"name test","url":"https://official-domestic.oss-cn-shenzhen.aliyuncs.com/01c755c3b673a8a361a96539faaaabff5b802b8c31dc4e218a2efe62457883ea.webp","asset_type":"Image"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-20260701121457-xpd75"}}`, recorder.Body.String())
	require.JSONEq(t, `{"model":"doubao-asset","input":{"action":"upload","asset_type":"Image","name":"name test","url":"https://official-domestic.oss-cn-shenzhen.aliyuncs.com/01c755c3b673a8a361a96539faaaabff5b802b8c31dc4e218a2efe62457883ea.webp"}}`, uploadBody)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/assets/asset-20260701121457-xpd75?model=doubao-asset", nil)
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"model":"doubao-asset","input":{"action":"query","asset_id":"asset-20260701121457-xpd75"}}`, queryBody)
	require.JSONEq(t, `{"code":"MissingParameter.Id","message":"The required parameter Id is missing.","data":{"ResponseMetadata":{"RequestId":"20260701122118A6D201C3CBA463E45365","Action":"GetAsset","Version":"2024-01-01","Service":"ark","Region":"cn-beijing","Error":{"Code":"MissingParameter.Id","Message":"The required parameter Id is missing.","Data":{"__Message.parameter":"Id"}}}}}`, recorder.Body.String())
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
	createConfigurableResourceAbility(t, db, channel.Id, "dreamina-seedance-2-0-fast-260128", true, 0)

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
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"AssetType":"Image","CreateTime":null,"GroupId":"group-service","Id":"asset-service","Name":"child","ProjectName":"default","Status":"Active","URL":"asset-url","UpdateTime":null}}`, recorder.Body.String())
	require.JSONEq(t, `{"asset_id":"asset-service","task_id":"task-service"}`, assetGetBody)
}

func TestRelayConfigurableResourceProxiesServiceInferenceAssetsUploadAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	var groupBody string
	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/v1/asset-groups":
			groupBody = string(bodyBytes)
			_, _ = w.Write([]byte(`{"id":"group-20260624151622-mwbjg","name":"jie222","description":"lifeng test"}`))
		case "/v1/assets":
			upstreamBody = string(bodyBytes)
			_, _ = w.Write([]byte(`{"id":"asset-service","task_id":"task-service","status":"processing"}`))
		default:
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
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
	createConfigurableResourceAbility(t, db, channel.Id, "dreamina-seedance-2-0-fast-260128", true, 0)

	router := gin.New()
	router.POST("/api/assets/upload", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload", strings.NewReader(`{"url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-service"}}`, recorder.Body.String())
	require.JSONEq(t, `{"name":"jie222","description":"lifeng test"}`, groupBody)
	require.JSONEq(t, `{"group_id":"group-20260624151622-mwbjg","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`, upstreamBody)
}

func TestRelayConfigurableResourceSkipsServiceInferenceAssetGroupPreRequestWhenGroupIDProvided(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	preRequestHit := false
	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/v1/asset-groups":
			preRequestHit = true
			_, _ = w.Write([]byte(`{"id":"group-created"}`))
		case "/v1/assets":
			upstreamBody = string(bodyBytes)
			_, _ = w.Write([]byte(`{"id":"asset-service","task_id":"task-service","status":"processing"}`))
		default:
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
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
	createConfigurableResourceAbility(t, db, channel.Id, "dreamina-seedance-2-0-fast-260128", true, 0)

	router := gin.New()
	router.POST("/api/assets/upload", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload", strings.NewReader(`{"group_id":"group-existing","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.False(t, preRequestHit, "pre_request should be skipped when group_id is provided")
	require.JSONEq(t, `{"group_id":"group-existing","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`, upstreamBody)
}

func TestRelayConfigurableResourceReusesManagedAssetGroupWhenValidationSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	validateHit := false
	createHit := false
	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/asset-groups/group-cached":
			validateHit = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"group-cached","name":"cached","description":"cached group"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/asset-groups":
			createHit = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"group-created"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/assets":
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			upstreamBody = string(bodyBytes)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"asset-service","task_id":"task-service","status":"processing"}`))
		default:
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{Id: 10, Username: "resource-user", Password: "password", Group: "default", Quota: 100000, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 99, UserId: 10, Name: "resource-token", Key: "resourcetokenkey", Status: common.TokenStatusEnabled, CreatedTime: 1, AccessedTime: 1, ExpiredTime: -1, RemainQuota: 100000, UnlimitedQuota: true, Group: "default"}).Error)
	require.NoError(t, db.Create(&model.ConfigurableResourceState{
		ChannelID:    20,
		ProfileID:    "seedance2-service-inference",
		ResourceID:   "assets_upload",
		PreRequestID: "asset_group",
		UserID:       10,
		TokenID:      99,
		StateKey:     "asset_group_id",
		StateValue:   "group-cached",
		Status:       model.ConfigurableResourceStateStatusActive,
		CreatedAt:    common.GetTimestamp(),
		UpdatedAt:    common.GetTimestamp(),
	}).Error)

	channel := &model.Channel{Id: 20, Type: constant.ChannelTypeConfigurable, Key: "upstream-secret", Status: common.ChannelStatusEnabled, Name: "seedance-service-inference", Group: "default", Models: "dreamina-seedance-2-0-fast-260128", BaseURL: &upstream.URL, CreatedTime: common.GetTimestamp()}
	channel.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"}})
	require.NoError(t, db.Create(channel).Error)
	createConfigurableResourceAbility(t, db, channel.Id, "dreamina-seedance-2-0-fast-260128", true, 0)

	router := gin.New()
	router.POST("/api/assets/upload", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload?model=dreamina-seedance-2-0-fast-260128", strings.NewReader(`{"url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, validateHit, "cached group should be validated")
	require.False(t, createHit, "valid cached group should be reused without creating a new group")
	require.JSONEq(t, `{"group_id":"group-cached","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`, upstreamBody)
}

func TestRelayConfigurableResourceCreatesManagedAssetGroupWhenValidationFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	validateHit := false
	createHit := false
	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/asset-groups/group-stale":
			validateHit = true
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"group not found"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/asset-groups":
			createHit = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"group-created","name":"jie222","description":"lifeng test"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/assets":
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			upstreamBody = string(bodyBytes)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"asset-service","task_id":"task-service","status":"processing"}`))
		default:
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	require.NoError(t, db.Create(&model.User{Id: 10, Username: "resource-user", Password: "password", Group: "default", Quota: 100000, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 99, UserId: 10, Name: "resource-token", Key: "resourcetokenkey", Status: common.TokenStatusEnabled, CreatedTime: 1, AccessedTime: 1, ExpiredTime: -1, RemainQuota: 100000, UnlimitedQuota: true, Group: "default"}).Error)
	require.NoError(t, db.Create(&model.ConfigurableResourceState{
		ChannelID:    20,
		ProfileID:    "seedance2-service-inference",
		ResourceID:   "assets_upload",
		PreRequestID: "asset_group",
		UserID:       10,
		TokenID:      99,
		StateKey:     "asset_group_id",
		StateValue:   "group-stale",
		Status:       model.ConfigurableResourceStateStatusActive,
		CreatedAt:    common.GetTimestamp(),
		UpdatedAt:    common.GetTimestamp(),
	}).Error)

	channel := &model.Channel{Id: 20, Type: constant.ChannelTypeConfigurable, Key: "upstream-secret", Status: common.ChannelStatusEnabled, Name: "seedance-service-inference", Group: "default", Models: "dreamina-seedance-2-0-fast-260128", BaseURL: &upstream.URL, CreatedTime: common.GetTimestamp()}
	channel.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"}})
	require.NoError(t, db.Create(channel).Error)
	createConfigurableResourceAbility(t, db, channel.Id, "dreamina-seedance-2-0-fast-260128", true, 0)

	router := gin.New()
	router.POST("/api/assets/upload", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload?model=dreamina-seedance-2-0-fast-260128", strings.NewReader(`{"url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, validateHit, "cached group should be validated")
	require.True(t, createHit, "stale cached group should trigger group creation")
	require.JSONEq(t, `{"group_id":"group-created","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`, upstreamBody)

	var state model.ConfigurableResourceState
	require.NoError(t, db.Where("channel_id = ? AND state_key = ?", 20, "asset_group_id").First(&state).Error)
	require.Equal(t, "group-created", state.StateValue)
	require.Equal(t, model.ConfigurableResourceStateStatusActive, state.Status)
}

func TestRelayConfigurableResourceUsesUploadQueryModelToSelectDreaminaChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	materialHit := false
	materialUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		materialHit = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"Id":"asset-material"}}`))
	}))
	defer materialUpstream.Close()

	dreaminaHit := false
	dreaminaUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/v1/asset-groups":
			_, _ = w.Write([]byte(`{"id":"group-dreamina","name":"jie222","description":"lifeng test"}`))
		case "/v1/assets":
			dreaminaHit = true
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"group_id":"group-dreamina","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`, string(bodyBytes))
			_, _ = w.Write([]byte(`{"id":"asset-dreamina","task_id":"task-dreamina","status":"processing"}`))
		default:
			t.Fatalf("unexpected dreamina upstream path: %s", r.URL.Path)
		}
	}))
	defer dreaminaUpstream.Close()

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

	materialPriority := int64(20)
	materialChannel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "material-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "seedance-material-higher-priority",
		Group:       "default",
		Models:      "doubao-seedance-2-0-fast-260128",
		BaseURL:     &materialUpstream.URL,
		Priority:    &materialPriority,
		CreatedTime: common.GetTimestamp(),
	}
	materialChannel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2-api-assets"},
	})
	require.NoError(t, db.Create(materialChannel).Error)
	createConfigurableResourceAbility(t, db, materialChannel.Id, "doubao-seedance-2-0-fast-260128", true, materialPriority)

	dreaminaPriority := int64(1)
	dreaminaChannel := &model.Channel{
		Id:          21,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "dreamina-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "dreamina-service-inference",
		Group:       "default",
		Models:      "dreamina-seedance-2-0-fast-260128",
		BaseURL:     &dreaminaUpstream.URL,
		Priority:    &dreaminaPriority,
		CreatedTime: common.GetTimestamp(),
	}
	dreaminaChannel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"},
	})
	require.NoError(t, db.Create(dreaminaChannel).Error)
	createConfigurableResourceAbility(t, db, dreaminaChannel.Id, "dreamina-seedance-2-0-fast-260128", true, dreaminaPriority)

	router := gin.New()
	router.POST("/api/assets/upload", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload?model=dreamina-seedance-2-0-fast-260128", strings.NewReader(`{"url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, dreaminaHit, "expected query model to select dreamina service inference channel")
	require.False(t, materialHit, "higher-priority non-dreamina asset channel should be skipped")
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-dreamina"}}`, recorder.Body.String())
}

func TestRelayConfigurableResourceUsesAssetDetailQueryModelToSelectDreaminaChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	materialHit := false
	materialUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		materialHit = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"Id":"asset-material"}}`))
	}))
	defer materialUpstream.Close()

	dreaminaHit := false
	dreaminaUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dreaminaHit = true
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/assets/get", r.URL.Path)
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"asset_id":"asset-dreamina","task_id":"task-dreamina"}`, string(bodyBytes))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"asset-dreamina","name":"child","url":"asset-url","asset_type":"Image","group_id":"group-dreamina","status":"completed","error":null,"created_at":"2026-03-25T15:22:11Z","updated_at":"2026-03-25T15:22:12Z"}`))
	}))
	defer dreaminaUpstream.Close()

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

	materialPriority := int64(20)
	materialChannel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "material-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "seedance-material-higher-priority",
		Group:       "default",
		Models:      "doubao-seedance-2-0-fast-260128",
		BaseURL:     &materialUpstream.URL,
		Priority:    &materialPriority,
		CreatedTime: common.GetTimestamp(),
	}
	materialChannel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2-api-assets"},
	})
	require.NoError(t, db.Create(materialChannel).Error)
	createConfigurableResourceAbility(t, db, materialChannel.Id, "doubao-seedance-2-0-fast-260128", true, materialPriority)

	dreaminaPriority := int64(1)
	dreaminaChannel := &model.Channel{
		Id:          21,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "dreamina-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "dreamina-service-inference",
		Group:       "default",
		Models:      "dreamina-seedance-2-0-fast-260128",
		BaseURL:     &dreaminaUpstream.URL,
		Priority:    &dreaminaPriority,
		CreatedTime: common.GetTimestamp(),
	}
	dreaminaChannel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"},
	})
	require.NoError(t, db.Create(dreaminaChannel).Error)
	createConfigurableResourceAbility(t, db, dreaminaChannel.Id, "dreamina-seedance-2-0-fast-260128", true, dreaminaPriority)

	router := gin.New()
	router.GET("/api/assets/:id", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/assets/asset-dreamina?model=dreamina-seedance-2-0-fast-260128&task_id=task-dreamina", nil)
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, dreaminaHit, "expected query model to select dreamina asset detail channel")
	require.False(t, materialHit, "higher-priority non-dreamina asset detail channel should be skipped")
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"AssetType":"Image","CreateTime":"2026-03-25T15:22:11Z","GroupId":"group-dreamina","Id":"asset-dreamina","Name":"child","ProjectName":"default","Status":"Active","URL":"asset-url","UpdateTime":"2026-03-25T15:22:12Z"}}`, recorder.Body.String())
}

func TestRelayConfigurableResourceSelectsHighestPrioritySupportedAssetsUploadChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	lowPriorityHit := false
	lowPriorityUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lowPriorityHit = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"Id":"asset-low"}}`))
	}))
	defer lowPriorityUpstream.Close()

	highPriorityHit := false
	highPriorityUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/v1/asset-groups":
			_, _ = w.Write([]byte(`{"id":"group-high","name":"jie222","description":"lifeng test"}`))
		case "/v1/assets":
			highPriorityHit = true
			_, _ = w.Write([]byte(`{"id":"asset-high","task_id":"task-high","status":"processing"}`))
		default:
			t.Fatalf("unexpected high priority upstream path: %s", r.URL.Path)
		}
	}))
	defer highPriorityUpstream.Close()

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

	lowPriority := int64(1)
	highPriority := int64(10)
	lowChannel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "low-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "seedance-material-low",
		Group:       "default",
		Models:      "dreamina-seedance-2-0-fast-260128",
		BaseURL:     &lowPriorityUpstream.URL,
		Priority:    &lowPriority,
		CreatedTime: common.GetTimestamp(),
	}
	lowChannel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2"},
	})
	require.NoError(t, db.Create(lowChannel).Error)
	createConfigurableResourceAbility(t, db, lowChannel.Id, "dreamina-seedance-2-0-fast-260128", true, lowPriority)

	highChannel := &model.Channel{
		Id:          21,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "high-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "seedance-service-inference-high",
		Group:       "default",
		Models:      "dreamina-seedance-2-0-fast-260128",
		BaseURL:     &highPriorityUpstream.URL,
		Priority:    &highPriority,
		CreatedTime: common.GetTimestamp(),
	}
	highChannel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"},
	})
	require.NoError(t, db.Create(highChannel).Error)
	createConfigurableResourceAbility(t, db, highChannel.Id, "dreamina-seedance-2-0-fast-260128", true, highPriority)

	router := gin.New()
	router.POST("/api/assets/upload", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/upload", strings.NewReader(`{"group_id":"group-service","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, highPriorityHit, "expected highest-priority supported asset channel to be used")
	require.False(t, lowPriorityHit, "expected lower-priority supported asset channel to be skipped")
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-high"}}`, recorder.Body.String())
}

func TestRelayConfigurableResourceSkipsChannelsWithoutEnabledAbilityForAssetsUpload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	upstreamHit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"asset-disabled"}`))
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

	priority := int64(10)
	channel := &model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeConfigurable,
		Key:         "upstream-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "seedance-disabled-ability",
		Group:       "default",
		Models:      "dreamina-seedance-2-0-fast-260128",
		BaseURL:     &upstream.URL,
		Priority:    &priority,
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"},
	})
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "dreamina-seedance-2-0-fast-260128",
		ChannelId: channel.Id,
		Enabled:   false,
		Priority:  &priority,
	}).Error)

	router := gin.New()
	router.POST("/api/assets", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets", strings.NewReader(`{"model":"dreamina-seedance-2-0-fast-260128","group_id":"group-service","url":"https://cdn.example.com/image.png","asset_type":"Image","name":"child"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer resourcetokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	require.False(t, upstreamHit, "disabled ability channel should not receive asset upload")
	require.Contains(t, recorder.Body.String(), "no available configurable resource channel")
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
