package searchcatalog

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNormalizeQueryTrimsAndCollapsesWhitespace(t *testing.T) {
	got, err := normalizeQuery("  warm\t shadows\n  ")
	if err != nil || got != "warm shadows" {
		t.Fatalf("normalizeQuery() = %q, %v", got, err)
	}
}

func TestPrometheusSearchMetricsRecordSafeFields(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewPrometheusMetrics(registry)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}
	metrics.RecordSearch(SearchObservation{
		Duration:    150 * time.Millisecond,
		Page:        "first",
		Outcome:     "partial",
		ResultCount: 3,
		ProviderStatus: []ProviderStatus{{
			Provider:        ProviderUnsplash,
			Status:          "failed",
			FailureCategory: FailureNetwork,
			LatencyMS:       42,
		}},
	})
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, family := range families {
		if strings.Contains(family.GetName(), "api_key") || strings.Contains(family.GetName(), "payload") {
			t.Fatalf("metrics expose a secret-bearing field: %s", family.GetName())
		}
	}
}

func TestNormalizeQueryRejectsOverlongInput(t *testing.T) {
	if _, err := normalizeQuery(strings.Repeat("x", MaxSearchQueryRunes+1)); err == nil {
		t.Fatal("normalizeQuery should reject input over 200 runes")
	}
}

func TestDedupeResultsUsesProviderAndStableSourceIdentity(t *testing.T) {
	results := dedupeResults([]Result{
		{ID: "1", Provider: ProviderUnsplash, ThumbnailURL: "thumb", PreviewURL: "preview", SourceURL: "source"},
		{ID: "1", Provider: ProviderUnsplash, ThumbnailURL: "thumb-2", PreviewURL: "preview-2", SourceURL: "source-2"},
		{ID: "1", Provider: ProviderPexels, ThumbnailURL: "thumb", PreviewURL: "preview", SourceURL: "source"},
	})
	if len(results) != 2 {
		t.Fatalf("dedupeResults() length = %d, want 2", len(results))
	}
}

func TestCursorRoundTrip(t *testing.T) {
	want := searchCursor{Unsplash: 3, Pexels: 4, PexelsDone: true}
	got, err := decodeCursor(encodeCursor(want))
	if err != nil || got != want {
		t.Fatalf("cursor round trip = %#v, %v; want %#v", got, err, want)
	}
}

func TestNextCursorStopsCallingExhaustedProvider(t *testing.T) {
	got := nextCursor(
		searchCursor{Unsplash: 1, Pexels: 1},
		[]Provider{ProviderUnsplash, ProviderPexels},
		[]ProviderStatus{{Provider: ProviderUnsplash, Status: "ok"}, {Provider: ProviderPexels, Status: "ok"}},
		map[Provider]adapterPage{
			ProviderUnsplash: {HasMore: false},
			ProviderPexels:   {HasMore: true},
		},
	)
	if !got.UnsplashDone || got.Unsplash != 1 || got.PexelsDone || got.Pexels != 2 {
		t.Fatalf("nextCursor() = %#v, want exhausted Unsplash and advanced Pexels", got)
	}
}

func TestFilterProviderResultsRejectsUntrustedURLs(t *testing.T) {
	results := filterProviderResults(ProviderUnsplash, []Result{
		{ID: "safe", ThumbnailURL: "https://images.unsplash.com/thumb", PreviewURL: "https://images.unsplash.com/preview", SourceURL: "https://unsplash.com/photos/safe", Width: 100, Height: 100},
		{ID: "unsafe", ThumbnailURL: "https://evil.example/thumb", PreviewURL: "https://images.unsplash.com/preview", SourceURL: "https://unsplash.com/photos/unsafe", Width: 100, Height: 100},
	})
	if len(results) != 1 || results[0].ID != "safe" {
		t.Fatalf("filterProviderResults() = %#v, want only safe result", results)
	}
}

func TestSearchGracefullyDegradesWhenNoProvidersConfigured(t *testing.T) {
	service := NewSearchService(nil)
	resp, err := service.Search(t.Context(), "minimal art", "", 24)
	if err != nil {
		t.Fatalf("expected nil error on unconfigured providers, got: %v", err)
	}
	if resp.Query != "minimal art" {
		t.Fatalf("expected query 'minimal art', got: %q", resp.Query)
	}
	if len(resp.Results) != 0 {
		t.Fatalf("expected empty results, got %d items", len(resp.Results))
	}
	if resp.HasMore {
		t.Fatalf("expected has_more=false, got true")
	}
}
