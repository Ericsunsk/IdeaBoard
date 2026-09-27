package httpapi

import (
	"context"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"ideaboard/api/internal/platform/applog"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is the wire header for request correlation. Clients may
// supply it; otherwise the middleware mints one. The same value is echoed
// back on the response so callers can correlate client errors with server logs.
// The value mirrors response.RequestIDHeader (kept as an alias so the
// response envelope and middleware cannot drift apart).
const RequestIDHeader = response.RequestIDHeader

type requestIDContextKey struct{}

// RequestIDFromContext returns the request ID bound to ctx, or "" when absent.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDContextKey{}).(string); ok {
		return id
	}
	return ""
}

// RequestIDMiddleware binds a correlation ID to every request: it echoes a
// client-supplied X-Request-ID (trimmed, max 128 chars) or mints a UUIDv4,
// then exposes it via response header, gin context and request context. It
// also seeds the applog request scope so downstream logs carry request_id
// without manual plumbing; principal enrichment happens later in
// ResolvePrincipal (post-auth by middleware order).
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := normalizeRequestID(c.GetHeader(RequestIDHeader))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(RequestIDHeader, requestID)
		c.Header(RequestIDHeader, requestID)
		if c.Request != nil {
			ctx := context.WithValue(c.Request.Context(), requestIDContextKey{}, requestID)
			c.Request = c.Request.WithContext(applog.ContextWithRequestID(ctx, requestID))
		}
		c.Next()
	}
}

func normalizeRequestID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > 128 {
		return ""
	}
	return trimmed
}

// AccessLogMiddleware emits one structured slog line per request. It stays
// intentionally small: latency, status, method, path and request ID. Detailed
// tracing stays out of scope until OpenTelemetry lands.
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		// request_id rides the applog scope seeded by RequestIDMiddleware
		// (first in the global chain), so it is not plumbed manually here.
		logger := applog.FromContext(c.Request.Context())
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
		}
		if status >= http.StatusInternalServerError {
			logger.Error("http request", attrs...)
			return
		}
		logger.Info("http request", attrs...)
	}
}

// RecoveryMiddleware converts panics into the shared error envelope so
// clients never see Gin's default plain-text 500. The stack is logged with
// the request ID for triage; the wire response stays a safe fixed message.
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				applog.FromContext(c.Request.Context()).Error("http panic recovered",
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				if !c.Writer.Written() {
					response.Error(c, http.StatusInternalServerError, "服务器内部错误")
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
