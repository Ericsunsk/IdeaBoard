package httpapi

import "github.com/gin-gonic/gin"

func registerInternalRoutes(r *gin.Engine, deps InternalRouteDeps) {
	if deps.UserHandler != nil {
		r.POST("/internal/users/login-metadata", deps.UserHandler.UpdateLoginMetadata)
	}
}
