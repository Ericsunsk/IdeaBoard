package bootstrap

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// buildCORSMiddleware returns a Gin middleware for CORS handling.
// In production with no explicit allowlist, reject cross-origin browser requests.
// If origins are configured, enforce the same allowlist in every environment.
func buildCORSMiddleware(appEnv string, allowedOrigins []string) gin.HandlerFunc {
	allowed := buildAllowedOriginSet(allowedOrigins)
	hasAllowedOrigins := len(allowed) > 0
	rejectUnconfiguredProductionOrigin := !hasAllowedOrigins && isProductionRuntime(appEnv)
	if rejectUnconfiguredProductionOrigin {
		slog.Warn("生产环境未配置有效 APP_CORS_ALLOWED_ORIGINS，将拒绝跨域请求。请设置该环境变量或在反向代理中处理 CORS。")
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		if !hasAllowedOrigins {
			if rejectUnconfiguredProductionOrigin || c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		if !allowed[normalizeCORSOrigin(origin)] {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Cookie, X-Requested-With")
		c.Header("Vary", "Origin")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func buildAllowedOriginSet(origins []string) map[string]bool {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		normalized := normalizeCORSOrigin(origin)
		if normalized == "" {
			continue
		}
		allowed[normalized] = true
	}
	return allowed
}

func normalizeCORSOrigin(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + strings.ToLower(parsed.Host)
}
