package workspacemedia

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/contract"
	taskdomain "ideaboard/api/internal/tasking/domain"
)

func newMemoryTestStore() *InMemoryWorkspaceMediaStore {
	return NewInMemoryWorkspaceMediaStoreForTests()
}

func mustCreateSession(t *testing.T, ctx context.Context, store WorkspaceMediaStore, ws string, owner int64) *Session {
	t.Helper()
	session, err := store.CreateSession(ctx, CreateSessionParams{
		WorkspaceID:    ws,
		OwnerUserID:    owner,
		Filename:       "photo.png",
		MIMEType:       "image/png",
		SizeBytes:      1024,
		ChecksumSHA256: testChecksum,
		MediaKind:      MediaKindImage,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if !contract.IsUploadID(session.ID) {
		t.Fatalf("session id = %q, want upload_ shape", session.ID)
	}
	if session.Status != StatusPending {
		t.Fatalf("session status = %q, want pending", session.Status)
	}
	if !strings.HasPrefix(session.StorageKey, "uploads/ws/") {
		t.Fatalf("storage key = %q, want reserved canonical uploads/ws slot", session.StorageKey)
	}
	return session
}

func completeFacts() CompleteFacts {
	return CompleteFacts{ChecksumSHA256: testChecksum, SizeBytes: 1024, Width: 100, Height: 80}
}

// TestMemorySessionLifecycle proves create -> transferring (first
// read) -> complete with atomic asset creation.
func TestMemorySessionLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()

	session := mustCreateSession(t, ctx, store, "ws_a", 7)

	got, err := store.GetSession(ctx, "ws_a", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusTransferring {
		t.Fatalf("first read status = %q, want transferring", got.Status)
	}

	completed, asset, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{})
	if err != nil {
		t.Fatalf("CompleteSession failed: %v", err)
	}
	if completed.Status != StatusCompleted || completed.AssetID == "" {
		t.Fatalf("completed = %+v, want completed with asset", completed)
	}
	if !contract.IsAssetID(asset.ID) {
		t.Fatalf("asset id = %q, want asset_ shape", asset.ID)
	}
	if asset.Status != AssetStatusReady {
		t.Fatalf("asset status = %q, want ready", asset.Status)
	}
	if asset.Width != 100 || asset.Height != 80 {
		t.Fatalf("asset dims = %dx%d, want 100x80", asset.Width, asset.Height)
	}
	if asset.ChecksumSHA256 != testChecksum || asset.SizeBytes != 1024 || asset.MIMEType != "image/png" {
		t.Fatalf("asset declarations did not round-trip: %+v", asset)
	}

	// Terminal sessions fail closed.
	if _, _, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{}); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("re-complete err = %v, want state conflict", err)
	}
	if _, err := store.FailSession(ctx, "ws_a", session.ID, "late"); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("fail-after-complete err = %v, want state conflict", err)
	}

	stored, err := store.GetAsset(ctx, "ws_a", asset.ID)
	if err != nil || stored.ID != asset.ID {
		t.Fatalf("GetAsset = %+v %v", stored, err)
	}
	listed, err := store.ListAssets(ctx, "ws_a", 10, 0)
	if err != nil || len(listed) != 1 || listed[0].ID != asset.ID {
		t.Fatalf("ListAssets = %+v %v", listed, err)
	}
}

func TestMemoryCompleteMismatchFailsSession(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()
	session := mustCreateSession(t, ctx, store, "ws_a", 7)

	facts := completeFacts()
	facts.SizeBytes = 2048
	_, _, err := store.CompleteSession(ctx, "ws_a", session.ID, facts, CreateAssetParams{})
	var validationErr *ValidationError
	if !isValidationErrorAs(err, &validationErr) || validationErr.Fields["size_bytes"] == "" {
		t.Fatalf("mismatch err = %v, want size_bytes field error", err)
	}
	// The session failed terminally: completion can no longer proceed.
	got, err := store.GetSession(ctx, "ws_a", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed {
		t.Fatalf("mismatched session status = %q, want failed", got.Status)
	}
}

func TestMemoryLazyExpiry(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()
	now := time.Now().UTC()
	store.SetClockForTests(func() time.Time { return now })

	session := mustCreateSession(t, ctx, store, "ws_a", 7)

	// Past the pending TTL the next read expires the session.
	now = now.Add(UploadPendingTTL + time.Minute)
	if _, err := store.GetSession(ctx, "ws_a", session.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired read err = %v, want expired", err)
	}
	if _, _, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{}); !errors.Is(err, ErrExpired) {
		t.Fatalf("complete expired err = %v, want expired", err)
	}
}

func TestMemoryCrossWorkspaceIsolation(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()
	session := mustCreateSession(t, ctx, store, "ws_a", 7)

	if _, err := store.GetSession(ctx, "ws_b", session.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace get err = %v, want not found", err)
	}
	if _, _, err := store.CompleteSession(ctx, "ws_b", session.ID, completeFacts(), CreateAssetParams{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace complete err = %v, want not found", err)
	}
	if _, err := store.GetSession(ctx, "ws_a", "upload_missing0001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown get err = %v, want not found", err)
	}
	if _, err := store.GetSession(ctx, "ws_a", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed get err = %v, want not found", err)
	}
	listed, err := store.ListAssets(ctx, "ws_b", 10, 0)
	if err != nil || len(listed) != 0 {
		t.Fatalf("foreign workspace assets = %+v %v, want empty", listed, err)
	}
}

func TestMemoryArchiveIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()
	session := mustCreateSession(t, ctx, store, "ws_a", 7)
	_, asset, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{})
	if err != nil {
		t.Fatal(err)
	}

	archived, err := store.ArchiveAsset(ctx, "ws_a", asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !archived.IsArchived() {
		t.Fatal("archived asset must carry the marker")
	}
	replay, err := store.ArchiveAsset(ctx, "ws_a", asset.ID)
	if err != nil || !replay.IsArchived() {
		t.Fatalf("archive replay = %+v %v, want converged marker", replay, err)
	}
	// Archived assets stay metadata-readable but leave the list.
	if _, err := store.GetAsset(ctx, "ws_a", asset.ID); err != nil {
		t.Fatalf("archived get = %v, want readable", err)
	}
	listed, err := store.ListAssets(ctx, "ws_a", 10, 0)
	if err != nil || len(listed) != 0 {
		t.Fatalf("list after archive = %+v %v, want empty", listed, err)
	}
	if _, err := store.ArchiveAsset(ctx, "ws_b", asset.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace archive err = %v, want not found", err)
	}
}

func TestMemoryUpdateMetadata(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()
	session := mustCreateSession(t, ctx, store, "ws_a", 7)
	_, asset, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateAssetMetadata(ctx, "ws_a", asset.ID, UpdateMetadataParams{
		Width: 200, Height: 160, HasDims: true, Metadata: map[string]any{"source": "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Width != 200 || updated.Height != 160 {
		t.Fatalf("dims = %dx%d, want 200x160", updated.Width, updated.Height)
	}
	if _, err := store.UpdateAssetMetadata(ctx, "ws_a", "asset_missing0001", UpdateMetadataParams{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update unknown err = %v, want not found", err)
	}
}

func TestTaskAssetAdapter(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()
	adapter := &TaskAssetAdapter{Store: store}

	session := mustCreateSession(t, ctx, store, "ws_a", 7)
	_, asset, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := adapter.GetAsset(ctx, "ws_a", asset.ID)
	if err != nil {
		t.Fatalf("GetAsset failed: %v", err)
	}
	if info.ID != asset.ID || info.Status != AssetStatusReady || info.Archived {
		t.Fatalf("info = %+v, want ready asset", info)
	}
	if _, err := adapter.GetAsset(ctx, "ws_a", "asset_missing0001"); !errors.Is(err, taskdomain.ErrMediaNotFound) {
		t.Fatalf("unknown err = %v, want media not found", err)
	}
	if _, err := adapter.GetAsset(ctx, "ws_b", asset.ID); !errors.Is(err, taskdomain.ErrMediaNotFound) {
		t.Fatalf("cross-workspace err = %v, want media not found", err)
	}
	if _, err := store.ArchiveAsset(ctx, "ws_a", asset.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetAsset(ctx, "ws_a", asset.ID); !errors.Is(err, taskdomain.ErrMediaArchived) {
		t.Fatalf("archived err = %v, want media archived", err)
	}
	var nilAdapter *TaskAssetAdapter
	if _, err := nilAdapter.GetAsset(ctx, "ws_a", asset.ID); !errors.Is(err, taskdomain.ErrMediaNotFound) {
		t.Fatalf("nil adapter err = %v, want media not found", err)
	}
}

func TestMemoryListTaskAssetsSeeded(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()
	session := mustCreateSession(t, ctx, store, "ws_a", 7)
	_, asset, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedTaskAssetForTests("ws_a", "task_1", asset.OwnerUserID, "", &TaskAsset{Asset: asset, OutputIndex: 0, Role: "result"})

	items, err := store.ListTaskAssets(ctx, "ws_a", "task_1")
	if err != nil || len(items) != 1 || items[0].Asset.ID != asset.ID || items[0].Role != "result" {
		t.Fatalf("task assets = %+v %v", items, err)
	}
	empty, err := store.ListTaskAssets(ctx, "ws_a", "task_missing")
	if err != nil || len(empty) != 0 {
		t.Fatalf("unknown task assets = %+v %v, want empty", empty, err)
	}
}
