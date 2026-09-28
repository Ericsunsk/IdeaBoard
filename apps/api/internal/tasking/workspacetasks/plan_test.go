package workspacetasks

// Submit-time durable plan tests (#847): Create persists the task without a
// submit-time model mint. The single-value admin mapping is removed; #848
// pins the RoutingSnapshot in this transaction instead. Canonical params
// ride CanonicalParams untouched, and an empty ResolvedModel fails the
// worker closed until the snapshot lands.

import (
	"context"
	"errors"
	"testing"
)

func planTestService() (*InMemoryStore, *fakeHolder, *fakeEnqueuer, *Service) {
	store := NewInMemoryStoreForTests()
	holder := newFakeHolder()
	queue := &fakeEnqueuer{}
	service := NewService(ServiceDeps{
		Tasks: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Routing: allowTestRouting,
	})
	return store, holder, queue, service
}

func TestCreatePersistsWithoutResolvedModel(t *testing.T) {
	ctx := context.Background()
	store, _, _, service := planTestService()
	created, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
		CanonicalParams: `{"schema_version":6,"service":"render"}`,
	}, "plan-key-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Service != "render" {
		t.Fatalf("service = %q, want render", created.Service)
	}
	if created.ResolvedModel != "" {
		t.Fatalf("resolved_model = %q, want empty until the #848 snapshot lands", created.ResolvedModel)
	}
	if created.CanonicalParams != `{"schema_version":6,"service":"render"}` {
		t.Fatalf("canonical_params = %q, want passthrough untouched", created.CanonicalParams)
	}
	// The persisted fact survives the store round-trip (durable plan).
	loaded, err := store.GetTask(ctx, "ws_abc", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loaded.Service != "render" || loaded.ResolvedModel != "" {
		t.Fatalf("loaded service = %q resolved_model = %q, want render and empty", loaded.Service, loaded.ResolvedModel)
	}
}

func TestCreateRejectsUnknownService(t *testing.T) {
	ctx := context.Background()
	_, _, _, service := planTestService()
	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "nope",
	}, "plan-key-2")
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Create err = %T %v, want *ValidationError", err, err)
	}
	if validationErr.Fields["service"] == "" {
		t.Fatalf("Create fields = %v, want a service entry", validationErr.Fields)
	}
}

func TestCreateRejectsBeforeHoldWhenRoutingIsUnavailable(t *testing.T) {
	ctx := context.Background()
	store, holder, queue, service := planTestService()
	quoter := service.quoter.(*fakeQuoter)
	service = NewService(ServiceDeps{
		Tasks: store, Quoter: quoter, Holder: holder, Queue: queue,
	})

	_, quoteErr := service.Quote(ctx, QuoteParams{Service: "render"})
	if !errors.Is(quoteErr, ErrRoutingUnavailable) {
		t.Fatalf("Quote err = %v, want ErrRoutingUnavailable", quoteErr)
	}

	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "routing-key-1")
	if !errors.Is(err, ErrRoutingUnavailable) {
		t.Fatalf("Create err = %v, want ErrRoutingUnavailable", err)
	}
	count, _ := store.CountTasks(ctx, "ws_abc")
	if count != 0 {
		t.Fatalf("tasks = %d, want 0", count)
	}
	if len(holder.holds) != 0 || len(holder.released) != 0 {
		t.Fatalf("routing rejection must not touch credits: holds=%v released=%v", holder.holds, holder.released)
	}
	if queue.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0", queue.calls)
	}
	if quoter.calls != 0 {
		t.Fatalf("quote calls = %d, want 0", quoter.calls)
	}
}
