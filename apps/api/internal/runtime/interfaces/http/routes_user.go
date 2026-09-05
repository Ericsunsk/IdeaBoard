package httpapi

import (
	"ideaboard/api/internal/identity/auth"

	"github.com/gin-gonic/gin"
)

func registerAuthenticatedUserRoutes(r *gin.Engine, authDeps AuthDeps, h UserRouteDeps) {
	userAPI := r.Group("/api/user")
	userAPI.Use(auth.AuthMiddleware(authDeps.AuthServiceClient, authDeps.UserRepo, authDeps.UserLifecycle))
	registerUserRoutes(userAPI, h)
}

func registerUserRoutes(userAPI *gin.RouterGroup, h UserRouteDeps) {
	mediaAssetHandler := newMediaAssetHandler(h.MediaAssetRegistry, h.MediaStorageService)
	userAPI.GET("/info", h.AuthHandler.GetCurrentUser)
	userAPI.GET("/profile/avatar/content", profileAvatarContentHandler(h.Uploads))
	userAPI.POST("/media-assets/delivery/batch", mediaAssetHandler.MetadataBatch)
	userAPI.GET("/media-assets/:asset_id", mediaAssetHandler.Metadata)
	userAPI.GET("/media-assets/:asset_id/original/content", mediaAssetHandler.Original)
	userAPI.GET("/media-assets/:asset_id/derivatives/:tier/:format/content", mediaAssetHandler.Content)
	userAPI.POST("/billing/quotes", h.BillingHandler.Quote)
	userAPI.GET("/billing/records", h.BillingRecords.List)
	userAPI.GET("/credits/wallet", h.CreditUserHandler.GetCurrentWallet)
	userAPI.GET("/usage", h.UsageHandler.GetCurrent)
	userAPI.POST("/rewards/redeem", h.RewardUserHandler.Redeem)
	userAPI.GET("/rewards/redemptions", h.RewardUserHandler.ListRedemptions)
	userAPI.GET("/payments/purchase-options", h.PaymentUserHandler.GetPurchaseOptions)
	userAPI.POST("/payments/checkout-sessions", h.PaymentUserHandler.CreateCheckoutSession)
	userAPI.GET("/payments/orders/:public_token", h.PaymentUserHandler.GetPaymentOrderReturnStatus)
	userAPI.GET("/payments/pending-orders", h.PaymentUserHandler.ListPendingOrders)
	userAPI.POST("/payments/pending-orders/:public_token/continue", h.PaymentUserHandler.ContinuePendingOrder)
	userAPI.POST("/payments/pending-orders/:public_token/abandon", h.PaymentUserHandler.AbandonPendingOrder)
	userAPI.GET("/payments/recharges", h.PaymentUserHandler.ListRecharges)
	userAPI.GET("/events", h.UserEventStreamHandler.StreamUserEvents)

	registerUserTaskRoutes(userAPI, h)
	registerUserBoardRoutes(userAPI, h)
	registerUserUploadRoutes(userAPI, h)
	if h.VisionHandler != nil {
		userAPI.POST("/search/vision-extract", requestBodyLimitMiddleware(visionRequestBodyLimitBytes), h.VisionHandler.Extract)
	}
	if h.SearchMetricsHandler != nil {
		userAPI.POST("/search/metrics", h.SearchMetricsHandler.RecordImageFailure)
	}
	if h.LibraryHandler != nil {
		userAPI.GET("/library/folders", h.LibraryHandler.ListFolders)
		userAPI.POST("/library/folders", h.LibraryHandler.CreateFolder)
		userAPI.PATCH("/library/folders/:folder_id", h.LibraryHandler.RenameFolder)
		userAPI.DELETE("/library/folders/:folder_id", h.LibraryHandler.DeleteFolder)
		userAPI.GET("/library/assets", h.LibraryHandler.ListAssets)
		userAPI.POST("/library/assets", h.LibraryHandler.SaveAsset)
		userAPI.POST("/library/assets/lookup", h.LibraryHandler.LookupAssets)
		userAPI.POST("/library/assets/:asset_id/materialize", h.LibraryHandler.MaterializeAsset)
		userAPI.PATCH("/library/assets/:asset_id", h.LibraryHandler.MoveAsset)
		userAPI.DELETE("/library/assets/:asset_id", h.LibraryHandler.DeleteAsset)
	}
}

func registerUserTaskRoutes(userAPI *gin.RouterGroup, h UserRouteDeps) {
	userAPI.POST("/tasks/submit/readiness", h.TaskHandler.CheckSubmitTaskReadiness)
	userAPI.POST("/tasks/submit/readiness/matrix", h.TaskHandler.CheckSubmitTaskReadinessMatrix)
	userAPI.POST("/tasks/submit", h.TaskHandler.SubmitTask)
	userAPI.POST("/tasks/:task_id/cancel", h.TaskHandler.CancelUserTask)
	userAPI.GET("/tasks/:task_id", h.TaskHandler.GetUserTask)
	userAPI.GET("/tasks", h.TaskHandler.GetUserTasks)
}

func registerUserBoardRoutes(userAPI *gin.RouterGroup, h UserRouteDeps) {
	userAPI.GET("/boards", h.BoardHandler.ListBoards)
	userAPI.PUT("/boards/:board_id", h.BoardHandler.UpsertBoard)
	userAPI.DELETE("/boards/:board_id", h.BoardHandler.DeleteBoard)
	userAPI.GET("/boards/:board_id/members", h.BoardHandler.ListMembers)
	userAPI.POST("/boards/:board_id/members/invite", h.BoardHandler.InviteMember)
	userAPI.PUT("/boards/:board_id/members/:member_public_id/role", h.BoardHandler.UpdateMemberRole)
	userAPI.DELETE("/boards/:board_id/members/:member_public_id", h.BoardHandler.RemoveMember)
	userAPI.GET("/boards/:board_id/share-tokens", h.BoardHandler.ListShareTokens)
	userAPI.POST("/boards/:board_id/share-tokens", h.BoardHandler.CreateShareToken)
	userAPI.POST("/boards/:board_id/share-tokens/:token_id/revoke", h.BoardHandler.RevokeShareToken)
	userAPI.POST("/boards/share-tokens/join", h.BoardHandler.JoinByShareToken)
}

func registerUserUploadRoutes(userAPI *gin.RouterGroup, h UserRouteDeps) {
	userAPI.POST(RouteUserProfileAvatar, requestBodyLimitMiddleware(uploadRequestBodyLimitBytes), uploadProfileAvatarHandler(h.Uploads))
	userAPI.POST(RouteUserBoardUploads, requestBodyLimitMiddleware(uploadRequestBodyLimitBytes), uploadBoardAssetHandler(h.Uploads))
	userAPI.GET(RouteUserDownload, downloadImageHandler(h.UploadDownloads))
}
