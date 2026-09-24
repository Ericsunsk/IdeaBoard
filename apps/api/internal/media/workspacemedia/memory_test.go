package workspacemedia

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/tasking/workspacetasks"
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
	if !IsUploadID(session.ID) {
		t.Fatalf("session id = %q, want upload_ shape", session.ID)
	}
	if session.Status != StatusPending {
		t.Fatalf("session status = %q, want pending", session.Status)
	}
	if !strings.HasPrefix(session.StorageKey, "ws-uploads/") {
		t.Fatalf("storage key = %q, want reserved ws-uploads slot", session.StorageKey)
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
	if !IsAssetID(asset.ID) {
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

// fakeDerivativeObjects is a map-backed DerivativeObjects fake.
type fakeDerivativeObjects struct {
	objects map[string][]byte
	missing bool
}

func (f *fakeDerivativeObjects) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*storageports.StoredUploadObject, error) {
	if f.missing {
		return nil, errFakeMissing
	}
	data := f.objects[objectKey]
	if len(data) == 0 {
		return nil, errFakeMissing
	}
	return &storageports.StoredUploadObject{ObjectKey: objectKey, ContentType: "image/png", Data: data}, nil
}

func (f *fakeDerivativeObjects) PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error {
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	f.objects[objectKey] = append([]byte(nil), data...)
	return nil
}

var errFakeMissing = errors.New("fake: object missing")

// fakeRenderer returns canned webp bytes with requested maxSide dims.
type fakeRenderer struct {
	calls    []int
	mime     string
	failOn   int
	emptyOut bool
}

func (f *fakeRenderer) Render(ctx context.Context, data []byte, sourceName string, maxSide int) ([]byte, int, int, string, error) {
	f.calls = append(f.calls, maxSide)
	if f.failOn == len(f.calls) {
		return nil, 0, 0, "", errors.New("fake: render failed")
	}
	if f.emptyOut {
		return nil, 0, 0, "", nil
	}
	mime := f.mime
	if mime == "" {
		mime = "image/webp"
	}
	return []byte("webp-bytes-" + string(rune('0'+len(f.calls)))), maxSide, maxSide / 2, mime, nil
}

func TestEnsureDerivativesSequential(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()
	session := mustCreateSession(t, ctx, store, "ws_a", 7)
	_, asset, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{})
	if err != nil {
		t.Fatal(err)
	}

	objects := &fakeDerivativeObjects{objects: map[string][]byte{asset.StorageKey: []byte("png-bytes")}}
	renderer := &fakeRenderer{}
	records, err := EnsureDerivatives(ctx, store, objects, renderer, "ws_a", asset)
	if err != nil {
		t.Fatalf("EnsureDerivatives failed: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3 display variants", len(records))
	}
	// Sequential in TierSpecs order: thumb 256, preview 640, large-preview 1280.
	wantSides := []int{256, 640, 1280}
	if len(renderer.calls) != len(wantSides) {
		t.Fatalf("render calls = %v, want %v", renderer.calls, wantSides)
	}
	for i, want := range wantSides {
		if renderer.calls[i] != want {
			t.Fatalf("render calls = %v, want %v", renderer.calls, wantSides)
		}
		if records[i].VariantName != []string{"thumb", "preview", "large-preview"}[i] {
			t.Fatalf("record %d variant = %q", i, records[i].VariantName)
		}
		if !strings.HasPrefix(records[i].StorageKey, "ws-uploads/") {
			t.Fatalf("record %d key = %q, want ws-uploads slot", i, records[i].StorageKey)
		}
	}
	listed, err := store.ListDerivatives(ctx, "ws_a", asset.ID)
	if err != nil || len(listed) != 3 {
		t.Fatalf("ListDerivatives = %+v %v", listed, err)
	}

	// Regeneration converges via upsert (no duplicate rows).
	again, err := EnsureDerivatives(ctx, store, objects, renderer, "ws_a", asset)
	if err != nil || len(again) != 3 {
		t.Fatalf("regenerate = %+v %v", again, err)
	}
	listed, err = store.ListDerivatives(ctx, "ws_a", asset.ID)
	if err != nil || len(listed) != 3 {
		t.Fatalf("ListDerivatives after regen = %+v %v", listed, err)
	}
}

func TestEnsureDerivativesSkips(t *testing.T) {
	ctx := context.Background()
	store := newMemoryTestStore()

	// Non-image assets are a no-op.
	if out, err := EnsureDerivatives(ctx, store, &fakeDerivativeObjects{}, &fakeRenderer{}, "ws_a", &Asset{ID: "asset_x", MediaKind: "video"}); err != nil || out != nil {
		t.Fatalf("non-image = %+v %v, want no-op", out, err)
	}
	// Missing original bytes fail with the typed 409-class error.
	session := mustCreateSession(t, ctx, store, "ws_a", 7)
	_, asset, err := store.CompleteSession(ctx, "ws_a", session.ID, completeFacts(), CreateAssetParams{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = EnsureDerivatives(ctx, store, &fakeDerivativeObjects{missing: true}, &fakeRenderer{}, "ws_a", asset)
	if !errors.Is(err, ErrDerivativeSourceMissing) {
		t.Fatalf("missing source err = %v, want derivative source missing", err)
	}
	// Render failures name the variant and keep earlier tiers.
	objects := &fakeDerivativeObjects{objects: map[string][]byte{asset.StorageKey: []byte("png-bytes")}}
	_, err = EnsureDerivatives(ctx, store, objects, &fakeRenderer{failOn: 2}, "ws_a", asset)
	if err == nil || !strings.Contains(err.Error(), "preview") {
		t.Fatalf("render failure err = %v, want preview attribution", err)
	}
	listed, err := store.ListDerivatives(ctx, "ws_a", asset.ID)
	if err != nil || len(listed) != 1 || listed[0].VariantName != "thumb" {
		t.Fatalf("partial derivatives = %+v %v, want [thumb]", listed, err)
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
	if _, err := adapter.GetAsset(ctx, "ws_a", "asset_missing0001"); !errors.Is(err, workspacetasks.ErrMediaNotFound) {
		t.Fatalf("unknown err = %v, want media not found", err)
	}
	if _, err := adapter.GetAsset(ctx, "ws_b", asset.ID); !errors.Is(err, workspacetasks.ErrMediaNotFound) {
		t.Fatalf("cross-workspace err = %v, want media not found", err)
	}
	if _, err := store.ArchiveAsset(ctx, "ws_a", asset.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetAsset(ctx, "ws_a", asset.ID); !errors.Is(err, workspacetasks.ErrMediaArchived) {
		t.Fatalf("archived err = %v, want media archived", err)
	}
	var nilAdapter *TaskAssetAdapter
	if _, err := nilAdapter.GetAsset(ctx, "ws_a", asset.ID); !errors.Is(err, workspacetasks.ErrMediaNotFound) {
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
	store.SeedTaskAssetForTests("ws_a", "task_1", &TaskAsset{Asset: asset, OutputIndex: 0, Role: "result"})

	items, err := store.ListTaskAssets(ctx, "ws_a", "task_1")
	if err != nil || len(items) != 1 || items[0].Asset.ID != asset.ID || items[0].Role != "result" {
		t.Fatalf("task assets = %+v %v", items, err)
	}
	empty, err := store.ListTaskAssets(ctx, "ws_a", "task_missing")
	if err != nil || len(empty) != 0 {
		t.Fatalf("unknown task assets = %+v %v, want empty", empty, err)
	}
}
