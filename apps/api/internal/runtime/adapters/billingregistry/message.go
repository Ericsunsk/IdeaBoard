package billingregistry

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgVisionSearchItem  = "视觉搜索"        // VisionSearchItem @ workflow.go
	msgVisionSearchItem2 = "从参考图提取搜索意图。" // VisionSearchItem @ workflow.go
)
