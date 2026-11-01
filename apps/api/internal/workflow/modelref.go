package workflow

import (
	"errors"
	"ideaboard/api/internal/platform/errorrender"
	"strings"
)

// Model reference helpers (execution-side truth over display strings).
//
// All AI image generation rides the sole Worker egress (ai-proxy): there
// is no gateway path inference, no per-provider endpoint branching, and no
// dialect shaping in the API. The snapshot pins the structured provider plus
// bare model (display form "provider/model"); execution re-splits it strictly
// here and sends canonical facts (provider, model, prompt, images,
// upload_url) to the Worker. A bare model without a provider fails closed;
// @path overrides are rejected directly (single Worker endpoint).

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
// the structured provider plus the bare model. It is the execution-side
// truth over display strings pinned in RoutingSnapshots (pool rows no longer
// store combined strings): a missing provider fails closed, whitespace is
// rejected, and @path overrides are rejected directly (single Worker
// endpoint; overrides have no target).
func ParseExecutionModelRef(raw string) (provider, model string, err error) {
	if strings.Contains(raw, "@") {
		return "", "", errors.New(errorrender.MsgSplitGatewayModel)
	}
	provider, model, err = SplitPoolDisplay(raw)
	if err != nil {
		return "", "", err
	}
	return provider, model, nil
}
