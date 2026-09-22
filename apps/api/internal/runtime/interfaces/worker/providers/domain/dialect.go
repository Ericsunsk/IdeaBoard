package domain

import (
	"fmt"

	"ideaboard/api/internal/workflow"
)

// ProviderRuntimeAdapter identifies the execution flow adapter for a runtime operation.
type ProviderRuntimeAdapter int

const (
	ProviderRuntimeAdapterModelImage ProviderRuntimeAdapter = iota
	ProviderRuntimeAdapterCutout
	ProviderRuntimeAdapterUpscale
)

// Gateway path inference lives in workflow (single source of truth shared by
// execution routing and route previews); this package keeps only operation
// dispatch.

// AdapterForOperation routes one runtime operation to its execution adapter
// without endpoint configuration.
func AdapterForOperation(operation workflow.RuntimeOperation) (ProviderRuntimeAdapter, error) {
	switch operation {
	case workflow.RuntimeOperationRender, workflow.RuntimeOperationVariations, workflow.RuntimeOperationFusion, workflow.RuntimeOperationGeneration,
		workflow.RuntimeOperationInpaint, workflow.RuntimeOperationOutpaint:
		return ProviderRuntimeAdapterModelImage, nil
	case workflow.RuntimeOperationCutout:
		return ProviderRuntimeAdapterCutout, nil
	case workflow.RuntimeOperationUpscale:
		return ProviderRuntimeAdapterUpscale, nil
	default:
		return 0, fmt.Errorf("网关不支持操作 %q", string(operation))
	}
}
