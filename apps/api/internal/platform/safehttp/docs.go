// Package safehttp is the single outbound-egress home: SSRF-safe transport,
// policy bridge, and retrying client.
//
// Layers (one package, one concept):
//   - safehttp.go: SSRF allowlist transport (ValidateURLWithAllowHostsCtx,
//     DoWithAllowHosts, DoRequestWithAllowHosts, DoPublicRequest).
//   - policy.go: admin-driven egress policy bridge (ConfigurePolicyProvider,
//     AllowHosts) plus configured-endpoint validation.
//   - configured.go: operator-trusted endpoints (ConfiguredEndpoint +
//     RequestOptions.Configured): load-time scheme+host validation only,
//     fresh per-request DNS with dial pinning (never pooled, so no verdict
//     is cached), private IPs permitted, request and every redirect hop
//     pinned to the endpoint host. User-influenced URLs stay on the strict
//     allowlist path.
//   - client.go / metrics.go / retry.go: standard retrying outbound client
//     (Client.Do) with timeout cap, bounded retries, Retry-After respect,
//     and the shared RED metrics set.
//
// Callers build one Client with a policy Config and send requests through
// Client.Do, which pins egress through the SSRF allowlist. Retries stay
// bounded by MaxAttempts and by the caller's overall timeout; every retry
// and timeout is emitted as a structured slog record and a Prometheus
// counter so #766 stays observable without waiting for the RED metrics
// set (#765).
//
// Retry policy is deliberately conservative: only transport failures,
// timeouts, 408, 429 and 5xx are retried. 4xx (auth, quota, input) never
// retries. 429/503 answers carrying a Retry-After header wait for the
// advertised delay (capped by MaxRetryAfterWait) instead of backoff.
//
// The former platform/outbound and platform/outboundhttp packages were
// removed after consolidation onto this implementation (no duplicated logic
// remains); new code imports safehttp directly.
package safehttp
