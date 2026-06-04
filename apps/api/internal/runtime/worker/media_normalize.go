package worker

import (
	"api/internal/platform/media"
	"api/internal/platform/mediarefs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strings"

	"api/internal/storage"
	"api/internal/workflow"
)

func resolvePublicAssetBaseURL() *url.URL {
	for _, key := range []string{"APP_PUBLIC_ASSET_BASE_URL", "APP_PUBLIC_BASE_URL"} {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			slog.Warn("ignore invalid public asset base URL", "key", key, "value", raw)
			continue
		}
		return parsed
	}
	return nil
}

func inferLocalAssetMimeType(filename string, data []byte) string {
	if len(data) > 0 {
		if ext := media.DetectImageExtensionFromBytes(data); ext != "" {
			if mimeType := media.ImageMIMEFromExtension(ext); mimeType != "" {
				return mimeType
			}
		}
	}
	if ext, ok := media.NormalizeImageExtFromFilename(filename); ok {
		if mimeType := media.ImageMIMEFromExtension(ext); mimeType != "" {
			return mimeType
		}
	}
	return "image/png"
}

func absolutizeAssetURL(base *url.URL, raw string) (string, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "data:") {
		return trimmed, false, false
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return trimmed, false, false
	}
	if !strings.HasPrefix(trimmed, "/") {
		return trimmed, false, false
	}
	if strings.HasPrefix(trimmed, "/uploads/") {
		cleaned := path.Clean(trimmed)
		if !strings.HasPrefix(cleaned, "/uploads/") {
			return trimmed, false, true
		}
		if _, err := storage.ParseUploadResource(cleaned); err != nil {
			return trimmed, false, true
		}
	}
	if base == nil {
		return trimmed, false, true
	}
	rel, err := url.Parse(trimmed)
	if err != nil {
		return trimmed, false, true
	}
	return base.ResolveReference(rel).String(), true, false
}

func (c *ProviderExecutor) normalizeUpstreamMediaParams(params map[string]interface{}) {
	if len(params) == 0 {
		return
	}

	base := resolvePublicAssetBaseURL()
	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")
	converted := 0
	unresolvedKeys := map[string]struct{}{}

	normalizeStringKey := func(key string) {
		raw, ok := params[key]
		if !ok {
			return
		}
		value, ok := raw.(string)
		if !ok {
			return
		}
		normalized, changed, unresolved := absolutizeAssetURL(base, value)
		if changed {
			params[key] = normalized
			converted++
			return
		}
		if unresolved {
			unresolvedKeys[key] = struct{}{}
		}
	}

	normalizeListKey := func(key string) {
		raw, ok := params[key]
		if !ok {
			return
		}
		switch values := raw.(type) {
		case []string:
			updated := false
			for index, item := range values {
				normalized, changed, unresolved := absolutizeAssetURL(base, item)
				if changed {
					values[index] = normalized
					updated = true
					converted++
					continue
				}
				if unresolved {
					unresolvedKeys[key] = struct{}{}
				}
			}
			if updated {
				params[key] = values
			}
		case []interface{}:
			updated := false
			for index, item := range values {
				text, ok := item.(string)
				if !ok {
					continue
				}
				normalized, changed, unresolved := absolutizeAssetURL(base, text)
				if changed {
					values[index] = normalized
					updated = true
					converted++
					continue
				}
				if unresolved {
					unresolvedKeys[key] = struct{}{}
				}
			}
			if updated {
				params[key] = values
			}
		}
	}

	for _, key := range mediarefs.CanonicalStringKeys() {
		normalizeStringKey(key)
	}
	for _, key := range mediarefs.CanonicalListKeys() {
		normalizeListKey(key)
	}

	if converted > 0 {
		slog.Info("normalized upstream media URLs", "feature", featureSlug, "converted", converted)
	}
	if len(unresolvedKeys) > 0 {
		keys := make([]string, 0, len(unresolvedKeys))
		for key := range unresolvedKeys {
			keys = append(keys, key)
		}
		slog.Warn("upstream media URLs remain relative; configure APP_PUBLIC_ASSET_BASE_URL", "feature", featureSlug, "keys", strings.Join(keys, ","))
	}
}
