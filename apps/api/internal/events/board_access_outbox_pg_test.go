package events

// Board access outbox PostgreSQL durability (issue #940). Requires
// API_TEST_DATABASE_DSN; testdb SKIPs gracefully otherwise. Proves the
// pending queue contract against the migrated
// board_access_event_outbox table: idempotent insert, pending ordering,
// delivered flip, attempt backoff, and delivered-only pruning.

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

func TestBoardAccessOutboxPostgresQueue(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	store := NewBoardAccessOutboxStore(pool)
	ctx := context.Background()
	var occurredAt time.Time
	if err := pool.QueryRow(ctx, `SELECT now() - interval '1 minute'`).Scan(&occurredAt); err != nil {
		t.Fatal(err)
	}

	first := NewBoardAccessOutboxRecord("ws_pgtest001", "board_pgtest001", "usr_pgtest001", 2, occurredAt)
	if err := insertBoardAccessRecordForTest(ctx, store, first); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// Idempotent on the deterministic evt_ id.
	if err := insertBoardAccessRecordForTest(ctx, store, first); err != nil {
		t.Fatalf("reinsert: %v", err)
	}
	second := NewBoardAccessOutboxRecord("ws_pgtest001", "board_pgtest001", "usr_pgtest002", 2, occurredAt)
	if err := insertBoardAccessRecordForTest(ctx, store, second); err != nil {
		t.Fatalf("insert second: %v", err)
	}

	pending, err := store.Pending(ctx, 100)
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending=%d, want 2 (idempotent reinsert collapses)", len(pending))
	}
	if pending[0].EventID != first.EventID || pending[1].EventID != second.EventID {
		t.Fatalf("pending order=%q,%q, want insertion order", pending[0].EventID, pending[1].EventID)
	}
	envelope, err := pending[0].AccessChangedEnvelope()
	if err != nil {
		t.Fatalf("pending envelope: %v", err)
	}
	if envelope.ID != first.EventID {
		t.Fatalf("envelope id=%q, want stable %q", envelope.ID, first.EventID)
	}

	if err := store.MarkAttempt(ctx, first.EventID, context.DeadlineExceeded); err != nil {
		t.Fatalf("mark attempt: %v", err)
	}
	pending, err = store.Pending(ctx, 100)
	if err != nil {
		t.Fatalf("pending after attempt: %v", err)
	}
	// Backoff pushes next_attempt_at into the future: the row stays
	// pending but leaves the due set until then.
	if len(pending) != 1 || pending[0].EventID != second.EventID {
		t.Fatalf("pending after backoff=%d, want only the second row due", len(pending))
	}

	if err := store.MarkDelivered(ctx, second.EventID); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	pending, err = store.Pending(ctx, 100)
	if err != nil {
		t.Fatalf("pending after delivered: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending=%d, want 0 (one backed off, one delivered)", len(pending))
	}

	// Prune keeps pending rows and drops only old delivered audit rows.
	if _, err := pool.Exec(ctx, `UPDATE board_access_event_outbox SET delivered_at = now() - interval '8 days' WHERE event_id = $1`, second.EventID); err != nil {
		t.Fatalf("age delivered row: %v", err)
	}
	pruned, err := store.PruneDelivered(ctx, time.Now().UTC().Add(-BoardAccessOutboxRetention))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned=%d, want 1 delivered audit row", pruned)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM board_access_event_outbox`).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining=%d, want the pending row to survive pruning", remaining)
	}
}

func insertBoardAccessRecordForTest(ctx context.Context, store *BoardAccessOutboxStore, record BoardAccessOutboxRecord) error {
	tx, err := store.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := store.InsertTx(ctx, tx, record); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
