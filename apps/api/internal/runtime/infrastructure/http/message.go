package httpinfra

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgNewOriginPolicy = "生产环境已配置 APP_CORS_ALLOWED_ORIGINS，但 Gateway 仅支持 same-origin；该 allowlist 将被忽略。" // NewOriginPolicy @ cors.go
)
