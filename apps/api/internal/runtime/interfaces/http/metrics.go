package httpapi

import (
	"time"

	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"

	"github.com/gin-gonic/gin"
)

// REDMetricsMiddleware records every HTTP request on the RED minimal set:
// per-route latency histogram plus per-route/status request counters. Route
// is the Gin template (c.FullPath); requests matching no route are counted
// under "unmatched" so raw paths never become label values. A nil set keeps
// the middleware a pure pass-through for unit tests.
func REDMetricsMiddleware(set *redmetrics.Set) gin.HandlerFunc {
	return func(c *gin.Context) {
		if set == nil {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		if status < 100 {
			status = 200
		}
		method := ""
		route := redmetrics.UnmatchedRoute
		if c.Request != nil {
			method = c.Request.Method
		}
		if fullPath := c.FullPath(); fullPath != "" {
			route = fullPath
		}
		set.ObserveHTTP(method, route, status, time.Since(start))
	}
}
