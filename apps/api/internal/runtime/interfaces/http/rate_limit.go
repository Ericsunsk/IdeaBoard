package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const (
	defaultRateLimitRPS           = 60
	defaultRateLimitBurst         = 120
	defaultRateLimitMaxTrackedIPs = 10_000
	rateLimitEntryTTL             = 10 * time.Minute
	rateLimitCleanupEvery         = 1 * time.Minute
	rateLimitSettingsTimeout      = 2 * time.Second
)

type ipRateLimiterStore struct {
	mu         sync.Mutex
	entries    map[string]*rateLimiterEntry
	maxEntries int
	lastClean  time.Time
	now        func() time.Time
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
	policy   rateLimitPolicy
}

type rateLimitPolicy struct {
	limit rate.Limit
	burst int
}

var rateLimitStore = &ipRateLimiterStore{
	entries:    make(map[string]*rateLimiterEntry),
	maxEntries: defaultRateLimitMaxTrackedIPs,
	now:        time.Now,
}

func rateLimitMiddleware(provider settings.RateLimitSettingProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c)
		if ip == "" {
			c.Next()
			return
		}

		policy := resolveRateLimitPolicy(c.Request.Context(), provider)
		if err := c.Request.Context().Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				response.Error(c, http.StatusServiceUnavailable, msgRateLimitMiddleware)
			}
			c.Abort()
			return
		}
		if !rateLimitStore.allow(ip, policy) {
			response.Error(c, http.StatusTooManyRequests, msgRateLimitMiddleware2)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *ipRateLimiterStore) allow(ip string, policy rateLimitPolicy) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.maybeCleanupLocked(now)

	entry, ok := s.entries[ip]
	if ok {
		entry.lastSeen = now
		if entry.policy != policy {
			entry.limiter = rate.NewLimiter(policy.limit, policy.burst)
			entry.policy = policy
		}
		return entry.limiter.AllowN(now, 1)
	}

	if s.maxEntries > 0 && len(s.entries) >= s.maxEntries {
		s.evictOldestLocked(now)
	}

	entry = &rateLimiterEntry{
		limiter:  rate.NewLimiter(policy.limit, policy.burst),
		lastSeen: now,
		policy:   policy,
	}
	s.entries[ip] = entry
	return entry.limiter.AllowN(now, 1)
}

func resolveRateLimitPolicy(ctx context.Context, provider settings.RateLimitSettingProvider) rateLimitPolicy {
	if provider == nil {
		return defaultRateLimitPolicy()
	}
	settingsCtx, cancel := context.WithTimeout(ctx, rateLimitSettingsTimeout)
	defer cancel()
	setting, err := provider.GetRateLimit(settingsCtx)
	if err != nil {
		slog.WarnContext(ctx, "runtime rate limit settings read failed; using defaults", "err", err)
		return defaultRateLimitPolicy()
	}
	policy, ok := rateLimitPolicyFromSetting(setting)
	if !ok {
		slog.WarnContext(ctx, "runtime rate limit settings invalid; using defaults", "window_seconds", setting.WindowSeconds, "max_requests", setting.MaxRequests)
		return defaultRateLimitPolicy()
	}
	return policy
}

func defaultRateLimitPolicy() rateLimitPolicy {
	return rateLimitPolicy{limit: rate.Limit(defaultRateLimitRPS), burst: defaultRateLimitBurst}
}

func rateLimitPolicyFromSetting(setting settings.RateLimitSetting) (rateLimitPolicy, bool) {
	if setting.WindowSeconds <= 0 || setting.MaxRequests <= 0 {
		return rateLimitPolicy{}, false
	}
	return rateLimitPolicy{
		limit: rate.Every(time.Duration(setting.WindowSeconds) * time.Second / time.Duration(setting.MaxRequests)),
		burst: setting.MaxRequests,
	}, true
}

func (s *ipRateLimiterStore) maybeCleanupLocked(now time.Time) {
	if now.Before(s.lastClean.Add(rateLimitCleanupEvery)) {
		return
	}
	s.lastClean = now

	for ip, entry := range s.entries {
		if now.Sub(entry.lastSeen) > rateLimitEntryTTL {
			delete(s.entries, ip)
		}
	}
}

func (s *ipRateLimiterStore) evictOldestLocked(now time.Time) {
	var oldestIP string
	var oldestTime time.Time
	for ip, entry := range s.entries {
		if oldestIP == "" || entry.lastSeen.Before(oldestTime) {
			oldestIP = ip
			oldestTime = entry.lastSeen
		}
	}
	if oldestIP != "" {
		delete(s.entries, oldestIP)
	}
}

func clientIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if ip := c.ClientIP(); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
