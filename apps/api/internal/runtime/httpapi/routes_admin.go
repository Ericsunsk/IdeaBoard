package httpapi

import (
	"ideaboard/api/internal/identity/auth"

	"github.com/gin-gonic/gin"
)

func registerAuthenticatedAdminRoutes(r *gin.Engine, authDeps AuthDeps, h AdminRouteDeps) {
	adminAPI := r.Group("/api/admin")
	adminAPI.Use(auth.AuthMiddleware(authDeps.AuthServiceClient, authDeps.UserRepo, authDeps.UserLifecycle), auth.AdminMiddleware())
	registerAdminRoutes(adminAPI, h)
}

func registerAdminRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	registerAdminSettingsRoutes(adminAPI, h)
	registerAdminUserRoutes(adminAPI, h)
	registerAdminAccountRoutes(adminAPI, h)
	registerAdminTaskRoutes(adminAPI, h)
	registerAdminUploadRoutes(adminAPI, h)
	registerAdminStorageRoutes(adminAPI, h)
	registerAdminProviderEndpointRoutes(adminAPI, h)
	registerAdminLineDraftRoutes(adminAPI, h)
	registerAdminFeaturePromptRoutes(adminAPI, h)
	registerAdminCreditRoutes(adminAPI, h)
	registerAdminBillingRoutes(adminAPI, h)
	registerAdminRewardRoutes(adminAPI, h)
}

func registerAdminRewardRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.POST("/rewards/codes/search", h.RewardHandler.SearchRewardCodes)
	adminAPI.POST("/rewards/codes", h.RewardHandler.CreateRewardCode)
	adminAPI.GET("/rewards/codes/:id/redemptions", h.RewardHandler.ListAdminRedemptions)
	adminAPI.GET("/rewards/codes/:id/redemptions/:redemption_id", h.RewardHandler.GetAdminRedemption)
	adminAPI.PATCH("/rewards/codes/:id", h.RewardHandler.UpdateRewardCode)
	adminAPI.PATCH("/rewards/codes/:id/enabled", h.RewardHandler.SetRewardCodeEnabled)
	adminAPI.DELETE("/rewards/codes/:id", h.RewardHandler.DeleteRewardCode)
}

func registerAdminCreditRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.POST("/credits/grants", h.CreditHandler.Grant)
	adminAPI.POST("/credits/deductions", h.CreditHandler.Deduct)
	adminAPI.POST("/credits/corrections", h.CreditHandler.Correct)
	adminAPI.POST("/credits/refunds", h.CreditHandler.Refund)
	adminAPI.GET("/credits/wallets/:user_id", h.CreditHandler.GetWallet)
	adminAPI.GET("/credits/wallets/:user_id/ledger", h.CreditHandler.ListLedger)
	adminAPI.GET("/credits/health", h.CreditHandler.GetCreditHealth)
	adminAPI.POST("/credits/reconciliation/runs", h.CreditHandler.StartReconciliation)
	adminAPI.GET("/credits/reconciliation/runs", h.CreditHandler.ListReconciliationRuns)
	adminAPI.GET("/credits/reconciliation/runs/:run_id", h.CreditHandler.GetReconciliationRun)
	adminAPI.GET("/credits/reconciliation/findings", h.CreditHandler.ListGlobalReconciliationFindings)
}

func registerAdminBillingRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/billing/catalog", h.BillingHandler.ListBillableCatalog)
	adminAPI.POST("/billing/catalog/:sku/prices", h.BillingHandler.PublishPrice)
	adminAPI.GET("/billing/catalog/:sku/prices", h.BillingHandler.ListBillableItemPrices)
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
	adminAPI.GET(RouteAdminSettingsPublicURLs, h.AdminHandler.GetPublicURLSettings)
	adminAPI.PUT(RouteAdminSettingsPublicURLs, h.AdminHandler.UpdatePublicURLSettings)
	adminAPI.GET(RouteAdminSettingsTaskMonitor, h.AdminHandler.GetTaskMonitorSettings)
	adminAPI.PUT(RouteAdminSettingsTaskMonitor, h.AdminHandler.UpdateTaskMonitorSettings)
	adminAPI.GET(RouteAdminSettingsOperational, h.AdminHandler.GetOperationalSettings)
	adminAPI.PUT(RouteAdminSettingsOperational, h.AdminHandler.UpdateOperationalSettings)
}

func registerAdminUserRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/users", h.AdminHandler.ListUsers)
	adminAPI.POST("/users/:id/ban", h.AdminHandler.BanUser)
	adminAPI.POST("/users/:id/unban", h.AdminHandler.UnbanUser)
	adminAPI.POST("/users/:id/revoke-sessions", h.AdminHandler.RevokeUserSessions)
	adminAPI.PUT("/users/:id/role", h.AdminHandler.UpdateUserRole)
	adminAPI.DELETE("/users/:id", h.AdminHandler.DeleteUser)
}

func registerAdminAccountRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/accounts", h.AccountHandler.ListAccounts)
	adminAPI.GET("/accounts/limits", h.AccountHandler.GetAccountLimits)
	adminAPI.GET("/accounts/:id", h.AccountHandler.GetAccount)
	adminAPI.POST("/accounts", h.AccountHandler.CreateAccount)
	adminAPI.PUT("/accounts/:id", h.AccountHandler.UpdateAccount)
	adminAPI.PATCH("/accounts/:id/models", h.AccountHandler.PatchAccountModel)
	adminAPI.DELETE("/accounts/:id/models", h.AccountHandler.DeleteAccountModel)
	adminAPI.DELETE("/accounts/:id", h.AccountHandler.DeleteAccount)
	adminAPI.POST("/accounts/:id/start", h.AccountHandler.StartAccount)
	adminAPI.POST("/accounts/:id/stop", h.AccountHandler.StopAccount)
	adminAPI.GET("/accounts/:id/queue", h.AccountHandler.GetAccountQueue)
}

func registerAdminTaskRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.POST("/tasks/submit", h.TaskHandler.SubmitTask)
	adminAPI.POST("/tasks/bulk-delete", h.TaskHandler.DeleteTasks)
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

func registerAdminProviderEndpointRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/provider-endpoints", h.ProviderEndpointHandler.List)
	adminAPI.POST("/provider-endpoints", h.ProviderEndpointHandler.Create)
	adminAPI.PUT("/provider-endpoints/:id", h.ProviderEndpointHandler.Update)
	adminAPI.DELETE("/provider-endpoints/:id", h.ProviderEndpointHandler.Delete)
	adminAPI.POST("/provider-endpoints/detect-models", h.ProviderEndpointHandler.DetectModels)
}

func registerAdminLineDraftRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/line-draft-modes", h.LineDraftHandler.List)
	adminAPI.POST("/line-draft-modes", h.LineDraftHandler.Create)
	adminAPI.PUT("/line-draft-modes/:id", h.LineDraftHandler.Update)
	adminAPI.DELETE("/line-draft-modes/:id", h.LineDraftHandler.Delete)
}

func registerAdminFeaturePromptRoutes(adminAPI *gin.RouterGroup, h AdminRouteDeps) {
	adminAPI.GET("/feature-prompts", h.FeaturePromptHandler.List)
	adminAPI.GET("/feature-prompts/:slug", h.FeaturePromptHandler.Get)
	adminAPI.PUT("/feature-prompts/:slug", h.FeaturePromptHandler.Save)
}
