package domain

import (
	"testing"
)

func validAssetInput() AssetInput {
	return AssetInput{
		Provider:    ProviderUnsplash,
		SourceID:    "photo-123",
		SourceURL:   "https://unsplash.com/photos/photo-123",
		PreviewURL:  "https://images.unsplash.com/photo-123?w=800",
		OriginalURL: "https://images.unsplash.com/photo-123",
		Title:       "A photo",
		Author:      "Author",
		Width:       1200,
		Height:      800,
	}
}

func TestNormalizeAndValidateAssetNormalizesInput(t *testing.T) {
	input := validAssetInput()
	input.Provider = " UNSPLASH "
	input.SourceID = " photo-123 "
	input.SourceURL = " https://unsplash.com/photos/photo-123 "
	got, err := NormalizeAndValidateAsset(input)
	if err != nil {
		t.Fatalf("NormalizeAndValidateAsset returned error: %v", err)
	}
	if got.Provider != ProviderUnsplash || got.SourceID != "photo-123" || got.SourceURL != "https://unsplash.com/photos/photo-123" {
		t.Fatalf("normalized asset = %#v", got)
	}
}

func TestNormalizeAndValidateAssetRejectsUntrustedURLs(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "http", url: "http://images.unsplash.com/photo-123"},
		{name: "userinfo", url: "https://user:pass@images.unsplash.com/photo-123"},
		{name: "lookalike", url: "https://images.unsplash.com.attacker.example/photo-123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validAssetInput()
			input.PreviewURL = tc.url
			if _, err := NormalizeAndValidateAsset(input); err != ErrInvalidSource {
				t.Fatalf("error = %v, want %v", err, ErrInvalidSource)
			}
		})
	}
}

func TestNormalizeAndValidateAssetRejectsUnsupportedProvider(t *testing.T) {
	input := validAssetInput()
	input.Provider = "google"
	if _, err := NormalizeAndValidateAsset(input); err != ErrInvalidProvider {
		t.Fatalf("error = %v, want %v", err, ErrInvalidProvider)
	}
}

func TestNormalizeFolderName(t *testing.T) {
	if got, err := NormalizeFolderName("  Travel  "); err != nil || got != "Travel" {
		t.Fatalf("NormalizeFolderName = %q, %v", got, err)
	}
	for _, raw := range []string{"", "   "} {
		if _, err := NormalizeFolderName(raw); err != ErrInvalidFolderName {
			t.Fatalf("NormalizeFolderName(%q) error = %v, want %v", raw, err, ErrInvalidFolderName)
		}
	}
}

func TestParseFolderSelection(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want FolderSelection
	}{
		{name: "default all", raw: "", want: FolderSelection{Kind: FolderSelectionAll}},
		{name: "all", raw: "all", want: FolderSelection{Kind: FolderSelectionAll}},
		{name: "quick saves", raw: "quick-saves", want: FolderSelection{Kind: FolderSelectionQuickSaves}},
		{name: "user folder", raw: "42", want: FolderSelection{Kind: FolderSelectionUser, ID: 42}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFolderSelection(tc.raw)
			if err != nil {
				t.Fatalf("ParseFolderSelection returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("selection = %#v, want %#v", got, tc.want)
			}
		})
	}
	for _, raw := range []string{"0", "-1", "wat"} {
		if _, err := ParseFolderSelection(raw); err != ErrFolderNotFound {
			t.Fatalf("ParseFolderSelection(%q) error = %v, want %v", raw, err, ErrFolderNotFound)
		}
	}
}
