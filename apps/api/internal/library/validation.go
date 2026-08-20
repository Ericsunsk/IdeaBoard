package library

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

const maxFolderNameRunes = 100

func normalizeFolderName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxFolderNameRunes {
		return "", ErrInvalidFolderName
	}
	return name, nil
}

func validateOwner(ownerUserID int64) error {
	if ownerUserID <= 0 {
		return ErrInvalidOwner
	}
	return nil
}

func normalizeAndValidateAsset(input AssetInput) (AssetInput, error) {
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.PreviewURL = strings.TrimSpace(input.PreviewURL)
	input.OriginalURL = strings.TrimSpace(input.OriginalURL)
	input.Title = strings.TrimSpace(input.Title)
	input.Author = strings.TrimSpace(input.Author)
	if input.Provider != ProviderUnsplash && input.Provider != ProviderPexels {
		return AssetInput{}, ErrInvalidProvider
	}
	if input.SourceID == "" || utf8.RuneCountInString(input.SourceID) > 128 ||
		input.SourceURL == "" || input.PreviewURL == "" || input.OriginalURL == "" {
		return AssetInput{}, ErrInvalidSource
	}
	if utf8.RuneCountInString(input.Title) > 255 || utf8.RuneCountInString(input.Author) > 255 ||
		input.Width < 0 || input.Height < 0 {
		return AssetInput{}, ErrInvalidSource
	}
	for _, rawURL := range []string{input.SourceURL, input.PreviewURL, input.OriginalURL} {
		if !isAllowedProviderURL(input.Provider, rawURL) {
			return AssetInput{}, ErrInvalidSource
		}
	}
	return input, nil
}

func isAllowedProviderURL(provider, raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	allowed := []string(nil)
	switch provider {
	case ProviderUnsplash:
		allowed = []string{"unsplash.com", "images.unsplash.com", "source.unsplash.com"}
	case ProviderPexels:
		allowed = []string{"pexels.com", "images.pexels.com", "videos.pexels.com"}
	}
	for _, domain := range allowed {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
