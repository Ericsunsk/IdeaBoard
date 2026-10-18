package refunds

import (
	postgres "ideaboard/api/internal/payments/infrastructure/postgres"
)

// Store owns the payment refund aggregate. It embeds the root
// *postgres.Repository so transaction boundaries (direct and committed
// refund transactions, context-bound executors) are shared, while refund
// methods live only here.
type Store struct {
	*postgres.Repository
}

// NewStore wraps a root payments repository with the refund aggregate.
func NewStore(repo *postgres.Repository) *Store {
	return &Store{Repository: repo}
}
