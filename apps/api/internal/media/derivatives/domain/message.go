package domain

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgValidateInputFacts  = "图片处理输入规格未配置"                    // ValidateInputFacts @ model.go
	msgValidateInputFacts2 = "图片尺寸无效: %dx%d"                  // ValidateInputFacts @ model.go
	msgValidateInputFacts3 = "图片尺寸超过处理输入限制: %dx%d (最大边长 %d)"  // ValidateInputFacts @ model.go
	msgValidateInputFacts4 = "图片总像素超过处理输入限制: %dx%d (最大像素 %d)" // ValidateInputFacts @ model.go
	msgValidateInputFacts5 = "图片体积超过处理输入限制: %d 字节 (最大 %d 字节)" // ValidateInputFacts @ model.go
)
