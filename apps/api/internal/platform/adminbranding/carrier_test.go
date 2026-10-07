package adminbranding

// Control audit atomicity (#892): the Brand Logo override production
// write paths (upsert/delete) join the guard-owned ambient transaction
// instead of committing through independent pool writes outside the
// outer transaction.
//
// What is proven here, against real PostgreSQL (isolated fixture schema):
//   - Upsert on a carrier lands inside the carrier: the override row is
//     readable from the still-open carrier (read-your-writes through
//     the same repository) and invisible on the pool, then durable
//     after the carrier commits (an owned commit would already be
//     visible on the pool, so the pair proves no independent pool
//     write bypassed the outer transaction and no double commit ran).
//   - A carrier rollback leaves no override row behind.
//   - Joint commit with the Control audit row (control_audit_records
//     via AppendRecordTx on the same carrier): business fact and
//     Control audit become readable together after one commit.
//   - Control audit failure on the same carrier rolls the business
//     fact back with it (no orphan business modification).
//   - Delete joins the carrier the same way, including the rollback
//     path.
//   - With no carrier the owned self-contained path runs unchanged
//     (immediately visible on the pool).

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/testdb"
)

func TestBrandLogoUpsertJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)

	if _, err := repo.Upsert(carrier, ScopeProviderEndpoint, "carrier-target", "openai"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Upsert() on carrier error = %v", err)
	}
	// Same-transaction visibility: readable from INSIDE the carrier ...
	inside, err := repo.List(carrier)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("List() inside carrier error = %v", err)
	}
	if len(inside) != 1 || inside[0].TargetKey != "carrier-target" || inside[0].LogoKey != "openai" {
		_ = tx.Rollback(ctx)
		t.Fatalf("List() inside carrier = %+v, want the staged override", inside)
	}
	// ... and invisible outside it before the carrier commits (an
	// independent pool write would already be visible here).
	outside, err := repo.List(ctx)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("List() on pool error = %v", err)
	}
	if len(outside) != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("List() on pool before commit = %+v, want empty (write bypassed the carrier)", outside)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	after, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() after commit error = %v", err)
	}
	if len(after) != 1 || after[0].LogoKey != "openai" {
		t.Fatalf("List() after commit = %+v, want the override", after)
	}
}

func TestBrandLogoUpsertRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)

	if _, err := repo.Upsert(carrier, ScopeProviderEndpoint, "carrier-rollback", "openai"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Upsert() on carrier error = %v", err)
	}
	// The business fails after the override write: the guard rolls the
	// whole carrier back, override included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	after, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() after rollback error = %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("List() after rollback = %+v, want empty", after)
	}
}

func TestBrandLogoUpsertCommitsJointlyWithControlAudit(t *testing.T) {
	pool := testdb.Open(t)
	repo := NewRepository(pool)
	auditStore := audit.NewStore(pool)
	ctx := context.Background()
	requestID := fmt.Sprintf("req-brandlogo-carrier-%d", time.Now().UnixNano())

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)

	if _, err := repo.Upsert(carrier, ScopeProviderEndpoint, "carrier-joint", "openai"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Upsert() on carrier error = %v", err)
	}
	if err := auditStore.AppendRecordTx(ctx, tx, audit.RecordParams{
		OperatorUserID: 7,
		OperatorType:   "human_session",
		Operation:      "platform.upsert_brand_logo_override",
		Capability:     "control:platform",
		TargetType:     "brand_logo_override",
		TargetID:       "provider-endpoint:carrier-joint",
		Reason:         "carrier joint commit",
		Outcome:        audit.OutcomeSuccess,
		RequestID:      requestID,
		IdempotencyKey: "brandlogo-carrier-1",
		Route:          "PUT /platform/brand-logo-overrides/{scope}/{target_key}",
		Status:         200,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append Control audit in tx: %v", err)
	}
	// Both rows must be readable from INSIDE the still-open carrier
	// (a post-commit dual-write could never satisfy this).
	inside, err := repo.List(carrier)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("List() inside carrier error = %v", err)
	}
	if len(inside) != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("overrides inside tx = %d, want 1", len(inside))
	}
	var insideAudit int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&insideAudit); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("read Control audit inside tx: %v", err)
	}
	if insideAudit != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("audit rows inside tx = %d, want 1", insideAudit)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	// Success: business fact and Control audit are both readable.
	after, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() after commit error = %v", err)
	}
	if len(after) != 1 || after[0].LogoKey != "openai" {
		t.Fatalf("List() after commit = %+v, want the override", after)
	}
	var afterAudit int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&afterAudit); err != nil {
		t.Fatalf("count Control audit after commit: %v", err)
	}
	if afterAudit != 1 {
		t.Fatalf("audit rows after commit = %d, want 1 (business and audit commit together)", afterAudit)
	}
}

func TestBrandLogoUpsertControlAuditFailureRollsBackBusiness(t *testing.T) {
	pool := testdb.Open(t)
	repo := NewRepository(pool)
	auditStore := audit.NewStore(pool)
	ctx := context.Background()
	requestID := fmt.Sprintf("req-brandlogo-carrier-fail-%d", time.Now().UnixNano())

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)

	if _, err := repo.Upsert(carrier, ScopeProviderEndpoint, "carrier-audit-fail", "openai"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Upsert() on carrier error = %v", err)
	}
	// A CHECK-violating Control audit write (empty capability/target):
	// the participant MUST report it so the caller rolls the business
	// write back with it.
	bad := audit.RecordParams{
		OperatorUserID: 7,
		OperatorType:   "human_session",
		Operation:      "platform.upsert_brand_logo_override",
		Capability:     "",
		TargetType:     "",
		TargetID:       "",
		Reason:         "carrier audit failure",
		Outcome:        audit.OutcomeSuccess,
		RequestID:      requestID,
		IdempotencyKey: "brandlogo-carrier-fail-1",
		Route:          "PUT /platform/brand-logo-overrides/{scope}/{target_key}",
		Status:         200,
	}
	if err := auditStore.AppendRecordTx(ctx, tx, bad); err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("CHECK-violating Control audit write succeeded, want error")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	after, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() after audit failure error = %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("List() after audit failure = %+v, want empty (audit failure rolls the business back)", after)
	}
	var controlAuditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&controlAuditCount); err != nil {
		t.Fatalf("count Control audit after failure: %v", err)
	}
	if controlAuditCount != 0 {
		t.Fatalf("audit rows after failure = %d, want 0", controlAuditCount)
	}
}

func TestBrandLogoDeleteJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	// Owned seed (no carrier): immediately durable.
	if _, err := repo.Upsert(ctx, ScopeProviderEndpoint, "carrier-delete", "openai"); err != nil {
		t.Fatalf("seed Upsert() error = %v", err)
	}

	t.Run("commit removes the row", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin carrier tx: %v", err)
		}
		carrier := dbutil.ContextWithAmbientTx(ctx, tx)
		deleted, err := repo.Delete(carrier, ScopeProviderEndpoint, "carrier-delete")
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("Delete() on carrier error = %v", err)
		}
		if !deleted {
			_ = tx.Rollback(ctx)
			t.Fatal("Delete() on carrier reported false, want true")
		}
		inside, err := repo.List(carrier)
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("List() inside carrier error = %v", err)
		}
		if len(inside) != 0 {
			_ = tx.Rollback(ctx)
			t.Fatalf("List() inside carrier = %+v, want empty (delete joined)", inside)
		}
		outside, err := repo.List(ctx)
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("List() on pool error = %v", err)
		}
		if len(outside) != 1 {
			_ = tx.Rollback(ctx)
			t.Fatalf("List() on pool before commit = %+v, want the row (carrier bypassed)", outside)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit carrier tx: %v", err)
		}
		after, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() after commit error = %v", err)
		}
		if len(after) != 0 {
			t.Fatalf("List() after commit = %+v, want empty", after)
		}
	})

	t.Run("rollback keeps the row", func(t *testing.T) {
		if _, err := repo.Upsert(ctx, ScopeProviderEndpoint, "carrier-delete-rollback", "openai"); err != nil {
			t.Fatalf("seed Upsert() error = %v", err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin carrier tx: %v", err)
		}
		carrier := dbutil.ContextWithAmbientTx(ctx, tx)
		if _, err := repo.Delete(carrier, ScopeProviderEndpoint, "carrier-delete-rollback"); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("Delete() on carrier error = %v", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("rollback carrier tx: %v", err)
		}
		after, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() after rollback error = %v", err)
		}
		found := false
		for _, item := range after {
			if item.TargetKey == "carrier-delete-rollback" {
				found = true
			}
		}
		if !found {
			t.Fatalf("List() after rollback = %+v, want the row back", after)
		}
	})
}
