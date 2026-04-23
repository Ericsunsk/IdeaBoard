package storage

import (
	"net/url"
	"strings"
)

const kodoThumbnailQuery = "imageView2/2/w/640/interlace/1/q/75"

func (s *StorageService) PreviewURLTransformer() func(string) string {
	identity := func(raw string) string { return raw }

	profile, err := s.resolveActiveProfile()
	if err != nil || profile == nil {
		return identity
	}
	if profile.Provider != StorageProviderKodo {
		return identity
	}

	cfg := s.kodoConfigFromProfile(profile)
	if cfg == nil || !cfg.ThumbnailMode {
		return identity
	}

	accessHost := extractHost(cfg.AccessDomain)
	if accessHost == "" {
		return identity
	}

	return func(raw string) string {
		return appendKodoThumbnailQuery(raw, accessHost)
	}
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

	if query == "" {
		parsed.RawQuery = kodoThumbnailQuery
	} else {
		parsed.RawQuery = query + "&" + kodoThumbnailQuery
	}
	return parsed.String()
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
