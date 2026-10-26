package workspacefacts

import (
	"errors"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

// Shared facts must see uncommitted caller writes without publishing them or
// owning the transaction lifecycle. A pool-bound lookup would fail this test.
func TestFactsStayInCallerTransaction(t *testing.T) {
	pool := testdb.Open(t)
	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	var organizationID, workspaceID int64
	if err := tx.QueryRow(ctx, `INSERT INTO organizations (public_id, name)
VALUES ('org_facts_transaction', 'Facts organization') RETURNING internal_id`).Scan(&organizationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO workspaces (public_id, organization_id, name)
VALUES ('ws_facts_transaction', $1, 'Facts workspace') RETURNING internal_id`, organizationID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}

	q := dbsqlc.New(tx)
	if got, err := ResolveInternalID(ctx, q, " ws_facts_transaction "); err != nil || got != workspaceID {
		t.Fatalf("transaction workspace ID = %d, %v; want %d, nil", got, err, workspaceID)
	}
	if closed, err := IsOrganizationClosed(ctx, q, " ws_facts_transaction "); err != nil || closed {
		t.Fatalf("new organization closed = %t, %v; want false, nil", closed, err)
	}
	if _, err := ResolveInternalID(ctx, dbsqlc.New(pool), "ws_facts_transaction"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("uncommitted workspace visible outside transaction: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE organizations SET closed_at = now() WHERE internal_id = $1`, organizationID); err != nil {
		t.Fatal(err)
	}
	if closed, err := IsOrganizationClosed(ctx, q, "ws_facts_transaction"); err != nil || !closed {
		t.Fatalf("transaction organization closed = %t, %v; want true, nil", closed, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveInternalID(ctx, dbsqlc.New(pool), "ws_facts_transaction"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("workspace survived caller rollback: %v", err)
	}
}
