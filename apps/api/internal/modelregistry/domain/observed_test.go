package domain

import (
	"testing"
	"time"
)

func floatValue(t *testing.T, value *float64) float64 {
	t.Helper()
	if value == nil {
		t.Fatal("rate is nil, want a value")
	}
	return *value
}

func findObservedRow(rows []ModelObservedRow, model string) ModelObservedRow {
	for _, row := range rows {
		if row.Model == model {
			return row
		}
	}
	return ModelObservedRow{}
}

func TestAggregateModelObservationsComputesRates(t *testing.T) {
	since := time.Now().UTC().Add(-ObservedWindowDays * 24 * time.Hour)
	rows := AggregateModelObservations(AttemptObservation{
		Aggregates: []ModelAttemptAggregate{
			{Model: "fal/flux-pro", Provider: "fal", Service: "generation", Total: 10, Completed: 8, Failed: 2, CacheHits: 4},
		},
		Failures: []ModelFailureCount{
			{Model: "fal/flux-pro", Provider: "fal", FailureStage: "provider", FailureCategory: "timeout", Count: 2},
		},
	},
		[]SKUCatalogCard{{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: true, UnitCreditsByVariant: map[string]int64{"1k": 25}}},
		since)

	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if !row.HasObservations || row.AttemptsTotal != 10 || row.Completed != 8 || row.Failed != 2 || row.CacheHits != 4 {
		t.Fatalf("row counts = %+v", row)
	}
	if got := floatValue(t, row.SuccessRate); got != 0.8 {
		t.Fatalf("success_rate = %v, want 0.8", got)
	}
	if got := floatValue(t, row.CacheHitRate); got != 0.4 {
		t.Fatalf("cache_hit_rate = %v, want 0.4", got)
	}
	if row.TopFailureStage != "provider" || row.TopFailureCat != "timeout" || row.TopFailureCount != 2 {
		t.Fatalf("top failure = %+v", row)
	}
	if !row.HasSKUMargin || len(row.SKUMargins) != 1 {
		t.Fatalf("sku margins = %+v", row.SKUMargins)
	}
	margin := row.SKUMargins[0]
	if margin.SKU != "ai-image:generation" || margin.Service != "generation" || !margin.OnSale || margin.UnitCreditsByVariant["1k"] != 25 {
		t.Fatalf("margin charged side = %+v", margin)
	}
}

func TestAggregateModelObservationsTopFailureTiebreakIsDeterministic(t *testing.T) {
	since := time.Now().UTC()
	rows := AggregateModelObservations(AttemptObservation{
		Aggregates: []ModelAttemptAggregate{{Model: "fal/x", Provider: "fal", Total: 4, Completed: 0, Failed: 4}},
		Failures: []ModelFailureCount{
			{Model: "fal/x", Provider: "fal", FailureStage: "worker", FailureCategory: "oom", Count: 2},
			{Model: "fal/x", Provider: "fal", FailureStage: "provider", FailureCategory: "timeout", Count: 2},
		},
	}, nil, since)
	row := findObservedRow(rows, "fal/x")
	if row.TopFailureStage != "provider" || row.TopFailureCat != "timeout" || row.TopFailureCount != 2 {
		t.Fatalf("top failure = %q/%q x%d, want provider/timeout x2", row.TopFailureStage, row.TopFailureCat, row.TopFailureCount)
	}
}

func TestAggregateModelObservationsEmptyYieldsNoRows(t *testing.T) {
	since := time.Now().UTC()
	rows := AggregateModelObservations(AttemptObservation{},
		[]SKUCatalogCard{{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: false, UnitCreditsByVariant: map[string]int64{}}},
		since)
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none without attempts", rows)
	}
}

func TestAggregateModelObservationsAggregateWithoutServiceHasNoMargin(t *testing.T) {
	since := time.Now().UTC()
	rows := AggregateModelObservations(AttemptObservation{
		Aggregates: []ModelAttemptAggregate{{Model: "fal/retired", Provider: "fal", Total: 3, Completed: 3}},
	}, nil, since)
	row := findObservedRow(rows, "fal/retired")
	if !row.HasObservations || row.HasSKUMargin || len(row.SKUMargins) != 0 {
		t.Fatalf("unmapped row = %+v", row)
	}
	if got := floatValue(t, row.SuccessRate); got != 1 {
		t.Fatalf("success_rate = %v, want 1", got)
	}
	if row.SKUMargins == nil {
		t.Fatal("sku_margins must serialize as [] not null")
	}
}

func TestAggregateModelObservationsMarginCostIsNeverFabricated(t *testing.T) {
	since := time.Now().UTC()
	rows := AggregateModelObservations(AttemptObservation{
		Aggregates: []ModelAttemptAggregate{
			{Model: "fal/flux-pro", Provider: "fal", Service: "generation", Total: 1, Completed: 1},
			{Model: "fal/flux-pro", Provider: "fal", Service: "vision", Total: 1, Completed: 1},
		},
	},
		[]SKUCatalogCard{
			{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: true, UnitCreditsByVariant: map[string]int64{"1k": 25}},
			{SKU: "ai-search:vision", DisplayName: "视觉搜索", OnSale: true, UnitCreditsByVariant: map[string]int64{"default": 3}},
		}, since)
	row := findObservedRow(rows, "fal/flux-pro")
	if len(row.SKUMargins) != 2 {
		t.Fatalf("margins = %+v, want one row per observed service", row.SKUMargins)
	}
	bySKU := map[string]ModelSKUMargin{}
	for _, margin := range row.SKUMargins {
		bySKU[margin.SKU] = margin
	}
	if bySKU["ai-search:vision"].Service != "vision" {
		t.Fatalf("vision service must map to ai-search:vision: %+v", bySKU)
	}
	for sku, margin := range bySKU {
		if margin.ActualCostCredits != nil || margin.HasActualCost || margin.MarginCredits != nil {
			t.Fatalf("margin %s fabricates cost: %+v", sku, margin)
		}
		if margin.CostSource != ObservedCostSourceUnavailable || margin.MarginStatus != ObservedMarginUnknownNoCost || margin.CostNote == "" {
			t.Fatalf("margin %s missing unavailable marker: %+v", sku, margin)
		}
	}
}

func TestAggregateModelObservationsRequestedModelFallback(t *testing.T) {
	since := time.Now().UTC()
	// Repository-level COALESCE is bypassed here: the pure function trusts
	// its inputs, so this covers rows keyed by requested model only.
	rows := AggregateModelObservations(AttemptObservation{
		Aggregates: []ModelAttemptAggregate{{Model: "fal/requested-only", Provider: "", Total: 2, Completed: 1, Failed: 1}},
	}, nil, since)
	row := findObservedRow(rows, "fal/requested-only")
	if !row.HasObservations || row.Provider != "" {
		t.Fatalf("row = %+v", row)
	}
}

func TestAggregateModelObservationsSortsByAttemptsDesc(t *testing.T) {
	since := time.Now().UTC()
	rows := AggregateModelObservations(AttemptObservation{
		Aggregates: []ModelAttemptAggregate{
			{Model: "b-model", Provider: "fal", Total: 1, Completed: 1},
			{Model: "a-model", Provider: "fal", Total: 5, Completed: 5},
		},
	}, nil, since)
	if len(rows) != 2 || rows[0].Model != "a-model" || rows[1].Model != "b-model" {
		t.Fatalf("rows order = %+v", rows)
	}
}
