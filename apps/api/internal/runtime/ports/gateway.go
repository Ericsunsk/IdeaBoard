package ports

import (
	"context"
	"fmt"
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
	GatewayHeaderCacheTTL          = "cf-aig-cache-ttl"
	GatewayHeaderCacheKey          = "cf-aig-cache-key"
	GatewayHeaderMaxAttempts       = "cf-aig-max-attempts"
	GatewayHeaderModel             = "cf-aig-model"
	GatewayHeaderProvider          = "cf-aig-provider"
	GatewayHeaderStep              = "cf-aig-step"
	GatewayHeaderCacheStatus       = "cf-aig-cache-status"
	GatewayHeaderLogID             = "cf-aig-log-id"
	GatewayHeaderEventID           = "cf-aig-event-id"
)

// Gateway metadata keys attached to every request. The gateway keeps at most
// five entries; we send two so dashboard filtering stays stable.
const (
	GatewayMetadataTaskID  = "task_id"
	GatewayMetadataFeature = "feature"
)

// Gateway cache TTL bounds enforced by Cloudflare (60 seconds to one month).
const (
	GatewayMinCacheTTLSeconds = 60
	GatewayMaxCacheTTLSeconds = 31 * 24 * 60 * 60
)

// GatewayMaxAttempts caps per-request attempts (Cloudflare allows at most five tries).
const GatewayMaxAttempts = 5

// GatewayEndpointKind selects the Cloudflare gateway surface for one call.
// The unified schema only covers text; images stay provider-native.
type GatewayEndpointKind string

const (
	// GatewayEndpointCompatChat is the OpenAI-compatible surface for
	// supported text/vision routes.
	GatewayEndpointCompatChat GatewayEndpointKind = "compat-chat"
	// GatewayEndpointProviderNative passes the provider-native dialect
	// through untouched (required for image generation).
	GatewayEndpointProviderNative GatewayEndpointKind = "provider-native"
)

// GatewayEndpoint is one resolved gateway surface.
type GatewayEndpoint struct {
	Kind GatewayEndpointKind
	// Provider and Path serve provider-native calls only, e.g. Provider
	// "openai", Path "v1/images/generations".
	Provider string
	Path     string
}

// Validate rejects unknown surfaces and incomplete native endpoints.
func (e GatewayEndpoint) Validate() error {
	switch e.Kind {
	case GatewayEndpointCompatChat:
		return nil
	case GatewayEndpointProviderNative:
		if strings.TrimSpace(e.Provider) == "" || strings.TrimSpace(e.Path) == "" {
			return fmt.Errorf("provider-native gateway endpoint requires provider and path")
		}
		return nil
	default:
		return fmt.Errorf("unknown gateway endpoint kind %q", string(e.Kind))
	}
}

// RequestPath renders the URL path appended to the gateway base URL.
func (e GatewayEndpoint) RequestPath() string {
	if e.Kind == GatewayEndpointProviderNative {
		return "/" + strings.Trim(strings.TrimSpace(e.Provider), "/") +
			"/" + strings.Trim(strings.TrimSpace(e.Path), "/")
	}
	return "/compat/chat/completions"
}

// GatewayCacheMode selects per-request caching. The gateway-global switch
// stays off; every cached request is an explicit opt-in.
type GatewayCacheMode string

const (
	// GatewayCacheInherit sends no cache headers.
	GatewayCacheInherit GatewayCacheMode = ""
	// GatewayCacheSkip bypasses cache for this request (images, probes).
	GatewayCacheSkip GatewayCacheMode = "skip"
	// GatewayCacheTTL opts the request into caching (deterministic text only).
	GatewayCacheTTL GatewayCacheMode = "ttl"
)

// GatewayCachePolicy is the per-request cache contract.
type GatewayCachePolicy struct {
	Mode GatewayCacheMode
	// TTLSeconds applies to TTL mode only (60 seconds to one month).
	TTLSeconds int
	// Key overrides the default exact-match key. Requests sharing a key
	// share cached responses — only group equivalent requests.
	Key string
}

// Validate rejects incoherent policies fail-fast.
func (p GatewayCachePolicy) Validate() error {
	switch p.Mode {
	case GatewayCacheInherit:
		if p.TTLSeconds != 0 || strings.TrimSpace(p.Key) != "" {
			return fmt.Errorf("gateway cache ttl/key require ttl mode")
		}
		return nil
	case GatewayCacheSkip:
		if p.TTLSeconds != 0 || strings.TrimSpace(p.Key) != "" {
			return fmt.Errorf("gateway cache skip accepts no ttl/key")
		}
		return nil
	case GatewayCacheTTL:
		if p.TTLSeconds != 0 && (p.TTLSeconds < GatewayMinCacheTTLSeconds || p.TTLSeconds > GatewayMaxCacheTTLSeconds) {
			return fmt.Errorf("gateway cache ttl %d out of range [%d, %d]", p.TTLSeconds, GatewayMinCacheTTLSeconds, GatewayMaxCacheTTLSeconds)
		}
		return nil
	default:
		return fmt.Errorf("unknown gateway cache mode %q", string(p.Mode))
	}
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
	Cache       GatewayCachePolicy
	// MaxAttempts overrides gateway retries: 0 keeps the gateway default,
	// 1 forces single-shot (probes). Above the gateway maximum is rejected.
	MaxAttempts int
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
	if err := r.Endpoint.Validate(); err != nil {
		return err
	}
	if err := r.Cache.Validate(); err != nil {
		return err
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
			return "", "", "", fmt.Errorf(msgSplitGatewayModel)
		}
	}
	parts := strings.SplitN(strings.TrimSpace(rest), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" ||
		strings.ContainsAny(rest, " \t\n") {
		return "", "", "", fmt.Errorf(msgSplitGatewayModel2)
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
