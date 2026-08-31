package bootstrap

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// buildCORSMiddleware returns a Gin middleware for CORS handling.
// Production is same-origin only: configured legacy allowlists are ignored so
// stale environment variables cannot reopen the retired cross-origin API.
// Development keeps an explicit allowlist for local tooling and integration tests.
func buildCORSMiddleware(appEnv string, allowedOrigins []string) gin.HandlerFunc {
	policy := newOriginPolicy(appEnv, allowedOrigins)
	return func(c *gin.Context) {
		decision := policy.Decide(c.Request)
		if !decision.Allowed {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		for key, value := range decision.Headers {
			c.Header(key, value)
		}
		if decision.Preflight {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

type originDecision struct {
	Allowed   bool
	Preflight bool
	Headers   map[string]string
}

type originPolicy struct {
	production bool
	allowed    map[string]bool
}

func newOriginPolicy(appEnv string, allowedOrigins []string) originPolicy {
	allowed := buildAllowedOriginSet(allowedOrigins)
	if isProductionRuntime(appEnv) && len(allowed) > 0 {
		slog.Warn("生产环境已配置 APP_CORS_ALLOWED_ORIGINS，但 Gateway 仅支持 same-origin；该 allowlist 将被忽略。")
	}
	return originPolicy{production: isProductionRuntime(appEnv), allowed: allowed}
}

func (p originPolicy) Decide(request *http.Request) originDecision {
	if request == nil || request.Header.Get("Origin") == "" {
		return originDecision{Allowed: true}
	}
	if isSameOriginRequest(request) {
		return originDecision{Allowed: true}
	}
	if p.production {
		return originDecision{}
	}
	if len(p.allowed) == 0 {
		if request.Method == http.MethodOptions {
			return originDecision{}
		}
		return originDecision{Allowed: true}
	}
	origin := normalizeCORSOrigin(request.Header.Get("Origin"))
	if !p.allowed[origin] {
		return originDecision{}
	}
	if request.Method == http.MethodOptions {
		return originDecision{Allowed: true, Preflight: true, Headers: corsResponseHeaders(origin)}
	}
	return originDecision{Allowed: true, Headers: corsResponseHeaders(origin)}
}

func corsResponseHeaders(origin string) map[string]string {
	return map[string]string{
		"Access-Control-Allow-Origin":      origin,
		"Access-Control-Allow-Credentials": "true",
		"Access-Control-Allow-Methods":     "GET, POST, PUT, DELETE, PATCH, OPTIONS",
		"Access-Control-Allow-Headers":     "Content-Type, Authorization, Cookie, X-Requested-With, Idempotency-Key",
		"Vary":                             "Origin",
	}
}

func isSameOriginRequest(request *http.Request) bool {
	if request == nil {
		return false
	}
	origin := normalizeCORSOrigin(request.Header.Get("Origin"))
	if origin == "" || strings.TrimSpace(request.Host) == "" {
		return false
	}
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	} else if forwarded := strings.ToLower(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto"))); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return origin == scheme+"://"+strings.ToLower(strings.TrimSpace(request.Host))
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
