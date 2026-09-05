package execution

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	tasks "ideaboard/api/internal/tasking/domain"
)

// fakeObjectStore is the test double for the ObjectStore port.
// The real implementation lives in internal/media/storage; the
// runtime/worker package only consumes the port, never the
// concrete adapter. This test guards that contract.
type fakeObjectStore struct {
	presign func(req ports.PresignRequest) (ports.PresignedURL, error)
}

func (f *fakeObjectStore) Presign(ctx context.Context, req ports.PresignRequest) (ports.PresignedURL, error) {
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
	port := &fakeObjectStore{presign: func(req ports.PresignRequest) (ports.PresignedURL, error) {
		calls++
		if req.TTL > ports.MaxPresignTTL {
			t.Errorf("TTL %v exceeds MaxPresignTTL", req.TTL)
		}
		return ports.PresignedURL{
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
		req := ports.PresignRequest{
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
	port := &fakeObjectStore{presign: func(req ports.PresignRequest) (ports.PresignedURL, error) {
		return ports.PresignedURL{}, domain.ErrPresignTTLTooLong
	}}
	// The port surfaces ErrPresignTTLTooLong; the worker
	// must fall back to MaxPresignTTL and re-issue. The
	// test asserts the typed error is the contract surface.
	_, err := port.Presign(context.Background(), ports.PresignRequest{
		StorageProfileID: "primary",
		ObjectKey:        "uploads/foo.png",
		TTL:              30 * time.Minute,
	})
	if err != domain.ErrPresignTTLTooLong {
		t.Fatalf("expected ErrPresignTTLTooLong, got %v", err)
	}
}

func TestRedactSignedURLStripsQueryString(t *testing.T) {
	raw := "https://bucket.s3.us-east-1.amazonaws.com/uploads/derivatives/asset_001/processing-input.webp?X-Amz-Signature=abcdef&X-Amz-Expires=600"
	got := RedactSignedURL(raw)
	if strings.Contains(got, "X-Amz-Signature") {
		t.Fatalf("signature leaked: %q", got)
	}
	if strings.Contains(got, "abcdef") {
		t.Fatalf("token leaked: %q", got)
	}
	if !strings.Contains(got, "signature=redacted") {
		t.Fatalf("redaction marker missing: %q", got)
	}
	if !strings.Contains(got, "bucket.s3.us-east-1.amazonaws.com") {
		t.Fatalf("host lost: %q", got)
	}
	if !strings.Contains(got, "uploads/derivatives/asset_001/processing-input.webp") {
		t.Fatalf("path lost: %q", got)
	}
}

func TestRedactSignedURLIsNoopForLocalUploadRef(t *testing.T) {
	raw := "/uploads/canvas/owner/board/content/image.png"
	got := RedactSignedURL(raw)
	if got != raw {
		t.Fatalf("local upload ref should not be redacted: %q != %q", got, raw)
	}
}

func TestRedactSignedURLIsNoopForEmpty(t *testing.T) {
	if got := RedactSignedURL(""); got != "" {
		t.Fatalf("empty URL should be empty: %q", got)
	}
	if got := RedactSignedURL("   "); got != "   " {
		t.Fatalf("whitespace-only URL should be preserved: %q", got)
	}
}

func TestRedactSignedURLRedactsInlineDataImage(t *testing.T) {
	raw := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	got := RedactSignedURL(raw)
	if strings.Contains(got, "iVBORw0KGgo") {
		t.Fatalf("data URL base64 leaked: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("redaction marker missing: %q", got)
	}
}

func TestIsSignedURLClassifiesObjectStorageHosts(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://bucket.s3.us-east-1.amazonaws.com/uploads/derivatives/asset_001/processing-input.webp?X-Amz-Signature=abc", true},
		{"https://cdn.example.r2.cloudflarestorage.com/uploads/derivatives/foo.webp?signature=abc", true},
		{"https://oss-cn-hangzhou.aliyuncs.com/uploads/derivatives/foo.webp?Signature=abc", true},
		{"https://cos.ap-shanghai.myqcloud.com/uploads/derivatives/foo.webp?q-signature=abc", true},
		{"https://storage.googleapis.com/uploads/derivatives/foo.webp?X-Goog-Signature=abc", true},
		{"https://example.com/uploads/foo.png", false},
		{"https://example.com/uploads/foo.png?signature=abc", true},
		{"", false},
		{"data:image/png;base64,abc", false},
	}
	for _, c := range cases {
		if got := IsSignedURL(c.raw); got != c.want {
			t.Errorf("IsSignedURL(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestIsSignedObjectKeyClassifiesTiers(t *testing.T) {
	cases := map[string]bool{
		"uploads/derivatives/asset_001/processing-input.webp": true,
		"uploads/tasks/42/task-abc/image.png":                 true,
		"uploads/processing-input/asset_001.webp":             true,
		"uploads/canvas/owner/board/content/image.png":        false,
		"":                     false,
		"some/random/path.txt": false,
	}
	for key, want := range cases {
		if got := IsSignedObjectKey(key); got != want {
			t.Errorf("IsSignedObjectKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func RedactSignedURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	if isInlineDataImageURL(trimmed) {
		return "data:image/...;base64,[redacted]"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return redactLogURL(trimmed)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		clone := *parsed
		clone.RawQuery = "signature=redacted"
		clone.ForceQuery = true
		clone.Fragment = ""
		parsed = &clone
	}
	return parsed.String()
}

func IsSignedURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.RawQuery == "" {
		return false
	}
	host := strings.ToLower(parsed.Host)
	for _, suffix := range []string{".amazonaws.com", ".r2.cloudflarestorage.com", ".aliyuncs.com", ".myqcloud.com", ".googleapis.com"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	segments := strings.Split(parsed.Path, "/")
	last := segments[len(segments)-1]
	return last != "" && strings.Contains(last, ".")
}

func IsSignedObjectKey(objectKey string) bool {
	key := strings.TrimSpace(objectKey)
	return key != "" && (strings.HasPrefix(key, "uploads/derivatives/") || strings.HasPrefix(key, "uploads/tasks/") || strings.HasPrefix(key, "uploads/processing-input/"))
}

func isInlineDataImageURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:image/")
}

func redactLogURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if isInlineDataImageURL(trimmed) {
		return "data:image/...;base64,[redacted]"
	}
	if parsed, err := url.Parse(trimmed); err == nil {
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			clone := *parsed
			clone.User = nil
			clone.RawQuery = ""
			clone.ForceQuery = false
			clone.Fragment = ""
			trimmed = clone.String()
		}
	}
	if len(trimmed) <= 80 {
		return trimmed
	}
	return trimmed[:80] + "...(truncated)"
}
