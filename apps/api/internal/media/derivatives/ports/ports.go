package ports

import (
	"context"

	"ideaboard/api/internal/media/derivatives/domain"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
)

type EventPublisher interface {
	Publish(userID int64, event domain.Event)
}

type Renderer interface {
	Render(ctx context.Context, data []byte, sourceName string, maxSide int) ([]byte, int, int, string, error)
}

type Storage interface {
	ReadObjectAtProfile(ctx context.Context, profileID, key string, maxBytes int64) (*storageports.StoredUploadObject, error)
	PutObjectAtProfile(ctx context.Context, profileID, key string, data []byte, mimeType string) error
	DeleteObjectByStorageProfile(ctx context.Context, profileID, key string) error
	ListProfiles(ctx context.Context) ([]storagedomain.StorageProfile, error)
	ListObjectKeysByStorageProfile(ctx context.Context, profileID, prefix string) ([]string, error)
}

type Verifier interface {
	Verify(ctx context.Context, storageProfileID, storageKey, checksum, mimeType string, sizeBytes int64) error
}
