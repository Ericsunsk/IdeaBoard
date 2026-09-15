package httpapi

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func registerGlobalMiddleware(r *gin.Engine, deps Dependencies) {
	// RequestID first so access logs, recovery and downstream handlers
	// share one correlation ID. Recovery stays ahead of CORS/rate-limit
	// so panics there also render the shared error envelope.
	r.Use(RequestIDMiddleware())
	r.Use(AccessLogMiddleware())
	r.Use(RecoveryMiddleware())
	r.Use(SecurityHeadersMiddleware())
	r.Use(deps.CORSMiddleware)
	// Apply the request deadline before rate-limit settings can perform a
	// cold-cache database read.
	r.Use(RequestTimeoutMiddleware(DefaultRequestTimeout))
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
	if req == nil || req.URL == nil || strings.ToUpper(strings.TrimSpace(req.Method)) != http.MethodPost {
		return false
	}
	// URL.Path is used only for route classification here; it is never joined
	// with a filesystem root. Avoid filesystem-style path normalization so
	// traversal-like input cannot be reclassified as an upload route.
	requestPath := req.URL.Path
	// Full paths are derived from the group + route constants so the bypass
	// cannot drift from route registration.
	if requestPath == UserAPIGroupPath+RouteUserProfileAvatar || requestPath == AdminAPIGroupPath+RouteAdminUpload {
		return true
	}
	if requestPath == UserAPIGroupPath+RouteUserVisionExtract {
		return true
	}
	return isBoardUploadPath(requestPath)
}

func isBoardUploadPath(requestPath string) bool {
	// Match the route shape exactly: /api/user/boards/:board_id/uploads.
	// Splitting URL path segments keeps this classifier independent from
	// filesystem path semantics and rejects traversal-like or empty segments.
	segments := strings.Split(requestPath, "/")
	return len(segments) == 6 &&
		segments[1] == "api" &&
		segments[2] == "user" &&
		segments[3] == "boards" &&
		segments[4] != "" && segments[4] != "." && segments[4] != ".." &&
		segments[5] == "uploads"
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
