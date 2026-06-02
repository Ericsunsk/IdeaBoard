package storage

import (
	"fmt"
	"path"
	"strings"
	"time"
)

func buildObjectKey(dirPrefix string, ext string) (string, error) {
	cleanExt := strings.TrimSpace(ext)
	if cleanExt == "" {
		cleanExt = ".png"
	}
	if !strings.HasPrefix(cleanExt, ".") {
		cleanExt = "." + cleanExt
	}

	baseDir := "uploads"
	if strings.TrimSpace(dirPrefix) != "" {
		rawPrefix := strings.TrimSpace(dirPrefix)
		if hasParentDirectorySegment(rawPrefix) {
			return "", fmt.Errorf("上传目录前缀无效")
		}
		prefix := path.Clean(rawPrefix)
		prefix = strings.TrimPrefix(prefix, "/")
		if hasParentDirectorySegment(prefix) {
			return "", fmt.Errorf("上传目录前缀无效")
		}
		if prefix != "." && prefix != "" {
			baseDir = path.Join(baseDir, prefix)
		}
	}
	now := time.Now()
	return fmt.Sprintf("%s/%s/%d%s", baseDir, now.Format("2006/01/02"), now.UnixNano(), cleanExt), nil
}

func hasParentDirectorySegment(value string) bool {
	for _, part := range strings.Split(strings.ReplaceAll(value, "\\", "/"), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
