package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubscriptionDiscountTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.RedisEnabled = false
	model.InitColForTest()
	require.NoError(t, db.AutoMigrate(
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.Token{},
	))

	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
}

func TestAdminUpdateSubscriptionPlanPersistsDiscountDescription(t *testing.T) {
	setupSubscriptionDiscountTestDB(t)
	gin.SetMode(gin.TestMode)

	plan := model.SubscriptionPlan{
		Title:         "Pro",
		PriceAmount:   19,
		Currency:      "USD",
		DurationUnit:  model.SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   100000,
	}
	require.NoError(t, model.DB.Create(&plan).Error)

	body := `{"plan":{"title":"Pro","price_amount":19,"currency":"USD","duration_unit":"month","duration_value":1,"enabled":true,"total_amount":100000,"discount_description":"限时 4 折"}}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/subscription/admin/plans/1", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	AdminUpdateSubscriptionPlan(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var updated model.SubscriptionPlan
	require.NoError(t, model.DB.First(&updated, plan.Id).Error)
	assert.Equal(t, "限时 4 折", updated.DiscountDescription)
}
