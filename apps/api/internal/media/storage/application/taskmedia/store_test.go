package taskmedia

import (
	"context"
	"testing"

	"ideaboard/api/internal/media/storage/ports"
)

type stubStorageRuntime struct {
	obj *ports.StoredUploadObject
}

func (s *stubStorageRuntime) ReadUploadResourceByURL(ctx context.Context, rawURL string, maxBytes int64) (*ports.StoredUploadObject, error) {
	return s.obj, nil
}
func (s *stubStorageRuntime) Presign(ctx context.Context, req ports.PresignRequest) (ports.PresignedURL, error) {
	return ports.PresignedURL{}, nil
}
func (s *stubStorageRuntime) StoreRemoteRefAtProfile(ctx context.Context, profileID, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return nil, nil
}
func (s *stubStorageRuntime) StoreBytesRefAtProfile(ctx context.Context, profileID, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return nil, nil
}
func (s *stubStorageRuntime) PrepareUploadRef(ctx context.Context, filename string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return nil, nil
}
func (s *stubStorageRuntime) DeleteObjectByStorageProfile(ctx context.Context, storageProfileID string, objectKey string) error {
	return nil
}

func TestRuntimeTaskMediaStoreRead(t *testing.T) {
	runtime := &stubStorageRuntime{
		obj: &ports.StoredUploadObject{
			ObjectKey:   "uploads/test.png",
			Filename:    "test.png",
			ContentType: "image/png",
			Data:        []byte("image data"),
		},
	}
	store := NewRuntimeTaskMediaStore(runtime, nil)
	obj, err := store.ReadUploadObject(context.Background(), "/uploads/test.png", 1024)
	if err != nil {
		t.Fatalf("ReadUploadObject error: %v", err)
	}
	if obj.Filename != "test.png" {
		t.Fatalf("filename = %q, want test.png", obj.Filename)
	}
	if string(obj.Data) != "image data" {
		t.Fatalf("data = %q, want image data", obj.Data)
	}
}
