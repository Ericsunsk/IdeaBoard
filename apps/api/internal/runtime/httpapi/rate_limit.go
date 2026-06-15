package httpapi

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

const (
	defaultRateLimitRPS    = 60
	defaultRateLimitBurst  = 120
	rateLimitMaxTrackedIPs = 10_000
	rateLimitTokenScale    = 1_000_000
)

type rateLimitState struct {
	tokens   atomic.Int64 // available tokens (scaled by 1e6 to avoid float)
	lastTime atomic.Int64 // last refill unix nano
	seen     atomic.Bool
}

var (
	rateLimitStates sync.Map
	rateLimitCount  atomic.Int64
)

func rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			c.Next()
			return
		}

		entry, _ := rateLimitStates.LoadOrStore(ip, &rateLimitState{})
		state := entry.(*rateLimitState)

		now := time.Now().UnixNano()
		if state.seen.CompareAndSwap(false, true) {
			state.tokens.Store(defaultRateLimitBurst * rateLimitTokenScale)
			state.lastTime.Store(now)
		}

		last := state.lastTime.Swap(now)
		elapsed := now - last
		if elapsed < 0 {
			elapsed = 0
		}

		// Refill: add tokens proportional to elapsed time (scaled by 1e6)
		addTokens := elapsed * defaultRateLimitRPS * rateLimitTokenScale / int64(time.Second)
		state.tokens.Add(addTokens)
		if state.tokens.Load() > defaultRateLimitBurst*rateLimitTokenScale {
			state.tokens.Store(defaultRateLimitBurst * rateLimitTokenScale)
		}

		if state.tokens.Add(-rateLimitTokenScale) < 0 {
			response.Error(c, http.StatusTooManyRequests, "请求过于频繁，请稍后重试")
			c.Abort()
			return
		}

		// Track approximate pool size and trim if needed
		if count := rateLimitCount.Add(1); count > rateLimitMaxTrackedIPs*2 {
			rateLimitCount.Store(rateLimitMaxTrackedIPs)
			rateLimitStates.Range(func(key, _ any) bool {
				rateLimitStates.Delete(key)
				return true
			})
		}
		c.Next()
	}
}
