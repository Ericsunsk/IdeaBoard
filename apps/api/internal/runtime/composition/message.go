package composition

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgCallGatewayAPI = "上游图像服务未配置，请联系管理员" // CallGatewayAPI @ workspace_gateway_adapter.go
)
