package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api/internal/accounts"
	"api/internal/platform/settings"
	"api/internal/storage"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMaterializeProviderResultImagesStoresInlineDataURLToLocalStorage(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	db, err := gorm.Open(sqlite.Open("file:provider_storage?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&settings.SystemConfig{}); err != nil {
		t.Fatalf("auto migrate system configs: %v", err)
	}

	executor := &ProviderExecutor{
		storageService: storage.NewStorageService(settings.NewSystemConfigRepository(db)),
	}
	dataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNg+M8AAAICAQBx3Y4ZAAAAAElFTkSuQmCC"

	urls, err := executor.materializeProviderResultImages(context.Background(), 42, "task-owned", &accounts.ApiAccount{}, []string{dataURL})
	if err != nil {
		t.Fatalf("materializeProviderResultImages returned error: %v", err)
	}
	if len(urls) != 1 {
		t.Fatalf("urls len=%d, want 1", len(urls))
	}
	if strings.HasPrefix(urls[0], "data:image/") {
		t.Fatalf("inline data URL should be stored before task result persistence")
	}
	if !strings.HasPrefix(urls[0], "/uploads/tasks/42/task-owned/") {
		t.Fatalf("stored URL=%q, want task output path", urls[0])
	}
	if _, err := os.Stat(filepath.FromSlash(strings.TrimPrefix(urls[0], "/"))); err != nil {
		t.Fatalf("stored file not found: %v", err)
	}
}
