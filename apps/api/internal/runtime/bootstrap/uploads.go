package bootstrap

import (
	"fmt"
	"os"
)

const runtimeUploadDir = "uploads"

func ensureRuntimeUploadDir() error {
	if err := os.MkdirAll(runtimeUploadDir, 0700); err != nil {
		return fmt.Errorf("failed to ensure runtime upload directory %q: %w", runtimeUploadDir, err)
	}
	return nil
}
