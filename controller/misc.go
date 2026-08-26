package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func TestStatus(c *gin.Context) {
	err := model.PingDB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "数据库连接失败",
		})
		return
	}
	// 获取HTTP统计信息
	httpStats := middleware.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Server is running",
		"http_stats": httpStats,
	})
	return
}

func GetReadiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	checks := gin.H{}
	ready := true

	if err := model.CheckDatabaseReadiness(ctx, model.DB); err != nil {
		checks["database"] = "unavailable"
		ready = false
	} else {
		checks["database"] = "ok"
	}

	if model.LOG_DB != model.DB {
		if err := model.CheckDatabaseReadiness(ctx, model.LOG_DB); err != nil {
			checks["log_database"] = "unavailable"
			ready = false
		} else {
			checks["log_database"] = "ok"
		}
	} else {
		checks["log_database"] = checks["database"]
	}

	if common.RedisEnabled {
		if common.RDB == nil || common.RDB.Ping(ctx).Err() != nil {
			checks["redis"] = "unavailable"
			ready = false
		} else {
			checks["redis"] = "ok"
		}
	} else {
		checks["redis"] = "disabled"
	}

	nodeType := "slave"
	if common.IsMasterNode {
		nodeType = "master"
	}
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{
		"success": ready,
		"data": gin.H{
			"checks":    checks,
			"node_name": common.NodeName,
			"node_type": nodeType,
			"version":   common.Version,
		},
	})
}

func GetStatus(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Writer.Header().Add("Vary", "Cookie")
	c.Writer.Header().Add("Vary", "Authorization")

	cs := console_setting.GetConsoleSetting()
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()

	passkeySetting := system_setting.GetPasskeySettings()
	legalSetting := system_setting.GetLegalSettings()
	mainlandChinaPresentationEnabled := common.OptionMap[common.MainlandChinaPresentationOptionKey] == "true"
	defaultUserDisplayCurrency := operation_setting.GetDefaultUserDisplayCurrency()
	if mainlandChinaPresentationEnabled {
		defaultUserDisplayCurrency = operation_setting.QuotaDisplayTypeCNY
	}

	data := gin.H{
		"version":                     common.Version,
		"start_time":                  common.StartTime,
		"email_verification":          common.EmailVerificationEnabled,
		"github_oauth":                common.GitHubOAuthEnabled,
		"github_client_id":            common.GitHubClientId,
		"discord_oauth":               system_setting.GetDiscordSettings().Enabled,
		"discord_client_id":           system_setting.GetDiscordSettings().ClientId,
		"linuxdo_oauth":               common.LinuxDOOAuthEnabled,
		"linuxdo_client_id":           common.LinuxDOClientId,
		"linuxdo_minimum_trust_level": common.LinuxDOMinimumTrustLevel,
		"telegram_oauth":              common.TelegramOAuthEnabled,
		"telegram_bot_name":           common.TelegramBotName,
		"theme":                       "default",
		"system_name":                 common.SystemName,
		"logo":                        common.Logo,
		"footer_html":                 common.Footer,
		"footer_company_name":         common.OptionMap[common.FooterCompanyNameOptionKey],
		"customer_service_script":     common.OptionMap["CustomerServiceScript"],
		"google_analytics_id":         common.OptionMap["GoogleAnalyticsId"],
		"wechat_qrcode":               common.WeChatAccountQRCodeImageURL,
		"wechat_login":                common.WeChatAuthEnabled,
		"server_address":              system_setting.ServerAddress,
		"turnstile_check":             common.TurnstileCheckEnabled,
		"turnstile_site_key":          common.TurnstileSiteKey,
		"docs_link":                   operation_setting.GetGeneralSetting().DocsLink,
		"quota_per_unit":              common.QuotaPerUnit,
		// 兼容旧前端：保留 display_in_currency，同时提供新的 quota_display_type
		"display_in_currency":                 operation_setting.IsCurrencyDisplay(),
		"quota_display_type":                  operation_setting.GetQuotaDisplayType(),
		"default_user_display_currency":       defaultUserDisplayCurrency,
		"custom_currency_symbol":              operation_setting.GetGeneralSetting().CustomCurrencySymbol,
		"custom_currency_exchange_rate":       operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate,
		"enable_batch_update":                 common.BatchUpdateEnabled,
		"enable_drawing":                      common.DrawingEnabled,
		"enable_task":                         common.TaskEnabled,
		"enable_data_export":                  common.DataExportEnabled,
		"data_export_default_time":            common.DataExportDefaultTime,
		"default_collapse_sidebar":            common.DefaultCollapseSidebar,
		"mj_notify_enabled":                   setting.MjNotifyEnabled,
		"chats":                               setting.Chats,
		"demo_site_enabled":                   operation_setting.DemoSiteEnabled,
		"self_use_mode_enabled":               operation_setting.SelfUseModeEnabled,
		"register_enabled":                    common.RegisterEnabled,
		"password_register_enabled":           common.PasswordRegisterEnabled,
		"default_use_auto_group":              setting.DefaultUseAutoGroup,
		"mainland_china_presentation_enabled": mainlandChinaPresentationEnabled,

		"usd_exchange_rate":              operation_setting.USDExchangeRate,
		"price":                          operation_setting.Price,
		"stripe_unit_price":              setting.StripeUnitPrice,
		"affiliate_topup_reward_ratio":   common.AffiliateTopUpRewardRatio * 100,
		"affiliate_topup_reward_limit":   common.AffiliateTopUpRewardLimit,
		"affiliate_consume_reward_ratio": common.AffiliateConsumeRewardRatio * 100,
		"quota_for_inviter":              common.QuotaForInviter,

		// 面板启用开关
		"api_info_enabled":      cs.ApiInfoEnabled,
		"uptime_kuma_enabled":   cs.UptimeKumaEnabled,
		"announcements_enabled": cs.AnnouncementsEnabled,
		"faq_enabled":           cs.FAQEnabled,

		// 模块管理配置
		"HeaderNavModules":    common.OptionMap["HeaderNavModules"],
		"SidebarModulesAdmin": common.OptionMap["SidebarModulesAdmin"],

		"oidc_enabled":                system_setting.GetOIDCSettings().Enabled,
		"oidc_client_id":              system_setting.GetOIDCSettings().ClientId,
		"oidc_authorization_endpoint": system_setting.GetOIDCSettings().AuthorizationEndpoint,
		"oidc_display_name":           system_setting.GetOIDCSettings().GetEffectiveDisplayName(),
		"passkey_login":               passkeySetting.Enabled,
		"passkey_display_name":        passkeySetting.RPDisplayName,
		"passkey_rp_id":               passkeySetting.RPID,
		"passkey_origins":             passkeySetting.Origins,
		"passkey_allow_insecure":      passkeySetting.AllowInsecureOrigin,
		"passkey_user_verification":   passkeySetting.UserVerification,
		"passkey_attachment":          passkeySetting.AttachmentPreference,
		"setup":                       constant.Setup,
		"user_agreement_enabled":      legalSetting.UserAgreement != "",
		"privacy_policy_enabled":      legalSetting.PrivacyPolicy != "",
		"checkin_enabled":             operation_setting.GetCheckinSetting().Enabled,
	}

	// 根据启用状态注入可选内容
	if cs.ApiInfoEnabled {
		data["api_info"] = console_setting.GetApiInfo()
	}
	agentCtx, isAgentSite := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
	announcements := make([]map[string]interface{}, 0)
	if cs.AnnouncementsEnabled {
		announcements = console_setting.GetAnnouncements()
		if isAgentSite && agentCtx != nil {
			announcements = console_setting.FilterAnnouncementsForAgentSite(announcements)
		}
		announcements = console_setting.FilterAnnouncementsForUserGroup(
			announcements,
			c.GetString("group"),
			c.GetInt("id") > 0,
		)
		for _, announcement := range announcements {
			delete(announcement, "target_groups")
		}
	}
	if isAgentSite && agentCtx != nil {
		agentAnnouncements, err := model.ListPublishedAgentAnnouncements(agentCtx.AgentID, 20, time.Now().Unix())
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to load agent %d announcements: %s", agentCtx.AgentID, err.Error()))
		} else {
			for _, announcement := range agentAnnouncements {
				announcements = append(announcements, announcement.PublicData())
			}
			if len(agentAnnouncements) > 0 {
				data["announcements_enabled"] = true
			}
		}
	}
	if len(announcements) > 0 || cs.AnnouncementsEnabled {
		console_setting.SortAnnouncements(announcements)
		data["announcements"] = announcements
	}
	if cs.FAQEnabled {
		data["faq"] = console_setting.GetFAQ()
	}
	if isAgentSite && agentCtx != nil {
		agentservice.ApplyBrandingToStatus(data, agentCtx.Branding)
		agentServerAddress := buildAgentServerAddress(c, agentCtx.Domain)
		data["server_address"] = agentServerAddress
		if apiInfo, ok := data["api_info"].([]map[string]interface{}); ok {
			data["api_info"] = rewriteAPIInfoURLs(apiInfo, agentServerAddress)
		}
		data["agent"] = gin.H{
			"id":             agentCtx.AgentID,
			"domain":         agentCtx.Domain,
			"server_address": agentServerAddress,
			"branding":       agentCtx.Branding,
		}
	}

	// Add enabled custom OAuth providers
	customProviders := oauth.GetEnabledCustomProviders()
	if len(customProviders) > 0 {
		type CustomOAuthInfo struct {
			Id                    int    `json:"id"`
			Name                  string `json:"name"`
			Slug                  string `json:"slug"`
			Icon                  string `json:"icon"`
			ClientId              string `json:"client_id"`
			AuthorizationEndpoint string `json:"authorization_endpoint"`
			Scopes                string `json:"scopes"`
		}
		providersInfo := make([]CustomOAuthInfo, 0, len(customProviders))
		for _, p := range customProviders {
			config := p.GetConfig()
			providersInfo = append(providersInfo, CustomOAuthInfo{
				Id:                    config.Id,
				Name:                  config.Name,
				Slug:                  config.Slug,
				Icon:                  config.Icon,
				ClientId:              config.ClientId,
				AuthorizationEndpoint: config.AuthorizationEndpoint,
				Scopes:                config.Scopes,
			})
		}
		data["custom_oauth_providers"] = providersInfo
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
	return
}

func buildAgentServerAddress(c *gin.Context, domain string) string {
	domain = agentservice.NormalizeHost(domain)
	if domain == "" {
		return system_setting.ServerAddress
	}
	scheme := requestScheme(c.Request)
	if scheme == "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, domain)
}

func requestScheme(r *http.Request) string {
	if r == nil {
		return ""
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		parts := strings.Split(proto, ",")
		return strings.ToLower(strings.TrimSpace(parts[0]))
	}
	if proto := r.Header.Get("X-Forwarded-Protocol"); proto != "" {
		parts := strings.Split(proto, ",")
		return strings.ToLower(strings.TrimSpace(parts[0]))
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Ssl"), "on") {
		return "https"
	}
	if r.TLS != nil {
		return "https"
	}
	if r.URL != nil && r.URL.Scheme != "" {
		return strings.ToLower(r.URL.Scheme)
	}
	return "http"
}

func rewriteAPIInfoURLs(items []map[string]interface{}, serverAddress string) []map[string]interface{} {
	base, err := url.Parse(serverAddress)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return items
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		next := make(map[string]interface{}, len(item))
		for key, value := range item {
			next[key] = value
		}
		if rawURL, ok := item["url"].(string); ok {
			next["url"] = rewriteURLOrigin(rawURL, base)
		}
		result = append(result, next)
	}
	return result
}

func rewriteURLOrigin(rawURL string, base *url.URL) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return rawURL
	}
	if parsed.IsAbs() {
		parsed.Scheme = base.Scheme
		parsed.Host = base.Host
		return parsed.String()
	}
	if strings.HasPrefix(parsed.Path, "/") {
		next := *base
		next.Path = parsed.Path
		next.RawQuery = parsed.RawQuery
		next.Fragment = parsed.Fragment
		return next.String()
	}
	return rawURL
}

func GetNotice(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["Notice"],
	})
	return
}

func GetAbout(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["About"],
	})
	return
}

func GetUserAgreement(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    system_setting.GetLegalSettings().UserAgreement,
	})
	return
}

func GetPrivacyPolicy(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    system_setting.GetLegalSettings().PrivacyPolicy,
	})
	return
}

func GetMidjourney(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["Midjourney"],
	})
	return
}

func GetHomePageContent(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	content := common.OptionMap["HomePageContent"]
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		if agentContent, hasAgentContent := agentservice.HomePageContentFromBranding(agentCtx.Branding); hasAgentContent {
			content = agentContent
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    content,
	})
	return
}

func SendEmailVerification(c *gin.Context) {
	email := model.NormalizeEmail(c.Query("email"))
	if err := common.Validate.Var(email, "required,email"); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的邮箱地址",
		})
		return
	}
	localPart := parts[0]
	domainPart := parts[1]
	if common.EmailDomainRestrictionEnabled {
		allowed := false
		for _, domain := range common.EmailDomainWhitelist {
			if domainPart == domain {
				allowed = true
				break
			}
		}
		if !allowed {
			common.ApiErrorI18n(c, i18n.MsgUserEmailDomainNotAllowed)
			return
		}
	}
	if common.EmailAliasRestrictionEnabled {
		containsSpecialSymbols := strings.Contains(localPart, "+") || strings.Contains(localPart, ".")
		if containsSpecialSymbols {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员已启用邮箱地址别名限制，您的邮箱地址由于包含特殊符号而被拒绝。",
			})
			return
		}
	}

	if model.IsEmailAlreadyTaken(email) {
		common.ApiErrorI18n(c, i18n.MsgUserEmailAlreadyTaken)
		return
	}
	code := common.GenerateVerificationCode(6)
	common.RegisterVerificationCodeWithKey(email, code, common.EmailVerificationPurpose)
	subject := fmt.Sprintf("%s邮箱验证邮件", common.SystemName)
	content := fmt.Sprintf("<p>您好，你正在进行%s邮箱验证。</p>"+
		"<p>您的验证码为: <strong>%s</strong></p>"+
		"<p>验证码 %d 分钟内有效，如果不是本人操作，请忽略。</p>", common.SystemName, code, common.VerificationValidMinutes)
	err := common.SendEmail(subject, email, content)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func SendPasswordResetEmail(c *gin.Context) {
	email := model.NormalizeEmail(c.Query("email"))
	if err := common.Validate.Var(email, "required,email"); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if _, err := model.GetUniqueUserByEmail(email); err == nil {
		code := common.GenerateVerificationCode(0)
		common.RegisterVerificationCodeWithKey(email, code, common.PasswordResetPurpose)
		link := fmt.Sprintf("%s/user/reset?email=%s&token=%s", system_setting.ServerAddress, email, code)
		subject := fmt.Sprintf("%s密码重置", common.SystemName)
		content := fmt.Sprintf("<p>您好，你正在进行%s密码重置。</p>"+
			"<p>点击 <a href='%s'>此处</a> 进行密码重置。</p>"+
			"<p>如果链接无法点击，请尝试点击下面的链接或将其复制到浏览器中打开：<br> %s </p>"+
			"<p>重置链接 %d 分钟内有效，如果不是本人操作，请忽略。</p>", common.SystemName, link, link, common.VerificationValidMinutes)
		err := common.SendEmail(subject, email, content)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("failed to send password reset email to %s: %s", email, err.Error()))
		}
	} else if err != nil && !errors.Is(err, model.ErrEmailNotFound) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("skip password reset email for %s: %s", email, err.Error()))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

type PasswordResetRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

func ResetPassword(c *gin.Context) {
	var req PasswordResetRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req.Email = model.NormalizeEmail(req.Email)
	if req.Email == "" || req.Token == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if !common.VerifyCodeWithKey(req.Email, req.Token, common.PasswordResetPurpose) {
		common.ApiErrorI18n(c, i18n.MsgUserPasswordResetLinkInvalid)
		return
	}
	password := common.GenerateVerificationCode(12)
	err = model.ResetUserPasswordByEmail(req.Email, password)
	if err != nil {
		if errors.Is(err, model.ErrEmailNotFound) || errors.Is(err, model.ErrEmailAmbiguous) {
			common.ApiErrorI18n(c, i18n.MsgUserPasswordResetLinkInvalid)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.DeleteKey(req.Email, common.PasswordResetPurpose)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    password,
	})
	return
}
