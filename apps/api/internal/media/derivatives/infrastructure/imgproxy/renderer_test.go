package imgproxy_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
)

type activeProfile struct {
	profile *storagedomain.StorageProfile
}

func (p *activeProfile) ResolveActiveProfile(context.Context) (*storagedomain.StorageProfile, error) {
	return p.profile, nil
}

func rendererConfig(baseURL string) imgproxy.Config {
	return imgproxy.Config{BaseURL: baseURL, KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
}

func TestRendererWorkspaceImage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"Cookie", "Authorization", "X-Control-Key"} {
			if r.Header.Get(name) != "" {
				t.Errorf("credentials forwarded: %s", name)
			}
		}
		if !strings.Contains(r.URL.Path, "/rs:fit:256:256/plain/local:///uploads/ws/test/image.png@webp") {
			t.Errorf("unexpected render path: %s", r.URL.Path)
		}
		cfg := rendererConfig("")
		signature, path, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		wantSignature, err := cfg.SignPath("/" + path)
		if !ok || err != nil || signature != wantSignature {
			t.Error("invalid render signature")
		}
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte("rendered image"))
	}))
	defer upstream.Close()
	image, err := imgproxy.NewRenderer(rendererConfig(upstream.URL), nil).WorkspaceTier(context.Background(), "uploads/ws/test/image.png", "thumb")
	if err != nil || image.MIMEType != "image/webp" || string(image.Data) != "rendered image" {
		t.Fatalf("image = %#v, error = %v", image, err)
	}
}

func TestRendererRejectsInvalidResponses(t *testing.T) {
	var redirectCalls atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectCalls.Add(1) }))
	defer redirectTarget.Close()
	for _, tc := range []struct {
		name   string
		status int
		mime   string
		length int64
		body   string
		want   error
	}{
		{"redirect", 302, "image/webp", 0, "", imgproxy.ErrUnavailable},
		{"html", 200, "text/html", 0, "upstream details", imgproxy.ErrUnavailable},
		{"oversize", 200, "image/webp", 100*1024*1024 + 1, "", imgproxy.ErrUnavailable},
		{"truncated", 200, "image/webp", 100, "short", imgproxy.ErrUnavailable},
		{"empty", 200, "image/webp", 0, "", imgproxy.ErrUnavailable},
		{"missing", 404, "text/plain", 0, "private source key", imgproxy.ErrNotFound},
		{"failed", 500, "text/plain", 0, "private upstream error", imgproxy.ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.mime)
				if tc.length > 0 {
					w.Header().Set("Content-Length", strconv.FormatInt(tc.length, 10))
				}
				w.Header().Set("Location", redirectTarget.URL)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()
			image, err := imgproxy.NewRenderer(rendererConfig(upstream.URL), nil).WorkspaceTier(context.Background(), "uploads/ws/test/image.png", "thumb")
			if !errors.Is(err, tc.want) || len(image.Data) != 0 {
				t.Fatalf("image = %#v, error = %v, want %v", image, err, tc.want)
			}
		})
	}
	if redirectCalls.Load() != 0 {
		t.Fatal("renderer redirect was followed")
	}
}

func TestRendererCancellation(t *testing.T) {
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := imgproxy.NewRenderer(rendererConfig(upstream.URL), nil).WorkspaceTier(ctx, "uploads/ws/test/image.png", "thumb")
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, imgproxy.ErrUnavailable) {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestRendererResolvesActiveProfileForEachPublicImage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer upstream.Close()
	profiles := &activeProfile{}
	renderer := imgproxy.NewRenderer(rendererConfig(upstream.URL), profiles)
	for _, tc := range []struct {
		name    string
		profile *storagedomain.StorageProfile
		source  string
	}{
		{"local", &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal, Enabled: true}, "local:///"},
		{"s3", &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderS3Compatible, Enabled: true, Config: map[string]interface{}{"bucket_name": "public-images"}}, "s3://public-images/"},
		{"disabled", &storagedomain.StorageProfile{Provider: storagedomain.StorageProviderLocal}, ""},
		{"missing", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profiles.profile = tc.profile
			for _, render := range []func() (imgproxy.Image, error){
				func() (imgproxy.Image, error) {
					return renderer.Avatar(context.Background(), "uploads/avatars/test/image.png", 64)
				},
				func() (imgproxy.Image, error) {
					return renderer.Logo(context.Background(), "uploads/site/assets/logo.png")
				},
			} {
				image, err := render()
				if tc.source == "" {
					if !errors.Is(err, imgproxy.ErrSource) {
						t.Fatalf("source error = %v", err)
					}
				} else if err != nil || !strings.Contains(string(image.Data), "/plain/"+tc.source+"uploads/") {
					t.Fatalf("source = %q, error = %v", image.Data, err)
				}
			}
			image, err := renderer.WorkspaceTier(context.Background(), "uploads/ws/test/image.png", "thumb")
			if err != nil || !strings.Contains(string(image.Data), "/plain/local:///uploads/ws/") {
				t.Fatalf("workspace source = %q, error = %v", image.Data, err)
			}
		})
	}
}
