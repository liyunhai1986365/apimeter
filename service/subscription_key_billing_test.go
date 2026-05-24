package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBillingSessionSubscriptionKeyUsesBoundSubscriptionOnly(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)
	model.InitColForTest()

	seedUser(t, 9101, 100000)
	plan := &model.SubscriptionPlan{
		Id:            9102,
		Title:         "Bound Plan",
		DurationUnit:  model.SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   1000,
		Enabled:       true,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	now := time.Now().Unix()
	bound := &model.UserSubscription{
		Id:          9103,
		UserId:      9101,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  900,
		StartTime:   now - 3600,
		EndTime:     now + 3600,
		Status:      "active",
	}
	other := &model.UserSubscription{
		Id:          9104,
		UserId:      9101,
		PlanId:      plan.Id,
		AmountTotal: 10000,
		AmountUsed:  0,
		StartTime:   now - 3600,
		EndTime:     now + 3600,
		Status:      "active",
	}
	require.NoError(t, model.DB.Create(bound).Error)
	require.NoError(t, model.DB.Create(other).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id:                 9105,
		UserId:             9101,
		Key:                "bound-sub-token",
		Status:             common.TokenStatusEnabled,
		RemainQuota:        100,
		UnlimitedQuota:     false,
		BillingSource:      BillingSourceSubscription,
		UserSubscriptionId: bound.Id,
		SubscriptionPlanId: plan.Id,
	}).Error)

	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenBillingSource, BillingSourceSubscription)
		common.SetContextKey(c, constant.ContextKeyTokenUserSubscriptionId, bound.Id)
		info := &relaycommon.RelayInfo{
			RequestId:               "bound-subscription-key-request",
			UserId:                  9101,
			TokenId:                 9105,
			TokenKey:                "bound-sub-token",
			TokenUnlimited:          false,
			BillingSource:           BillingSourceSubscription,
			TokenUserSubscriptionId: bound.Id,
			TokenSubscriptionPlanId: plan.Id,
			OriginModelName:         "gpt-4.1",
		}
		session, apiErr := NewBillingSession(c, info, 500)
		if apiErr != nil {
			c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"source":          session.funding.Source(),
			"subscription_id": info.SubscriptionId,
		})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())

	var refreshedOther model.UserSubscription
	require.NoError(t, model.DB.Where("id = ?", other.Id).First(&refreshedOther).Error)
	assert.Equal(t, int64(0), refreshedOther.AmountUsed)
}

func TestNewBillingSessionSystemTokenUsesWalletEvenWhenSubscriptionFirst(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)
	model.InitColForTest()

	seedUser(t, 9151, 100000)
	plan := &model.SubscriptionPlan{
		Id:            9152,
		Title:         "Active Plan",
		DurationUnit:  model.SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   10000,
		Enabled:       true,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	now := time.Now().Unix()
	sub := &model.UserSubscription{
		Id:          9153,
		UserId:      9151,
		PlanId:      plan.Id,
		AmountTotal: 10000,
		AmountUsed:  0,
		StartTime:   now - 3600,
		EndTime:     now + 3600,
		Status:      "active",
	}
	require.NoError(t, model.DB.Create(sub).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id:             9154,
		UserId:         9151,
		Key:            "system-wallet-token",
		Status:         common.TokenStatusEnabled,
		RemainQuota:    100000,
		UnlimitedQuota: false,
	}).Error)

	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		info := &relaycommon.RelayInfo{
			RequestId:       "system-token-wallet-request",
			UserId:          9151,
			TokenId:         9154,
			TokenKey:        "system-wallet-token",
			TokenUnlimited:  false,
			OriginModelName: "gpt-4.1",
			UserSetting: dto.UserSetting{
				BillingPreference: "subscription_first",
			},
		}
		session, apiErr := NewBillingSession(c, info, 500)
		if apiErr != nil {
			c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"source":          session.funding.Source(),
			"subscription_id": info.SubscriptionId,
		})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Source         string `json:"source"`
		SubscriptionId int    `json:"subscription_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, BillingSourceWallet, response.Source)
	assert.Zero(t, response.SubscriptionId)

	var refreshedSub model.UserSubscription
	require.NoError(t, model.DB.Where("id = ?", sub.Id).First(&refreshedSub).Error)
	assert.Equal(t, int64(0), refreshedSub.AmountUsed)
	assert.Equal(t, 99500, getUserQuota(t, 9151))
}

func TestPreConsumeTokenQuotaConsumesSubscriptionKeyBalance(t *testing.T) {
	truncate(t)
	model.InitColForTest()

	require.NoError(t, model.DB.Create(&model.Token{
		Id:                 9201,
		UserId:             9202,
		Key:                "sub-low-token",
		Status:             common.TokenStatusEnabled,
		RemainQuota:        600,
		UnlimitedQuota:     false,
		BillingSource:      BillingSourceSubscription,
		UserSubscriptionId: 9203,
	}).Error)

	info := &relaycommon.RelayInfo{
		TokenId:        9201,
		TokenKey:       "sub-low-token",
		TokenUnlimited: false,
		BillingSource:  BillingSourceSubscription,
	}

	require.NoError(t, PreConsumeTokenQuota(info, 500))
	assert.Equal(t, 100, getTokenRemainQuota(t, 9201))
	assert.Equal(t, 500, getTokenUsedQuota(t, 9201))
}
