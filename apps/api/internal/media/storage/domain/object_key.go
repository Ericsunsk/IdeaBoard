package domain

import (
	"fmt"
	"path"
	"strings"
	"time"
)

func BuildObjectKey(dirPrefix string, ext string) (string, error) {
	cleanExt := strings.TrimSpace(ext)
	if cleanExt == "" {
		cleanExt = ".png"
	}
	if !strings.HasPrefix(cleanExt, ".") {
		cleanExt = "." + cleanExt
	}

	baseDir := UploadObjectRoot
	if strings.TrimSpace(dirPrefix) != "" {
		rawPrefix := strings.TrimSpace(dirPrefix)
		prefix := path.Clean(rawPrefix)
		prefix = strings.TrimPrefix(prefix, "/")
		if prefix != "." && prefix != "" {
			baseDir = path.Join(baseDir, prefix)
		}
	}
	now := time.Now()
	// Single-source traversal defense: the returned key is anchored by
	// NormalizeUploadObjectKey (lexical Clean + uploads/ws-uploads root
	// check), so interior ".." segments that resolve inside the root
	// normalize instead of failing, while escapes still error.
	return NormalizeUploadObjectKey(fmt.Sprintf("%s/%s/%d%s", baseDir, now.Format("2006/01/02"), now.UnixNano(), cleanExt))
}

func StableUploadObjectKey(dirPrefix, filename string) (string, error) {
	base := path.Base(strings.TrimSpace(filename))
	if base == "." || base == "" || base == ".." || strings.ContainsAny(base, `/\\`) {
		return "", fmt.Errorf("上传文件名无效")
	}
	baseDir := UploadObjectRoot
	if strings.TrimSpace(dirPrefix) != "" {
		prefix := strings.Trim(path.Clean(strings.TrimSpace(dirPrefix)), "/")
		if prefix != "" && prefix != "." {
			baseDir = path.Join(baseDir, prefix)
		}
	}
	return NormalizeUploadObjectKey(path.Join(baseDir, base))
}

func CanonicalObjectKeyExtension(objectKey, ext string) string {
	trimmed := strings.TrimSpace(objectKey)
	if trimmed == "" || ext == "" {
		return trimmed
	}
	base := path.Base(trimmed)
	if base == "." || base == "" || base == ".." {
		return trimmed
	}
	base = strings.TrimSuffix(base, path.Ext(base)) + ext
	return path.Join(path.Dir(trimmed), base)
}

func CanonicalStorageFilename(filename, ext string) string {
	base := strings.TrimSpace(path.Base(filename))
	if base == "" || base == "." || base == "/" || base == "\\" {
		base = "image"
	}
	oldExt := path.Ext(base)
	if oldExt != "" {
		base = strings.TrimSuffix(base, oldExt)
	}
	if base == "" {
		base = "image"
	}
	return base + ext
}
