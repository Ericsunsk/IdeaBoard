package configuration

import (
	"ideaboard/api/internal/platform/database"
)

// IsNotFound identifies an absent published Stripe configuration without
// exposing database details to application workflows.
func IsNotFound(err error) bool {
	return database.IsNotFound(err)
}
