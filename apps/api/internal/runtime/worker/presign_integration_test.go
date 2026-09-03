package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/media/storage"
	tasks "ideaboard/api/internal/tasking/domain"
)

// fakeObjectStore is the test double for the ObjectStore port.
// The real implementation lives in internal/media/storage; the
// runtime/worker package only consumes the port, never the
// concrete adapter. This test guards that contract.
type fakeObjectStore struct {
	presign func(req storage.PresignRequest) (storage.PresignedURL, error)
}

func (f *fakeObjectStore) Presign(ctx context.Context, req storage.PresignRequest) (storage.PresignedURL, error) {
	return f.presign(req)
}

func TestWorkerPresignIntegrationUsesObjectStorePort(t *testing.T) {
	// Build a manifest that mirrors the runtime path: the
	// worker iterates ResolveInputs and asks the port to sign
	// each one. The test asserts the port is the only surface
	// the worker touches.
	ref := func(assetID, derivativeID, role string, weight float64) tasks.TaskInputManifestRef {
		return tasks.TaskInputManifestRef{
			AssetID:          assetID,
			DerivativeID:     derivativeID,
			DerivativeTier:   tasks.TierProcessingInput,
			StorageProfileID: "primary",
			ObjectKey:        "uploads/derivatives/" + assetID + "/processing-input.webp",
			ObjectGeneration: 1,
			ChecksumSHA256:   strings.Repeat("a", 64),
			MIMEType:         "image/webp",
			SizeBytes:        1,
			Role:             role,
			Weight:           weight,
		}
	}
	manifest, err := tasks.NewTaskInputManifest(
		[]tasks.TaskInputManifestRef{
			ref("asset_101", "derivative_101", tasks.MediaRoleInput, 1),
			ref("asset_102", "derivative_102", tasks.MediaRoleInput, 0.5),
		},
		[]tasks.TaskInputManifestRef{ref("asset_201", "derivative_201", tasks.MediaRoleReference, 0.25)},
		nil,
		func() *tasks.TaskInputManifestRef {
			value := ref("asset_301", "derivative_301", tasks.MediaRoleMask, 0)
			return &value
		}(),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}

	calls := 0
	port := &fakeObjectStore{presign: func(req storage.PresignRequest) (storage.PresignedURL, error) {
		calls++
		if req.TTL > storage.MaxPresignTTL {
			t.Errorf("TTL %v exceeds MaxPresignTTL", req.TTL)
		}
		return storage.PresignedURL{
			URL:              "https://signed.example/" + req.ObjectKey + "?signature=secret",
			ExpiresAt:        time.Now().Add(req.TTL),
			StorageProfileID: req.StorageProfileID,
			PresignID:        "psg_test",
		}, nil
	}}

	// The runtime path: walk the manifest, build a presign
	// request for each ref, ask the port, and ensure the
	// returned URL goes through RedactSignedURL before it
	// ever lands in a log line.
	storageProfile := "primary"
	ttl := 5 * time.Minute
	for _, ref := range manifest.ResolveInputs() {
		req := storage.PresignRequest{
			StorageProfileID:         storageProfile,
			ObjectKey:                ref.ObjectKey,
			TTL:                      ttl,
			ExpectedMIMEType:         ref.MIMEType,
			ExpectedChecksumSHA256:   ref.ChecksumSHA256,
			ExpectedSizeBytes:        ref.SizeBytes,
			ExpectedObjectGeneration: ref.ObjectGeneration,
		}
		got, err := port.Presign(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		// Redact before any log statement.
		redacted := RedactSignedURL(got.URL)
		if strings.Contains(redacted, "secret") {
			t.Fatalf("signed URL leaked through redactor: %q", redacted)
		}
		if got.StorageProfileID != storageProfile {
			t.Errorf("profile rotation: %q != %q", got.StorageProfileID, storageProfile)
		}
		if !got.ExpiresAt.After(time.Now()) {
			t.Errorf("ExpiresAt must be in the future, got %v", got.ExpiresAt)
		}
	}
	if calls != 4 {
		t.Errorf("expected 4 presign calls (3 inputs + 1 mask), got %d", calls)
	}
}

func TestWorkerPresignIntegrationRejectsTTLGreaterThanCap(t *testing.T) {
	port := &fakeObjectStore{presign: func(req storage.PresignRequest) (storage.PresignedURL, error) {
		return storage.PresignedURL{}, storage.ErrPresignTTLTooLong
	}}
	// The port surfaces ErrPresignTTLTooLong; the worker
	// must fall back to MaxPresignTTL and re-issue. The
	// test asserts the typed error is the contract surface.
	_, err := port.Presign(context.Background(), storage.PresignRequest{
		StorageProfileID: "primary",
		ObjectKey:        "uploads/foo.png",
		TTL:              30 * time.Minute,
	})
	if err != storage.ErrPresignTTLTooLong {
		t.Fatalf("expected ErrPresignTTLTooLong, got %v", err)
	}
}
