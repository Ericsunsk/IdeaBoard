package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

func TestCatalogPricingAndQuoteLifecycle(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	if _, err := service.Quote(ctx, "ideaboard:not-found", 1); !errors.Is(err, ErrSKUNotFound) {
		t.Fatalf("Quote() missing SKU error = %v, want ErrSKUNotFound", err)
	}

	definition := testBillableDefinition("ai-image:partial-edit", "局部重绘")
	priceV1, err := service.PublishPrice(ctx, PublishPriceInput{Definition: definition, UnitCredits: 25, ActorID: 7})
	if err != nil {
		t.Fatalf("CreatePrice(v1) error = %v", err)
	}
	quoteV1, err := service.Quote(ctx, "AI-IMAGE:PARTIAL-EDIT", 4)
	if err != nil {
		t.Fatalf("Quote(v1) error = %v", err)
	}
	if quoteV1.DisplayName != "局部重绘" || quoteV1.PriceVersionID != priceV1.PriceVersionID || quoteV1.PriceVersion != 1 || quoteV1.UnitCredits != 25 || quoteV1.TotalCredits != 100 {
		t.Fatalf("Quote(v1) = %+v, price = %+v", quoteV1, priceV1)
	}

	renamedDefinition := definition
	renamedDefinition.DisplayName = "AI Image Generation"
	priceV2, err := service.PublishPrice(ctx, PublishPriceInput{Definition: renamedDefinition, UnitCredits: 40, ActorID: 8})
	if err != nil {
		t.Fatalf("CreatePrice(v2) error = %v", err)
	}
	quoteV2, err := service.Quote(ctx, "ai-image:partial-edit", 3)
	if err != nil {
		t.Fatalf("Quote(v2) error = %v", err)
	}
	if priceV2.Version != 2 || quoteV2.PriceVersionID != priceV2.PriceVersionID || quoteV2.DisplayName != "AI Image Generation" || quoteV2.TotalCredits != 120 {
		t.Fatalf("Quote(v2) = %+v, price = %+v", quoteV2, priceV2)
	}
	if total, err := quoteV1.Total(4); err != nil || total != 100 {
		t.Fatalf("old snapshot changed after repricing: total=%d err=%v", total, err)
	}

	if quoteV1.DisplayName != "局部重绘" {
		t.Fatalf("old quote display name changed after repricing: %q", quoteV1.DisplayName)
	}
	if _, err := service.SetAvailability(ctx, "ai-image:partial-edit", false, 10); err != nil {
		t.Fatalf("SetAvailability() error = %v", err)
	}
	if _, err := service.Quote(ctx, "ai-image:partial-edit", 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("Quote() with disabled SKU error = %v", err)
	}

	catalog, err := service.ListCatalogFacts(ctx)
	if err != nil {
		t.Fatalf("ListCatalogFacts() error = %v", err)
	}
	if len(catalog) != 1 || catalog[0].CurrentPrice == nil || catalog[0].CurrentPrice.Version != 2 {
		t.Fatalf("ListCatalogFacts() = %+v", catalog)
	}

	if _, err := service.PublishPrice(ctx, PublishPriceInput{Definition: testBillableDefinition("ai-image:remove-bg", "抠图"), UnitCredits: 0, ActorID: 7}); err != nil {
		t.Fatalf("PublishPrice(free) error = %v", err)
	}
	freeQuote, err := service.Quote(ctx, "ai-image:remove-bg", 99)
	if err != nil || freeQuote.UnitCredits != 0 || freeQuote.TotalCredits != 0 {
		t.Fatalf("Quote(free) = %+v, %v", freeQuote, err)
	}
}

func TestCatalogCommandsPersistAppendOnlyAudit(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	price, err := service.PublishPrice(ctx, PublishPriceInput{Definition: testBillableDefinition("test:audit", "Audit"), UnitCredits: 9, ActorID: 13})
	if err != nil {
		t.Fatalf("CreatePrice() error = %v", err)
	}
	if _, err := service.SetAvailability(ctx, "test:audit", false, 14); err != nil {
		t.Fatalf("SetAvailability() error = %v", err)
	}
	rows, err := db.QueryContext(ctx, `
SELECT action_type, actor_id, reason, COALESCE(price_version_id, 0), metadata
FROM billing_admin_actions WHERE sku = 'test:audit' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct {
		action  string
		actorID int64
		reason  string
		priceID int64
	}{
		{action: "create_price", actorID: 13, reason: "price published", priceID: price.PriceVersionID},
		{action: "set_availability", actorID: 14, reason: "availability updated", priceID: price.PriceVersionID},
	}
	for index, expected := range want {
		if !rows.Next() {
			t.Fatalf("audit row %d missing", index)
		}
		var action, reason string
		var actorID, priceID int64
		var metadata string
		if err := rows.Scan(&action, &actorID, &reason, &priceID, &metadata); err != nil {
			t.Fatal(err)
		}
		if action != expected.action || actorID != expected.actorID || reason != expected.reason || priceID != expected.priceID {
			t.Fatalf("audit row %d = (%q,%d,%q,%d), want %+v", index, action, actorID, reason, priceID, expected)
		}
		if action == "create_price" && (!strings.Contains(metadata, `"old": null`) || !strings.Contains(metadata, `"new"`) || !strings.Contains(metadata, `"outcome": "published"`)) {
			t.Fatalf("publish audit metadata = %s", metadata)
		}
	}
	if rows.Next() {
		t.Fatal("unexpected extra audit row")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, statement := range []string{
		`UPDATE billing_admin_actions SET reason = 'tampered' WHERE sku = 'test:audit'`,
		`DELETE FROM billing_admin_actions WHERE sku = 'test:audit'`,
	} {
		if _, err := db.ExecContext(ctx, statement); err == nil {
			t.Fatalf("statement unexpectedly succeeded: %s", statement)
		}
	}
}

func TestPublishingNewPricePreservesDisabledCatalogState(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	definition := testBillableDefinition("ai-image:partial-edit", "局部重绘")
	if _, err := service.PublishPrice(t.Context(), PublishPriceInput{Definition: definition, UnitCredits: 10, ActorID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetAvailability(t.Context(), string(definition.SKU), false, 1); err != nil {
		t.Fatal(err)
	}
	price, err := service.PublishPrice(t.Context(), PublishPriceInput{Definition: definition, UnitCredits: 20, ActorID: 2})
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.ListCatalogFacts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("ListCatalogFacts()=%+v", items)
	}
	item := items[0]
	if item.Enabled || item.CurrentPrice == nil || item.CurrentPrice.PriceVersionID != price.PriceVersionID || price.Version != 2 {
		t.Fatalf("disabled repricing item=%+v price=%+v", item, price)
	}
	if _, err := service.Quote(t.Context(), string(definition.SKU), 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("disabled feature quote error=%v", err)
	}
}

func TestCatalogAvailabilityTransitionsRequirePriceAndPersistAudit(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	definition := testBillableDefinition("ai-image:partial-edit", "局部重绘")
	registry, err := NewBillableItemRegistry([]BillableItemDefinition{definition})
	if err != nil {
		t.Fatal(err)
	}
	catalog := NewBillableItemCatalog(registry, service)

	if _, err := catalog.SetAvailability(t.Context(), string(definition.SKU), true, 7); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("enable unpriced error=%v, want ErrPricingUnavailable", err)
	}
	if _, err := catalog.PublishPrice(t.Context(), string(definition.SKU), 10, 7); err != nil {
		t.Fatal(err)
	}
	disabled, err := catalog.SetAvailability(t.Context(), string(definition.SKU), false, 8)
	if err != nil || disabled.Status != BillableItemStatusDisabled {
		t.Fatalf("disable=%+v error=%v", disabled, err)
	}
	if _, err := catalog.Quote(t.Context(), string(definition.SKU), 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("disabled quote error=%v, want ErrPricingUnavailable", err)
	}
	unchanged, err := catalog.SetAvailability(t.Context(), string(definition.SKU), false, 9)
	if err != nil || !unchanged.UpdatedAt.Equal(disabled.UpdatedAt) {
		t.Fatalf("unchanged=%+v disabled=%+v error=%v", unchanged, disabled, err)
	}
	active, err := catalog.SetAvailability(t.Context(), string(definition.SKU), true, 10)
	if err != nil || active.Status != BillableItemStatusActive || !active.UpdatedAt.After(disabled.UpdatedAt) {
		t.Fatalf("active=%+v disabled=%+v error=%v", active, disabled, err)
	}

	rows, err := db.QueryContext(t.Context(), `
SELECT actor_id, metadata->>'old', metadata->>'new', metadata->>'outcome'
FROM billing_admin_actions
WHERE sku = $1 AND action_type = 'set_availability'
ORDER BY id`, definition.SKU)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct {
		actorID          int64
		old, new, result string
	}{
		{8, "active", "disabled", "changed"},
		{9, "disabled", "disabled", "unchanged"},
		{10, "disabled", "active", "changed"},
	}
	for index, expected := range want {
		if !rows.Next() {
			t.Fatalf("audit row %d missing", index)
		}
		var actorID int64
		var oldStatus, newStatus, outcome string
		if err := rows.Scan(&actorID, &oldStatus, &newStatus, &outcome); err != nil {
			t.Fatal(err)
		}
		if actorID != expected.actorID || oldStatus != expected.old || newStatus != expected.new || outcome != expected.result {
			t.Fatalf("audit row %d=(%d,%q,%q,%q), want %+v", index, actorID, oldStatus, newStatus, outcome, expected)
		}
	}
}

func TestDatabasePreventsSKURenameReuseAndPriceMutation(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	price, err := service.PublishPrice(ctx, PublishPriceInput{Definition: testBillableDefinition("test:immutable", "Immutable"), UnitCredits: 5, ActorID: 1})
	if err != nil {
		t.Fatalf("CreatePrice() error = %v", err)
	}

	statements := []string{
		`UPDATE billing_skus SET sku = 'test:renamed' WHERE sku = 'test:immutable'`,
		`DELETE FROM billing_skus WHERE sku = 'test:immutable'`,
		`UPDATE billing_price_versions SET unit_credits = 6 WHERE price_version_id = $1`,
		`DELETE FROM billing_price_versions WHERE price_version_id = $1`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement, price.PriceVersionID); err == nil {
			t.Fatalf("statement unexpectedly succeeded: %s", statement)
		}
	}
}

func TestDeleteArchivedRemovesCatalogAggregateAndPreservesAudit(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	price := publishDeleteCandidate(t, service, "test:delete-success")

	result, err := service.DeleteArchived(t.Context(), "test:delete-success", 17)
	if err != nil {
		t.Fatalf("DeleteArchived() error = %v", err)
	}
	if result.SKU != "test:delete-success" {
		t.Fatalf("DeleteArchived() = %+v", result)
	}

	var skuCount, priceCount int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM billing_skus WHERE sku = $1`, result.SKU).Scan(&skuCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM billing_price_versions WHERE sku = $1`, result.SKU).Scan(&priceCount); err != nil {
		t.Fatal(err)
	}
	if skuCount != 0 || priceCount != 0 {
		t.Fatalf("remaining Catalog rows: sku=%d prices=%d", skuCount, priceCount)
	}

	var publishAuditCount, deleteAuditCount int
	var deleteActorID int64
	var deletedPriceCount string
	if err := db.QueryRowContext(t.Context(), `
SELECT
  count(*) FILTER (WHERE action_type = 'create_price' AND price_version_id = $2),
  count(*) FILTER (WHERE action_type = 'delete_archived'),
  COALESCE(max(actor_id) FILTER (WHERE action_type = 'delete_archived'), 0),
  COALESCE(max(metadata->>'price_version_count') FILTER (WHERE action_type = 'delete_archived'), '')
FROM billing_admin_actions
WHERE sku = $1`, result.SKU, price.PriceVersionID).Scan(
		&publishAuditCount, &deleteAuditCount, &deleteActorID, &deletedPriceCount,
	); err != nil {
		t.Fatal(err)
	}
	if publishAuditCount != 1 || deleteAuditCount != 1 || deleteActorID != 17 || deletedPriceCount != "1" {
		t.Fatalf("audit publish=%d delete=%d actor=%d price_count=%q", publishAuditCount, deleteAuditCount, deleteActorID, deletedPriceCount)
	}
}

func TestDeleteArchivedSnapshotConflictRollsBackCompletely(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	price := publishDeleteCandidate(t, service, "test:delete-conflict")
	insertFreeBillingSnapshot(t, db, "snapshot-delete-conflict", "test:delete-conflict", "Delete conflict", price)

	if _, err := service.DeleteArchived(t.Context(), "test:delete-conflict", 18); !errors.Is(err, ErrArchivedItemInUse) {
		t.Fatalf("DeleteArchived() error = %v, want ErrArchivedItemInUse", err)
	}

	var skuCount, priceCount, snapshotCount, deleteAuditCount int
	var currentPriceID int64
	if err := db.QueryRowContext(t.Context(), `
SELECT
  (SELECT count(*) FROM billing_skus WHERE sku = 'test:delete-conflict'),
  (SELECT count(*) FROM billing_price_versions WHERE sku = 'test:delete-conflict'),
  (SELECT count(*) FROM task_billing_snapshots WHERE task_id = 'snapshot-delete-conflict'),
  (SELECT count(*) FROM billing_admin_actions WHERE sku = 'test:delete-conflict' AND action_type = 'delete_archived'),
  (SELECT current_price_version_id FROM billing_skus WHERE sku = 'test:delete-conflict')
`).Scan(&skuCount, &priceCount, &snapshotCount, &deleteAuditCount, &currentPriceID); err != nil {
		t.Fatal(err)
	}
	if skuCount != 1 || priceCount != 1 || snapshotCount != 1 || deleteAuditCount != 0 || currentPriceID != price.PriceVersionID {
		t.Fatalf("rollback state sku=%d price=%d snapshot=%d delete_audit=%d current=%d", skuCount, priceCount, snapshotCount, deleteAuditCount, currentPriceID)
	}
}

func TestDeleteArchivedSerializesConcurrentSnapshotInsertInBothLockOrders(t *testing.T) {
	t.Run("delete lock first", func(t *testing.T) {
		db, pool := testdb.OpenWithPool(t)
		service := NewService(pool)
		price := publishDeleteCandidate(t, service, "test:delete-lock-first")
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		deleteTx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = deleteTx.Rollback(context.Background()) }()
		var deleted string
		if err := deleteTx.QueryRow(ctx, `SELECT delete_archived_billing_item($1, $2)`, "test:delete-lock-first", 21).Scan(&deleted); err != nil {
			t.Fatal(err)
		}

		insertDone := make(chan error, 1)
		go func() {
			_, err := pool.Exec(ctx, freeBillingSnapshotInsertSQL,
				"snapshot-delete-lock-first", "test:delete-lock-first", "Delete lock first",
				price.PriceVersionID, price.Version, price.UnitCredits)
			insertDone <- err
		}()
		assertOperationBlocked(t, insertDone)
		if err := deleteTx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-insertDone; err == nil {
			t.Fatal("concurrent Snapshot insert unexpectedly succeeded after Catalog delete committed")
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM billing_skus WHERE sku = 'test:delete-lock-first'`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("deleted SKU count=%d error=%v", count, err)
		}
	})

	t.Run("snapshot lock first", func(t *testing.T) {
		db, pool := testdb.OpenWithPool(t)
		service := NewService(pool)
		price := publishDeleteCandidate(t, service, "test:snapshot-lock-first")
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		snapshotTx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = snapshotTx.Rollback(context.Background()) }()
		if _, err := snapshotTx.Exec(ctx, freeBillingSnapshotInsertSQL,
			"snapshot-lock-first", "test:snapshot-lock-first", "Snapshot lock first",
			price.PriceVersionID, price.Version, price.UnitCredits); err != nil {
			t.Fatal(err)
		}

		deleteDone := make(chan error, 1)
		go func() {
			_, err := service.DeleteArchived(ctx, "test:snapshot-lock-first", 22)
			deleteDone <- err
		}()
		assertOperationBlocked(t, deleteDone)
		if err := snapshotTx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-deleteDone; !errors.Is(err, ErrArchivedItemInUse) {
			t.Fatalf("DeleteArchived() error = %v, want ErrArchivedItemInUse", err)
		}
		var skuCount, snapshotCount, deleteAuditCount int
		if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM billing_skus WHERE sku = 'test:snapshot-lock-first'),
  (SELECT count(*) FROM task_billing_snapshots WHERE task_id = 'snapshot-lock-first'),
  (SELECT count(*) FROM billing_admin_actions WHERE sku = 'test:snapshot-lock-first' AND action_type = 'delete_archived')
`).Scan(&skuCount, &snapshotCount, &deleteAuditCount); err != nil {
			t.Fatal(err)
		}
		if skuCount != 1 || snapshotCount != 1 || deleteAuditCount != 0 {
			t.Fatalf("serialized state sku=%d snapshot=%d delete_audit=%d", skuCount, snapshotCount, deleteAuditCount)
		}
	})
}

func TestConcurrentRepricingAndQuotesUseOneVersionSnapshot(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()

	definition := testBillableDefinition("test:concurrent", "Concurrent")
	initial, err := service.PublishPrice(ctx, PublishPriceInput{Definition: definition, UnitCredits: 10, ActorID: 1})
	if err != nil {
		t.Fatalf("CreatePrice(initial) error = %v", err)
	}

	prices := []PriceInfo{initial}
	quotes := make([]Quote, 0, 100)
	var priceMu sync.Mutex
	var quoteMu sync.Mutex
	errCh := make(chan error, 120)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		unitCredits := int64(20 + i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			price, err := service.PublishPrice(ctx, PublishPriceInput{Definition: definition, UnitCredits: unitCredits, ActorID: 2})
			if err != nil {
				errCh <- fmt.Errorf("CreatePrice(%d): %w", unitCredits, err)
				return
			}
			priceMu.Lock()
			prices = append(prices, price)
			priceMu.Unlock()
		}()
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			quote, err := service.Quote(ctx, "test:concurrent", 3)
			if err != nil {
				errCh <- fmt.Errorf("Quote(): %w", err)
				return
			}
			quoteMu.Lock()
			quotes = append(quotes, quote)
			quoteMu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	byVersion := make(map[int64]PriceInfo, len(prices))
	for _, price := range prices {
		if _, exists := byVersion[price.Version]; exists {
			t.Fatalf("duplicate price version %d", price.Version)
		}
		byVersion[price.Version] = price
	}
	if len(byVersion) != 11 {
		t.Fatalf("price version count = %d, want 11", len(byVersion))
	}
	for _, quote := range quotes {
		price, ok := byVersion[quote.PriceVersion]
		if !ok || price.PriceVersionID != quote.PriceVersionID || price.UnitCredits != quote.UnitCredits {
			t.Fatalf("mixed quote snapshot = %+v, known price = %+v", quote, price)
		}
		if quote.TotalCredits != quote.UnitCredits*3 {
			t.Fatalf("quote total = %+v", quote)
		}
	}
}

func testBillableDefinition(sku SKU, displayName string) BillableItemDefinition {
	namespace, _, _ := strings.Cut(string(sku), ":")
	return BillableItemDefinition{
		SKU: sku, Namespace: namespace, ProductKind: "test-feature", DisplayName: displayName,
	}
}

const freeBillingSnapshotInsertSQL = `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, sku, display_name, price_version_id,
  price_version, unit_credits, quantity, total_credits, quoted_at
) VALUES ($1, 1, 'test-feature', $2, $3, $4, $5, $6, 1, 0, clock_timestamp())`

func publishDeleteCandidate(t *testing.T, service *Service, sku SKU) PriceInfo {
	t.Helper()
	price, err := service.PublishPrice(t.Context(), PublishPriceInput{
		Definition:  testBillableDefinition(sku, strings.TrimPrefix(string(sku), "test:")),
		UnitCredits: 0,
		ActorID:     7,
	})
	if err != nil {
		t.Fatalf("PublishPrice(%s) error = %v", sku, err)
	}
	return price
}

type billingSnapshotExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertFreeBillingSnapshot(t *testing.T, db billingSnapshotExecer, taskID string, sku SKU, displayName string, price PriceInfo) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), freeBillingSnapshotInsertSQL,
		taskID, sku, displayName, price.PriceVersionID, price.Version, price.UnitCredits); err != nil {
		t.Fatalf("insert free Billing Snapshot: %v", err)
	}
}

func assertOperationBlocked(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("operation completed before competing transaction released its lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}
