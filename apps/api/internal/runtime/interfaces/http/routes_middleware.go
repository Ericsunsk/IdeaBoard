package http

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/telemetry"

	"github.com/gin-gonic/gin"
)

func registerGlobalMiddleware(r *gin.Engine, deps Dependencies) {
	// RequestID first so access logs, recovery and downstream handlers
	// share one correlation ID. Recovery stays ahead of CORS/rate-limit
	// so panics there also render the shared error envelope.
	r.Use(RequestIDMiddleware())
	r.Use(telemetry.HTTPMiddleware())
	r.Use(REDMetricsMiddleware(deps.REDMetrics))
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

// isUploadRequest reports whether a request carries upload bytes that must
// bypass the global JSON body limit. The legacy /api/user + /api/admin
// upload bypasses were deleted with the #809 hard cut; the v1 workspace
// upload/media surface is JSON session-based and honors the global limit
// like every other JSON route. No POST path bypasses today.
func isUploadRequest(*http.Request) bool {
	return false
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
			response.Error(c, http.StatusRequestEntityTooLarge, MsgBadRequest2)
			c.Abort()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limitBytes)
		c.Next()
	}
}
