package storage

import (
	"fmt"
	"path"
	"strings"
	"time"
)

func buildObjectKey(dirPrefix string, ext string) string {
	cleanExt := strings.TrimSpace(ext)
	if cleanExt == "" {
		cleanExt = ".png"
	}
	if !strings.HasPrefix(cleanExt, ".") {
		cleanExt = "." + cleanExt
	}

	baseDir := "uploads"
	if strings.TrimSpace(dirPrefix) != "" {
		prefix := path.Clean(strings.TrimSpace(dirPrefix))
		prefix = strings.TrimPrefix(prefix, "/")
		if prefix != "." && prefix != "" {
			baseDir = path.Join(baseDir, prefix)
		}
	}
	return fmt.Sprintf("%s/%s/%d%s", baseDir, time.Now().Format("2006/01/02"), time.Now().UnixNano(), cleanExt)
}
