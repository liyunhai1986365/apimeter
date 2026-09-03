package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	captchasvc "github.com/QuantumNous/new-api/service/captcha"
	"github.com/gin-gonic/gin"
)

type captchaGenerateRequest struct {
	Scene string `json:"scene"`
}

type captchaVerifyRequest struct {
	Scene      string `json:"scene"`
	CaptchaKey string `json:"captcha_key"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
}

func GenerateCaptcha(c *gin.Context) {
	if !common.GoCaptchaCheckEnabled {
		common.ApiErrorI18n(c, i18n.MsgCaptchaDisabled)
		return
	}
	var request captchaGenerateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !captchasvc.IsValidScene(request.Scene) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	challenge, err := captchasvc.Generate(request.Scene)
	if err != nil {
		common.SysError("failed to generate behavior CAPTCHA: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgCaptchaGenerateFailed)
		return
	}
	common.ApiSuccess(c, challenge)
}

func VerifyCaptcha(c *gin.Context) {
	if !common.GoCaptchaCheckEnabled {
		common.ApiErrorI18n(c, i18n.MsgCaptchaDisabled)
		return
	}
	var request captchaVerifyRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil ||
		!captchasvc.IsValidScene(request.Scene) || request.CaptchaKey == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	proof, err := captchasvc.Verify(request.Scene, request.CaptchaKey, captchasvc.Position{X: request.X, Y: request.Y})
	if err != nil {
		switch {
		case errors.Is(err, captchasvc.ErrVerificationFailed):
			common.ApiErrorI18n(c, i18n.MsgCaptchaVerificationFailed)
		case captchasvc.IsInvalidTokenError(err):
			common.ApiErrorI18n(c, i18n.MsgCaptchaInvalid)
		default:
			common.SysError("failed to verify behavior CAPTCHA: " + err.Error())
			common.ApiErrorI18n(c, i18n.MsgCaptchaInvalid)
		}
		return
	}
	common.ApiSuccess(c, gin.H{"token": proof})
}
