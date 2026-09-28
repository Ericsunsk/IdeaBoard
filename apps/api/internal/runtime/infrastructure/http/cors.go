package httpinfra

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// BuildCORSMiddleware returns a Gin middleware for CORS handling.
// Production is same-origin only: configured legacy allowlists are ignored so
// stale environment variables cannot reopen the retired cross-origin API.
// Development keeps an explicit allowlist for local tooling and integration tests.
func BuildCORSMiddleware(appEnv string, allowedOrigins []string) gin.HandlerFunc {
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
	production       bool
	allowed          map[string]bool
	allowlistInvalid bool
}

func newOriginPolicy(appEnv string, allowedOrigins []string) originPolicy {
	allowed, configured, invalid := buildAllowedOriginSet(allowedOrigins)
	if isProductionRuntime(appEnv) && configured {
		slog.Warn(msgNewOriginPolicy)
	}
	return originPolicy{
		production:       isProductionRuntime(appEnv),
		allowed:          allowed,
		allowlistInvalid: invalid,
	}
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
	if p.allowlistInvalid {
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
		if !isAllowedPreflight(request) {
			return originDecision{}
		}
		return originDecision{Allowed: true, Preflight: true, Headers: corsResponseHeaders(origin)}
	}
	return originDecision{Allowed: true, Headers: corsResponseHeaders(origin)}
}

var allowedCORSMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
	http.MethodPatch:   true,
	http.MethodOptions: true,
}

var allowedCORSHeaders = map[string]bool{
	"content-type":     true,
	"authorization":    true,
	"cookie":           true,
	"x-requested-with": true,
	"idempotency-key":  true,
}

func isAllowedPreflight(request *http.Request) bool {
	method := strings.ToUpper(strings.TrimSpace(request.Header.Get("Access-Control-Request-Method")))
	if !allowedCORSMethods[method] || method == http.MethodOptions {
		return false
	}
	for _, rawHeader := range strings.Split(request.Header.Get("Access-Control-Request-Headers"), ",") {
		header := strings.ToLower(strings.TrimSpace(rawHeader))
		if header == "" {
			continue
		}
		if !allowedCORSHeaders[header] {
			return false
		}
	}
	return true
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
	return origin == normalizeCORSOrigin(scheme+"://"+strings.TrimSpace(request.Host))
}

func buildAllowedOriginSet(origins []string) (map[string]bool, bool, bool) {
	allowed := make(map[string]bool, len(origins))
	configured := false
	invalid := false
	for _, origin := range origins {
		if strings.TrimSpace(origin) == "" {
			continue
		}
		configured = true
		normalized := normalizeCORSOrigin(origin)
		if normalized == "" {
			invalid = true
			continue
		}
		allowed[normalized] = true
	}
	return allowed, configured, invalid
}

func normalizeCORSOrigin(raw string) string {
	parsed, ok := parseCORSOrigin(raw)
	if !ok {
		return ""
	}
	scheme := normalizeCORSScheme(parsed.Scheme)
	if scheme == "" {
		return ""
	}
	host := normalizeCORSHost(parsed, scheme)
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func parseCORSOrigin(raw string) (*url.URL, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !isCORSOriginURL(parsed) {
		return nil, false
	}
	return parsed, true
}

func isCORSOriginURL(parsed *url.URL) bool {
	if parsed == nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return false
	}
	return parsed.RawQuery == "" && parsed.Fragment == ""
}

func normalizeCORSScheme(raw string) string {
	scheme := strings.ToLower(raw)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme
}

func normalizeCORSHost(parsed *url.URL, scheme string) string {
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return ""
	}
	port, ok := normalizeCORSPort(parsed.Port(), scheme)
	if !ok {
		return ""
	}
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	if port == "" {
		return hostname
	}
	return hostname + ":" + port
}

func normalizeCORSPort(raw, scheme string) (string, bool) {
	if raw == "" {
		return "", true
	}
	portNumber, err := strconv.Atoi(raw)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", false
	}
	if (scheme == "http" && raw == "80") || (scheme == "https" && raw == "443") {
		return "", true
	}
	return raw, true
}

func isProductionRuntime(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production", "release":
		return true
	default:
		return false
	}
}
