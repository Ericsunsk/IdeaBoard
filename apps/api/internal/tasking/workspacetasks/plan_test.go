package workspacetasks

// Submit-time durable plan tests: Create mints the resolved model into
// the persisted task (ResolvedModel) via the Models seam. The
// canonical params (prompt, mask refs, params snapshot, execution plan)
// ride CanonicalParams untouched; the model is the only plan fact minted
// here, with submit failing closed when the target cannot be resolved.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubModelResolver struct {
	models map[string]string
	err    error
	calls  []string
}

func (s *stubModelResolver) ResolveModel(_ context.Context, feature string) (string, error) {
	s.calls = append(s.calls, feature)
	if s.err != nil {
		return "", s.err
	}
	return s.models[feature], nil
}

func planTestService(models ModelResolver) (*InMemoryStore, *fakeHolder, *fakeEnqueuer, *Service) {
	store := NewInMemoryStoreForTests()
	holder := newFakeHolder()
	queue := &fakeEnqueuer{}
	service := NewService(ServiceDeps{
		Tasks: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: holder, Queue: queue, Models: models,
	})
	return store, holder, queue, service
}

func TestCreateMintsResolvedModel(t *testing.T) {
	ctx := context.Background()
	store, _, _, service := planTestService(&stubModelResolver{
		models: map[string]string{"render": "openrouter/fake-image-model"},
	})
	created, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, WorkflowKind: "render",
		CanonicalParams: `{"schema_version":5,"feature":"render"}`,
	}, "plan-key-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ResolvedModel != "openrouter/fake-image-model" {
		t.Fatalf("resolved_model = %q, want the submit-time mapping", created.ResolvedModel)
	}
	if created.CanonicalParams != `{"schema_version":5,"feature":"render"}` {
		t.Fatalf("canonical_params = %q, want passthrough untouched", created.CanonicalParams)
	}
	// The minted fact survives the store round-trip (durable plan).
	loaded, err := store.GetTask(ctx, "ws_abc", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loaded.ResolvedModel != "openrouter/fake-image-model" {
		t.Fatalf("loaded resolved_model = %q, want the minted model", loaded.ResolvedModel)
	}
}

func TestCreateWithoutModelsSeamFailsClosed(t *testing.T) {
	ctx := context.Background()
	_, _, _, service := planTestService(nil)
	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, WorkflowKind: "render",
	}, "plan-key-2")
	if err == nil || !strings.Contains(err.Error(), "model resolver unavailable") {
		t.Fatalf("Create err = %v, want missing resolver failure", err)
	}
}

func TestCreateAlwaysUsesSubmitTimeModelMapping(t *testing.T) {
	ctx := context.Background()
	stub := &stubModelResolver{models: map[string]string{"render": "openrouter/seam-model"}}
	_, _, _, service := planTestService(stub)
	created, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, WorkflowKind: "render",
	}, "plan-key-3")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ResolvedModel != "openrouter/seam-model" {
		t.Fatalf("resolved_model = %q, want the submit-time mapping", created.ResolvedModel)
	}
	if len(stub.calls) != 1 || stub.calls[0] != "render" {
		t.Fatalf("resolver calls = %v, want one canonical-kind lookup", stub.calls)
	}
}

func TestCreateResolverFailureBlocksSettlement(t *testing.T) {
	ctx := context.Background()
	_, _, _, service := planTestService(&stubModelResolver{err: errors.New("fake: mapping store down")})
	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, WorkflowKind: "render",
	}, "plan-key-4")
	if err == nil || !strings.Contains(err.Error(), "mapping store down") {
		t.Fatalf("Create err = %v, want resolver failure", err)
	}
}

func TestCreateMintsCanonicalKindForModelLookup(t *testing.T) {
	ctx := context.Background()
	stub := &stubModelResolver{models: map[string]string{"render": "openrouter/fake-image-model"}}
	_, _, _, service := planTestService(stub)
	if _, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, WorkflowKind: "render",
	}, "plan-key-5"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(stub.calls) != 1 || strings.TrimSpace(stub.calls[0]) != "render" {
		t.Fatalf("resolver calls = %v, want one canonical-kind lookup", stub.calls)
	}
}
