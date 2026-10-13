package postgres

// Control audit atomicity (#892): the Billing Catalog production write
// paths join the guard-owned ambient transaction instead of committing
// through independent pool writes outside the outer transaction.
//
// What is proven here, against real PostgreSQL (isolated fixture schema):
//   - PublishPrices on a carrier lands inside the carrier: the SKU row
//     and its domain audit row (billing_admin_actions, written by the
//     SECURITY DEFINER function in the same statement) are readable
//     from the still-open carrier and invisible on the pool, then
//     durable after the carrier commits (an owned commit would already
//     be visible on the pool, so the pair proves no independent pool
//     write bypassed the outer transaction and no double commit ran).
//   - A carrier rollback leaves neither the price fact nor its domain
//     audit row behind.
//   - Joint commit with the Control audit row (control_audit_records
//     via AppendRecordTx on the same carrier): business fact and
//     Control audit become readable together after one commit.
//   - Control audit failure on the same carrier rolls the business
//     fact back with it (no orphan business modification).
//   - SetAvailability and DeleteArchived join the carrier the same
//     way (inside-visible / outside-invisible / commit-durable).
//   - With no carrier the owned self-contained path runs unchanged
//     (immediately visible on the pool; the existing store suite is
//     the wider proof and is not duplicated here).

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/testdb"
)

func TestPublishPricesJoinsAmbientCarrierTx(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewStore(pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)

	definition := testBillableDefinition("test:carrier-publish", "Carrier Publish")
	if _, err := service.PublishPrices(carrier, billing.PublishPricesInput{
		Definition: definition,
		Prices:     map[billing.OutputResolution]int64{billing.OutputResolution1K: 30},
		ActorID:    7,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("PublishPrices() on carrier error = %v", err)
	}

	// Same-transaction visibility: SKU fact and domain audit row must
	// be readable from INSIDE the still-open carrier ...
	var insideSKU, insideAudit int
	if err := tx.QueryRow(carrier, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-publish").Scan(&insideSKU); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count skus inside carrier: %v", err)
	}
	if err := tx.QueryRow(carrier, `SELECT count(*) FROM billing_admin_actions WHERE sku = $1 AND action_type = 'create_price'`, "test:carrier-publish").Scan(&insideAudit); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count domain audit inside carrier: %v", err)
	}
	if insideSKU != 1 || insideAudit != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("inside carrier sku=%d audit=%d, want 1/1", insideSKU, insideAudit)
	}
	// ... and invisible outside it before the carrier commits (an
	// independent pool write would already be visible here).
	var outsideSKU int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-publish").Scan(&outsideSKU); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count skus on pool: %v", err)
	}
	if outsideSKU != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("skus visible on pool before commit = %d, want 0 (write bypassed the carrier)", outsideSKU)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	var afterSKU, afterAudit int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-publish").Scan(&afterSKU); err != nil {
		t.Fatalf("count skus after commit: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_admin_actions WHERE sku = $1 AND action_type = 'create_price'`, "test:carrier-publish").Scan(&afterAudit); err != nil {
		t.Fatalf("count domain audit after commit: %v", err)
	}
	if afterSKU != 1 || afterAudit != 1 {
		t.Fatalf("after commit sku=%d audit=%d, want 1/1", afterSKU, afterAudit)
	}
}

func TestPublishPricesRollsBackWithAmbientCarrier(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewStore(pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)

	definition := testBillableDefinition("test:carrier-rollback", "Carrier Rollback")
	if _, err := service.PublishPrices(carrier, billing.PublishPricesInput{
		Definition: definition,
		Prices:     map[billing.OutputResolution]int64{billing.OutputResolution1K: 30},
		ActorID:    7,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("PublishPrices() on carrier error = %v", err)
	}
	// The business fails after the catalog write: the guard rolls the
	// whole carrier back, catalog write and domain audit included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	var skuCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-rollback").Scan(&skuCount); err != nil {
		t.Fatalf("count skus after rollback: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_admin_actions WHERE sku = $1`, "test:carrier-rollback").Scan(&auditCount); err != nil {
		t.Fatalf("count domain audit after rollback: %v", err)
	}
	if skuCount != 0 || auditCount != 0 {
		t.Fatalf("after rollback sku=%d audit=%d, want 0/0", skuCount, auditCount)
	}
}

func TestPublishPricesCommitsJointlyWithControlAudit(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewStore(pool)
	auditStore := audit.NewStore(pool)
	ctx := context.Background()
	requestID := fmt.Sprintf("req-billing-carrier-%d", time.Now().UnixNano())

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)

	definition := testBillableDefinition("test:carrier-joint", "Carrier Joint")
	if _, err := service.PublishPrices(carrier, billing.PublishPricesInput{
		Definition: definition,
		Prices:     map[billing.OutputResolution]int64{billing.OutputResolution1K: 30},
		ActorID:    7,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("PublishPrices() on carrier error = %v", err)
	}
	if err := auditStore.AppendRecordTx(ctx, tx, audit.RecordParams{
		OperatorUserID: 7,
		OperatorType:   "human_session",
		Operation:      "billing.publish_prices",
		Capability:     "control:billing",
		TargetType:     "billing_sku",
		TargetID:       "test:carrier-joint",
		Outcome:        audit.OutcomeSuccess,
		RequestID:      requestID,
		IdempotencyKey: "billing-carrier-1",
		Route:          "POST /billing/catalog/{sku}/prices",
		Status:         200,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append Control audit in tx: %v", err)
	}
	// Both rows must be readable from INSIDE the still-open carrier
	// (a post-commit dual-write could never satisfy this).
	var insideBiz, insideAudit int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-joint").Scan(&insideBiz); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("read business fact inside tx: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&insideAudit); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("read Control audit inside tx: %v", err)
	}
	if insideBiz != 1 || insideAudit != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("inside tx business=%d audit=%d, want 1/1", insideBiz, insideAudit)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	// Success: business fact and Control audit are both readable.
	var afterBiz, afterAudit int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-joint").Scan(&afterBiz); err != nil {
		t.Fatalf("count business after commit: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&afterAudit); err != nil {
		t.Fatalf("count Control audit after commit: %v", err)
	}
	if afterBiz != 1 || afterAudit != 1 {
		t.Fatalf("after commit business=%d audit=%d, want 1/1", afterBiz, afterAudit)
	}
}

func TestPublishPricesControlAuditFailureRollsBackBusiness(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewStore(pool)
	auditStore := audit.NewStore(pool)
	ctx := context.Background()
	requestID := fmt.Sprintf("req-billing-carrier-fail-%d", time.Now().UnixNano())

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)

	definition := testBillableDefinition("test:carrier-audit-fail", "Carrier Audit Fail")
	if _, err := service.PublishPrices(carrier, billing.PublishPricesInput{
		Definition: definition,
		Prices:     map[billing.OutputResolution]int64{billing.OutputResolution1K: 30},
		ActorID:    7,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("PublishPrices() on carrier error = %v", err)
	}
	// A CHECK-violating Control audit write (empty capability/target):
	// the participant MUST report it so the caller rolls the business
	// write back with it.
	bad := audit.RecordParams{
		OperatorUserID: 7,
		OperatorType:   "human_session",
		Operation:      "billing.publish_prices",
		Capability:     "",
		TargetType:     "",
		TargetID:       "",
		Outcome:        audit.OutcomeSuccess,
		RequestID:      requestID,
		IdempotencyKey: "billing-carrier-fail-1",
		Route:          "POST /billing/catalog/{sku}/prices",
		Status:         200,
	}
	if err := auditStore.AppendRecordTx(ctx, tx, bad); err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("CHECK-violating Control audit write succeeded, want error")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	var skuCount, domainAuditCount, controlAuditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-audit-fail").Scan(&skuCount); err != nil {
		t.Fatalf("count business after audit failure: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_admin_actions WHERE sku = $1`, "test:carrier-audit-fail").Scan(&domainAuditCount); err != nil {
		t.Fatalf("count domain audit after audit failure: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM control_audit_records WHERE request_correlation_id = $1`, requestID).Scan(&controlAuditCount); err != nil {
		t.Fatalf("count Control audit after failure: %v", err)
	}
	if skuCount != 0 || domainAuditCount != 0 || controlAuditCount != 0 {
		t.Fatalf("after audit failure business=%d domain_audit=%d control_audit=%d, want 0/0/0", skuCount, domainAuditCount, controlAuditCount)
	}
}

func TestOtherCatalogWritesJoinAmbientCarrierTx(t *testing.T) {
	t.Run("availability", func(t *testing.T) {
		_, pool := testdb.OpenWithPool(t)
		service := NewStore(pool)
		ctx := context.Background()
		definition := testBillableDefinition("test:carrier-avail", "Carrier Avail")
		if _, err := service.PublishPrice(ctx, PublishPriceInput{Definition: definition, UnitCredits: 10, ActorID: 7}); err != nil {
			t.Fatalf("seed PublishPrice() error = %v", err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin carrier tx: %v", err)
		}
		carrier := dbutil.ContextWithAmbientTx(ctx, tx)
		if _, err := service.SetAvailability(carrier, "test:carrier-avail", false, 8); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("SetAvailability() on carrier error = %v", err)
		}
		var inside bool
		if err := tx.QueryRow(carrier, `SELECT enabled FROM billing_skus WHERE sku = $1`, "test:carrier-avail").Scan(&inside); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("read availability inside carrier: %v", err)
		}
		if inside {
			_ = tx.Rollback(ctx)
			t.Fatal("availability inside carrier still enabled, want disabled")
		}
		var outside bool
		if err := pool.QueryRow(ctx, `SELECT enabled FROM billing_skus WHERE sku = $1`, "test:carrier-avail").Scan(&outside); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("read availability on pool: %v", err)
		}
		if !outside {
			_ = tx.Rollback(ctx)
			t.Fatal("availability visible on pool before commit, want still enabled (carrier bypassed)")
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit carrier tx: %v", err)
		}
		var after bool
		if err := pool.QueryRow(ctx, `SELECT enabled FROM billing_skus WHERE sku = $1`, "test:carrier-avail").Scan(&after); err != nil {
			t.Fatalf("read availability after commit: %v", err)
		}
		if after {
			t.Fatal("availability after commit still enabled, want disabled")
		}
		var auditCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_admin_actions WHERE sku = $1 AND action_type = 'set_availability'`, "test:carrier-avail").Scan(&auditCount); err != nil {
			t.Fatalf("count availability audit after commit: %v", err)
		}
		if auditCount != 1 {
			t.Fatalf("availability audit rows after commit = %d, want 1", auditCount)
		}
	})

	t.Run("delete", func(t *testing.T) {
		_, pool := testdb.OpenWithPool(t)
		service := NewStore(pool)
		ctx := context.Background()
		// Registry-absent SKU: the application gate rejects
		// registry-present SKUs, the store only needs a deletable
		// aggregate with no durable references.
		definition := testBillableDefinition("test:carrier-delete", "Carrier Delete")
		if _, err := service.PublishPrice(ctx, PublishPriceInput{Definition: definition, UnitCredits: 5, ActorID: 7}); err != nil {
			t.Fatalf("seed PublishPrice() error = %v", err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin carrier tx: %v", err)
		}
		carrier := dbutil.ContextWithAmbientTx(ctx, tx)
		if _, err := service.DeleteArchived(carrier, "test:carrier-delete", 9); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("DeleteArchived() on carrier error = %v", err)
		}
		var inside int
		if err := tx.QueryRow(carrier, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-delete").Scan(&inside); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("count skus inside carrier: %v", err)
		}
		if inside != 0 {
			_ = tx.Rollback(ctx)
			t.Fatalf("skus inside carrier = %d, want 0 (delete joined)", inside)
		}
		var outside int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-delete").Scan(&outside); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("count skus on pool: %v", err)
		}
		if outside != 1 {
			_ = tx.Rollback(ctx)
			t.Fatalf("skus on pool before commit = %d, want 1 (carrier bypassed)", outside)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit carrier tx: %v", err)
		}
		var after, deleteAudit int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-delete").Scan(&after); err != nil {
			t.Fatalf("count skus after commit: %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_admin_actions WHERE sku = $1 AND action_type = 'delete_archived'`, "test:carrier-delete").Scan(&deleteAudit); err != nil {
			t.Fatalf("count delete audit after commit: %v", err)
		}
		if after != 0 || deleteAudit != 1 {
			t.Fatalf("after commit sku=%d delete_audit=%d, want 0/1", after, deleteAudit)
		}
	})

	t.Run("owned path without carrier stays self-contained", func(t *testing.T) {
		_, pool := testdb.OpenWithPool(t)
		service := NewStore(pool)
		ctx := context.Background()
		definition := testBillableDefinition("test:carrier-owned", "Carrier Owned")
		if _, err := service.PublishPrice(ctx, PublishPriceInput{Definition: definition, UnitCredits: 11, ActorID: 7}); err != nil {
			t.Fatalf("PublishPrice() without carrier error = %v", err)
		}
		// No ambient transaction: the self-contained write commits
		// on its own (immediately visible, no second commit needed).
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = $1`, "test:carrier-owned").Scan(&count); err != nil {
			t.Fatalf("count skus: %v", err)
		}
		if count != 1 {
			t.Fatalf("owned-path skus = %d, want 1", count)
		}
	})
}
