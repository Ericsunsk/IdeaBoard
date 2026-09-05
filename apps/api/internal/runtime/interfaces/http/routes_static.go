package httpapi

import "github.com/gin-gonic/gin"

func registerStaticUploadRoutes(r *gin.Engine, deps StaticUploadDeps) {
	uploadsHandler := uploadStaticHandler(deps)
	r.GET("/uploads/*filepath", uploadsHandler)
	r.HEAD("/uploads/*filepath", uploadsHandler)
}
