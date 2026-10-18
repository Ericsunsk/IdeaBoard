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
	firstAsset := saveReadThroughAsset(t, store, "ws_list", "photo-list-1")
	secondAsset := saveReadThroughAsset(t, store, "ws_list", "photo-list-2")

	if empty, err := store.ListMaterializations(ctx, "ws_list"); err != nil || len(empty) != 0 {
		t.Fatalf("list empty = %d %v, want 0 nil", len(empty), err)
	}
	if missing, err := store.ListMaterializations(ctx, "ws_missing"); err != nil || len(missing) != 0 {
		t.Fatalf("list unknown workspace = %d %v, want empty", len(missing), err)
	}

	// Claim (the production trigger path) converges per asset, so the
	// ordering fixture uses one job per asset.
	var ids []string
	for _, asset := range []string{firstAsset, secondAsset} {
		matID, err := domain.NewV1MaterializationPublicID()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.ClaimMaterialization(ctx, "ws_list", asset, matID); err != nil {
			t.Fatalf("ClaimMaterialization: %v", err)
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
		if record.Status != domain.V1MaterializationPending {
			t.Fatalf("record = %+v, want pending", record)
		}
	}
	if all[0].AssetPublicID != firstAsset || all[1].AssetPublicID != secondAsset {
		t.Fatalf("records = %+v, want links to %q, %q", all, firstAsset, secondAsset)
	}
	_ = secondAsset
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

func TestInMemorySaveManagedMediaConverges(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceLibraryStoreForTests()
	w, h := int32(640), int32(480)
	store.RegisterManagedMediaForTests("ws_mem", "asset_mem000000000000000001", &w, &h, false)
	store.RegisterManagedMediaForTests("ws_mem", "asset_mem000000000000000002", nil, nil, false)
	store.RegisterManagedMediaForTests("ws_mem", "asset_mem000000000000000003", nil, nil, true)

	auto, created, err := store.SaveManagedMediaAsset(ctx, "ws_mem", domain.V1SaveManagedAssetInput{
		MediaAssetID: "asset_mem000000000000000001", Title: "auto",
		Lineage:        &domain.TaskLineage{TaskID: "task_aaaaaaaaaaaaaaaaaaaaaaaa", ResultIndex: 1},
		OperatorUserID: 7,
	}, mustV1AssetID(t))
	if err != nil || !created {
		t.Fatalf("auto = %v %v, want created", auto, err)
	}
	if auto.Source != domain.AssetSourceMedia || auto.Width == nil || *auto.Width != 640 {
		t.Fatalf("auto = %+v, want media with projected dims", auto)
	}
	unknown, created, err := store.SaveManagedMediaAsset(ctx, "ws_mem", domain.V1SaveManagedAssetInput{
		MediaAssetID: "asset_mem000000000000000002", OperatorUserID: 7,
	}, mustV1AssetID(t))
	if err != nil || !created || unknown.Width != nil || unknown.Height != nil {
		t.Fatalf("unknown = %+v %v, want created with null dims", unknown, err)
	}

	// Manual repeat converges without touching folder/lineage/metadata.
	manual, created, err := store.SaveManagedMediaAsset(ctx, "ws_mem", domain.V1SaveManagedAssetInput{
		MediaAssetID: "asset_mem000000000000000001", Title: "override", OperatorUserID: 9,
	}, mustV1AssetID(t))
	if err != nil || created || manual.PublicID != auto.PublicID || manual.Title != "auto" || manual.Lineage == nil {
		t.Fatalf("manual = %+v %v, want existing row kept", manual, err)
	}

	// Forbidden media fail closed.
	for _, tt := range []struct {
		name  string
		ws    string
		media string
	}{
		{"cross-workspace", "ws_other", "asset_mem000000000000000001"},
		{"archived", "ws_mem", "asset_mem000000000000000003"},
		{"missing", "ws_mem", "asset_missing00000000000001"},
		{"no operator", "ws_mem", "asset_mem000000000000000002"},
	} {
		op := int64(7)
		if tt.name == "no operator" {
			op = 0
		}
		if _, _, err := store.SaveManagedMediaAsset(ctx, tt.ws, domain.V1SaveManagedAssetInput{
			MediaAssetID: tt.media, OperatorUserID: op,
		}, mustV1AssetID(t)); err == nil {
			t.Fatalf("%s saved, want failure", tt.name)
		}
	}
	all, err := store.ListAssets(ctx, "ws_mem")
	if err != nil || len(all) != 2 {
		t.Fatalf("assets = %d %v, want 2", len(all), err)
	}
}

func mustV1AssetID(t *testing.T) string {
	t.Helper()
	id, err := domain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
