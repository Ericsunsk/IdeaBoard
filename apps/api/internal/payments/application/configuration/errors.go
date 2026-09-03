package configuration

import (
	"database/sql"
	"errors"
)

// IsNotFound identifies an absent published Stripe configuration without
// exposing database details to application workflows.
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
