package http

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/media/workspacemedia"
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
	// Bound multipart overhead separately from the handler's per-file limit.
	upload := requestBodyLimitMiddleware(workspacemedia.MaxUploadSizeBytes + 1024*1024)
	return func(c *gin.Context) {
		if isUploadRequest(c.Request) {
			upload(c)
			return
		}
		base(c)
	}
}

// isUploadRequest matches only the direct workspace-media upload route.
func isUploadRequest(req *http.Request) bool {
	if req == nil || req.URL == nil || req.Method != http.MethodPost {
		return false
	}
	parts := strings.Split(req.URL.Path, "/")
	return len(parts) == 7 && parts[0] == "" && parts[1] == "api" &&
		parts[2] == "v1" && parts[3] == "workspaces" && parts[4] != "" &&
		parts[4] != "." && parts[4] != ".." && parts[5] == "media-assets" && parts[6] == "direct"
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
