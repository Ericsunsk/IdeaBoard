package workflow

import (
	"errors"
	"ideaboard/api/internal/platform/errorrender"
	"strings"

	"github.com/tidwall/gjson"
)

// Gateway path inference lives here (not in the worker providers adapter and
// not duplicated in frontend previews) so execution routing and route
// previews always agree on one implementation.

// GatewayProviderPath renders the gateway-relative path prefix for one
// provider slug: the clean lowercase slug.
func GatewayProviderPath(provider string) string {
	clean := strings.ToLower(strings.TrimSpace(provider))
	if clean == "" {
		return ""
	}
	return clean
}

// IsFirstPartyGatewayProvider reports whether a provider slug resolves to a
// native gateway namespace (non-custom provider).
func IsFirstPartyGatewayProvider(provider string) bool {
	clean := strings.ToLower(strings.TrimSpace(provider))
	if clean == "" || strings.HasPrefix(clean, "custom-") {
		return false
	}
	return true
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

	cleanProvider := strings.ToLower(strings.TrimSpace(provider))
	if cleanProvider == "openrouter" {
		return "api/v1/chat/completions", GatewayPathSourceOpenRouter
	}
	if cleanProvider == "anthropic" {
		return "v1/messages", GatewayPathSourceAnthropic
	}
	if isChatModel {
		return "v1/chat/completions", GatewayPathSourceChat
	}

	cleanModel := strings.TrimSpace(model)
	isFal := IsFalProvider(cleanProvider)
	if cleanModel != "" {
		if strings.Contains(cleanModel, "/") {
			if isFal {
				return cleanModel, GatewayPathSourceFal
			}
			return cleanModel, GatewayPathSourceOverride
		}
		if isFal {
			return "fal-ai/" + cleanModel, GatewayPathSourceFal
		}
	}

	return "v1/images/generations", GatewayPathSourceGenerations
}

// IsFalProvider reports whether a provider is Fal or a Fal variant.
func IsFalProvider(provider string) bool {
	clean := strings.ToLower(strings.TrimSpace(provider))
	switch clean {
	case "fal", "fal_ai", "fal-ai", "fal.ai":
		return true
	default:
		return strings.HasPrefix(clean, "fal-") || strings.HasPrefix(clean, "fal_")
	}
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

// SplitModelIdentifier splits a "provider/model" identifier; without a slash
// the provider falls back to the supplied default.
func SplitModelIdentifier(rawID, fallbackProvider string) (provider, model string) {
	if rawID = strings.TrimSpace(rawID); strings.Contains(rawID, "/") {
		parts := strings.SplitN(rawID, "/", 2)
		return strings.ToLower(parts[0]), parts[1]
	}
	return strings.ToLower(strings.TrimSpace(fallbackProvider)), rawID
}

// FormatPoolModel renders the canonical display form of one structured
// pool entry: provider + "/" + bare model. Selection sorting, snapshot
// pinning, and Admin display derive this string from the stored
// provider/model columns; it is never stored, and execution re-splits it
// strictly via SplitPoolDisplay/ParseExecutionModelRef.
func FormatPoolModel(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.TrimSpace(model)
}

// SplitPoolDisplay splits a canonical provider/model display string back
// into its structured fields. The provider is lowercased; the bare model
// preserves case and may itself contain nested slashes (openrouter's
// vendor/model). A display without a provider prefix (bare model) is
// rejected: execution must fail closed, never infer a provider.
func SplitPoolDisplay(display string) (provider, model string, err error) {
	trimmed := strings.TrimSpace(display)
	if !strings.Contains(trimmed, "/") {
		return "", "", errors.New(errorrender.MsgSplitGatewayModel2)
	}
	parts := strings.SplitN(trimmed, "/", 2)
	provider = strings.ToLower(strings.TrimSpace(parts[0]))
	model = strings.TrimSpace(parts[1])
	if provider == "" || model == "" || strings.ContainsAny(trimmed, " \t\n") {
		return "", "", errors.New(errorrender.MsgSplitGatewayModel2)
	}
	return provider, model, nil
}

// ParseExecutionModelRef resolves one execution model reference in full:
// the structured provider plus the bare model plus the optional @path
// override. It is the execution-side truth over display strings pinned
// in RoutingSnapshots (pool rows no longer store combined strings):
// mapping @path always wins downstream, a missing provider fails closed,
// and unknown provider slugs stay open (custom-<slug> namespace, never
// vanished). Whitespace is rejected so pasted values fail fast.
func ParseExecutionModelRef(raw string) (provider, model, pathOverride string, err error) {
	rest := raw
	if i := strings.Index(rest, "@"); i >= 0 {
		rest, pathOverride = rest[:i], strings.TrimSpace(rest[i+1:])
		if pathOverride == "" || strings.ContainsAny(pathOverride, " \t\n") {
			return "", "", "", errors.New(errorrender.MsgSplitGatewayModel)
		}
	}
	provider, model, err = SplitPoolDisplay(rest)
	if err != nil {
		return "", "", "", err
	}
	return provider, model, pathOverride, nil
}
