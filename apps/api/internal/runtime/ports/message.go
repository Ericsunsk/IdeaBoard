package ports

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgSplitGatewayModel  = "模型网关路径无效"                     // SplitGatewayModel @ gateway.go
	msgSplitGatewayModel2 = "模型必须是 provider/model 格式且不含空白" // SplitGatewayModel @ gateway.go
	msgError              = "API 返回 %d: %s"                // Error @ provider.go
)
