package workflow

import (
	"ideaboard/api/internal/platform/errorrender"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// Gateway path inference lives here (not in the worker providers adapter and
// not duplicated in frontend previews) so execution routing and route
// previews always agree on one implementation.

// GatewayProviderPath renders the gateway-relative path prefix for one
// provider slug: the registry's native namespace, or the customer
// custom-provider namespace for slugs without one.
func GatewayProviderPath(provider string) string {
	clean := strings.ToLower(strings.TrimSpace(provider))
	if clean == "" {
		return ""
	}
	if entry, _ := ResolveProvider(clean); entry.GatewayNamespace != "" {
		return entry.GatewayNamespace
	}
	if strings.HasPrefix(clean, "custom-") {
		return clean
	}
	return "custom-" + clean
}

// IsFirstPartyGatewayProvider reports whether a provider slug resolves to a
// native gateway namespace (no custom- prefixing applied).
func IsFirstPartyGatewayProvider(provider string) bool {
	clean := strings.ToLower(strings.TrimSpace(provider))
	if clean == "" {
		return false
	}
	entry, known := ResolveProvider(clean)
	return known && entry.GatewayNamespace != ""
}

// GatewayPathSource names which inference rule produced a gateway path.
// Frontends map sources to display labels; routing itself never branches on them.
type GatewayPathSource string

const (
	GatewayPathSourceOverride    GatewayPathSource = "override"
	GatewayPathSourceOpenRouter  GatewayPathSource = "openrouter"
	GatewayPathSourceAnthropic   GatewayPathSource = "anthropic"
	GatewayPathSourceFal         GatewayPathSource = "fal"
	GatewayPathSourceChat        GatewayPathSource = "chat"
	GatewayPathSourceGenerations GatewayPathSource = "generations"
)

// Label renders the operator-facing display label for one source. It lives
// here (not hand-copied in frontends) so new sources never silently fall
// back to raw slugs in the UI.
func (s GatewayPathSource) Label() string {
	switch s {
	case GatewayPathSourceOverride:
		return errorrender.MsgLabel
	case GatewayPathSourceOpenRouter:
		return errorrender.MsgLabel2
	case GatewayPathSourceAnthropic:
		return errorrender.MsgLabel3
	case GatewayPathSourceFal:
		return errorrender.MsgLabel4
	case GatewayPathSourceChat:
		return errorrender.MsgLabel5
	case GatewayPathSourceGenerations:
		return errorrender.MsgLabel6
	default:
		return string(s)
	}
}

// IsChatModel determines whether a provider/model target expects an OpenAI-compatible
// Chat Completion multimodal payload rather than a dedicated image generation payload.
// It prioritizes explicit custom parameter declarations, followed by vendor and model heuristics.
func IsChatModel(provider, model, customParamsJSON string) bool {
	if trimmed := strings.TrimSpace(customParamsJSON); trimmed != "" && trimmed != "{}" {
		parsed := gjson.Parse(trimmed)
		if parsed.IsObject() {
			if t := strings.ToLower(parsed.Get("type").String()); t != "" {
				if t == "chat" || t == "multimodal" {
					return true
				}
				if t == "image" || t == "generation" || t == "media" {
					return false
				}
			}
			if p := strings.ToLower(parsed.Get("protocol").String()); p != "" {
				if p == "chat" || p == "openai-chat" {
					return true
				}
				if p == "image" || p == "openai-image" {
					return false
				}
			}
			if parsed.Get("modalities").Exists() {
				return true
			}
		}
	}

	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))

	if p == "openrouter" || p == "anthropic" {
		return true
	}

	chatKeywords := []string{
		"chat", "vision", "gemini", "claude", "gpt-4", "gpt-5", "pixtral", "qwen-vl", "llava",
	}
	for _, kw := range chatKeywords {
		if strings.Contains(m, kw) {
			return true
		}
	}

	return false
}

// InferGatewayPathSource resolves the gateway-relative endpoint path and the
// rule that produced it. An explicit path override always wins. The chat
// judgment arrives precomputed from ResolveRoute (customParams-aware); this
// function must not re-derive it, or explicit type=image declarations lose
// to model-name keyword heuristics.
func InferGatewayPathSource(provider, model, pathOverride string, isChatModel bool) (string, GatewayPathSource) {
	if path := strings.Trim(strings.TrimSpace(pathOverride), "/"); path != "" {
		return path, GatewayPathSourceOverride
	}

	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "openrouter" {
		return "api/v1/chat/completions", GatewayPathSourceOpenRouter
	}
	if p == "anthropic" {
		return "v1/messages", GatewayPathSourceAnthropic
	}
	if isChatModel {
		return "v1/chat/completions", GatewayPathSourceChat
	}
	if p == "fal" || p == "fal_ai" {
		return model, GatewayPathSourceFal
	}
	return "v1/images/generations", GatewayPathSourceGenerations
}

// RouteResolution is the single routing answer shared by execution and
// preview: namespace, path, the rule that produced the path, and the chat
// judgment. One seam, no split-brain between "what preview shows" and
// "what execution does".
type RouteResolution struct {
	Namespace string
	Path      string
	Source    GatewayPathSource
	IsChat    bool
}

// ResolveRoute resolves one provider/model target in full. customParamsJSON
// participates in the chat judgment (explicit type/protocol/modalities win);
// pathOverride wins over every default.
func ResolveRoute(provider, model, pathOverride, customParamsJSON string) RouteResolution {
	isChat := IsChatModel(provider, model, customParamsJSON)
	path, source := InferGatewayPathSource(provider, model, pathOverride, isChat)
	return RouteResolution{
		Namespace: GatewayProviderPath(provider),
		Path:      path,
		Source:    source,
		IsChat:    isChat,
	}
}

// GatewayProbeTimeout bounds one gateway availability probe. Connection
// diagnosis (ProbeGatewayConnection) keeps its own longer budget on purpose:
// an operator staring at a dialog tolerates 8s, a detection sweep does not.
const GatewayProbeTimeout = 5 * time.Second

// Reachability is the shared vocabulary for "can we talk to it", distinct
// from "is it configured". Reachable: the upstream answers and looks alive.
// Unavailable: it answers but attests nothing usable (auth rejection or
// unknown-provider signature). Unreachable: no usable answer at all
// (transport failure or unrecognized response); callers decide whether that
// fails the operation or just skips the candidate.
type Reachability int

const (
	ReachabilityUnreachable Reachability = iota
	ReachabilityUnavailable
	ReachabilityReachable
)

// ClassifyGatewayResponse reads one gateway HTTP response through the
// single Cloudflare signal table. 401/403/code-2008 mean "not configured
// here"; 9106/7001/7003 (any spacing) or plain 200 mean "upstream alive".
// Anything else is Unreachable: present but unrecognized. Transport errors
// never reach this function; they are Unreachable by construction at the
// call site.
func ClassifyGatewayResponse(statusCode int, body string) Reachability {
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden ||
		strings.Contains(body, `"code":2008`) || strings.Contains(body, `"code": 2008`) {
		return ReachabilityUnavailable
	}
	if strings.Contains(body, `"code":9106`) || strings.Contains(body, `"code": 9106`) ||
		strings.Contains(body, `"code":7001`) || strings.Contains(body, `"code": 7001`) ||
		strings.Contains(body, `"code":7003`) || strings.Contains(body, `"code": 7003`) ||
		statusCode == http.StatusOK {
		return ReachabilityReachable
	}
	return ReachabilityUnreachable
}

// SplitModelIdentifier splits a "provider/model" identifier; without a slash
// the provider falls back to the supplied default.
func SplitModelIdentifier(rawID, fallbackProvider string) (provider, model string) {
	if rawID = strings.TrimSpace(rawID); strings.Contains(rawID, "/") {
		parts := strings.SplitN(rawID, "/", 2)
		return strings.ToLower(parts[0]), parts[1]
	}
	return strings.ToLower(strings.TrimSpace(fallbackProvider)), rawID
}
