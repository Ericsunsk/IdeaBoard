package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	aigatewaydomain "ideaboard/api/internal/aigateway/domain"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func floatValue(t *testing.T, value *float64) float64 {
	t.Helper()
	if value == nil {
		t.Fatal("rate is nil, want a value")
	}
	return *value
}

func findObservedRow(rows []aigatewaydomain.ModelObservedRow, model string) aigatewaydomain.ModelObservedRow {
	for _, row := range rows {
		if row.Model == model {
			return row
		}
	}
	return aigatewaydomain.ModelObservedRow{}
}

func TestAggregateModelObservationsComputesRates(t *testing.T) {
	since := time.Now().UTC().Add(-aigatewaydomain.ObservedWindowDays * 24 * time.Hour)
	rows := aigatewaydomain.AggregateModelObservations(aigatewaydomain.AttemptObservation{
		Aggregates: []aigatewaydomain.ModelAttemptAggregate{
			{Model: "fal/flux-pro", Provider: "fal", Service: "generation", Total: 10, Completed: 8, Failed: 2, CacheHits: 4},
		},
		Failures: []aigatewaydomain.ModelFailureCount{
			{Model: "fal/flux-pro", Provider: "fal", FailureStage: "provider", FailureCategory: "timeout", Count: 2},
		},
	},
		[]aigatewaydomain.SKUCatalogCard{{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: true, UnitCreditsByVariant: map[string]int64{"1k": 25}}},
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
	rows := aigatewaydomain.AggregateModelObservations(aigatewaydomain.AttemptObservation{
		Aggregates: []aigatewaydomain.ModelAttemptAggregate{{Model: "fal/x", Provider: "fal", Total: 4, Completed: 0, Failed: 4}},
		Failures: []aigatewaydomain.ModelFailureCount{
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
	// #847: the mapped-but-idle union is gone with the single-value mapping.
	// Rows cover observed models only; callers render an empty window as 暂无数据.
	since := time.Now().UTC()
	rows := aigatewaydomain.AggregateModelObservations(aigatewaydomain.AttemptObservation{},
		[]aigatewaydomain.SKUCatalogCard{{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: false, UnitCreditsByVariant: map[string]int64{}}},
		since)
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none without attempts", rows)
	}
}

func TestAggregateModelObservationsAggregateWithoutServiceHasNoMargin(t *testing.T) {
	since := time.Now().UTC()
	rows := aigatewaydomain.AggregateModelObservations(aigatewaydomain.AttemptObservation{
		Aggregates: []aigatewaydomain.ModelAttemptAggregate{{Model: "fal/retired", Provider: "fal", Total: 3, Completed: 3}},
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
	rows := aigatewaydomain.AggregateModelObservations(aigatewaydomain.AttemptObservation{
		Aggregates: []aigatewaydomain.ModelAttemptAggregate{
			{Model: "fal/flux-pro", Provider: "fal", Service: "generation", Total: 1, Completed: 1},
			{Model: "fal/flux-pro", Provider: "fal", Service: "vision", Total: 1, Completed: 1},
		},
	},
		[]aigatewaydomain.SKUCatalogCard{
			{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: true, UnitCreditsByVariant: map[string]int64{"1k": 25}},
			{SKU: "ai-search:vision", DisplayName: "视觉搜索", OnSale: true, UnitCreditsByVariant: map[string]int64{"default": 3}},
		}, since)
	row := findObservedRow(rows, "fal/flux-pro")
	if len(row.SKUMargins) != 2 {
		t.Fatalf("margins = %+v, want one row per observed service", row.SKUMargins)
	}
	bySKU := map[string]aigatewaydomain.ModelSKUMargin{}
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
		if margin.CostSource != aigatewaydomain.ObservedCostSourceUnavailable || margin.MarginStatus != aigatewaydomain.ObservedMarginUnknownNoCost || margin.CostNote == "" {
			t.Fatalf("margin %s missing unavailable marker: %+v", sku, margin)
		}
	}
}

func TestAggregateModelObservationsRequestedModelFallback(t *testing.T) {
	since := time.Now().UTC()
	// Repository-level COALESCE is bypassed here: the pure function trusts
	// its inputs, so this covers rows keyed by requested model only.
	rows := aigatewaydomain.AggregateModelObservations(aigatewaydomain.AttemptObservation{
		Aggregates: []aigatewaydomain.ModelAttemptAggregate{{Model: "fal/requested-only", Provider: "", Total: 2, Completed: 1, Failed: 1}},
	}, nil, since)
	row := findObservedRow(rows, "fal/requested-only")
	if !row.HasObservations || row.Provider != "" {
		t.Fatalf("row = %+v", row)
	}
}

func TestAggregateModelObservationsSortsByAttemptsDesc(t *testing.T) {
	since := time.Now().UTC()
	rows := aigatewaydomain.AggregateModelObservations(aigatewaydomain.AttemptObservation{
		Aggregates: []aigatewaydomain.ModelAttemptAggregate{
			{Model: "b-model", Provider: "fal", Total: 1, Completed: 1},
			{Model: "a-model", Provider: "fal", Total: 5, Completed: 5},
		},
	}, nil, since)
	if len(rows) != 2 || rows[0].Model != "a-model" || rows[1].Model != "b-model" {
		t.Fatalf("rows order = %+v", rows)
	}
}

type fakeAttemptStore struct {
	obs aigatewaydomain.AttemptObservation
	err error
}

func (f *fakeAttemptStore) ListAttemptObservations(context.Context, time.Time) (aigatewaydomain.AttemptObservation, error) {
	return f.obs, f.err
}

type fakeSKUCatalog struct {
	cards []aigatewaydomain.SKUCatalogCard
	err   error
}

func (f *fakeSKUCatalog) ListSKUCatalog(context.Context) ([]aigatewaydomain.SKUCatalogCard, error) {
	return f.cards, f.err
}

func TestObservedServiceListsRows(t *testing.T) {
	svc := aigatewaydomain.NewObservedService(
		&fakeAttemptStore{obs: aigatewaydomain.AttemptObservation{
			Aggregates: []aigatewaydomain.ModelAttemptAggregate{{Model: "fal/flux-pro", Provider: "fal", Service: "generation", Total: 2, Completed: 2}},
		}},
		&fakeSKUCatalog{cards: []aigatewaydomain.SKUCatalogCard{{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: true, UnitCreditsByVariant: map[string]int64{"1k": 25}}}},
	)
	rows, err := svc.ListModelsObserved(context.Background())
	if err != nil {
		t.Fatalf("ListModelsObserved error = %v", err)
	}
	if len(rows) != 1 || rows[0].WindowDays != aigatewaydomain.ObservedWindowDays || rows[0].WindowSince.IsZero() {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestObservedServicePropagatesDependencyErrors(t *testing.T) {
	want := errors.New("boom")
	for name, svc := range map[string]*aigatewaydomain.ObservedService{
		"attempts": aigatewaydomain.NewObservedService(&fakeAttemptStore{err: want}, &fakeSKUCatalog{}),
		"billing":  aigatewaydomain.NewObservedService(&fakeAttemptStore{}, &fakeSKUCatalog{err: want}),
		"nil":      aigatewaydomain.NewObservedService(nil, nil),
	} {
		if _, err := svc.ListModelsObserved(context.Background()); err == nil {
			t.Fatalf("%s: want error, got nil", name)
		}
	}
}

func serveModelsObserved(t *testing.T, role principal.Role, handler *Handler) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/settings/ai-gateway/models-observed", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, role)
		handler.ListModelsObserved(c)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/ai-gateway/models-observed", nil))
	return rec
}

func TestObservedHandlerGatingAndEnvelope(t *testing.T) {
	// Non-admin gating rides the outer Control Plane guard (controlProxy +
	// guard); the handler performs no role check. Guard-level denial is
	// covered by TestControlAuthMatrix in runtime/interfaces/http.
	t.Run("fails closed without service", func(t *testing.T) {
		rec := serveModelsObserved(t, principal.RoleAdmin, NewHandler(&fakeGatewayStore{}))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
		}
	})

	t.Run("returns envelope with rows", func(t *testing.T) {
		handler := NewHandler(&fakeGatewayStore{})
		handler.SetObservedService(aigatewaydomain.NewObservedService(
			&fakeAttemptStore{obs: aigatewaydomain.AttemptObservation{
				Aggregates: []aigatewaydomain.ModelAttemptAggregate{{Model: "fal/flux-pro", Provider: "fal", Service: "generation", Total: 4, Completed: 3, Failed: 1, CacheHits: 1}},
				Failures:   []aigatewaydomain.ModelFailureCount{{Model: "fal/flux-pro", Provider: "fal", FailureStage: "provider", FailureCategory: "timeout", Count: 1}},
			}},
			&fakeSKUCatalog{cards: []aigatewaydomain.SKUCatalogCard{{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: true, UnitCreditsByVariant: map[string]int64{"1k": 25}}}},
		))
		rec := serveModelsObserved(t, principal.RoleAdmin, handler)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var payload struct {
			Data []aigatewaydomain.ModelObservedRow `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("body is not the envelope: %v", err)
		}
		if len(payload.Data) != 1 {
			t.Fatalf("data = %+v, want 1 row", payload.Data)
		}
		row := payload.Data[0]
		if row.Model != "fal/flux-pro" || !row.HasObservations || row.TopFailureStage != "provider" {
			t.Fatalf("row = %+v", row)
		}
		if row.SKUMargins[0].ActualCostCredits != nil || row.SKUMargins[0].MarginStatus != aigatewaydomain.ObservedMarginUnknownNoCost {
			t.Fatalf("margin = %+v, want unavailable cost marker", row.SKUMargins[0])
		}
	})
}
