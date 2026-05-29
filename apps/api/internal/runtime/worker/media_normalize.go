package worker

import (
	"api/internal/platform/media"
	"api/internal/platform/mediarefs"
	"encoding/base64"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"api/internal/workflow"
)

func resolvePublicAssetBaseURL() *url.URL {
	for _, key := range []string{"APP_PUBLIC_ASSET_BASE_URL", "APP_ASSET_BASE_URL", "PUBLIC_ASSET_BASE_URL", "APP_PUBLIC_BASE_URL", "APP_BASE_URL", "PUBLIC_BASE_URL"} {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			log.Printf("[Worker] 忽略无效的公网基址配置: %s=%q", key, raw)
			continue
		}
		return parsed
	}
	return nil
}

func resolveLocalUploadFilePath(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if !strings.HasPrefix(cleaned, "/uploads/") {
		return "", false
	}
	relative := filepath.FromSlash(strings.TrimPrefix(cleaned, "/"))
	candidates := []string{
		relative,
		filepath.Join("apps", "api", relative),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate, true
	}
	return "", false
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

func inlineLocalAssetDataURL(raw string) (string, bool, error) {
	localPath, ok := resolveLocalUploadFilePath(raw)
	if !ok {
		return "", false, nil
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", true, err
	}
	mimeType := inferLocalAssetMimeType(localPath, data)
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), true, nil
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
	inlined := 0
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
			if inlinedValue, handled, err := inlineLocalAssetDataURL(value); handled {
				if err != nil {
					log.Printf("[Worker] 本地媒体内联失败: feature=%s key=%s err=%v", featureSlug, key, err)
				} else {
					params[key] = inlinedValue
					inlined++
					return
				}
			}
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
					if inlinedValue, handled, err := inlineLocalAssetDataURL(item); handled {
						if err != nil {
							log.Printf("[Worker] 本地媒体内联失败: feature=%s key=%s index=%d err=%v", featureSlug, key, index, err)
						} else {
							values[index] = inlinedValue
							updated = true
							inlined++
							continue
						}
					}
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
					if inlinedValue, handled, err := inlineLocalAssetDataURL(text); handled {
						if err != nil {
							log.Printf("[Worker] 本地媒体内联失败: feature=%s key=%s index=%d err=%v", featureSlug, key, index, err)
						} else {
							values[index] = inlinedValue
							updated = true
							inlined++
							continue
						}
					}
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
		log.Printf("[Worker] 已补全上游媒体地址: feature=%s converted=%d", featureSlug, converted)
	}
	if inlined > 0 {
		log.Printf("[Worker] 已内联本地媒体到上游请求: feature=%s inlined=%d", featureSlug, inlined)
	}
	if len(unresolvedKeys) > 0 {
		keys := make([]string, 0, len(unresolvedKeys))
		for key := range unresolvedKeys {
			keys = append(keys, key)
		}
		log.Printf("[Worker] 上游媒体地址仍为相对路径，请配置 APP_PUBLIC_BASE_URL: feature=%s keys=%s", featureSlug, strings.Join(keys, ","))
	}
}
