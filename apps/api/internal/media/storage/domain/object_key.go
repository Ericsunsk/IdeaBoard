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
		if HasParentDirectorySegment(rawPrefix) {
			return "", fmt.Errorf("上传目录前缀无效")
		}
		prefix := path.Clean(rawPrefix)
		prefix = strings.TrimPrefix(prefix, "/")
		if HasParentDirectorySegment(prefix) {
			return "", fmt.Errorf("上传目录前缀无效")
		}
		if prefix != "." && prefix != "" {
			baseDir = path.Join(baseDir, prefix)
		}
	}
	now := time.Now()
	return fmt.Sprintf("%s/%s/%d%s", baseDir, now.Format("2006/01/02"), now.UnixNano(), cleanExt), nil
}

func StableUploadObjectKey(dirPrefix, filename string) (string, error) {
	base := path.Base(strings.TrimSpace(filename))
	if base == "." || base == "" || base == ".." || strings.ContainsAny(base, `/\\`) {
		return "", fmt.Errorf("上传文件名无效")
	}
	baseDir := UploadObjectRoot
	if strings.TrimSpace(dirPrefix) != "" {
		if HasParentDirectorySegment(dirPrefix) {
			return "", fmt.Errorf("上传目录前缀无效")
		}
		prefix := strings.Trim(path.Clean(strings.TrimSpace(dirPrefix)), "/")
		if prefix != "" && prefix != "." {
			baseDir = path.Join(baseDir, prefix)
		}
	}
	return NormalizeUploadObjectKey(path.Join(baseDir, base))
}

func HasParentDirectorySegment(value string) bool {
	for _, part := range strings.Split(strings.ReplaceAll(value, "\\", "/"), "/") {
		if part == ".." {
			return true
		}
	}
	return false
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
