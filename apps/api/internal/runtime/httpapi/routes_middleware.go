package httpapi

import (
	"net/http"
	"path"
	"strings"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func registerGlobalMiddleware(r *gin.Engine, deps Dependencies) {
	r.Use(deps.CORSMiddleware)
	// Reject disallowed cross-origin requests before they consume application rate-limit budget.
	r.Use(rateLimitMiddleware(deps.RateLimit))
	r.Use(globalRequestBodyLimitMiddleware())
}

func globalRequestBodyLimitMiddleware() gin.HandlerFunc {
	base := requestBodyLimitMiddleware(globalRequestBodyLimitBytes)
	return func(c *gin.Context) {
		if isUploadRequest(c.Request) {
			c.Next()
			return
		}
		base(c)
	}
}

func isUploadRequest(req *http.Request) bool {
	if req == nil || strings.ToUpper(strings.TrimSpace(req.Method)) != http.MethodPost {
		return false
	}
	requestPath := path.Clean(req.URL.Path)
	if requestPath == "/api/user/profile/avatar" || requestPath == "/api/admin/upload" {
		return true
	}
	if requestPath == "/api/user/search/vision-extract" {
		return true
	}
	return strings.HasPrefix(requestPath, "/api/user/boards/") &&
		strings.HasSuffix(requestPath, "/uploads")
}

func requestBodyLimitMiddleware(limitBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Body == nil || limitBytes <= 0 {
			c.Next()
			return
		}

		method := strings.ToUpper(strings.TrimSpace(c.Request.Method))
		switch method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			// continue
		default:
			c.Next()
			return
		}

		if c.Request.ContentLength > limitBytes {
			response.Error(c, http.StatusRequestEntityTooLarge, "请求体过大")
			c.Abort()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limitBytes)
		c.Next()
	}
}
