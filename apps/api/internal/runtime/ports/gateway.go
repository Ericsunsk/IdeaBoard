package ports

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"net/http"
	"strings"
	"time"
)

// Cloudflare gateway header contract. Names are pinned to the deployed
// documentation; changing them breaks every gateway call.
const (
	GatewayHeaderAuthorization     = "cf-aig-authorization"
	GatewayHeaderMetadata          = "cf-aig-metadata"
	GatewayHeaderCollectLogPayload = "cf-aig-collect-log-payload"
	GatewayHeaderSkipCache         = "cf-aig-skip-cache"
	GatewayHeaderMaxAttempts       = "cf-aig-max-attempts"
	// GatewayHeaderIdempotencyKey carries the execution token upstream as a
	// best-effort idempotency key: upstreams that understand it dedup
	// retried sends, others ignore the header. Local settlement semantics
	// never depend on it (the claim token fences terminal writes).
	GatewayHeaderIdempotencyKey = "Idempotency-Key"
	GatewayHeaderModel          = "cf-aig-model"
	GatewayHeaderProvider       = "cf-aig-provider"
	GatewayHeaderStep           = "cf-aig-step"
	GatewayHeaderCacheStatus    = "cf-aig-cache-status"
	GatewayHeaderLogID          = "cf-aig-log-id"
	GatewayHeaderEventID        = "cf-aig-event-id"
)

// Gateway metadata keys attached to every request. The gateway keeps at most
// five entries; we send two so dashboard filtering stays stable.
const (
	GatewayMetadataTaskID  = "task_id"
	GatewayMetadataFeature = "feature"
)

// GatewayMaxAttempts caps per-request attempts (Cloudflare allows at most five tries).
const GatewayMaxAttempts = 5

// GatewayDefaultTimeout bounds one gateway attempt. It nests inside the
// worker 360s execution budget, which nests inside the River 17min job
// timeout (ADR-0010). Single source for the 300s budget: gateway client
// default and provider execution share it.
const GatewayDefaultTimeout = 300 * time.Second

// GatewayEndpoint is one resolved provider-native gateway surface, e.g.
// Provider "openai", Path "v1/images/generations". The sole AI egress has
// no other surface (no OpenAI-compatible rewrite); chat judgment only
// drives dialect shaping, never endpoint selection.
type GatewayEndpoint struct {
	Provider string
	Path     string
}

// RequestPath renders the URL path appended to the gateway base URL.
func (e GatewayEndpoint) RequestPath() string {
	return "/" + strings.Trim(strings.TrimSpace(e.Provider), "/") +
		"/" + strings.Trim(strings.TrimSpace(e.Path), "/")
}

// GatewayRequest is one call through the sole AI egress.
type GatewayRequest struct {
	Endpoint GatewayEndpoint
	// Model is the requested provider-qualified model id (e.g.
	// "provider/model", taken from the admin mapping, never defaulted in
	// code). Native calls also carry it inside Body; this
	// field feeds receipt attribution when the gateway reports nothing.
	Model string
	// TaskID and Feature travel as gateway log metadata (searchable).
	TaskID  string
	Feature string
	// Body is the already-shaped request payload (dialect layer owns it).
	// GET requests carry no body.
	Body        []byte
	ContentType string
	Method      string
	// MaxAttempts overrides gateway retries: 0 keeps the gateway default,
	// 1 forces single-shot (probes). Above the gateway maximum is rejected.
	MaxAttempts int
	// ExecutionToken is the claim's execution token, forwarded upstream as a
	// best-effort idempotency key (Idempotency-Key header). Empty keeps the
	// previous behavior (no header). Local settlement never depends on
	// upstream dedup: the claim token still fences terminal writes.
	ExecutionToken string
	// Timeout bounds this call; zero keeps the client default.
	Timeout time.Duration
}

// HTTPMethod defaults empty methods to POST.
func (r GatewayRequest) HTTPMethod() string {
	if method := strings.ToUpper(strings.TrimSpace(r.Method)); method != "" {
		return method
	}
	return http.MethodPost
}

// Validate rejects unusable requests before any network I/O.
func (r GatewayRequest) Validate() error {
	if strings.TrimSpace(r.Endpoint.Provider) == "" || strings.TrimSpace(r.Endpoint.Path) == "" {
		return fmt.Errorf("provider-native gateway endpoint requires provider and path")
	}
	if r.HTTPMethod() == http.MethodGet && len(r.Body) != 0 {
		return fmt.Errorf("gateway GET requests carry no body")
	}
	if r.HTTPMethod() != http.MethodGet && len(r.Body) == 0 {
		return fmt.Errorf("gateway request requires a body")
	}
	if r.MaxAttempts < 0 || r.MaxAttempts > GatewayMaxAttempts {
		return fmt.Errorf("gateway max attempts %d out of range [0, %d]", r.MaxAttempts, GatewayMaxAttempts)
	}
	if r.Timeout < 0 {
		return fmt.Errorf("gateway timeout must not be negative")
	}
	return nil
}

// GatewayReceipt is the parsed gateway acknowledgement for one call.
type GatewayReceipt struct {
	Model       string
	Provider    string
	Step        string
	CacheStatus string
	LogID       string
	EventID     string
	// CacheHit is true only on an explicit cache HIT status.
	CacheHit bool
	// StatusCode mirrors the HTTP status for convenience.
	StatusCode int
}

// GatewayResponse is the raw gateway outcome; dialects parse Body.
type GatewayResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Receipt    GatewayReceipt
}

// GatewayClient is the sole AI egress seam. Implementations own transport,
// auth, timeout, and retry headers; dialects own Body shaping and parsing.
type GatewayClient interface {
	Do(ctx context.Context, req GatewayRequest) (GatewayResponse, error)
}

// GatewayClientFactory builds a GatewayClient per execution from current
// configuration. Resolving per execution keeps token rotation effective
// without restarts; implementations fail closed when unconfigured.
type GatewayClientFactory func(ctx context.Context) (GatewayClient, error)

// SplitGatewayModel parses an admin mapping value of the form
// provider/model[@gateway-path]. Providers stay open-ended on purpose:
// hardcoding them would break zero-code supplier onboarding. The optional
// @path overrides the default route and is required where no static route
// exists. Whitespace is rejected so pasted values fail fast.
func SplitGatewayModel(raw string) (provider, model, pathOverride string, err error) {
	rest := raw
	if i := strings.Index(rest, "@"); i >= 0 {
		rest, pathOverride = rest[:i], strings.TrimSpace(rest[i+1:])
		if pathOverride == "" || strings.ContainsAny(pathOverride, " \t\n") {
			return "", "", "", errors.New(errorrender.MsgSplitGatewayModel)
		}
	}
	parts := strings.SplitN(strings.TrimSpace(rest), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" ||
		strings.ContainsAny(rest, " \t\n") {
		return "", "", "", errors.New(errorrender.MsgSplitGatewayModel2)
	}
	return strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1]), pathOverride, nil
}

// ModelRouteOverrideReader resolves catalog-stored route overrides per
// model ID (custom path/params operators set in the model library). The
// mapping @path always wins; this only fills gaps. Implementations fail
// the lookup (never silent defaults); a nil reader disables the lookup.
type ModelRouteOverrideReader interface {
	LookupRouteOverride(ctx context.Context, modelID string) (pathOverride, customParams string, found bool, err error)
}
