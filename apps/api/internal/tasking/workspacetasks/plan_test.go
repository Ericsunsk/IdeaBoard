package workspacetasks

// Submit-time durable plan tests: Create persists the task with no
// submit-time model mint. The single-value admin mapping is removed and
// the durable envelope carries no model; the RoutingSnapshot pins in the
// submit transaction instead. Canonical params ride
// CanonicalParams untouched, and the transient ResolvedModel handoff
// stays empty outside worker execution.

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/generation/routing"
)

func planTestService() (*InMemoryStore, *fakeHolder, *fakeEnqueuer, *Service) {
	store := NewInMemoryStoreForTests()
	holder := newFakeHolder()
	queue := &fakeEnqueuer{}
	service := NewTestServiceForTests(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Resolver: allowTestPoolResolver(),
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
	store, holder, queue, _ := planTestService()
	quoter := &fakeQuoter{snapshot: testSnapshot()}
	// No resolver wired: submit fails closed with the canonical
	// unavailable sentinel before any quote, hold, task, or enqueue
	// work.
	service := NewTestServiceForTests(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: quoter, Holder: holder, Queue: queue,
	})

	_, quoteErr := service.Quote(ctx, QuoteParams{Service: "render"})
	if !errors.Is(quoteErr, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Quote err = %v, want ErrServiceResolutionUnavailable", quoteErr)
	}

	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "routing-key-1")
	if !errors.Is(err, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Create err = %v, want ErrServiceResolutionUnavailable", err)
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
