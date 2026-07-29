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
	userAPI.GET("/info", h.AuthHandler.GetCurrentUser)
	userAPI.POST("/billing/quotes", h.BillingHandler.Quote)
	userAPI.GET("/billing/records", h.BillingRecords.List)
	userAPI.GET("/credits/wallet", h.CreditHandler.GetCurrentWallet)
	userAPI.GET("/usage", h.UsageHandler.GetCurrent)
	userAPI.POST("/rewards/redeem", h.RewardHandler.Redeem)
	userAPI.GET("/rewards/redemptions", h.RewardHandler.ListRedemptions)

	registerUserTaskRoutes(userAPI, h)
	registerUserBoardRoutes(userAPI, h)
	registerUserAssetRoutes(userAPI, h)
	registerUserUploadRoutes(userAPI, h)
}

func registerUserTaskRoutes(userAPI *gin.RouterGroup, h UserRouteDeps) {
	userAPI.POST("/tasks/submit/readiness", h.TaskHandler.CheckSubmitTaskReadiness)
	userAPI.POST("/tasks/submit", h.TaskHandler.SubmitTask)
	userAPI.GET("/tasks/:task_id/events", h.TaskHandler.StreamUserTask)
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
	userAPI.PUT("/boards/:board_id/members/:member_uid/role", h.BoardHandler.UpdateMemberRole)
	userAPI.DELETE("/boards/:board_id/members/:member_uid", h.BoardHandler.RemoveMember)
	userAPI.GET("/boards/:board_id/share-tokens", h.BoardHandler.ListShareTokens)
	userAPI.POST("/boards/:board_id/share-tokens", h.BoardHandler.CreateShareToken)
	userAPI.POST("/boards/:board_id/share-tokens/:token_id/revoke", h.BoardHandler.RevokeShareToken)
	userAPI.POST("/boards/share-tokens/join", h.BoardHandler.JoinByShareToken)
}

func registerUserAssetRoutes(userAPI *gin.RouterGroup, h UserRouteDeps) {
	userAPI.GET("/line-draft-modes", h.LineDraftHandler.ListEnabled)
}

func registerUserUploadRoutes(userAPI *gin.RouterGroup, h UserRouteDeps) {
	userAPI.POST(RouteUserProfileAvatar, requestBodyLimitMiddleware(uploadRequestBodyLimitBytes), uploadProfileAvatarHandler(h.Uploads))
	userAPI.POST(RouteUserBoardUploads, requestBodyLimitMiddleware(uploadRequestBodyLimitBytes), uploadBoardAssetHandler(h.Uploads))
	userAPI.DELETE(RouteUserBoardUploads, deleteBoardUploadHandler(h.Uploads))
	userAPI.GET(RouteUserDownload, downloadImageHandler())
}
