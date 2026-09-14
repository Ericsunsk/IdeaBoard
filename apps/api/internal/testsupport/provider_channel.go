package testsupport

import (
	"context"
	"database/sql"
	"testing"
)

// QueryRower is the minimal database seam for provider endpoint fixtures.
// Both *sql.DB and the test-only database interfaces satisfy it.
type QueryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// CreateProviderTestChannel inserts a minimal openai-compatible provider
// endpoint row with a generation capability for tests and returns its id. It
// is the single source behind the former per-package createRiverTestChannel /
// createAccountTestChannel copies, which differed only in the name prefix.
func CreateProviderTestChannel(t testing.TB, db QueryRower, namePrefix, suffix string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
INSERT INTO provider_endpoints (api_type, protocol, name, base_url, status)
VALUES ('openai-compatible', 'model-api', $1, $2, 1)
RETURNING id`,
		namePrefix+suffix,
		"https://example.com/"+suffix,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create provider test channel failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO provider_endpoint_capabilities (provider_endpoint_id, capability)
VALUES ($1, 'generation')`, id); err != nil {
		t.Fatalf("create provider test channel capability failed: %v", err)
	}
	return id
}
