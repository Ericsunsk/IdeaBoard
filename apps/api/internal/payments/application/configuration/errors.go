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

// ResolveSecrets @ service.go
// ResolveSecrets @ service.go
// PersistCredentialMaterial @ service.go
// PersistCredentialMaterial @ service.go
// GatewayFailure @ validation.go
