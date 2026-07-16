package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

func TestTokenAuthDefaultsLegacyEmptyTokenGroupToAuto(t *testing.T) {
	db := openTokenAuthTestDB(t)
	seedTokenAuthAutoGroupFixture(t, db)
	require.NoError(t, db.Model(&model.Token{}).Where("key = ?", "autotokenkey").Update("group", "").Error)

	router := gin.New()
	router.POST("/v1/chat/completions", TokenAuth(), Distribute(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"using_group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
			"token_group": common.GetContextKeyString(c, constant.ContextKeyTokenGroup),
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
		TokenGroup string `json:"token_group"`
		AutoGroup  string `json:"auto_group"`
		ChannelId  int    `json:"channel_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, service.AutoGroupName, response.UsingGroup)
	require.Equal(t, service.AutoGroupName, response.TokenGroup)
	require.Equal(t, "default", response.AutoGroup)
	require.Equal(t, 1001, response.ChannelId)
}

func TestTokenAuthDefaultsLegacyEmptyGroupToAutoForConfigurableNativeVideo(t *testing.T) {
	db := openTokenAuthTestDB(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"渠道特供":1}`))

	require.NoError(t, db.Create(&model.User{
		Id:       1,
		Username: "native-video-user",
		Password: "password",
		Group:    "渠道特供",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         1,
		Name:           "legacy-empty-group-token",
		Key:            "legacyemptygroupkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "",
	}).Error)

	weight := uint(100)
	priority := int64(10)
	autoBan := 1
	channel := model.Channel{
		Id:       1049,
		Type:     constant.ChannelTypeConfigurable,
		Key:      "upstream-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "seedance-configurable",
		Group:    "default",
		Models:   "doubao-seedance-2-0-mini-260615",
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-ark-task-assets"},
	})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "doubao-seedance-2-0-mini-260615",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()

	router := gin.New()
	router.POST(
		"/api/v3/contents/generations/tasks",
		ConfigurableNativeProfile("doubao-seedance-2", relayconstant.RelayModeVideoSubmit),
		TokenAuth(),
		Distribute(),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"using_group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
				"auto_group":  common.GetContextKeyString(c, constant.ContextKeyAutoGroup),
				"channel_id":  common.GetContextKeyInt(c, constant.ContextKeyChannelId),
			})
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"test"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer legacyemptygroupkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		UsingGroup string `json:"using_group"`
		AutoGroup  string `json:"auto_group"`
		ChannelId  int    `json:"channel_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, service.AutoGroupName, response.UsingGroup)
	require.Equal(t, "default", response.AutoGroup)
	require.Equal(t, channel.Id, response.ChannelId)
}

func TestDistributeRejectsFixedImageChannelWithoutExplicitProtocolCapability(t *testing.T) {
	db := openTokenAuthTestDB(t)

	autoBan := 1
	require.NoError(t, db.Create(&model.Channel{
		Id:      9101,
		Type:    constant.ChannelTypeGemini,
		Key:     "upstream-key",
		Status:  common.ChannelStatusEnabled,
		Name:    "unconfigured-fixed-image-channel",
		Group:   "default",
		Models:  "custom-provider-photo-v9",
		AutoBan: &autoBan,
	}).Error)

	router := gin.New()
	router.POST("/v1/images/generations", func(c *gin.Context) {
		c.Set("id", 1)
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "9101")
		c.Next()
	}, Distribute(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"channel_id": common.GetContextKeyInt(c, constant.ContextKeyChannelId)})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{
		"model":"custom-provider-photo-v9",
		"prompt":"cat"
	}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "openai.image.generations")
}

func TestTokenAuthAllowsOwnedProviderGroupAndDistributorUsesOwnedChannel(t *testing.T) {
	db := openTokenAuthTestDB(t)

	require.NoError(t, db.Create(&model.User{
		Id:       7,
		Username: "owned-provider-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	group := model.BuildUserOwnedProviderGroup(7, 7001)
	require.NoError(t, db.Create(&model.Token{
		UserId:         7,
		Name:           "owned-provider-token",
		Key:            "ownedprovidertokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          group,
	}).Error)

	weight := uint(100)
	priority := int64(10)
	autoBan := 1
	require.NoError(t, db.Create(&model.Channel{
		Id:          7001,
		Type:        constant.ChannelTypeOpenAI,
		Key:         "owned-upstream-key",
		Status:      common.ChannelStatusEnabled,
		Name:        "user-openai",
		Group:       group,
		Models:      "gpt-owned",
		Weight:      &weight,
		Priority:    &priority,
		AutoBan:     &autoBan,
		Scope:       model.ChannelScopeUserOwned,
		OwnerUserId: 7,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     "gpt-owned",
		ChannelId: 7001,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()

	router := gin.New()
	router.POST("/v1/chat/completions", TokenAuth(), Distribute(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"using_group":    common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
			"channel_id":     common.GetContextKeyInt(c, constant.ContextKeyChannelId),
			"billing_source": common.GetContextKeyString(c, constant.ContextKeyTokenBillingSource),
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-owned"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer ownedprovidertokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		UsingGroup    string `json:"using_group"`
		ChannelId     int    `json:"channel_id"`
		BillingSource string `json:"billing_source"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, group, response.UsingGroup)
	require.Equal(t, 7001, response.ChannelId)
	require.Equal(t, service.BillingSourceUserOwnedProvider, response.BillingSource)
}

func TestTokenAuthAllowsOwnedProviderGroupPolicy(t *testing.T) {
	db := openTokenAuthTestDB(t)

	require.NoError(t, db.Create(&model.User{
		Id:       8,
		Username: "owned-provider-policy-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	group := model.BuildUserOwnedProviderGroup(8, 8001)
	require.NoError(t, db.Create(&model.Token{
		UserId:         8,
		Name:           "owned-provider-policy-token",
		Key:            "ownedproviderpolicytokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "auto",
		GroupPolicy:    `{"type":"ordered","groups":["` + group + `"]}`,
	}).Error)

	weight := uint(100)
	priority := int64(10)
	autoBan := 1
	require.NoError(t, db.Create(&model.Channel{
		Id:          8001,
		Type:        constant.ChannelTypeOpenAI,
		Key:         "owned-policy-upstream-key",
		Status:      common.ChannelStatusEnabled,
		Name:        "user-openai-policy",
		Group:       group,
		Models:      "gpt-owned-policy",
		Weight:      &weight,
		Priority:    &priority,
		AutoBan:     &autoBan,
		Scope:       model.ChannelScopeUserOwned,
		OwnerUserId: 8,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     "gpt-owned-policy",
		ChannelId: 8001,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()

	router := gin.New()
	router.POST("/v1/chat/completions", TokenAuth(), Distribute(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"channel_id":     common.GetContextKeyInt(c, constant.ContextKeyChannelId),
			"auto_group":     common.GetContextKeyString(c, constant.ContextKeyAutoGroup),
			"billing_source": common.GetContextKeyString(c, constant.ContextKeyTokenBillingSource),
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-owned-policy"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer ownedproviderpolicytokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		ChannelId     int    `json:"channel_id"`
		AutoGroup     string `json:"auto_group"`
		BillingSource string `json:"billing_source"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 8001, response.ChannelId)
	require.Equal(t, group, response.AutoGroup)
	require.Equal(t, service.BillingSourceUserOwnedProvider, response.BillingSource)
}

func TestTokenAuthOwnedProviderPolicySkipsGroupsWithoutRequestedModel(t *testing.T) {
	db := openTokenAuthTestDB(t)

	require.NoError(t, db.Create(&model.User{
		Id:       9,
		Username: "owned-provider-policy-fallback-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	firstGroup := model.BuildUserOwnedProviderGroup(9, 9001)
	secondGroup := model.BuildUserOwnedProviderGroup(9, 9002)
	require.NoError(t, db.Create(&model.Token{
		UserId:          9,
		Name:            "owned-provider-policy-fallback-token",
		Key:             "ownedproviderpolicyfallbacktokenkey",
		Status:          common.TokenStatusEnabled,
		CreatedTime:     1,
		AccessedTime:    1,
		ExpiredTime:     -1,
		RemainQuota:     100000,
		UnlimitedQuota:  true,
		Group:           firstGroup,
		GroupPolicy:     `{"type":"ordered","groups":["` + firstGroup + `","` + secondGroup + `"]}`,
		CrossGroupRetry: true,
	}).Error)

	weight := uint(100)
	priority := int64(10)
	autoBan := 1
	require.NoError(t, db.Create(&[]model.Channel{
		{
			Id:          9001,
			Type:        constant.ChannelTypeOpenAI,
			Key:         "owned-policy-first-key",
			Status:      common.ChannelStatusEnabled,
			Name:        "user-openai-policy-first",
			Group:       firstGroup,
			Models:      "gpt-4o",
			Weight:      &weight,
			Priority:    &priority,
			AutoBan:     &autoBan,
			Scope:       model.ChannelScopeUserOwned,
			OwnerUserId: 9,
		},
		{
			Id:          9002,
			Type:        constant.ChannelTypeOpenAI,
			Key:         "owned-policy-second-key",
			Status:      common.ChannelStatusEnabled,
			Name:        "user-openai-policy-second",
			Group:       secondGroup,
			Models:      "gpt-5.5",
			Weight:      &weight,
			Priority:    &priority,
			AutoBan:     &autoBan,
			Scope:       model.ChannelScopeUserOwned,
			OwnerUserId: 9,
		},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: firstGroup, Model: "gpt-4o", ChannelId: 9001, Enabled: true, Priority: &priority, Weight: weight},
		{Group: secondGroup, Model: "gpt-5.5", ChannelId: 9002, Enabled: true, Priority: &priority, Weight: weight},
	}).Error)
	model.InitChannelCache()

	router := gin.New()
	router.POST("/v1/chat/completions", TokenAuth(), Distribute(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"channel_id":     common.GetContextKeyInt(c, constant.ContextKeyChannelId),
			"auto_group":     common.GetContextKeyString(c, constant.ContextKeyAutoGroup),
			"billing_source": common.GetContextKeyString(c, constant.ContextKeyTokenBillingSource),
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer ownedproviderpolicyfallbacktokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		ChannelId     int    `json:"channel_id"`
		AutoGroup     string `json:"auto_group"`
		BillingSource string `json:"billing_source"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 9002, response.ChannelId)
	require.Equal(t, secondGroup, response.AutoGroup)
	require.Equal(t, service.BillingSourceUserOwnedProvider, response.BillingSource)
}

func TestDistributeAffinityRespectsRoutingStrategyGroupOrder(t *testing.T) {
	db := openTokenAuthTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","cheap":"低价分组","expensive":"高价分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["expensive","cheap"]`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"cheap":0.5,"expensive":2}`))

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "affinity-price-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         10,
		Name:           "affinity-price-token",
		Key:            "affinitypricekey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "auto",
		GroupPolicy:    `{"type":"ordered","groups":["expensive"]}`,
	}).Error)

	weight := uint(100)
	priority := int64(10)
	autoBan := 1
	require.NoError(t, db.Create(&[]model.Channel{
		{
			Id:       10101,
			Type:     constant.ChannelTypeOpenAI,
			Key:      "cheap-key",
			Status:   common.ChannelStatusEnabled,
			Name:     "cheap-channel",
			Group:    "cheap",
			Models:   "gpt-test",
			Weight:   &weight,
			Priority: &priority,
			AutoBan:  &autoBan,
		},
		{
			Id:       10102,
			Type:     constant.ChannelTypeOpenAI,
			Key:      "expensive-key",
			Status:   common.ChannelStatusEnabled,
			Name:     "expensive-channel",
			Group:    "expensive",
			Models:   "gpt-test",
			Weight:   &weight,
			Priority: &priority,
			AutoBan:  &autoBan,
		},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "cheap", Model: "gpt-test", ChannelId: 10101, Enabled: true, Priority: &priority, Weight: weight},
		{Group: "expensive", Model: "gpt-test", ChannelId: 10102, Enabled: true, Priority: &priority, Weight: weight},
	}).Error)
	require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{
		Strategy:  model.RoutingStrategyPriceFirst,
		UserGroup: "default",
		Groups:    `["cheap","expensive"]`,
		Scores:    `{"cheap":{"group":"cheap","rank":1},"expensive":{"group":"expensive","rank":2}}`,
		Config:    `{}`,
	}))
	model.InitChannelCache()

	rule := operation_setting.ChannelAffinityRule{
		Name:       "test-price-order-affinity",
		ModelRegex: []string{"^gpt-test$"},
		PathRegex:  []string{"/v1/chat/completions"},
		KeySources: []operation_setting.ChannelAffinityKeySource{
			{Type: "request_header", Key: "X-Affinity-Key"},
		},
		IncludeRuleName:  true,
		IncludeModelName: true,
	}
	affinityValue := fmt.Sprintf("price-order-%d", time.Now().UnixNano())
	affinitySetting := operation_setting.GetChannelAffinitySetting()
	originalRules := affinitySetting.Rules
	affinitySetting.Rules = append([]operation_setting.ChannelAffinityRule{rule}, originalRules...)
	t.Cleanup(func() {
		affinitySetting.Rules = originalRules
	})

	router := gin.New()
	router.POST("/v1/chat/completions", TokenAuth(), Distribute(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"channel_id": common.GetContextKeyInt(c, constant.ContextKeyChannelId),
			"auto_group": common.GetContextKeyString(c, constant.ContextKeyAutoGroup),
		})
	})

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	firstRequest.Header.Set("Content-Type", "application/json")
	firstRequest.Header.Set("Authorization", "Bearer affinitypricekey")
	firstRequest.Header.Set("X-Affinity-Key", affinityValue)
	router.ServeHTTP(firstRecorder, firstRequest)
	require.Equal(t, http.StatusOK, firstRecorder.Code, firstRecorder.Body.String())

	var token model.Token
	require.NoError(t, db.Where("key = ?", "affinitypricekey").First(&token).Error)
	token.Group = "auto"
	token.GroupPolicy = `{"type":"routing_strategy","strategy":"price_first"}`
	require.NoError(t, token.Update())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer affinitypricekey")
	request.Header.Set("X-Affinity-Key", affinityValue)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		ChannelId int    `json:"channel_id"`
		AutoGroup string `json:"auto_group"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 10101, response.ChannelId)
	require.Equal(t, "cheap", response.AutoGroup)
}

func TestTokenAuthAllowsSubscriptionKeyWithZeroTokenQuota(t *testing.T) {
	db := openTokenAuthTestDB(t)

	require.NoError(t, db.AutoMigrate(&model.UserSubscription{}))
	require.NoError(t, db.Create(&model.User{
		Id:       2,
		Username: "subscription-key-user",
		Password: "password",
		Group:    "default",
		Quota:    0,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:          2002,
		UserId:      2,
		PlanId:      3003,
		AmountTotal: 10000,
		AmountUsed:  0,
		StartTime:   1,
		EndTime:     common.GetTimestamp() + 3600,
		Status:      "active",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:             2,
		Name:               "subscription-token",
		Key:                "subtokenkey",
		Status:             common.TokenStatusEnabled,
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        common.GetTimestamp() + 3600,
		RemainQuota:        0,
		UnlimitedQuota:     false,
		BillingSource:      "subscription",
		SubscriptionPlanId: 3003,
		UserSubscriptionId: 2002,
		Group:              "auto",
	}).Error)

	router := gin.New()
	router.GET("/auth-only", TokenAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"billing_source":       common.GetContextKeyString(c, constant.ContextKeyTokenBillingSource),
			"user_subscription_id": common.GetContextKeyInt(c, constant.ContextKeyTokenUserSubscriptionId),
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth-only", nil)
	request.Header.Set("Authorization", "Bearer subtokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		BillingSource      string `json:"billing_source"`
		UserSubscriptionId int    `json:"user_subscription_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "subscription", response.BillingSource)
	require.Equal(t, 2002, response.UserSubscriptionId)
}

func TestTokenAuthAcceptsSubscriptionKeyPrefix(t *testing.T) {
	db := openTokenAuthTestDB(t)

	require.NoError(t, db.AutoMigrate(&model.UserSubscription{}))
	require.NoError(t, db.Create(&model.User{
		Id:       3,
		Username: "subscription-prefixed-key-user",
		Password: "password",
		Group:    "default",
		Quota:    0,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:          3002,
		UserId:      3,
		PlanId:      3004,
		AmountTotal: 10000,
		AmountUsed:  0,
		StartTime:   1,
		EndTime:     common.GetTimestamp() + 3600,
		Status:      "active",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:             3,
		Name:               "subscription-token",
		Key:                "prefixedsubtokenkey",
		Status:             common.TokenStatusEnabled,
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        common.GetTimestamp() + 3600,
		RemainQuota:        0,
		UnlimitedQuota:     false,
		BillingSource:      "subscription",
		SubscriptionPlanId: 3004,
		UserSubscriptionId: 3002,
		Group:              "auto",
	}).Error)

	router := gin.New()
	router.GET("/auth-only", TokenAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"token_key":            c.GetString("token_key"),
			"billing_source":       common.GetContextKeyString(c, constant.ContextKeyTokenBillingSource),
			"user_subscription_id": common.GetContextKeyInt(c, constant.ContextKeyTokenUserSubscriptionId),
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth-only", nil)
	request.Header.Set("Authorization", "Bearer sp-prefixedsubtokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		TokenKey           string `json:"token_key"`
		BillingSource      string `json:"billing_source"`
		UserSubscriptionId int    `json:"user_subscription_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "prefixedsubtokenkey", response.TokenKey)
	require.Equal(t, "subscription", response.BillingSource)
	require.Equal(t, 3002, response.UserSubscriptionId)
}

func TestTokenAuthStoresTokenImageSettingsInContext(t *testing.T) {
	db := openTokenAuthTestDB(t)

	require.NoError(t, db.Create(&model.User{
		Id:       4,
		Username: "image-settings-user",
		Password: "password",
		Group:    "default",
		Quota:    100000,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         4,
		Name:           "image-settings-token",
		Key:            "imagesettingstokenkey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100000,
		UnlimitedQuota: true,
		Group:          "auto",
		ImageSettings:  model.TokenImageSettings{Format: "b64_json", Store: "force_store_url_and_base64"},
	}).Error)

	router := gin.New()
	router.GET("/auth-only", TokenAuth(), func(c *gin.Context) {
		settings, _ := common.GetContextKeyType[model.TokenImageSettings](c, constant.ContextKeyTokenImageSettings)
		c.JSON(http.StatusOK, gin.H{
			"format": settings.Format,
			"store":  settings.Store,
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth-only", nil)
	request.Header.Set("Authorization", "Bearer imagesettingstokenkey")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"format":"b64_json","store":"force_store_url_and_base64"}`, recorder.Body.String())
}
