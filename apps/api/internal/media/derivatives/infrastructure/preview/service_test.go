package preview

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/storage/infrastructure/remote"
)

func previewFixture(t *testing.T, status int) (*Service, *int) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "uploads")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		_, source, found := strings.Cut(req.URL.Path, "/plain/local:///uploads/")
		if !found || !strings.HasSuffix(source, "@webp") {
			t.Error("unexpected local render source")
			w.WriteHeader(500)
			return
		}
		name := strings.TrimSuffix(source, "@webp")
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != "preview source" {
			t.Error("staging source unavailable")
		}
		w.Header().Set("Content-Type", "image/webp")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("rendered preview"))
	}))
	t.Cleanup(upstream.Close)
	config := imgproxy.Config{BaseURL: upstream.URL, KeyHex: strings.Repeat("ab", 32), SaltHex: strings.Repeat("cd", 32)}
	service, err := NewService(config, root, imgproxy.NewRenderer(config, nil))
	if err != nil {
		t.Fatal(err)
	}
	service.download = func(ctx context.Context, raw string, hosts []string, max int64) (*remote.ImageFile, error) {
		if raw != "https://images.unsplash.com/截屏%20one.png?width=1200" || len(hosts) != 2 || max != maxSourceBytes {
			t.Error("source identity or fetch policy changed")
		}
		file, err := os.CreateTemp(t.TempDir(), "source-*")
		if err != nil {
			return nil, err
		}
		if _, err = file.WriteString("preview source"); err != nil {
			t.Fatal(err)
		}
		if _, err = file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		return &remote.ImageFile{File: file, Path: file.Name()}, nil
	}
	return service, &requests
}

func TestPreviewCapabilityLifecycle(t *testing.T) {
	s, requests := previewFixture(t, http.StatusOK)
	now := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	display := s.SearchDisplay("unsplash", "source-one", "https://images.unsplash.com/截屏%20one.png?width=1200")
	if display == nil || len(display.Variants) != 3 || *requests != 0 {
		t.Fatal("descriptor must not fetch sources")
	}
	if display.ExpiresAt != "2026-10-16T01:00:00Z" {
		t.Fatal("wrong descriptor expiry")
	}
	for _, variant := range display.Variants {
		token := strings.Split(variant.URL, "/")[4]
		if strings.Contains(token, "unsplash") {
			t.Fatal("source leaked in token")
		}
		image, expires, err := s.RenderCapability(context.Background(), token, variant.Tier)
		if err != nil || string(image.Data) != "rendered preview" || !expires.Equal(now.Add(capabilityLifetime)) {
			t.Fatalf("render failed: %v", err)
		}
		entries, _ := os.ReadDir(s.root)
		if len(entries) != 0 {
			t.Fatal("staging source retained after render")
		}
		if _, _, err = s.RenderCapability(context.Background(), token+"x", variant.Tier); !errors.Is(err, ErrCapability) {
			t.Fatal("tampered capability accepted")
		}
	}
	if *requests != 3 {
		t.Fatal("unexpected renderer request count")
	}
	now = now.Add(capabilityLifetime)
	token := strings.Split(display.Variants[0].URL, "/")[4]
	if _, _, err := s.RenderCapability(context.Background(), token, "thumb"); !errors.Is(err, ErrCapability) {
		t.Fatal("expired capability accepted")
	}
}

func TestPreviewSourceValidationBeforeFetch(t *testing.T) {
	s, requests := previewFixture(t, http.StatusOK)
	for _, raw := range []string{"http://images.unsplash.com/a", "https://images.unsplash.com:444/a", "https://images.unsplash.com.evil.test/a", "https://user@images.unsplash.com/a", "https://images.unsplash.com/a#fragment", "https://127.0.0.1/a", "https://unsplash.com/a"} {
		if s.SearchDisplay("unsplash", "id", raw) != nil {
			t.Errorf("untrusted source accepted: %s", raw)
		}
		if _, err := s.RenderSource(context.Background(), "unsplash", raw, "thumb"); !errors.Is(err, imgproxy.ErrSource) {
			t.Fatal("invalid source reached fetcher")
		}
	}
	for _, tier := range []string{"processing-input", "original", "huge", ""} {
		if _, err := s.RenderSource(context.Background(), "unsplash", "https://images.unsplash.com/截屏%20one.png?width=1200", tier); !errors.Is(err, imgproxy.ErrSource) {
			t.Fatal("invalid tier reached fetcher")
		}
	}
	if *requests != 0 {
		t.Fatal("invalid requests reached renderer")
	}
}

func TestPreviewCleanupOnRenderFailureAndCancellation(t *testing.T) {
	s, _ := previewFixture(t, http.StatusForbidden)
	if _, err := s.RenderSource(context.Background(), "unsplash", "https://images.unsplash.com/截屏%20one.png?width=1200", "thumb"); !errors.Is(err, imgproxy.ErrUnavailable) {
		t.Fatal("expected renderer failure")
	}
	entries, _ := os.ReadDir(s.root)
	if len(entries) != 0 {
		t.Fatal("staging leaked on renderer failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.RenderSource(ctx, "unsplash", "https://images.unsplash.com/截屏%20one.png?width=1200", "thumb"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
	entries, _ = os.ReadDir(s.root)
	if len(entries) != 0 {
		t.Fatal("staging leaked on cancellation")
	}
}

func TestPreviewCapabilityCannotCrossSigningConfig(t *testing.T) {
	s, _ := previewFixture(t, http.StatusOK)
	display := s.SearchDisplay("unsplash", "id", "https://images.unsplash.com/截屏%20one.png?width=1200")
	token := strings.Split(display.Variants[0].URL, "/")[4]
	cfg := imgproxy.Config{BaseURL: "http://unused.test", KeyHex: strings.Repeat("ef", 32), SaltHex: strings.Repeat("cd", 32)}
	other, err := NewService(cfg, t.TempDir(), imgproxy.NewRenderer(cfg, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = other.RenderCapability(context.Background(), token, "thumb"); !errors.Is(err, ErrCapability) {
		t.Fatal("other key accepted capability")
	}
}
