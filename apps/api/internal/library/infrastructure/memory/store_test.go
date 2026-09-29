package memory

// In-memory job-listing tests (read-through scan + trigger dedup
// backing for tests; postgres parity lives in service_test.go).

import (
	"context"
	"sync"
	"testing"

	"ideaboard/api/internal/library/domain"
)

func saveReadThroughAsset(t *testing.T, store *InMemoryWorkspaceLibraryStore, ws, sourceID string) string {
	t.Helper()
	publicID, err := domain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	providerURL := "https://images.unsplash.com/" + sourceID
	saved, created, err := store.SaveAsset(context.Background(), ws, domain.V1SaveAssetInput{
		Asset: domain.AssetInput{
			Provider: domain.ProviderUnsplash, SourceID: sourceID,
			SourceURL: providerURL, PreviewURL: providerURL, OriginalURL: providerURL,
		},
	}, publicID)
	if err != nil || !created || saved == nil {
		t.Fatalf("SaveAsset = %+v %v %v, want created", saved, created, err)
	}
	return saved.PublicID
}

func TestInMemoryListMaterializations(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceLibraryStoreForTests()
	asset := saveReadThroughAsset(t, store, "ws_list", "photo-list-1")

	if empty, err := store.ListMaterializations(ctx, "ws_list"); err != nil || len(empty) != 0 {
		t.Fatalf("list empty = %d %v, want 0 nil", len(empty), err)
	}
	if missing, err := store.ListMaterializations(ctx, "ws_missing"); err != nil || len(missing) != 0 {
		t.Fatalf("list unknown workspace = %d %v, want empty", len(missing), err)
	}

	var ids []string
	for range 2 {
		matID, err := domain.NewV1MaterializationPublicID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateMaterialization(ctx, "ws_list", asset, matID); err != nil {
			t.Fatalf("CreateMaterialization: %v", err)
		}
		ids = append(ids, matID)
	}
	all, err := store.ListMaterializations(ctx, "ws_list")
	if err != nil {
		t.Fatalf("ListMaterializations: %v", err)
	}
	if len(all) != 2 || all[0].PublicID != ids[0] || all[1].PublicID != ids[1] {
		t.Fatalf("list order = %v, want creation order %v", all, ids)
	}
	for _, record := range all {
		if record.AssetPublicID != asset || record.Status != domain.V1MaterializationPending {
			t.Fatalf("record = %+v, want pending link to %q", record, asset)
		}
	}
}

func TestInMemoryClaimMaterializationConvergesConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceLibraryStoreForTests()
	asset := saveReadThroughAsset(t, store, "ws_claim", "photo-claim-1")
	const callers = 16
	results := make(chan struct {
		job     *domain.V1Materialization
		created bool
		err     error
	}, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, err := domain.NewV1MaterializationPublicID()
			if err != nil {
				results <- struct {
					job     *domain.V1Materialization
					created bool
					err     error
				}{err: err}
				return
			}
			job, created, err := store.ClaimMaterialization(ctx, "ws_claim", asset, id)
			results <- struct {
				job     *domain.V1Materialization
				created bool
				err     error
			}{job: job, created: created, err: err}
		}(i)
	}
	wg.Wait()
	close(results)
	var createdCount int
	var publicID string
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.job == nil {
			t.Fatal("claim returned nil job")
		}
		if result.created {
			createdCount++
			publicID = result.job.PublicID
		}
	}
	if createdCount != 1 {
		t.Fatalf("created claims = %d, want 1", createdCount)
	}
	all, err := store.ListMaterializations(ctx, "ws_claim")
	if err != nil || len(all) != 1 || all[0].PublicID != publicID {
		t.Fatalf("jobs = %+v, err=%v, want one job %q", all, err, publicID)
	}

	if _, err := store.SetMaterializationResult(ctx, "ws_claim", publicID, domain.V1MaterializationRunning, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMaterializationResult(ctx, "ws_claim", publicID, domain.V1MaterializationFailed, "", "", "retry"); err != nil {
		t.Fatal(err)
	}
	retryID, err := domain.NewV1MaterializationPublicID()
	if err != nil {
		t.Fatal(err)
	}
	job, created, err := store.ClaimMaterialization(ctx, "ws_claim", asset, retryID)
	if err != nil || !created || job.PublicID != retryID {
		t.Fatalf("failed-job retry = %+v, created=%v, err=%v", job, created, err)
	}
}
