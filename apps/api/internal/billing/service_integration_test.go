package billing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"ideaboard/api/internal/testdb"
)

func TestCatalogPricingAndQuoteLifecycle(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	created, err := service.CreateSKU(ctx, CreateSKUInput{
		SKU: "IdeaBoard:Image.Generate", DisplayName: "AI 生图", Description: "首个计费产品", Enabled: true, ActorID: 7,
	})
	if err != nil {
		t.Fatalf("CreateSKU() error = %v", err)
	}
	if created.SKU != "ideaboard:image.generate" || created.CurrentPrice != nil {
		t.Fatalf("CreateSKU() = %+v", created)
	}
	if _, err := service.Quote(ctx, "ideaboard:image.generate", 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("Quote() before price error = %v, want ErrPricingUnavailable", err)
	}
	if _, err := service.Quote(ctx, "ideaboard:not-found", 1); !errors.Is(err, ErrSKUNotFound) {
		t.Fatalf("Quote() missing SKU error = %v, want ErrSKUNotFound", err)
	}

	priceV1, err := service.CreatePrice(ctx, CreatePriceInput{
		SKU: "ideaboard:image.generate", UnitCredits: 25, Enabled: true, ActorID: 7,
	})
	if err != nil {
		t.Fatalf("CreatePrice(v1) error = %v", err)
	}
	quoteV1, err := service.Quote(ctx, "IDEABOARD:IMAGE.GENERATE", 4)
	if err != nil {
		t.Fatalf("Quote(v1) error = %v", err)
	}
	if quoteV1.PriceVersionID != priceV1.PriceVersionID || quoteV1.PriceVersion != 1 || quoteV1.UnitCredits != 25 || quoteV1.TotalCredits != 100 {
		t.Fatalf("Quote(v1) = %+v, price = %+v", quoteV1, priceV1)
	}

	priceV2, err := service.CreatePrice(ctx, CreatePriceInput{
		SKU: "ideaboard:image.generate", UnitCredits: 40, Enabled: true, ActorID: 8,
	})
	if err != nil {
		t.Fatalf("CreatePrice(v2) error = %v", err)
	}
	quoteV2, err := service.Quote(ctx, "ideaboard:image.generate", 3)
	if err != nil {
		t.Fatalf("Quote(v2) error = %v", err)
	}
	if priceV2.Version != 2 || quoteV2.PriceVersionID != priceV2.PriceVersionID || quoteV2.TotalCredits != 120 {
		t.Fatalf("Quote(v2) = %+v, price = %+v", quoteV2, priceV2)
	}
	if total, err := quoteV1.Total(4); err != nil || total != 100 {
		t.Fatalf("old snapshot changed after repricing: total=%d err=%v", total, err)
	}

	if _, err := service.SetPriceEnabled(ctx, SetPriceEnabledInput{
		SKU: "ideaboard:image.generate", Version: 2, Enabled: false, ActorID: 9,
	}); err != nil {
		t.Fatalf("SetPriceEnabled(false) error = %v", err)
	}
	if _, err := service.Quote(ctx, "ideaboard:image.generate", 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("Quote() with disabled current price error = %v", err)
	}
	if _, err := service.SetPriceEnabled(ctx, SetPriceEnabledInput{
		SKU: "ideaboard:image.generate", Version: 2, Enabled: true, ActorID: 9,
	}); err != nil {
		t.Fatalf("SetPriceEnabled(true) error = %v", err)
	}

	disabled := false
	newName := "AI Image Generation"
	updated, err := service.UpdateSKU(ctx, UpdateSKUInput{
		SKU: "ideaboard:image.generate", DisplayName: &newName, Enabled: &disabled, ActorID: 10,
	})
	if err != nil {
		t.Fatalf("UpdateSKU() error = %v", err)
	}
	if updated.DisplayName != newName || updated.Enabled {
		t.Fatalf("UpdateSKU() = %+v", updated)
	}
	if _, err := service.Quote(ctx, "ideaboard:image.generate", 1); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("Quote() with disabled SKU error = %v", err)
	}

	prices, err := service.ListPrices(ctx, ListPricesInput{SKU: "ideaboard:image.generate", Limit: 10})
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

	if _, err := service.CreateSKU(ctx, CreateSKUInput{SKU: "ideaboard:free.preview", DisplayName: "Free Preview", Enabled: true, ActorID: 7}); err != nil {
		t.Fatalf("CreateSKU(free) error = %v", err)
	}
	if _, err := service.CreatePrice(ctx, CreatePriceInput{SKU: "ideaboard:free.preview", UnitCredits: 0, Enabled: true, ActorID: 7}); err != nil {
		t.Fatalf("CreatePrice(free) error = %v", err)
	}
	freeQuote, err := service.Quote(ctx, "ideaboard:free.preview", 99)
	if err != nil || freeQuote.UnitCredits != 0 || freeQuote.TotalCredits != 0 {
		t.Fatalf("Quote(free) = %+v, %v", freeQuote, err)
	}
}

func TestDatabasePreventsSKURenameReuseAndPriceMutation(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := t.Context()

	if _, err := service.CreateSKU(ctx, CreateSKUInput{SKU: "test:immutable", DisplayName: "Immutable", Enabled: true, ActorID: 1}); err != nil {
		t.Fatalf("CreateSKU() error = %v", err)
	}
	price, err := service.CreatePrice(ctx, CreatePriceInput{SKU: "test:immutable", UnitCredits: 5, Enabled: true, ActorID: 1})
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

	if _, err := service.CreateSKU(ctx, CreateSKUInput{SKU: "test:concurrent", DisplayName: "Concurrent", Enabled: true, ActorID: 1}); err != nil {
		t.Fatalf("CreateSKU() error = %v", err)
	}
	initial, err := service.CreatePrice(ctx, CreatePriceInput{SKU: "test:concurrent", UnitCredits: 10, Enabled: true, ActorID: 1})
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
			price, err := service.CreatePrice(ctx, CreatePriceInput{SKU: "test:concurrent", UnitCredits: unitCredits, Enabled: true, ActorID: 2})
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
