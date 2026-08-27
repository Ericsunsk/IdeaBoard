package testdb

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestCanonicalSchemaSnapshotIsReadable(t *testing.T) {
	path, err := schemaSnapshotPath()
	if err != nil {
		t.Fatalf("resolve canonical schema snapshot path: %v", err)
	}

	snapshot, err := readSchemaSnapshot(path)
	if err != nil {
		t.Fatalf("read canonical schema snapshot: %v", err)
	}
	if !strings.Contains(snapshot, "CREATE TABLE users") {
		t.Fatal("canonical schema snapshot does not define users")
	}
}

func TestReadSchemaSnapshotFailsClosedWhenArtifactIsUnavailable(t *testing.T) {
	_, err := readSchemaSnapshot(t.TempDir() + "/missing-schema.sql")
	if err == nil {
		t.Fatal("readSchemaSnapshot unexpectedly succeeded for a missing artifact")
	}
}

func TestOpenAppliesCanonicalSnapshotAndKeepsSchemasIsolated(t *testing.T) {
	first := Open(t)
	second := Open(t)

	assertRelationExists(t, first, "library_folders")
	assertRelationExists(t, first, "ai_search_operations")

	const isolatedUserID = int64(987654321)
	if _, err := first.ExecContext(t.Context(), `
INSERT INTO users (id, display_name, storage_uid, role, status)
VALUES ($1, 'testdb-isolation', 'I65432', 'user', 1)
`, isolatedUserID); err != nil {
		t.Fatalf("insert fixture into first isolated schema: %v", err)
	}

	var exists bool
	if err := second.QueryRowContext(t.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, isolatedUserID).Scan(&exists); err != nil {
		t.Fatalf("query second isolated schema: %v", err)
	}
	if exists {
		t.Fatal("data from the first isolated schema leaked into the second schema")
	}
}

func TestOpenWithPoolUsesTheSnapshotSchema(t *testing.T) {
	_, pool := OpenWithPool(t)

	var exists bool
	if err := pool.QueryRow(t.Context(), `SELECT to_regclass('users') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("query pooled snapshot schema: %v", err)
	}
	if !exists {
		t.Fatal("pooled test database is not connected to the snapshot schema")
	}
}

func assertRelationExists(t *testing.T, db *sql.DB, relation string) {
	t.Helper()

	var resolved sql.NullString
	if err := db.QueryRowContext(context.Background(), `SELECT to_regclass($1)`, relation).Scan(&resolved); err != nil {
		t.Fatalf("resolve relation %q: %v", relation, err)
	}
	if !resolved.Valid || resolved.String == "" {
		t.Fatalf("canonical schema snapshot did not create relation %q", relation)
	}
}
