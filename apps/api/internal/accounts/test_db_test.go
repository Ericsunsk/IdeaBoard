package accounts

import (
	"database/sql"
	"testing"

	"api/internal/testdb"
)

func newAccountsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}
