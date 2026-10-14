package httpapi

import (
	"context"
	"net/http"
	"time"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// SecurityHeadersMiddleware sets baseline hardening headers for API and
// upload responses. HSTS stays out: TLS terminates at the reverse proxy and
// the API may serve plain HTTP behind it; the proxy owns HSTS.
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	}
}

// DefaultRequestTimeout bounds JSON API handlers via request context. Slow
// downstream calls (DB, Auth service) observe ctx cancellation instead of
// holding workers until the server WriteTimeout. Static upload bytes and
// webhooks are exempt: they own their own limits.
const DefaultRequestTimeout = 60 * time.Second

// RequestTimeoutMiddleware applies a context deadline to non-exempt routes.
// It never force-aborts a running handler (Gin handlers are not goroutine
// safe to interrupt); it cancels the context and renders 503 when the
// deadline passes before anything was written.
func RequestTimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	return func(c *gin.Context) {
		if isTimeoutExemptRequest(c.Request) {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
			response.Error(c, http.StatusServiceUnavailable, msgRateLimitMiddleware)
			c.Abort()
		}
	}
}

func isTimeoutExemptRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	path := req.URL.Path
	// The legacy /api/user/events SSE stream and /api/user/download
	// exemptions were deleted with the #809 hard cut. The v1 workspace
	// event stream honors the standard request timeout like every other
	// JSON route (clients resume with Last-Event-ID).
	if path == RoutePaymentStripeWebhook {
		return true
	}
	return isUploadRequest(req)
}
