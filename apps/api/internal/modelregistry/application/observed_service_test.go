package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/modelregistry/domain"
)

type fakeAttemptStore struct {
	obs domain.AttemptObservation
	err error
}

func (f *fakeAttemptStore) ListAttemptObservations(context.Context, time.Time) (domain.AttemptObservation, error) {
	return f.obs, f.err
}

type fakeSKUCatalog struct {
	cards []domain.SKUCatalogCard
	err   error
}

func (f *fakeSKUCatalog) ListSKUCatalog(context.Context) ([]domain.SKUCatalogCard, error) {
	return f.cards, f.err
}

func TestObservedServiceListsRows(t *testing.T) {
	svc := NewObservedService(
		&fakeAttemptStore{obs: domain.AttemptObservation{
			Aggregates: []domain.ModelAttemptAggregate{{Model: "fal/flux-pro", Provider: "fal", Service: "generation", Total: 2, Completed: 2}},
		}},
		&fakeSKUCatalog{cards: []domain.SKUCatalogCard{{SKU: "ai-image:generation", DisplayName: "Prompt to Image", OnSale: true, UnitCreditsByVariant: map[string]int64{"1k": 25}}}},
	)
	rows, err := svc.ListModelsObserved(context.Background())
	if err != nil {
		t.Fatalf("ListModelsObserved error = %v", err)
	}
	if len(rows) != 1 || rows[0].WindowDays != domain.ObservedWindowDays || rows[0].WindowSince.IsZero() {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestObservedServicePropagatesDependencyErrors(t *testing.T) {
	want := errors.New("boom")
	for name, svc := range map[string]*ObservedService{
		"attempts": NewObservedService(&fakeAttemptStore{err: want}, &fakeSKUCatalog{}),
		"billing":  NewObservedService(&fakeAttemptStore{}, &fakeSKUCatalog{err: want}),
		"nil":      NewObservedService(nil, nil),
	} {
		if _, err := svc.ListModelsObserved(context.Background()); err == nil {
			t.Fatalf("%s: want error, got nil", name)
		}
	}
}
