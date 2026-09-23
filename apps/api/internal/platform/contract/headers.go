package contract

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// Common headers (Baseline 5.7). The contract Request-Id is the canonical
// correlation header for v1 surfaces; the existing X-Request-ID transport
// (platform/response RequestIDHeader, set by the global middleware) is
// bridged by the contract middleware, not replaced.
const (
	// RequestIDHeader is the canonical v1 correlation header. It is
	// present on every normal API response and error (Baseline 5.3).
	RequestIDHeader = "Request-Id"
	// RateLimitLimitHeader carries the request quota per window.
	RateLimitLimitHeader = "RateLimit-Limit"
	// RateLimitRemainingHeader carries the remaining quota in the window.
	RateLimitRemainingHeader = "RateLimit-Remaining"
	// RateLimitResetHeader carries seconds until the window resets.
	RateLimitResetHeader = "RateLimit-Reset"
	// RetryAfterHeader carries seconds to wait before retrying (rate
	// limits and temporary failures with a known retry window).
	RetryAfterHeader = "Retry-After"
	// ETagHeader carries the resource version for optimistic updates.
	// Required for board document writes (with If-Match).
	ETagHeader = "ETag"
	// IfMatchHeader carries the expected resource version on updates.
	IfMatchHeader = "If-Match"
)

// RequestID resolves the correlation ID: contract Request-Id first,
// legacy X-Request-ID fallback.
func RequestID(c *gin.Context) string {
	return requestIDOf(c)
}

// SetRateLimit sets the RateLimit-* response headers. resetSeconds is
// seconds until the current window resets.
func SetRateLimit(c *gin.Context, limit, remaining int, resetSeconds int64) {
	c.Header(RateLimitLimitHeader, strconv.Itoa(limit))
	c.Header(RateLimitRemainingHeader, strconv.Itoa(remaining))
	c.Header(RateLimitResetHeader, strconv.FormatInt(resetSeconds, 10))
}

// SetRetryAfterSeconds sets Retry-After in delta-seconds.
func SetRetryAfterSeconds(c *gin.Context, seconds int64) {
	c.Header(RetryAfterHeader, strconv.FormatInt(seconds, 10))
}

// SetETag sets the ETag response header. Empty values are ignored so list
// and envelope responses never emit a meaningless validator.
func SetETag(c *gin.Context, etag string) {
	if etag != "" {
		c.Header(ETagHeader, etag)
	}
}

// IfMatch returns the If-Match request header ("*" or an entity-tag).
// Empty means the caller sent no precondition.
func IfMatch(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetHeader(IfMatchHeader)
}
