package domain

// ProviderRuntimeAdapter identifies the execution flow adapter for a runtime
// operation. Operation dispatch lives in the single routing seam
// (dialects.Resolve, shared by execution and preview); this package keeps
// only the adapter identity.
type ProviderRuntimeAdapter int

const (
	ProviderRuntimeAdapterModelImage ProviderRuntimeAdapter = iota
	ProviderRuntimeAdapterCutout
	ProviderRuntimeAdapterUpscale
)
