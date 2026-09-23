package workspaceuploads

import (
	"context"
	"strings"
	"testing"
	"time"
)

const testChecksum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestValidateCreateInput(t *testing.T) {
	valid := func() (string, string, int64, string, string) {
		return "photo.png", "image/png", 1024, testChecksum, MediaKindImage
	}
	if err := ValidateCreateInput(valid()); err != nil {
		t.Fatalf("valid input = %v", err)
	}
	cases := []struct {
		name   string
		mutate func(filename, mime *string, size *int64, checksum, kind *string)
		field  string
	}{
		{"blank filename", func(f, m *string, s *int64, c, k *string) { *f = "  " }, "filename"},
		{"path filename", func(f, m *string, s *int64, c, k *string) { *f = "a/b.png" }, "filename"},
		{"bad mime", func(f, m *string, s *int64, c, k *string) { *m = "application/pdf" }, "mime_type"},
		{"zero size", func(f, m *string, s *int64, c, k *string) { *s = 0 }, "size_bytes"},
		{"oversize", func(f, m *string, s *int64, c, k *string) { *s = MaxUploadSizeBytes + 1 }, "size_bytes"},
		{"bad checksum", func(f, m *string, s *int64, c, k *string) { *c = "abc" }, "checksum_sha256"},
		{"bad kind", func(f, m *string, s *int64, c, k *string) { *k = "video" }, "media_kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filename, mime, size, checksum, kind := valid()
			tc.mutate(&filename, &mime, &size, &checksum, &kind)
			err := ValidateCreateInput(filename, mime, size, checksum, kind)
			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("err = %v, want *ValidationError", err)
			}
			if _, ok := validationErr.Fields[tc.field]; !ok {
				t.Fatalf("fields = %v, want %q", validationErr.Fields, tc.field)
			}
		})
	}
}

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()
	now := time.Now().UTC()
	store.SetClockForTests(func() time.Time { return now })

	created, err := store.CreateSession(ctx, CreateSessionParams{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Filename: "a.png",
		MIMEType: "image/png", SizeBytes: 10, ChecksumSHA256: testChecksum, MediaKind: MediaKindImage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.ID, "upload_") || created.Status != StatusPending {
		t.Fatalf("created = %+v", created)
	}
	if created.MIMEType != "image/png" || created.ChecksumSHA256 != testChecksum {
		t.Fatalf("canonicalization = %+v", created)
	}

	// First read signals transfer start: pending -> transferring with a
	// refreshed (longer) expiry.
	read, err := store.GetSession(ctx, "ws_abc", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Status != StatusTransferring {
		t.Fatalf("after read status = %q, want transferring", read.Status)
	}
	if !read.ExpiresAt.After(created.ExpiresAt) {
		t.Fatal("transferring expiry must extend past the pending TTL")
	}

	completed, err := store.CompleteSession(ctx, "ws_abc", created.ID, "asset_xyz")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted || completed.AssetID != "asset_xyz" {
		t.Fatalf("completed = %+v", completed)
	}
	if _, err := store.CompleteSession(ctx, "ws_abc", created.ID, "asset_other"); err != ErrStateConflict {
		t.Fatalf("double complete = %v, want ErrStateConflict", err)
	}

	// Cross-workspace ids are invisible.
	if _, err := store.GetSession(ctx, "ws_other", created.ID); err != ErrNotFound {
		t.Fatalf("cross-workspace read = %v, want ErrNotFound", err)
	}

	// Failure path.
	failing, err := store.CreateSession(ctx, CreateSessionParams{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Filename: "b.png",
		MIMEType: "image/png", SizeBytes: 10, ChecksumSHA256: testChecksum, MediaKind: MediaKindImage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FailSession(ctx, "ws_abc", failing.ID, "boom"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteSession(ctx, "ws_abc", failing.ID, "asset_x"); err != ErrStateConflict {
		t.Fatalf("complete after fail = %v, want ErrStateConflict", err)
	}
}

func TestSessionLazyExpiry(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()
	now := time.Now().UTC()
	store.SetClockForTests(func() time.Time { return now })

	pending, err := store.CreateSession(ctx, CreateSessionParams{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Filename: "a.png",
		MIMEType: "image/png", SizeBytes: 10, ChecksumSHA256: testChecksum, MediaKind: MediaKindImage,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Past the pending TTL without any read: lazy-expire on read.
	now = now.Add(UploadPendingTTL + time.Minute)
	if _, err := store.GetSession(ctx, "ws_abc", pending.ID); err != ErrExpired {
		t.Fatalf("expired read = %v, want ErrExpired", err)
	}
	// Terminal: completing an expired session conflicts.
	if _, err := store.CompleteSession(ctx, "ws_abc", pending.ID, "asset_x"); err != ErrExpired {
		t.Fatalf("complete expired = %v, want ErrExpired", err)
	}

	// Transferring sessions get the longer transfer TTL.
	store2 := NewInMemorySessionStore()
	base := time.Now().UTC()
	current := base
	store2.SetClockForTests(func() time.Time { return current })
	active, err := store2.CreateSession(ctx, CreateSessionParams{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Filename: "a.png",
		MIMEType: "image/png", SizeBytes: 10, ChecksumSHA256: testChecksum, MediaKind: MediaKindImage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store2.GetSession(ctx, "ws_abc", active.ID); err != nil {
		t.Fatal(err)
	}
	current = base.Add(UploadPendingTTL + time.Minute)
	if _, err := store2.GetSession(ctx, "ws_abc", active.ID); err != nil {
		t.Fatalf("transferring within transfer TTL = %v, want nil", err)
	}
	current = base.Add(UploadTransferTTL + time.Minute)
	if _, err := store2.GetSession(ctx, "ws_abc", active.ID); err != ErrExpired {
		t.Fatalf("transferring past transfer TTL = %v, want ErrExpired", err)
	}
}

func TestAssetStore(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryAssetStore()

	first, err := store.CreateAsset(ctx, CreateAssetParams{
		WorkspaceID: "ws_abc", OwnerUserID: 7, MediaKind: MediaKindImage,
		MIMEType: "image/png", Width: 4, Height: 3, SizeBytes: 10,
		ChecksumSHA256: testChecksum, StorageProfileID: DefaultStorageProfile, StorageKey: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.ID, "asset_") || first.Status != StatusPending {
		t.Fatalf("created = %+v", first)
	}
	second, err := store.CreateAsset(ctx, CreateAssetParams{
		WorkspaceID: "ws_abc", OwnerUserID: 7, MediaKind: MediaKindImage,
		MIMEType: "image/png", SizeBytes: 11,
		ChecksumSHA256: testChecksum, StorageProfileID: DefaultStorageProfile, StorageKey: "k2",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.GetAsset(ctx, "ws_abc", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StorageKey == "" || got.StorageProfileID == "" {
		t.Fatalf("server-side locator must be retained: %+v", got)
	}
	if _, err := store.GetAsset(ctx, "ws_other", first.ID); err != ErrNotFound {
		t.Fatalf("cross-workspace asset = %v, want ErrNotFound", err)
	}

	// Newest-first ordering.
	items, err := store.ListAssets(ctx, "ws_abc", 10, 0)
	if err != nil || len(items) != 2 || items[0].ID != second.ID {
		t.Fatalf("list = %v, %v", items, err)
	}

	// Archive is idempotent and hides from the list while staying
	// readable by id with the lifecycle marker.
	archived, err := store.ArchiveAsset(ctx, "ws_abc", first.ID)
	if err != nil || !archived.IsArchived() {
		t.Fatalf("archive = %v, %v", archived, err)
	}
	again, err := store.ArchiveAsset(ctx, "ws_abc", first.ID)
	if err != nil || again.ArchivedAt.UnixNano() != archived.ArchivedAt.UnixNano() {
		t.Fatalf("re-archive = %v, %v", again, err)
	}
	items, err = store.ListAssets(ctx, "ws_abc", 10, 0)
	if err != nil || len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("list after archive = %v, %v", items, err)
	}
	if _, err := store.ArchiveAsset(ctx, "ws_other", second.ID); err != ErrNotFound {
		t.Fatalf("cross-workspace archive = %v, want ErrNotFound", err)
	}
}

func TestSlotKeyNeverLeaksBucket(t *testing.T) {
	key := SlotKey("ws_abc123", "upload_def456", "photo.png")
	if strings.Contains(key, "bucket") || strings.Contains(key, "secret") {
		t.Fatalf("slot key = %q", key)
	}
	if !strings.HasPrefix(key, "ws-uploads/abc123/def456/") {
		t.Fatalf("slot key = %q, want namespaced path", key)
	}
	if got := SanitizeFilename("../../etc/passwd"); strings.Contains(got, "/") {
		t.Fatalf("sanitized = %q", got)
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	raw := EncodeAssetMetadata("ws_abc")
	if DecodeAssetWorkspace(raw) != "ws_abc" {
		t.Fatalf("workspace = %q", DecodeAssetWorkspace(raw))
	}
	if DecodeAssetArchivedAt(raw) != nil {
		t.Fatal("fresh metadata must carry no archive marker")
	}
	if DecodeAssetWorkspace([]byte(`{}`)) != "" {
		t.Fatal("legacy rows without binding must decode empty (invisible)")
	}
	if DecodeAssetWorkspace([]byte(`nope`)) != "" {
		t.Fatal("corrupt metadata must decode empty (invisible)")
	}
}
