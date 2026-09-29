package adapters

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgTaskCommandMetadata = "AI 生图任务 Idea 预授权" // TaskCommandMetadata @ workspace_credits_adapter.go
)
