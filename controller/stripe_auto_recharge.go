package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type stripeAutoRechargeUpdateRequest struct {
	Enabled     bool    `json:"enabled"`
	Threshold   float64 `json:"threshold"`
	TopUpAmount int64   `json:"topup_amount"`
	Consent     bool    `json:"consent"`
}

func GetStripeAutoRecharge(c *gin.Context) {
	status, err := service.GetStripeAutoRechargeStatus(c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, "读取 Stripe 自动充值设置失败")
		return
	}
	common.ApiSuccess(c, status)
}

func CreateStripeAutoRechargeSetup(c *gin.Context) {
	successURL := paymentReturnPathForRequest(c, "/console/topup?stripe_setup_session_id="+stripeCheckoutSessionPlaceholder)
	cancelURL := paymentReturnPathForRequest(c, "/console/topup")
	setupURL, err := service.CreateStripeAutoRechargeSetupSession(c.GetInt("id"), successURL, cancelURL)
	if err != nil {
		if errors.Is(err, model.ErrStripeAutoRechargeBusy) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "自动充值正在处理中，请稍后再绑定银行卡"})
			return
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe auto recharge setup failed user_id=%d error=%q", c.GetInt("id"), err.Error()))
		common.ApiErrorMsg(c, "创建 Stripe 绑卡页面失败")
		return
	}
	common.ApiSuccess(c, gin.H{"setup_url": setupURL})
}

func ConfirmStripeAutoRechargeSetup(c *gin.Context) {
	sessionId := strings.TrimSpace(c.Query("session_id"))
	if err := service.CompleteStripeAutoRechargeSetupSession(c.Request.Context(), sessionId, c.GetInt("id")); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Stripe auto recharge setup confirmation failed user_id=%d session_id=%s error=%q", c.GetInt("id"), sessionId, err.Error()))
		common.ApiErrorMsg(c, "无法确认银行卡绑定状态")
		return
	}
	status, err := service.GetStripeAutoRechargeStatus(c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, "读取 Stripe 自动充值设置失败")
		return
	}
	common.ApiSuccess(c, status)
}

func UpdateStripeAutoRecharge(c *gin.Context) {
	var req stripeAutoRechargeUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}
	if err := service.UpdateStripeAutoRecharge(c.GetInt("id"), req.Enabled, req.Threshold, req.TopUpAmount, req.Consent); err != nil {
		if errors.Is(err, model.ErrStripeAutoRechargeBusy) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "自动充值正在处理中，请稍后重试"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	status, err := service.GetStripeAutoRechargeStatus(c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, "读取 Stripe 自动充值设置失败")
		return
	}
	common.ApiSuccess(c, status)
}

func DeleteStripeAutoRecharge(c *gin.Context) {
	if err := service.DeleteStripeAutoRechargePaymentMethod(c.GetInt("id")); err != nil {
		if errors.Is(err, model.ErrStripeAutoRechargeBusy) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "自动充值正在处理中，暂时不能解绑银行卡"})
			return
		}
		common.ApiErrorMsg(c, "解绑银行卡失败")
		return
	}
	common.ApiSuccess(c, nil)
}
