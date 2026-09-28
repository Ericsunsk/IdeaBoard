package outbound

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgValidateConfiguredEndpoint = "无效的服务地址" // ValidateConfiguredEndpoint @ validator.go
	msgValidateMediaHTTPURL       = "媒体地址不可用" // ValidateMediaHTTPURL @ validator.go
)
