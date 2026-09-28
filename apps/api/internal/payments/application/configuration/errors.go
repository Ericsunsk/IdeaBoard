package configuration

import (
	"ideaboard/api/internal/platform/database"
)

// IsNotFound identifies an absent published Stripe configuration without
// exposing database details to application workflows.
func IsNotFound(err error) bool {
	return database.IsNotFound(err)
}

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgResolveSecrets             = "已保存的 Stripe API Key 无法读取"        // ResolveSecrets @ service.go
	msgResolveSecrets2            = "已保存的 Stripe Webhook Secret 无法读取" // ResolveSecrets @ service.go
	msgPersistCredentialMaterial  = "Stripe API Key 加密失败"             // PersistCredentialMaterial @ service.go
	msgPersistCredentialMaterial2 = "Stripe Webhook Secret 加密失败"      // PersistCredentialMaterial @ service.go
	msgGatewayFailure             = "Stripe 服务暂不可用"                   // GatewayFailure @ validation.go
)
