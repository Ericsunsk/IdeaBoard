package storage

import (
	"net/url"
	"strings"
)

const kodoThumbnailQuery = "imageView2/2/w/640/interlace/1/q/75"

func (s *StorageService) PreviewURLTransformer() func(string) string {
	identity := func(raw string) string { return raw }

	s.previewMu.RLock()
	if !s.previewDirty && s.previewTransformer != nil {
		cached := s.previewTransformer
		s.previewMu.RUnlock()
		return cached
	}
	s.previewMu.RUnlock()

	profile, err := s.resolvePreviewProfile()
	if err != nil {
		return identity
	}
	if profile == nil {
		s.cachePreviewURLTransformer(identity)
		return identity
	}
	if profile.Provider != StorageProviderKodo {
		s.cachePreviewURLTransformer(identity)
		return identity
	}

	cfg := s.kodoConfigFromProfile(profile)
	if cfg == nil || !cfg.ThumbnailMode {
		s.cachePreviewURLTransformer(identity)
		return identity
	}

	accessHost := extractHost(cfg.AccessDomain)
	if accessHost == "" {
		s.cachePreviewURLTransformer(identity)
		return identity
	}

	transformer := func(raw string) string {
		return appendKodoThumbnailQuery(raw, accessHost)
	}
	s.cachePreviewURLTransformer(transformer)
	return transformer
}

func appendKodoThumbnailQuery(raw, accessHost string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	if !strings.EqualFold(parsed.Hostname(), accessHost) {
		return raw
	}

	query := parsed.RawQuery
	if strings.Contains(query, "imageView2") || strings.Contains(query, "imageMogr2") {
		return raw
	}
	if hasKodoPrivateSignature(query) {
		return raw
	}

	if query == "" {
		parsed.RawQuery = kodoThumbnailQuery
	} else {
		parsed.RawQuery = query + "&" + kodoThumbnailQuery
	}
	return parsed.String()
}

func hasKodoPrivateSignature(rawQuery string) bool {
	if strings.TrimSpace(rawQuery) == "" {
		return false
	}
	for _, pair := range strings.Split(rawQuery, "&") {
		key := pair
		if idx := strings.Index(key, "="); idx >= 0 {
			key = key[:idx]
		}
		key = strings.TrimSpace(key)
		if strings.EqualFold(key, "token") || strings.EqualFold(key, "e") {
			return true
		}
	}
	return false
}

func extractHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.TrimSpace(parsed.Hostname())
}

func (s *StorageService) resolvePreviewProfile() (*StorageProfile, error) {
	if s.previewResolver != nil {
		return s.previewResolver()
	}
	return s.resolveActiveProfile()
}

func (s *StorageService) cachePreviewURLTransformer(transformer func(string) string) {
	if transformer == nil {
		transformer = func(raw string) string { return raw }
	}
	s.previewMu.Lock()
	s.previewTransformer = transformer
	s.previewDirty = false
	s.previewMu.Unlock()
}

func (s *StorageService) invalidatePreviewURLTransformerCache() {
	s.previewMu.Lock()
	s.previewTransformer = nil
	s.previewDirty = true
	s.previewMu.Unlock()
}
