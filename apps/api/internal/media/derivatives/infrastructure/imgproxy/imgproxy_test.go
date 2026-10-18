package imgproxy

import (
	"context"
	"errors"
	"ideaboard/api/internal/media/derivatives/domain"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSignPathMatchesDocumentedVector(t *testing.T) {
	cfg := Config{BaseURL: "http://imgproxy.example.com", KeyHex: "736563726574", SaltHex: "68656C6C6F"}
	got, err := cfg.SignPath("/rs:fill:300:400:0/g:sm/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdl/cy9jdXJpb3NpdHku/anBn.png")
	if err != nil {
		t.Fatal(err)
	}
	if want := "oKfUtW34Dvo2BGQehJFR4Nr0_rIjOtdtzJ3QFsUcXH8"; got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
}

func TestTierURLGeometry(t *testing.T) {
	cfg := Config{BaseURL: "http://127.0.0.1:8080", KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
	url, err := cfg.TierURL(domain.TierThumb, "local:///uploads/tasks/u/b/content/image.png", time.Unix(1893456000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:8080/") || !strings.Contains(url, "/rs:fit:256:256/plain/local:///uploads/tasks/u/b/content/image.png@webp") {
		t.Fatalf("unexpected tier URL %s", url)
	}
	if _, err := cfg.TierURL("nope", "local:///x.png", time.Unix(1893456000, 0)); err == nil {
		t.Fatal("unknown tier should fail")
	}
}

func TestTierURLRequiresExpiry(t *testing.T) {
	cfg := Config{BaseURL: "http://127.0.0.1:8080", KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
	expiry := time.Unix(1893456000, 0).UTC()
	url, err := cfg.TierURL(domain.TierThumb, "local:///uploads/tasks/u/b/content/image.png", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "/exp:1893456000/rs:fit:256:256/plain/local:///uploads/tasks/u/b/content/image.png@webp") {
		t.Fatalf("expiring tier URL missing exp segment: %s", url)
	}
	if _, err := cfg.TierURL(domain.TierThumb, "local:///uploads/tasks/u/b/content/image.png", time.Time{}); err == nil {
		t.Fatal("zero tier expiry should fail")
	}
}

func TestAvatarURLGeometry(t *testing.T) {
	cfg := Config{BaseURL: "http://127.0.0.1:8080", KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
	if DefaultAvatarSize != 64 {
		t.Fatalf("DefaultAvatarSize = %d, want 64", DefaultAvatarSize)
	}
	url, err := cfg.AvatarURL(64, "local:///uploads/avatars/AVTR01/abcdef1234567890.png", time.Unix(1893456000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:8080/") || !strings.Contains(url, "/rs:fill:64:64/plain/local:///uploads/avatars/AVTR01/abcdef1234567890.png@webp") {
		t.Fatalf("unexpected avatar URL %s", url)
	}
	if _, err := cfg.AvatarURL(64, "local:///uploads/avatars/AVTR01/abcdef1234567890.png", time.Time{}); err == nil {
		t.Fatal("zero avatar expiry should fail")
	}
	expiry := time.Unix(1893456000, 0).UTC()
	expiring, err := cfg.AvatarURL(128, "local:///uploads/avatars/AVTR01/abcdef1234567890.png", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(expiring, "/exp:1893456000/rs:fill:128:128/plain/local:///uploads/avatars/AVTR01/abcdef1234567890.png@webp") {
		t.Fatalf("expiring avatar URL missing exp segment: %s", expiring)
	}
	for _, size := range []int{0, 16, 48, 100, 256 + 1} {
		if _, err := cfg.AvatarURL(size, "local:///uploads/avatars/AVTR01/abcdef1234567890.png", expiry); err == nil {
			t.Fatalf("avatar size %d should fail", size)
		}
	}
	if _, err := cfg.AvatarURL(64, "  ", expiry); err == nil {
		t.Fatal("blank avatar source should fail")
	}
}

func TestValidateRejectsWeakBindings(t *testing.T) {
	base := Config{BaseURL: "http://127.0.0.1:8080", KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"empty base":  func(c *Config) { c.BaseURL = "" },
		"bad scheme":  func(c *Config) { c.BaseURL = "ftp://x" },
		"short key":   func(c *Config) { c.KeyHex = "ab" },
		"bad key hex": func(c *Config) { c.KeyHex = "zz" },
		"short salt":  func(c *Config) { c.SaltHex = "cd" },
	} {
		cfg := base
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("%s should fail validation", name)
		}
	}
}

func TestS3Source(t *testing.T) {
	// R2/S3-profile keys carry no local mount: the bucket comes from the
	// resolved storage profile and the key keeps the slot shape.
	got, err := S3Source("media-private", "uploads/ws/abc123/def456/photo.png")
	if err != nil || got != "s3://media-private/uploads/ws/abc123/def456/photo.png" {
		t.Fatalf("got %q err %v", got, err)
	}
	got, err = S3Source("  media-private ", "tenant-9/photo.png")
	if err != nil || got != "s3://media-private/tenant-9/photo.png" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := S3Source("  ", "a/b.png"); err == nil {
		t.Fatal("blank bucket should fail")
	}
	for _, tc := range []struct {
		name   string
		bucket string
		key    string
	}{
		{"bucket with slash", "a/b", "a/b.png"},
		{"dot bucket", ".", "a/b.png"},
		{"dotdot bucket", "..", "a/b.png"},
		{"blank key", "media-private", "  "},
		{"leading slash", "media-private", "/tenant-9/photo.png"},
		{"slash-only key", "media-private", "/"},
		{"empty segment", "media-private", "a//b.png"},
		{"dot segment", "media-private", "a/./b.png"},
		{"dotdot segment", "media-private", "a/../b.png"},
		{"trailing slash", "media-private", "a/b/"},
	} {
		if _, err := S3Source(tc.bucket, tc.key); err == nil {
			t.Fatalf("%s should fail (bucket %q key %q)", tc.name, tc.bucket, tc.key)
		}
	}
}

func TestLocalSource(t *testing.T) {
	// Builtin-local keys carry the uploads root, which is mounted below /data
	// by Compose and therefore remains part of the imgproxy source path.
	got, err := LocalSource("uploads/tasks/u/b/content/image.png")
	if err != nil || got != "local:///uploads/tasks/u/b/content/image.png" {
		t.Fatalf("got %q err %v", got, err)
	}
	// Canonical workspace slot root (single local keyspace): same
	// local:/// form, same traversal discipline.
	got, err = LocalSource("uploads/ws/abc123/def456/photo.png")
	if err != nil || got != "local:///uploads/ws/abc123/def456/photo.png" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := LocalSource("  "); err == nil {
		t.Fatal("blank key should fail")
	}
	for _, key := range []string{
		"uploads/",
		"uploads///",
		"uploads/.",
		"uploads/./image.png",
		"uploads/../image.png",
		"ws-uploads/abc123/def456/photo.png",
		"ws-uploads/",
		"ws-uploads/a/../../x.png",
		"media/image.png",
		"etc/passwd",
	} {
		if _, err := LocalSource(key); err == nil {
			t.Fatalf("invalid local source key %q should fail", key)
		}
	}
}

// merged from renderer_body_test.go (renderer.go aggregation: Renderer transport/cancellation body handling).
type canceledImageBody struct {
	cancel context.CancelFunc
}

func (b canceledImageBody) Read([]byte) (int, error) {
	b.cancel()
	return 0, context.Canceled
}

func (canceledImageBody) Close() error { return nil }

type imageTransport func(*http.Request) (*http.Response, error)

func (f imageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRendererCancellationWhileReadingImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	renderer := &Renderer{client: &http.Client{Transport: imageTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/webp"}},
			Body:       canceledImageBody{cancel: cancel},
		}, nil
	})}}
	if _, err := renderer.fetch(ctx, "http://renderer/image"); !errors.Is(err, context.Canceled) {
		t.Fatalf("body read error = %v, want context.Canceled", err)
	}
}

func TestRendererTransportTimeoutIsUnavailable(t *testing.T) {
	renderer := &Renderer{client: &http.Client{Transport: imageTransport(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})}}
	if _, err := renderer.fetch(context.Background(), "http://renderer/image"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("transport timeout error = %v, want ErrUnavailable", err)
	}
}
