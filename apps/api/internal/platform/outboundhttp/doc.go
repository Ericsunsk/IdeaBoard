// Package outboundhttp is the standard outbound HTTP client for every
// external call: timeout cap, bounded retries, and Retry-After respect.
//
// Callers build one Client with a policy Config and send requests through
// Client.Do, which pins egress through safehttp (SSRF allowlist) exactly
// like the legacy direct path. Retries stay bounded by MaxAttempts and by
// the caller's overall timeout; every retry and timeout is emitted as a
// structured slog record and a Prometheus counter so #766 stays observable
// without waiting for the RED metrics set (#765).
//
// Retry policy is deliberately conservative: only transport failures,
// timeouts, 408, 429 and 5xx are retried. 4xx (auth, quota, input) never
// retries. 429/503 answers carrying a Retry-After header wait for the
// advertised delay (capped by MaxRetryAfterWait) instead of backoff.
package outboundhttp
