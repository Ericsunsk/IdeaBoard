package dialects

import (
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"strings"

	providerdomain "ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	"ideaboard/api/internal/workflow"
)

// Route is the single routing answer shared by execution and preview:
// adapter dispatch, gateway namespace/path, the rule that produced the path,
// the chat judgment, and the shaping dialect. Resolve builds it in one
// resolve-then-shape step so preview output and the executed route cannot
// diverge.
//
// Routing decisions and codec selection mirror the legacy order exactly:
// operation dispatch first, then the shared chat judgment (it cuts across
// providers), then the Fal media pipeline, then generic OpenAI-compatible
// shaping. Vertex, Cloudflare and custom providers ride generic until vendor
// payload evidence justifies a dedicated file.
type Route struct {
	Adapter   providerdomain.ProviderRuntimeAdapter
	Namespace string
	Path      string
	Source    workflow.GatewayPathSource
	IsChat    bool
	Dialect   Dialect
}

// Resolve routes one provider/model target in full. pathOverride wins over
// every default. customParamsJSON participates in the model-image chat
// judgment (explicit type/protocol/modalities win). Pipeline operations
// (cutout/upscale) reject custom params fail-closed: they never shaped or
// decoded them, so silent acceptance would drop operator intent.
func Resolve(operation workflow.RuntimeOperation, provider, model, pathOverride, customParamsJSON string) (Route, error) {
	adapter, err := adapterForOperation(operation)
	if err != nil {
		return Route{}, err
	}
	if adapter == providerdomain.ProviderRuntimeAdapterCutout || adapter == providerdomain.ProviderRuntimeAdapterUpscale {
		if trimmed := strings.TrimSpace(customParamsJSON); trimmed != "" && trimmed != "{}" {
			return Route{}, fmt.Errorf("pipeline operation %q does not accept custom params", string(operation))
		}
	}
	resolved := workflow.ResolveRoute(provider, model, pathOverride, customParamsJSON)
	return Route{
		Adapter:   adapter,
		Namespace: resolved.Namespace,
		Path:      resolved.Path,
		Source:    resolved.Source,
		IsChat:    resolved.IsChat,
		Dialect:   dialectFromJudgment(resolved.IsChat, provider),
	}, nil
}

// adapterForOperation routes one runtime operation to its execution adapter
// without endpoint configuration. It inlines the former
// providerdomain.AdapterForOperation verbatim: same cases, same error text.
func adapterForOperation(operation workflow.RuntimeOperation) (providerdomain.ProviderRuntimeAdapter, error) {
	switch operation {
	case workflow.RuntimeOperationRender, workflow.RuntimeOperationVariations, workflow.RuntimeOperationFusion, workflow.RuntimeOperationGeneration,
		workflow.RuntimeOperationInpaint, workflow.RuntimeOperationOutpaint:
		return providerdomain.ProviderRuntimeAdapterModelImage, nil
	case workflow.RuntimeOperationCutout:
		return providerdomain.ProviderRuntimeAdapterCutout, nil
	case workflow.RuntimeOperationUpscale:
		return providerdomain.ProviderRuntimeAdapterUpscale, nil
	default:
		return 0, fmt.Errorf(errorrender.MsgAdapterForOperation, string(operation))
	}
}

// dialectFromJudgment selects the shaping dialect from a precomputed chat
// judgment. It inlines the former DialectFor priority verbatim: chat first,
// Fal media pipeline second, generic last.
func dialectFromJudgment(isChat bool, provider string) Dialect {
	if isChat {
		return NewChatDialect()
	}
	if workflow.IsFalProvider(provider) {
		return NewFalDialect()
	}
	return NewGenericDialect()
}
