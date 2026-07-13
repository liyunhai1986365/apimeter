package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	// Import oauth package to register providers via init()
	_ "github.com/QuantumNous/new-api/oauth"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.RouteTag("api"))
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", controller.PostSetup)
		apiRouter.GET("/status", controller.GetStatus)
		registerCommonConfigurableAssetAPIRoutes(apiRouter)
		apiRouter.GET("/relay-temp-images/:id", controller.ServeRelayTempImage)
		apiRouter.HEAD("/relay-temp-images/:id", controller.ServeRelayTempImage)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/dashboard/flow", middleware.UserAuth(), controller.GetFlowQuotaData)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.HeaderNavModuleAuth("pricing"), controller.GetPricing)
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.HeaderNavModulePublicOrUserAuth("pricing"))
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary)
			perfMetricsRoute.GET("/groups", controller.GetPerfMetricsGroups)
			perfMetricsRoute.GET("", controller.GetPerfMetrics)
		}
		apiRouter.GET("/rankings", middleware.HeaderNavModuleAuth("rankings"), controller.GetRankings)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.POST("/oauth/email/bind", middleware.CriticalRateLimit(), controller.EmailBind)
		// Non-standard OAuth (WeChat, Telegram) - keep original routes
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO) - unified route
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", controller.CreemWebhook)
		apiRouter.POST("/waffo/webhook", controller.WaffoWebhook)
		// :env separates test vs prod URLs so the operator can register each
		// in Pancake's matching webhook slot; handler enforces env match.
		apiRouter.POST("/waffo-pancake/webhook/:env", controller.WaffoPancakeWebhook)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.POST("/epay/notify", controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			selfRoute.Use(middleware.AgentUserAuth())
			{
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self/providers", controller.ListUserOwnedProviders)
				selfRoute.POST("/self/providers", controller.CreateUserOwnedProvider)
				selfRoute.PUT("/self/providers/:id", controller.UpdateUserOwnedProvider)
				selfRoute.DELETE("/self/providers/:id", controller.DeleteUserOwnedProvider)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)
				selfRoute.POST("/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
				selfRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPancakePay)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)
				selfRoute.POST("/setting/image-storage/test", controller.TestUserImageStorage)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin)

				// Custom OAuth bindings
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)
				adminRoute.DELETE("/:id/oauth/bindings/:provider_id", controller.UnbindCustomOAuthByAdmin)
				adminRoute.DELETE("/:id/bindings/:binding_type", controller.AdminClearUserBinding)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.POST("/:id/email", controller.SendAdminUserEmail)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}

		openMosaicRoute := apiRouter.Group("/integrations/openmosaic")
		{
			openMosaicRoute.GET("/authorize", middleware.BrowserSessionAuth(), controller.AuthorizeOpenMosaicSSO)
			openMosaicRoute.POST("/exchange", middleware.CriticalRateLimit(), controller.ExchangeOpenMosaicSSO)
		}

		// Subscription billing (plans, purchase, admin management)
		apiRouter.GET("/subscription/plans", controller.GetSubscriptionPlans)

		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			subscriptionRoute.POST("/self/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetSubscriptionTokenKey)
			subscriptionRoute.GET("/self/:id/usage", controller.GetSubscriptionKeyUsage)
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
			subscriptionRoute.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)
			subscriptionRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)
			subscriptionRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)
			subscriptionRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestWaffoPancakePay)
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", controller.SubscriptionEpayReturn)
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.POST("/announcements/email", controller.SendAnnouncementEmail)
			optionRoute.POST("/payment_compliance", controller.ConfirmPaymentCompliance)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.GET("/routing-strategies", controller.ListRoutingStrategySnapshots)
			optionRoute.POST("/routing-strategies/refresh", controller.RefreshRoutingStrategySnapshots)
			optionRoute.PUT("/routing-strategies/override", controller.SaveRoutingStrategySnapshotOverride)
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
			optionRoute.POST("/waffo-pancake/catalog", controller.ListWaffoPancakeCatalog)
			optionRoute.POST("/waffo-pancake/pair", controller.CreateWaffoPancakePair)
			optionRoute.POST("/waffo-pancake/save", controller.SaveWaffoPancake)
			optionRoute.POST("/waffo-pancake/subscription-product", controller.CreateWaffoPancakeSubscriptionProduct)
			optionRoute.POST("/waffo-pancake/subscription-product-options", controller.ListWaffoPancakeSubscriptionProductOptions)
			optionRoute.POST("/global-webhook/test", controller.SendGlobalWebhookTest)
		}

		// Custom OAuth provider management (root only)
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.POST("/discovery", controller.FetchCustomOAuthDiscovery)
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)
			customOAuthRoute.POST("/", controller.CreateCustomOAuthProvider)
			customOAuthRoute.PUT("/:id", controller.UpdateCustomOAuthProvider)
			customOAuthRoute.DELETE("/:id", controller.DeleteCustomOAuthProvider)
		}
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", controller.ForceGC)
			performanceRoute.GET("/logs", controller.GetLogFiles)
			performanceRoute.DELETE("/logs", controller.CleanupLogFiles)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		apiRouter.GET("/agent/domains/tls-ask", controller.AgentDomainTLSAsk)
		agentSelfRoute := apiRouter.Group("/agent")
		agentSelfRoute.Use(middleware.UserAuth())
		agentSelfRoute.Use(middleware.AgentOwnerAuth())
		{
			agentSelfRoute.GET("/self", controller.GetAgentSelf)
			agentSelfRoute.PUT("/self/branding", controller.UpdateAgentSelfBranding)
			agentSelfRoute.GET("/domains", controller.AgentListDomains)
			agentSelfRoute.POST("/domains", controller.AgentCreateDomain)
			agentSelfRoute.POST("/domains/:id/verify", controller.AgentVerifyDomainCNAME)
			agentSelfRoute.PUT("/domains/:id/status", controller.AgentUpdateDomainStatus)
			agentSelfRoute.GET("/group_ratios", controller.AgentListGroupRatios)
			agentSelfRoute.POST("/group_ratios", controller.AgentUpsertGroupRatio)
			agentSelfRoute.GET("/user_groups", controller.AgentListUserGroupConfigs)
			agentSelfRoute.POST("/user_groups", controller.AgentUpsertUserGroupConfig)
			agentSelfRoute.GET("/users", controller.AgentListUsers)
			agentSelfRoute.POST("/users", controller.AgentBindUser)
			agentSelfRoute.PUT("/users/:user_id/status", controller.AgentUpdateUserStatus)
			agentSelfRoute.PUT("/users/:user_id/group", controller.AgentUpdateUserGroup)
			agentSelfRoute.POST("/users/:user_id/quota", controller.AgentAdjustUserQuota)
			agentSelfRoute.GET("/users/:user_id/tokens", controller.AgentListUserTokens)
			agentSelfRoute.GET("/ledger", controller.AgentListLedger)
			agentSelfRoute.GET("/withdrawals", controller.AgentListWithdrawals)
			agentSelfRoute.POST("/withdrawals", controller.AgentSubmitWithdrawal)
		}
		agentAdminRoute := apiRouter.Group("/agents")
		agentAdminRoute.Use(middleware.AdminAuth())
		{
			agentAdminRoute.GET("/", controller.AdminListAgents)
			agentAdminRoute.GET("/domains", controller.AdminListAgentDomains)
			agentAdminRoute.POST("/", controller.AdminCreateAgent)
			agentAdminRoute.PUT("/:id", controller.AdminUpdateAgent)
			agentAdminRoute.PUT("/:id/status", controller.AdminUpdateAgentStatus)
			agentAdminRoute.GET("/:id/domains", controller.AgentListDomains)
			agentAdminRoute.POST("/:id/domains", controller.AgentCreateDomain)
			agentAdminRoute.POST("/:id/domains/:domain_id/verify", controller.AgentVerifyDomainCNAME)
			agentAdminRoute.PUT("/:id/domains/:domain_id/status", controller.AgentUpdateDomainStatus)
			agentAdminRoute.GET("/:id/group_ratios", controller.AgentListGroupRatios)
			agentAdminRoute.POST("/:id/group_ratios", controller.AgentUpsertGroupRatio)
			agentAdminRoute.GET("/:id/user_groups", controller.AgentListUserGroupConfigs)
			agentAdminRoute.POST("/:id/user_groups", controller.AgentUpsertUserGroupConfig)
			agentAdminRoute.GET("/:id/users", controller.AgentListUsers)
			agentAdminRoute.POST("/:id/users", controller.AgentBindUser)
			agentAdminRoute.PUT("/:id/users/:user_id/status", controller.AgentUpdateUserStatus)
			agentAdminRoute.PUT("/:id/users/:user_id/group", controller.AgentUpdateUserGroup)
			agentAdminRoute.POST("/:id/users/:user_id/quota", controller.AgentAdjustUserQuota)
			agentAdminRoute.GET("/:id/users/:user_id/tokens", controller.AgentListUserTokens)
			agentAdminRoute.GET("/:id/ledger", controller.AgentListLedger)
			agentAdminRoute.GET("/withdrawals", controller.AdminListAgentWithdrawals)
			agentAdminRoute.PUT("/withdrawals/:id/status", controller.AdminCompleteAgentWithdrawal)
		}
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth())
		{
			channelRoute.GET("/", controller.GetAllChannels)
			channelRoute.GET("/search", controller.SearchChannels)
			channelRoute.GET("/models", controller.ChannelListModels)
			channelRoute.GET("/models_enabled", controller.EnabledListModels)
			channelRoute.GET("/operation_records", controller.GetChannelOperationRecords)
			channelRoute.GET("/filter-options", controller.GetChannelFilterOptions)
			channelRoute.GET("/groups", controller.GetChannelGroups)
			channelRoute.GET("/protocol_profiles", controller.GetConfigurableProtocolProfiles)
			channelRoute.GET("/:id", controller.GetChannel)
			channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), controller.GetChannelKey)
			channelRoute.GET("/test", controller.TestAllChannels)
			channelRoute.GET("/test/:id", controller.TestChannel)
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)
			channelRoute.POST("/", middleware.RootAuth(), controller.AddChannel)
			channelRoute.PUT("/", middleware.RootAuth(), controller.UpdateChannel)
			channelRoute.POST("/:id/retry_policy_rules", middleware.RootAuth(), controller.AppendChannelRetryPolicyRule)
			channelRoute.DELETE("/disabled", middleware.RootAuth(), controller.DeleteDisabledChannel)
			channelRoute.DELETE("/:id/models", middleware.RootAuth(), controller.RemoveChannelModel)
			channelRoute.POST("/tag/disabled", middleware.RootAuth(), controller.DisableTagChannels)
			channelRoute.POST("/tag/enabled", middleware.RootAuth(), controller.EnableTagChannels)
			channelRoute.PUT("/tag", middleware.RootAuth(), controller.EditTagChannels)
			channelRoute.DELETE("/:id", middleware.RootAuth(), controller.DeleteChannel)
			channelRoute.POST("/batch", middleware.RootAuth(), controller.DeleteChannelBatch)
			channelRoute.POST("/fix", middleware.RootAuth(), controller.FixChannelsAbilities)
			channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)
			channelRoute.POST("/fetch_models", middleware.RootAuth(), controller.FetchModels)
			channelRoute.POST("/codex/oauth/start", middleware.RootAuth(), controller.StartCodexOAuth)
			channelRoute.POST("/codex/oauth/complete", middleware.RootAuth(), controller.CompleteCodexOAuth)
			channelRoute.POST("/:id/codex/oauth/start", middleware.RootAuth(), controller.StartCodexOAuthForChannel)
			channelRoute.POST("/:id/codex/oauth/complete", middleware.RootAuth(), controller.CompleteCodexOAuthForChannel)
			channelRoute.POST("/:id/codex/refresh", middleware.RootAuth(), controller.RefreshCodexChannelCredential)
			channelRoute.GET("/:id/codex/usage", controller.GetCodexChannelUsage)
			channelRoute.POST("/ollama/pull", middleware.RootAuth(), controller.OllamaPullModel)
			channelRoute.POST("/ollama/pull/stream", middleware.RootAuth(), controller.OllamaPullModelStream)
			channelRoute.DELETE("/ollama/delete", middleware.RootAuth(), controller.OllamaDeleteModel)
			channelRoute.GET("/ollama/version/:id", controller.OllamaVersion)
			channelRoute.POST("/batch/tag", middleware.RootAuth(), controller.BatchSetChannelTag)
			channelRoute.POST("/batch/groups", middleware.RootAuth(), controller.BatchSetChannelGroups)
			channelRoute.GET("/tag/models", controller.GetTagModels)
			channelRoute.POST("/copy/:id", middleware.RootAuth(), controller.CopyChannel)
			channelRoute.POST("/multi_key/manage", middleware.RootAuth(), controller.ManageMultiKeys)
			channelRoute.POST("/upstream_updates/apply", middleware.RootAuth(), controller.ApplyChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/apply_all", middleware.RootAuth(), controller.ApplyAllChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/detect", middleware.RootAuth(), controller.DetectChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/detect_all", middleware.RootAuth(), controller.DetectAllChannelUpstreamModelUpdates)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/filter-options", controller.GetTokenFilterOptions)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKey)
			tokenRoute.POST("/:id/quota-reset", controller.ResetTokenQuota)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/batch/keys", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKeysBatch)
		}
		workspaceRoute := apiRouter.Group("/workspaces")
		workspaceRoute.Use(middleware.UserAuth())
		{
			workspaceRoute.GET("/", controller.ListWorkspaces)
			workspaceRoute.GET("", controller.ListWorkspaces)
			workspaceRoute.GET("/:id/usage", controller.GetWorkspaceUsageStats)
			workspaceRoute.GET("/:id/quota-reset", controller.GetWorkspaceQuotaResetConfig)
			workspaceRoute.PUT("/:id/quota-reset", controller.UpdateWorkspaceQuotaResetConfig)
			workspaceRoute.POST("/:id/quota-reset/reset", controller.ResetWorkspaceQuotaNow)
			workspaceRoute.POST("/", controller.CreateWorkspace)
			workspaceRoute.PUT("/:id", controller.UpdateWorkspace)
			workspaceRoute.DELETE("/:id", controller.DeleteWorkspace)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthReadOnly())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/model_profit_stats", middleware.RootAuth(), controller.GetLogsModelProfitStats)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/error-request/:log_id", middleware.AdminAuth(), controller.GetErrorRequestLog)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)

		retryRoute := apiRouter.Group("/retry-route")
		retryRoute.Use(middleware.AdminAuth())
		{
			retryRoute.GET("/events", controller.GetRetryRouteEvents)
			retryRoute.GET("/events/stat", controller.GetRetryRouteEventStats)
			retryRoute.GET("/events/stat/rules", controller.GetRetryRouteRuleStats)
			retryRoute.GET("/events/:id", controller.GetRetryRouteEvent)
			retryRoute.POST("/rules/test", controller.TestRetryRouteRules)
		}

		billingRoute := apiRouter.Group("/billing")
		billingRoute.GET("/account-ledger", middleware.UserAuth(), controller.ListAccountLedgerEntries)
		billingRoute.GET("/breakdowns", middleware.UserAuth(), controller.ListBillingBreakdowns)
		billingRoute.GET("/breakdowns/export", middleware.UserAuth(), controller.ExportBillingBreakdowns)
		billingRoute.GET("/monthly-statements", middleware.UserAuth(), controller.ListBillingMonthlyStatements)
		billingRoute.POST("/monthly-statements/generate", middleware.UserAuth(), controller.GenerateBillingMonthlyStatement)
		billingRoute.GET("/monthly-statements/:statement_no/export", middleware.UserAuth(), controller.ExportBillingMonthlyStatement)
		billingRoute.GET("/monthly-statements/:statement_no/summaries", middleware.UserAuth(), controller.GetBillingMonthlyStatementSummaries)
		billingRoute.GET("/daily-reconciliations", middleware.UserAuth(), controller.ListDailyBillingReconciliations)
		billingRoute.POST("/admin/backfill-v2", middleware.AdminAuth(), controller.BackfillBillingV2)
		billingRoute.POST("/admin/generate-recent-month", middleware.AdminAuth(), controller.GenerateRecentMonthlyBillingStatements)

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/dimensions", middleware.AdminAuth(), controller.GetAllUsageDimensionTrends)
		dataRoute.GET("/users", middleware.AdminAuth(), controller.GetQuotaDatesByUser)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)
		dataRoute.GET("/self/dimensions", middleware.UserAuth(), controller.GetUserUsageDimensionTrends)
		dataRoute.GET("/self/tokens", middleware.UserAuth(), controller.GetUserTokenQuotaData)

		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/sort", controller.UpdateVendorSortOrder)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		newAPISupplierRoute := apiRouter.Group("/newapi_suppliers")
		newAPISupplierRoute.Use(middleware.AdminAuth())
		{
			newAPISupplierRoute.GET("/", controller.GetAllNewAPISuppliers)
			newAPISupplierRoute.GET("/search", controller.SearchNewAPISuppliers)
			newAPISupplierRoute.GET("/:id", controller.GetNewAPISupplier)
			newAPISupplierRoute.POST("/", controller.CreateNewAPISupplier)
			newAPISupplierRoute.PUT("/", controller.UpdateNewAPISupplier)
			newAPISupplierRoute.DELETE("/:id", controller.DeleteNewAPISupplier)
			newAPISupplierRoute.POST("/:id/balance", controller.QueryNewAPISupplierBalance)
			newAPISupplierRoute.POST("/:id/check", controller.CheckNewAPISupplier)
			newAPISupplierRoute.POST("/:id/test_model", controller.TestNewAPISupplierModel)
			newAPISupplierRoute.POST("/:id/configure_channels", controller.ConfigureNewAPISupplierChannels)
			newAPISupplierRoute.GET("/:id/channels", controller.ListNewAPISupplierChannels)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/monitor", controller.ListModelMonitorModels)
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id/monitor", controller.GetModelMonitor)
			modelsRoute.GET("/:id/test_channels", controller.GetModelChannelTestResults)
			modelsRoute.POST("/:id/test_channels", controller.TestModelChannels)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/sort", controller.UpdateModelSortOrder)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}

		// Deployments (model deployment management)
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)
			deploymentsRoute.POST("/settings/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/", controller.GetAllDeployments)
			deploymentsRoute.GET("/search", controller.SearchDeployments)
			deploymentsRoute.POST("/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)
			deploymentsRoute.GET("/locations", controller.GetLocations)
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas)
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability)
			deploymentsRoute.POST("/", controller.CreateDeployment)

			deploymentsRoute.GET("/:id", controller.GetDeployment)
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails)
			deploymentsRoute.PUT("/:id", controller.UpdateDeployment)
			deploymentsRoute.PUT("/:id/name", controller.UpdateDeploymentName)
			deploymentsRoute.POST("/:id/extend", controller.ExtendDeployment)
			deploymentsRoute.DELETE("/:id", controller.DeleteDeployment)
		}

		// System Tasks (scheduled task management)
		systemTaskRoute := apiRouter.Group("/system/tasks")
		systemTaskRoute.Use(middleware.RootAuth())
		{
			systemTaskRoute.GET("/", controller.ListSystemTasks)
			systemTaskRoute.POST("/", controller.CreateLogCleanupSystemTask)
			systemTaskRoute.GET("/current", controller.GetCurrentSystemTask)
			systemTaskRoute.GET("/:id", controller.GetSystemTask)
		}

		// System Info (system instances and monitoring)
		systemInfoRoute := apiRouter.Group("/system/info")
		systemInfoRoute.Use(middleware.RootAuth())
		{
			systemInfoRoute.GET("/instances", controller.ListSystemInstances)
		}
	}
}
