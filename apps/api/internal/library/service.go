package library

import (
	"ideaboard/api/internal/library/infrastructure/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service is the postgres WorkspaceLibraryStore (canonical
// implementation in infrastructure/postgres, one file per aggregate).
// Repository methods take context.Context first and use the
// dbWithContext pattern. It must never be replaced by the test-only
// InMemoryWorkspaceLibraryStore in production wiring.
type Service = postgres.Service

// errNotConfigured reports a Service built without a database pool
// (canonical sentinel in infrastructure/postgres).
var errNotConfigured = postgres.ErrNotConfigured

// NewService builds a postgres library Service over pool.
func NewService(db *pgxpool.Pool) *Service {
	return postgres.NewService(db)
}
