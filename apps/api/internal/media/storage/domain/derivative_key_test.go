package domain_test

import (
	"testing"

	"ideaboard/api/internal/media/storage/domain"
)

func TestImageDerivativeObjectKeyIsCanonical(t *testing.T) {
	key, err := domain.ImageDerivativeObjectKey("uploads/tasks/1/source.png", "asset-1", "preview", "webp")
	if err != nil {
		t.Fatal(err)
	}
	if key != "uploads/tasks/1/.ideaboard-derivatives/asset-1/preview.webp" {
		t.Fatalf("key = %q", key)
	}
	if domain.IsImageDerivativeObjectKey(key) == false {
		t.Fatal("derivative key should be detected")
	}
}
