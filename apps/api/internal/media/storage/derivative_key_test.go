package storage

import "testing"

func TestImageDerivativeObjectKeyIsCanonical(t *testing.T) {
	key, err := ImageDerivativeObjectKey("uploads/tasks/1/source.png", "asset-1", "preview", "webp")
	if err != nil {
		t.Fatal(err)
	}
	if key != "uploads/tasks/1/.ideaboard-derivatives/asset-1/preview.webp" {
		t.Fatalf("key = %q", key)
	}
	if IsImageDerivativeObjectKey(key) == false {
		t.Fatal("derivative key should be detected")
	}
}
