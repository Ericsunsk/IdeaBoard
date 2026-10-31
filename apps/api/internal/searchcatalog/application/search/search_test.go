package search

import (
	"strings"
	"testing"

	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
)

func TestNormalizeQueryTrimsAndCollapsesWhitespace(t *testing.T) {
	got, err := normalizeQuery("  warm\t shadows\n  ")
	if err != nil || got != "warm shadows" {
		t.Fatalf("normalizeQuery() = %q, %v", got, err)
	}
}

func TestNormalizeQueryRejectsOverlongInput(t *testing.T) {
	if _, err := normalizeQuery(strings.Repeat("x", MaxSearchQueryRunes+1)); err == nil {
		t.Fatal("normalizeQuery should reject input over 200 runes")
	}
}

func TestDedupeResultsUsesProviderAndStableSourceIdentity(t *testing.T) {
	results := dedupeResults([]domain.Result{
		{ID: "1", Provider: domain.ProviderUnsplash, ThumbnailURL: "thumb", PreviewURL: "preview", SourceURL: "source"},
		{ID: "1", Provider: domain.ProviderUnsplash, ThumbnailURL: "thumb-2", PreviewURL: "preview-2", SourceURL: "source-2"},
		{ID: "1", Provider: domain.ProviderPexels, ThumbnailURL: "thumb", PreviewURL: "preview", SourceURL: "source"},
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
		[]domain.Provider{domain.ProviderUnsplash, domain.ProviderPexels},
		[]domain.ProviderStatus{{Provider: domain.ProviderUnsplash, Status: "ok"}, {Provider: domain.ProviderPexels, Status: "ok"}},
		map[domain.Provider]ports.SearchProviderPage{
			domain.ProviderUnsplash: {HasMore: false},
			domain.ProviderPexels:   {HasMore: true},
		},
	)
	if !got.UnsplashDone || got.Unsplash != 1 || got.PexelsDone || got.Pexels != 2 {
		t.Fatalf("nextCursor() = %#v, want exhausted Unsplash and advanced Pexels", got)
	}
}

func TestFilterProviderResultsRejectsUntrustedURLs(t *testing.T) {
	results := filterProviderResults(domain.ProviderUnsplash, []domain.Result{
		{ID: "safe", ThumbnailURL: "https://images.unsplash.com/thumb", PreviewURL: "https://images.unsplash.com/preview", SourceURL: "https://unsplash.com/photos/safe", Width: 100, Height: 100},
		{ID: "unsafe", ThumbnailURL: "https://evil.example/thumb", PreviewURL: "https://images.unsplash.com/preview", SourceURL: "https://unsplash.com/photos/unsafe", Width: 100, Height: 100},
	})
	if len(results) != 1 || results[0].ID != "safe" {
		t.Fatalf("filterProviderResults() = %#v, want only safe result", results)
	}
}

func TestSearchGracefullyDegradesWhenNoProvidersConfigured(t *testing.T) {
	service := NewSearchService(nil, nil)
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
