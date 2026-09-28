package application

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgNormalizeQuery        = "搜索关键词不能超过 %d 个字符" // NormalizeQuery @ search.go
	msgDecodeCursor          = "分页游标无效"           // DecodeCursor @ search.go
	msgVisionFailureDetails  = "视觉搜索凭证无效"         // VisionFailureDetails @ vision_billing.go
	msgVisionFailureDetails2 = "视觉搜索权限或配额不可用"     // VisionFailureDetails @ vision_billing.go
	msgVisionFailureDetails3 = "视觉搜索返回格式无效"       // VisionFailureDetails @ vision_billing.go
	msgVisionFailureDetails4 = "视觉搜索暂未配置"         // VisionFailureDetails @ vision_billing.go
	msgVisionFailureDetails5 = "视觉搜索输入无效"         // VisionFailureDetails @ vision_billing.go
	msgVisionFailureDetails6 = "视觉搜索暂不可用"         // VisionFailureDetails @ vision_billing.go
)
