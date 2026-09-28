package principal

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgRequireCapability = "权限不足"    // RequireCapability @ principal.go
	msgRequire           = "未登录"     // Require @ principal.go
	msgRequireAdmin      = "需要管理员权限" // RequireAdmin @ principal.go
)
