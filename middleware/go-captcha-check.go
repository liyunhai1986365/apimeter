package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	captchasvc "github.com/QuantumNous/new-api/service/captcha"
	"github.com/gin-gonic/gin"
)

func GoCaptchaCheck(scene string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.GoCaptchaCheckEnabled {
			c.Next()
			return
		}
		token := c.GetHeader("X-Go-Captcha-Token")
		if token == "" {
			token = c.Query("go_captcha")
		}
		if token == "" {
			common.ApiErrorI18n(c, i18n.MsgCaptchaRequired)
			c.Abort()
			return
		}
		if err := captchasvc.ConsumeProof(scene, token); err != nil {
			if !captchasvc.IsInvalidTokenError(err) {
				common.SysError("failed to consume behavior CAPTCHA proof: " + err.Error())
			}
			common.ApiErrorI18n(c, i18n.MsgCaptchaInvalid)
			c.Abort()
			return
		}
		c.Next()
	}
}
