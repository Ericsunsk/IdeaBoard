package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
)

func TestCatalogPricingAndQuoteLifecycle(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	created, err := service.CreateSKU(ctx, CreateSKUInput{
		SKU: "ai-image:partial-edit", DisplayName: "局部重绘", Description: "首个计费产品", Enabled: true, Action: testsupport.AdminAction(t, 7, "launch image billing"),
	})
	if err != nil {
		t.Fatalf("CreateSKU() error = %v", err)
	}
	if created.SKU != "ai-image:partial-edit" || created.CurrentPrice != nil {
		t.Fatalf("CreateSKU() = %+v", created)
	}
	if _, err := service.Quote(ctx, "ai-image:partial-edit", 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("Quote() before price error = %v, want ErrPricingUnavailable", err)
	}
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

	priceV2, err := service.PublishPrice(ctx, PublishPriceInput{Definition: definition, UnitCredits: 40, ActorID: 8})
	if err != nil {
		t.Fatalf("CreatePrice(v2) error = %v", err)
	}
	quoteV2, err := service.Quote(ctx, "ai-image:partial-edit", 3)
	if err != nil {
		t.Fatalf("Quote(v2) error = %v", err)
	}
	if priceV2.Version != 2 || quoteV2.PriceVersionID != priceV2.PriceVersionID || quoteV2.TotalCredits != 120 {
		t.Fatalf("Quote(v2) = %+v, price = %+v", quoteV2, priceV2)
	}
	if total, err := quoteV1.Total(4); err != nil || total != 100 {
		t.Fatalf("old snapshot changed after repricing: total=%d err=%v", total, err)
	}

	disabled := false
	newName := "AI Image Generation"
	updated, err := service.UpdateSKU(ctx, UpdateSKUInput{
		SKU: "ai-image:partial-edit", DisplayName: &newName, Enabled: &disabled, Action: testsupport.AdminAction(t, 10, "retire image SKU"),
	})
	if err != nil {
		t.Fatalf("UpdateSKU() error = %v", err)
	}
	if updated.DisplayName != newName || updated.Enabled {
		t.Fatalf("UpdateSKU() = %+v", updated)
	}
	if quoteV1.DisplayName != "局部重绘" {
		t.Fatalf("old quote display name changed after SKU update: %q", quoteV1.DisplayName)
	}
	if _, err := service.Quote(ctx, "ai-image:partial-edit", 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("Quote() with disabled SKU error = %v", err)
	}

	prices, err := service.ListPrices(ctx, ListPricesInput{SKU: "ai-image:partial-edit", Limit: 10})
	if err != nil {
		t.Fatalf("ListPrices() error = %v", err)
	}
	if len(prices.Prices) != 2 || prices.Prices[0].Version != 2 || prices.Prices[1].Version != 1 {
		t.Fatalf("ListPrices() = %+v", prices)
	}
	catalog, err := service.ListCatalog(ctx, ListCatalogInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListCatalog() error = %v", err)
	}
	if len(catalog.Items) != 1 || catalog.Items[0].CurrentPrice == nil || catalog.Items[0].CurrentPrice.Version != 2 {
		t.Fatalf("ListCatalog() = %+v", catalog)
	}

	if _, err := service.PublishPrice(ctx, PublishPriceInput{Definition: testBillableDefinition("ai-image:remove-bg", "抠图"), UnitCredits: 0, ActorID: 7}); err != nil {
		t.Fatalf("PublishPrice(free) error = %v", err)
	}
	freeQuote, err := service.Quote(ctx, "ai-image:remove-bg", 99)
	if err != nil || freeQuote.UnitCredits != 0 || freeQuote.TotalCredits != 0 {
		t.Fatalf("Quote(free) = %+v, %v", freeQuote, err)
	}
}

func TestAdminBillingWritesRequireReasonAndPersistAppendOnlyAudit(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	if _, err := service.CreateSKU(ctx, CreateSKUInput{
		SKU: "test:audit", DisplayName: "Audit", Enabled: true, Action: testsupport.AdminAction(t, 11, "  create audited SKU  "),
	}); err != nil {
		t.Fatalf("CreateSKU() error = %v", err)
	}
	description := "audited description"
	if _, err := service.UpdateSKU(ctx, UpdateSKUInput{
		SKU: "test:audit", Description: &description, Action: testsupport.AdminAction(t, 12, "update audited SKU"),
	}); err != nil {
		t.Fatalf("UpdateSKU() error = %v", err)
	}
	price, err := service.PublishPrice(ctx, PublishPriceInput{Definition: testBillableDefinition("test:audit", "Audit"), UnitCredits: 9, ActorID: 13})
	if err != nil {
		t.Fatalf("CreatePrice() error = %v", err)
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
		{action: "create_sku", actorID: 11, reason: "create audited SKU"},
		{action: "update_sku", actorID: 12, reason: "update audited SKU"},
		{action: "create_price", actorID: 13, reason: "price published", priceID: price.PriceVersionID},
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
		`SELECT create_billing_sku('test:no-reason', 'No reason', '', TRUE, 1, '  ')`,
		`SELECT create_billing_sku('test:tab-reason', 'Tab reason', '', TRUE, 1, E'\t\n')`,
		`SELECT create_billing_sku('test:null-reason', 'Null reason', '', TRUE, 1, NULL)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err == nil {
			t.Fatalf("statement unexpectedly succeeded: %s", statement)
		}
	}
	if _, err := service.CreateSKU(ctx, CreateSKUInput{SKU: "test:missing-reason", DisplayName: "Missing", Enabled: true}); !errors.Is(err, ErrInvalidCatalogEntry) {
		t.Fatalf("CreateSKU(blank reason) error = %v, want ErrInvalidCatalogEntry", err)
	}
}

func TestPublishingNewPricePreservesDisabledCatalogState(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	definition := testBillableDefinition("ai-image:partial-edit", "局部重绘")
	if _, err := service.PublishPrice(t.Context(), PublishPriceInput{Definition: definition, UnitCredits: 10, ActorID: 1}); err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := service.UpdateSKU(t.Context(), UpdateSKUInput{
		SKU: string(definition.SKU), Enabled: &disabled, Action: testsupport.AdminAction(t, 1, "disable feature"),
	}); err != nil {
		t.Fatal(err)
	}
	price, err := service.PublishPrice(t.Context(), PublishPriceInput{Definition: definition, UnitCredits: 20, ActorID: 2})
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.GetSKU(t.Context(), string(definition.SKU))
	if err != nil {
		t.Fatal(err)
	}
	if item.Enabled || item.CurrentPrice == nil || item.CurrentPrice.PriceVersionID != price.PriceVersionID || price.Version != 2 {
		t.Fatalf("disabled repricing item=%+v price=%+v", item, price)
	}
	if _, err := service.Quote(t.Context(), string(definition.SKU), 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("disabled feature quote error=%v", err)
	}
}

func TestGetSKUsReturnsConfiguredItemsAndOmitsMissingSKUs(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	if _, err := service.CreateSKU(ctx, CreateSKUInput{SKU: "test:coverage-a", DisplayName: "Coverage A", Enabled: true, Action: testsupport.AdminAction(t, 1, "test setup")}); err != nil {
		t.Fatalf("CreateSKU(coverage-a) error = %v", err)
	}
	if _, err := service.PublishPrice(ctx, PublishPriceInput{Definition: testBillableDefinition("test:coverage-a", "Coverage A"), UnitCredits: 8, ActorID: 1}); err != nil {
		t.Fatalf("PublishPrice(coverage-a) error = %v", err)
	}
	if _, err := service.CreateSKU(ctx, CreateSKUInput{SKU: "test:coverage-b", DisplayName: "Coverage B", Enabled: false, Action: testsupport.AdminAction(t, 1, "test setup")}); err != nil {
		t.Fatalf("CreateSKU(coverage-b) error = %v", err)
	}

	items, err := service.GetSKUs(ctx, []string{"test:coverage-b", "test:missing", "TEST:COVERAGE-A", "test:coverage-a"})
	if err != nil {
		t.Fatalf("GetSKUs() error = %v", err)
	}
	if len(items) != 2 || items[0].SKU != "test:coverage-a" || items[1].SKU != "test:coverage-b" {
		t.Fatalf("GetSKUs() items = %+v", items)
	}
	if items[0].CurrentPrice == nil || items[0].CurrentPrice.UnitCredits != 8 {
		t.Fatalf("GetSKUs() coverage-a = %+v", items[0])
	}
	if items[1].Enabled || items[1].CurrentPrice != nil {
		t.Fatalf("GetSKUs() coverage-b = %+v", items[1])
	}
}

func TestDatabasePreventsSKURenameReuseAndPriceMutation(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	if _, err := service.CreateSKU(ctx, CreateSKUInput{SKU: "test:immutable", DisplayName: "Immutable", Enabled: true, Action: testsupport.AdminAction(t, 1, "test immutable catalog")}); err != nil {
		t.Fatalf("CreateSKU() error = %v", err)
	}
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

func TestConcurrentRepricingAndQuotesUseOneVersionSnapshot(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()

	if _, err := service.CreateSKU(ctx, CreateSKUInput{SKU: "test:concurrent", DisplayName: "Concurrent", Enabled: true, Action: testsupport.AdminAction(t, 1, "test concurrent pricing")}); err != nil {
		t.Fatalf("CreateSKU() error = %v", err)
	}
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
