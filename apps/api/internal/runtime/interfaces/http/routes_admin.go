package httpapi

import (
	"ideaboard/api/internal/identity/auth"

	"github.com/gin-gonic/gin"
)

func registerAuthenticatedAdminRoutes(r *gin.Engine, authDeps AuthDeps, h AdminRouteDeps) {
	adminAPI := r.Group(AdminAPIGroupPath)
	adminAPI.Use(auth.AuthMiddleware(authDeps.AuthServiceClient, authDeps.UserRepo, authDeps.UserLifecycle), auth.AdminMiddleware())
	registerAdminRoutes(adminAPI, h)
}

func registerAdminRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	registerAdminBrandLogoRoutes(adminAPI, h)
	registerAdminSettingsRoutes(adminAPI, h)
	registerAdminUserRoutes(adminAPI, h)
	registerAdminTaskRoutes(adminAPI, h)
	registerAdminUploadRoutes(adminAPI, h)
	registerAdminStorageRoutes(adminAPI, h)
	registerAdminSearchProviderRoutes(adminAPI, h)
	registerAdminFeaturePromptRoutes(adminAPI, h)
	registerAdminCreditRoutes(adminAPI, h)
	registerAdminBillingRoutes(adminAPI, h)
	registerAdminRewardRoutes(adminAPI, h)
	registerAdminPaymentRoutes(adminAPI, h)
}

func registerAdminBrandLogoRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	if h.AdminBrandLogoHandler == nil {
		return
	}
	adminAPI.GET(RouteAdminBrandLogoOverrides, h.AdminBrandLogoHandler.List)
	adminAPI.PUT(RouteAdminBrandLogoOverrides+"/:scope/:target_key", h.AdminBrandLogoHandler.Upsert)
	adminAPI.DELETE(RouteAdminBrandLogoOverrides+"/:scope/:target_key", h.AdminBrandLogoHandler.Delete)
}

func registerAdminSearchProviderRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	if h.SearchCatalogHandler == nil {
		return
	}
	adminAPI.GET("/search/providers", h.SearchCatalogHandler.List)
	adminAPI.PUT("/search/providers/:provider", h.SearchCatalogHandler.Update)
	adminAPI.POST("/search/providers/:provider/test", h.SearchCatalogHandler.Test)
}

func registerAdminPaymentRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET(RouteAdminStripeConfiguration, h.PaymentAdminHandler.GetStripeConfiguration)
	adminAPI.PUT(RouteAdminStripeConfiguration, h.PaymentAdminHandler.UpdateStripeConfiguration)
	adminAPI.GET("/payments/orders", h.PaymentAdminHandler.ListAdminPaymentOrders)
	adminAPI.GET("/payments/orders/:order_number", h.PaymentAdminHandler.GetAdminPaymentOrder)
	adminAPI.POST("/payments/orders/:order_number/refunds", h.PaymentAdminHandler.CreateAdminPaymentRefund)
}

func registerAdminRewardRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.POST("/rewards/codes/search", h.RewardAdminReadHandler.SearchRewardCodes)
	adminAPI.POST("/rewards/codes", h.RewardAdminCommandHandler.CreateRewardCode)
	adminAPI.GET("/rewards/codes/:id/redemptions", h.RewardAdminReadHandler.ListAdminRedemptions)
	adminAPI.GET("/rewards/codes/:id/redemptions/:redemption_id", h.RewardAdminReadHandler.GetAdminRedemption)
	adminAPI.PATCH("/rewards/codes/:id", h.RewardAdminCommandHandler.UpdateRewardCode)
	adminAPI.PATCH("/rewards/codes/:id/enabled", h.RewardAdminCommandHandler.SetRewardCodeEnabled)
	adminAPI.DELETE("/rewards/codes/:id", h.RewardAdminCommandHandler.DeleteRewardCode)
}

func registerAdminCreditRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.POST("/credits/grants", h.CreditAdminCommandHandler.Grant)
	adminAPI.POST("/credits/deductions", h.CreditAdminCommandHandler.Deduct)
	adminAPI.POST("/credits/corrections", h.CreditAdminCommandHandler.Correct)
	adminAPI.POST("/credits/refunds", h.CreditAdminCommandHandler.Refund)
	adminAPI.GET("/credits/wallets/:public_id", h.CreditAdminReadHandler.GetWallet)
	adminAPI.GET("/credits/wallets/:public_id/ledger", h.CreditAdminReadHandler.ListLedger)
	adminAPI.GET("/credits/health", h.CreditReconciliationHandler.GetCreditHealth)
	adminAPI.POST("/credits/reconciliation/runs", h.CreditReconciliationHandler.StartReconciliation)
	adminAPI.GET("/credits/reconciliation/runs", h.CreditReconciliationHandler.ListReconciliationRuns)
	adminAPI.GET("/credits/reconciliation/runs/:run_id", h.CreditReconciliationHandler.GetReconciliationRun)
	adminAPI.GET("/credits/reconciliation/findings", h.CreditReconciliationHandler.ListGlobalReconciliationFindings)
}

func registerAdminBillingRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/billing/catalog", h.BillingHandler.ListBillableCatalog)
	adminAPI.POST("/billing/catalog/:sku/prices", h.BillingHandler.PublishPrice)
	adminAPI.PUT("/billing/catalog/:sku/availability", h.BillingHandler.SetAvailability)
	adminAPI.DELETE("/billing/catalog/:sku", h.BillingHandler.DeleteArchivedItem)
}

func registerAdminSettingsRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/settings/registration", h.AdminHandler.GetRegistrationSetting)
	adminAPI.PUT("/settings/registration", h.AdminHandler.UpdateRegistrationSetting)
	adminAPI.GET(RouteAdminSettingsRateLimit, h.AdminHandler.GetRateLimitSettings)
	adminAPI.PUT(RouteAdminSettingsRateLimit, h.AdminHandler.UpdateRateLimitSettings)
	adminAPI.GET(RouteAdminSettingsBranding, h.AdminHandler.GetBrandingSettings)
	adminAPI.PUT(RouteAdminSettingsBranding, h.AdminHandler.UpdateBrandingSettings)
	adminAPI.GET(RouteAdminSettingsMediaUpload, h.AdminHandler.GetMediaUploadSettings)
	adminAPI.PUT(RouteAdminSettingsMediaUpload, h.AdminHandler.UpdateMediaUploadSettings)
	adminAPI.GET(RouteAdminSettingsOutbound, h.AdminHandler.GetOutboundPolicy)
	adminAPI.PUT(RouteAdminSettingsOutbound, h.AdminHandler.UpdateOutboundPolicy)
	if h.AIGatewayHandler != nil {
		adminAPI.GET(RouteAdminSettingsAIGateway, h.AIGatewayHandler.Get)
		adminAPI.PUT(RouteAdminSettingsAIGateway, h.AIGatewayHandler.Update)
		adminAPI.GET(RouteAdminSettingsAIGatewayModels, h.AIGatewayHandler.ListMappings)
		adminAPI.PUT(RouteAdminSettingsAIGatewayModels+"/:feature", h.AIGatewayHandler.SaveMapping)
		adminAPI.GET(RouteAdminSettingsAIGatewayCatalog, h.AIGatewayHandler.ListCatalog)
		adminAPI.PUT(RouteAdminSettingsAIGatewayCatalog, h.AIGatewayHandler.SaveCatalog)
		adminAPI.GET(RouteAdminSettingsAIGatewayProviders, h.AIGatewayHandler.ListProviders)
		adminAPI.GET(RouteAdminSettingsAIGatewayDetectModels, h.AIGatewayHandler.DetectModels)
		adminAPI.GET(RouteAdminSettingsAIGatewayModelsReference, h.AIGatewayHandler.GetModelReference)
		adminAPI.GET(RouteAdminSettingsAIGatewayBrandLogo, h.AIGatewayHandler.GetBrandLogo)
		adminAPI.GET(RouteAdminSettingsAIGatewayModelsObserved, h.AIGatewayHandler.ListModelsObserved)
		adminAPI.POST(RouteAdminSettingsAIGatewayRoutePreview, h.AIGatewayHandler.PreviewGatewayRoute)
		adminAPI.POST(RouteAdminSettingsAIGatewayTestConnection, h.AIGatewayHandler.TestConnection)
	}
	if h.MailConfigHandler != nil {
		adminAPI.GET(RouteAdminSettingsMail, h.MailConfigHandler.Get)
		adminAPI.PUT(RouteAdminSettingsMail, h.MailConfigHandler.Update)
		adminAPI.DELETE(RouteAdminSettingsMail, h.MailConfigHandler.Clear)
		adminAPI.POST(RouteAdminSettingsMail+"/test", h.MailConfigHandler.Test)
	}
}

func registerAdminUserRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/users", h.AdminHandler.ListUsers)
	if h.SocialProviderHandler != nil {
		adminAPI.GET(RouteAdminUsersOAuthProvider, h.SocialProviderHandler.Get)
		adminAPI.PUT(RouteAdminUsersOAuthProvider, h.SocialProviderHandler.Update)
	}
	adminAPI.POST("/users/:public_id/ban", h.AdminHandler.BanUser)
	adminAPI.POST("/users/:public_id/unban", h.AdminHandler.UnbanUser)
	adminAPI.POST("/users/:public_id/revoke-sessions", h.AdminHandler.RevokeUserSessions)
	adminAPI.PUT("/users/:public_id/role", h.AdminHandler.UpdateUserRole)
	adminAPI.DELETE("/users/:public_id", h.AdminHandler.DeleteUser)
	adminAPI.POST("/users/:public_id/impersonate", h.AdminHandler.ImpersonateUser)
}

func registerAdminTaskRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.POST("/tasks/submit", h.TaskHandler.SubmitTask)
	adminAPI.POST("/tasks/:task_id/cancel", h.TaskHandler.CancelAdminTask)
	adminAPI.GET("/tasks/:task_id", h.TaskHandler.GetTask)
	adminAPI.GET("/tasks", h.TaskHandler.ListTasks)
	adminAPI.GET("/monitor", h.TaskStatsHandler.GetMonitor)
	adminAPI.GET("/monitor/stream", h.TaskStatsHandler.StreamMonitor)
	adminAPI.GET("/statistics", h.TaskStatsHandler.GetStatistics)
	adminAPI.POST("/statistics/backfill", h.TaskStatsHandler.BackfillStatistics)
}

func registerAdminUploadRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.POST(RouteAdminUpload, requestBodyLimitMiddleware(uploadRequestBodyLimitBytes), adminUploadHandler(h.StorageService, h.UploadSettings))
}

func registerAdminStorageRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/storage/providers", h.StorageHandler.ListProviders)
	adminAPI.GET("/storage/profiles", h.StorageHandler.ListProfiles)
	adminAPI.POST("/storage/profiles", h.StorageHandler.CreateProfile)
	adminAPI.PUT("/storage/profiles/:id", h.StorageHandler.UpdateProfile)
	adminAPI.DELETE("/storage/profiles/:id", h.StorageHandler.DeleteProfile)
	adminAPI.POST("/storage/profiles/:id/apply", h.StorageHandler.ApplyProfile)
	adminAPI.POST("/storage/profiles/:id/test", h.StorageHandler.TestProfile)
	adminAPI.GET("/storage/profiles/:id/bucket-info", h.StorageHandler.GetBucketInfo)
}

func registerAdminFeaturePromptRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/feature-prompts", h.FeaturePromptHandler.List)
	adminAPI.GET("/feature-prompts/:slug", h.FeaturePromptHandler.Get)
	adminAPI.PUT("/feature-prompts/:slug", h.FeaturePromptHandler.Save)
}
