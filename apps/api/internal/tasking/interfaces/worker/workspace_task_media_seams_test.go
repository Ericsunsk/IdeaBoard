package worker

// Production media seam tests: the allowlisted result fetcher and the
// bounded derivative renderer behind the worker finalize path. All bytes
// are local fixtures served by httptest; no secrets, no live network.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// pngHitServer serves the fixture PNG and counts inbound requests so
// reject-path tests can prove a non-allowlisted URL is never fetched.
func pngHitServer(t *testing.T, hits *int64, payload []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
}

func fetcherTestRef(rawURL string) workspacetasks.OutputRef {
	return workspacetasks.OutputRef{
		Kind: workspacetasks.OutputKindRemote, URL: rawURL,
		Role: "result", ResultIndex: 0,
	}
}

// Allowlisted host (exact host:port from the executor policy) downloads.
func TestWorkspaceTaskResultFetcherAllowsListedHost(t *testing.T) {
	png := fixturePNGBytes(t)
	var hits int64
	server := pngHitServer(t, &hits, png)
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	ctx := taskruntime.WithWorkspaceResultAllowHosts(context.Background(), []string{parsed.Host})
	data, err := (taskruntime.WorkspaceTaskResultBytesFetcher{}).FetchResultBytes(ctx, rendererTestTask(), fetcherTestRef(server.URL+"/result.png"))
	if err != nil {
		t.Fatalf("FetchResultBytes: %v", err)
	}
	if !bytes.Equal(data, png) {
		t.Fatalf("fetched %d bytes, want the %d fixture bytes", len(data), len(png))
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

// Bare-host entries match any port (outbound policy normalization parity).
func TestWorkspaceTaskResultFetcherAllowsBareHostEntry(t *testing.T) {
	png := fixturePNGBytes(t)
	var hits int64
	server := pngHitServer(t, &hits, png)
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	ctx := taskruntime.WithWorkspaceResultAllowHosts(context.Background(), []string{parsed.Hostname()})
	if _, err := (taskruntime.WorkspaceTaskResultBytesFetcher{}).FetchResultBytes(ctx, rendererTestTask(), fetcherTestRef(server.URL+"/result.png")); err != nil {
		t.Fatalf("FetchResultBytes: %v", err)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

// Non-allowlisted hosts fail closed with an explicit error and are never
// fetched (hits stay 0); the error names the host but echoes no path.
func TestWorkspaceTaskResultFetcherRejectsNonAllowlistedHost(t *testing.T) {
	png := fixturePNGBytes(t)
	var hits int64
	server := pngHitServer(t, &hits, png)
	defer server.Close()
	ctx := taskruntime.WithWorkspaceResultAllowHosts(context.Background(), []string{"cdn.example.com"})
	_, err := (taskruntime.WorkspaceTaskResultBytesFetcher{}).FetchResultBytes(ctx, rendererTestTask(), fetcherTestRef(server.URL+"/result.png"))
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("err = %v, want explicit not-allowlisted failure", err)
	}
	if strings.Contains(err.Error(), "/result.png") {
		t.Fatalf("err echoes the URL path: %v", err)
	}
	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Fatalf("server hits = %d, want 0 (never fetched)", got)
	}
}

type emptyOutboundPolicy struct{}

func (emptyOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{}, nil
}

// No allowlist anywhere (empty ctx override + empty outbound policy)
// fails closed before any network.
func TestWorkspaceTaskResultFetcherFailsClosedWithoutAllowlist(t *testing.T) {
	restore := outbound.ConfigurePolicyProvider(emptyOutboundPolicy{})
	defer restore()
	var hits int64
	server := pngHitServer(t, &hits, fixturePNGBytes(t))
	defer server.Close()
	_, err := (taskruntime.WorkspaceTaskResultBytesFetcher{}).FetchResultBytes(context.Background(), rendererTestTask(), fetcherTestRef(server.URL+"/result.png"))
	if err == nil || !strings.Contains(err.Error(), "allowlist is not configured") {
		t.Fatalf("err = %v, want fail-closed allowlist failure", err)
	}
	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Fatalf("server hits = %d, want 0 (never fetched)", got)
	}
}

// Closed-world matching unit table: exact-host semantics, entry shapes,
// and fail-closed inputs. No network.
func TestWorkspaceTaskResultHostAllowlistMatching(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rawURL     string
		allowHosts []string
		wantErr    string
	}{
		{name: "exact host", rawURL: "https://cdn.example.com/a.png", allowHosts: []string{"cdn.example.com"}},
		{name: "exact host:port", rawURL: "https://cdn.example.com:8443/a.png", allowHosts: []string{"cdn.example.com:8443"}},
		{name: "port mismatch fails", rawURL: "https://cdn.example.com:8443/a.png", allowHosts: []string{"cdn.example.com:9443"}, wantErr: "not allowlisted"},
		{name: "bare host matches explicit port", rawURL: "https://cdn.example.com:8443/a.png", allowHosts: []string{"cdn.example.com"}},
		{name: "case insensitive", rawURL: "https://CDN.Example.COM/a.png", allowHosts: []string{"cdn.example.com"}},
		{name: "full URL entry shape", rawURL: "https://cdn.example.com/a.png", allowHosts: []string{"https://cdn.example.com/some/path"}},
		{name: "full URL default port is exact", rawURL: "https://cdn.example.com:8443/a.png", allowHosts: []string{"https://cdn.example.com/some/path"}, wantErr: "not allowlisted"},
		{name: "lookalike host fails", rawURL: "https://cdn.example.com.attacker.example/a.png", allowHosts: []string{"cdn.example.com"}, wantErr: "not allowlisted"},
		{name: "unlisted host fails", rawURL: "https://other.example.org/a.png", allowHosts: []string{"cdn.example.com"}, wantErr: "not allowlisted"},
		{name: "empty allowlist fails closed", rawURL: "https://cdn.example.com/a.png", allowHosts: nil, wantErr: "not configured"},
		{name: "blank entries do not count", rawURL: "https://cdn.example.com/a.png", allowHosts: []string{"  ", ""}, wantErr: "not configured"},
		{name: "empty URL", rawURL: "", allowHosts: []string{"cdn.example.com"}, wantErr: "is empty"},
		{name: "non-http scheme fails", rawURL: "ftp://cdn.example.com/a.png", allowHosts: []string{"cdn.example.com"}, wantErr: "is not allowed"},
		{name: "data URL fails", rawURL: "data:image/png;base64,abc", allowHosts: []string{"cdn.example.com"}, wantErr: "invalid"},
		{name: "userinfo fails", rawURL: "https://user:pass@cdn.example.com/a.png", allowHosts: []string{"cdn.example.com"}, wantErr: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := taskruntime.CheckWorkspaceResultHostAllowlist(tc.rawURL, tc.allowHosts)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("check = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("check = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// largeFixturePNG encodes a deterministic WxH PNG for scaling assertions.
func largeFixturePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	return buf.Bytes()
}

// The production renderer honors tier geometry through the domain TierSpecs
// path: downscales to maxSide, never upscales, deterministic PNG output.
func TestWorkspaceTaskDerivativeRendererTiers(t *testing.T) {
	ctx := context.Background()
	source := largeFixturePNG(t, 800, 600)
	for _, tier := range []string{derivativedomain.TierThumb, derivativedomain.TierPreview, derivativedomain.TierLargePreview} {
		spec := derivativedomain.TierSpecs[tier]
		rendered, width, height, mimeType, err := taskruntime.WorkspaceTaskDerivativeRenderer{}.Render(ctx, source, "fixture", spec.MaxSide)
		if err != nil {
			t.Fatalf("%s: Render: %v", tier, err)
		}
		if mimeType != "image/png" {
			t.Fatalf("%s: mime = %q, want image/png", tier, mimeType)
		}
		if !bytes.HasPrefix(rendered, []byte{0x89, 0x50, 0x4E, 0x47}) {
			t.Fatalf("%s: output lacks the PNG magic", tier)
		}
		decoded, err := png.Decode(bytes.NewReader(rendered))
		if err != nil {
			t.Fatalf("%s: decode rendered: %v", tier, err)
		}
		bounds := decoded.Bounds()
		if bounds.Dx() != width || bounds.Dy() != height {
			t.Fatalf("%s: dims = %dx%d, want reported %dx%d", tier, bounds.Dx(), bounds.Dy(), width, height)
		}
		if width > spec.MaxSide || height > spec.MaxSide {
			t.Fatalf("%s: dims = %dx%d exceed maxSide %d", tier, width, height, spec.MaxSide)
		}
	}
	// Spot geometry: 800x600 at thumb (256) scales to 256x192; at
	// large-preview (1280) the source is smaller, so no upscale.
	rendered, width, height, _, err := taskruntime.WorkspaceTaskDerivativeRenderer{}.Render(ctx, source, "fixture", derivativedomain.TierSpecs[derivativedomain.TierThumb].MaxSide)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}
	if width != 256 || height != 192 || len(rendered) == 0 {
		t.Fatalf("thumb = %dx%d (%d bytes), want 256x192 non-empty", width, height, len(rendered))
	}
	rendered, width, height, _, err = taskruntime.WorkspaceTaskDerivativeRenderer{}.Render(ctx, source, "fixture", derivativedomain.TierSpecs[derivativedomain.TierLargePreview].MaxSide)
	if err != nil {
		t.Fatalf("large-preview: %v", err)
	}
	if width != 800 || height != 600 || len(rendered) == 0 {
		t.Fatalf("large-preview = %dx%d (%d bytes), want 800x600 non-empty", width, height, len(rendered))
	}
}

// The production renderer drives the existing sequential EnsureDerivatives
// path end to end: three display tiers recorded for an image asset.
func TestWorkspaceTaskDerivativeRendererViaEnsureDerivatives(t *testing.T) {
	ctx := context.Background()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeRendererObjects{}
	source := largeFixturePNG(t, 800, 600)
	sum := fixtureChecksum(source)
	session, err := media.CreateSession(ctx, workspacemedia.CreateSessionParams{
		WorkspaceID: "ws_test", OwnerUserID: 7, Filename: "source.png",
		MIMEType: "image/png", SizeBytes: int64(len(source)),
		ChecksumSHA256: sum, MediaKind: workspacemedia.MediaKindImage,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := objects.PutObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, session.StorageKey, source, "image/png"); err != nil {
		t.Fatalf("stage source bytes: %v", err)
	}
	_, asset, err := media.CompleteSession(ctx, "ws_test", session.ID, workspacemedia.CompleteFacts{
		ChecksumSHA256: sum, SizeBytes: int64(len(source)),
		MIMEType: "image/png", MediaKind: workspacemedia.MediaKindImage, Width: 800, Height: 600,
	}, workspacemedia.CreateAssetParams{})
	if err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}
	derivatives, err := workspacemedia.EnsureDerivatives(ctx, media, objects, taskruntime.WorkspaceTaskDerivativeRenderer{}, "ws_test", asset)
	if err != nil {
		t.Fatalf("EnsureDerivatives: %v", err)
	}
	if len(derivatives) != 3 {
		t.Fatalf("derivatives = %d, want 3 tiers", len(derivatives))
	}
	for _, variant := range []string{derivativedomain.TierThumb, derivativedomain.TierPreview, derivativedomain.TierLargePreview} {
		found := false
		for _, record := range derivatives {
			if record.VariantName == variant {
				found = true
				if record.MIMEType != "image/png" || record.SizeBytes <= 0 {
					t.Fatalf("%s record = %+v, want png with bytes", variant, record)
				}
			}
		}
		if !found {
			t.Fatalf("tier %q missing from %+v", variant, derivatives)
		}
	}
}

// Renderer fail-closed inputs stay deterministic (empty source, garbage).
func TestWorkspaceTaskDerivativeRendererRejectsBadSource(t *testing.T) {
	ctx := context.Background()
	if _, _, _, _, err := (taskruntime.WorkspaceTaskDerivativeRenderer{}).Render(ctx, nil, "empty", 256); err == nil {
		t.Fatal("empty source = nil, want error")
	}
	if _, _, _, _, err := (taskruntime.WorkspaceTaskDerivativeRenderer{}).Render(ctx, []byte("not-an-image"), "bad", 256); err == nil {
		t.Fatal("garbage source = nil, want error")
	}
}
