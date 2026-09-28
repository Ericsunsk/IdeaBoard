package prompts

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgGet                          = "获取失败"                 // Get @ handler.go
	msgSave                         = "参数错误"                 // Save @ handler.go
	msgSave2                        = "保存失败"                 // Save @ handler.go
	msgFeaturePromptSlugFromRequest = "缺少 slug 参数"           // FeaturePromptSlugFromRequest @ handler.go
	msgFeaturePromptUnsupported     = "该功能不支持 System Prompt" // ErrFeaturePromptUnsupported
)
