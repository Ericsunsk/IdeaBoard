package application

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/safehttp"
)

// Models.dev read module: the single place for the shared allowlist check,
// outbound client, TTL cache posture and degrade semantics behind the
// reference (catalog facts) and brand-logo (provider marks) read seams.
// Results are reference data only, never billing input: billing charges
// solely on the billing SKU UnitCredits.
//
// Degrade contract (both seams, unchanged): invalid input is the only error;
// an allowlist gap degrades without any upstream fetch; upstream failures
// and misses degrade to an explicit empty outcome (reference Found=false
// with an error code, logo embedded default mark with a fallback flag) and
// never block catalog flows; fallbacks are never cached. Wire knobs below
// (URLs, TTLs, timeouts, targets, byte caps) are frozen: callers keep their
// per-seam values, this module only holds them in one place.

// modelsDevHost is the only host the models.dev read seams ever dial. It
// must be present in the runtime egress allowlist: absence fails closed
// (reference allowlist_missing, logo default mark) so the admin knows to
// configure it.
const modelsDevHost = "models.dev"

// modelsDevUserAgent is the outbound identity for every models.dev fetch.
const modelsDevUserAgent = "Mozilla/5.0 (compatible; IdeaBoard/1.0)"

// modelsDevCatalogURL is a var for test override: the models.dev provider
// catalog (per-provider models with cost/limit metadata, no auth).
// type=all is required: the default feed omits specialized model types.
var modelsDevCatalogURL = "https://models.dev/api.json?type=all"

// modelsDevLogoBaseURL is a var for test override: the models.dev logo root
// serving /{provider}.svg (unknown providers answer 200 with the generic
// default mark, so misses degrade without a special case).
var modelsDevLogoBaseURL = "https://models.dev/logos"

// modelsReferenceTTL bounds reuse of the fetched models.dev catalog.
const modelsReferenceTTL = 24 * time.Hour

// brandLogoTTL bounds reuse of one fetched logo: logos are near-static,
// a week keeps egress negligible while pod restarts still refresh.
const brandLogoTTL = 7 * 24 * time.Hour

// detectModelsReferenceTarget names models.dev catalog egress in outbound
// logs/metrics.
const detectModelsReferenceTarget = "models-dev-reference"

// brandLogoTarget names logo egress in outbound logs/metrics, separate from
// the reference catalog fetch.
const brandLogoTarget = "models-dev-brand-logo"

// brandLogoMaxBytes caps one upstream SVG read: provider marks are tiny,
// anything larger is treated as a failure and falls back.
const brandLogoMaxBytes = 256 << 10

// detectOutbound is the shared standard outbound client for detection
// egress and both models.dev read seams: timeout cap, bounded retries and
// Retry-After respect match the gateway migration (#766). Per-call
// Timeout/Target stay explicit so control-plane, Workers AI, community
// catalog, reference catalog and brand logo remain separately observable.
var detectOutbound = safehttp.NewClient(safehttp.Config{})

// modelsDevAllowlisted reports whether models.dev is explicitly in the
// runtime egress allowlist. Entries are hostnames (optionally with
// scheme/port/path); comparison is case-insensitive on the hostname.
func modelsDevAllowlisted(ctx context.Context) bool {
	for _, entry := range safehttp.AllowHosts(ctx) {
		if normalizeAllowlistHost(entry) == modelsDevHost {
			return true
		}
	}
	return false
}

// normalizeAllowlistHost trims one allowlist entry down to its hostname.
func normalizeAllowlistHost(entry string) string {
	value := strings.ToLower(strings.TrimSpace(entry))
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "https://")
	if i := strings.Index(value, "/"); i >= 0 {
		value = value[:i]
	}
	if i := strings.LastIndex(value, ":"); i >= 0 {
		if strings.Count(value, ":") == 1 {
			value = value[:i]
		} else {
			value = strings.Trim(value, "[]")
		}
	}
	return strings.TrimSpace(value)
}

// modelsDevCacheFresh reports whether a cached entry fetched at fetchedAt is
// still within its TTL. Both read seams share this posture: reuse within
// TTL, refetch past it, refresh flag bypasses it.
func modelsDevCacheFresh(fetchedAt time.Time, ttl time.Duration) bool {
	return time.Since(fetchedAt) < ttl
}

// modelsDevGet performs one bounded GET against a models.dev URL through the
// shared outbound client. The allowlist pre-check already gated this host;
// the fetch itself passes only the runtime policy (no auto-appended host)
// so the SSRF posture stays identical to every other standard-client call.
// Timeout/MaxAttempts/Target stay per-seam so wire semantics never change:
// catalog uses 15s with client-default retries, logo uses 10s single-shot.
func modelsDevGet(ctx context.Context, rawURL string, timeout time.Duration, maxAttempts int, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", modelsDevUserAgent)
	return detectOutbound.Do(ctx, req, safehttp.RequestOptions{
		Timeout:     timeout,
		MaxAttempts: maxAttempts,
		AllowHosts:  safehttp.AllowHosts(ctx),
		Target:      target,
	})
}
