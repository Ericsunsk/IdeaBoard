package ports

import (
	"context"

	storagedomain "ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
)

type Storage interface {
	ReadObjectAtProfile(ctx context.Context, profileID, key string, maxBytes int64) (*storageports.StoredUploadObject, error)
	PutObjectAtProfile(ctx context.Context, profileID, key string, data []byte, mimeType string) error
	DeleteObjectByStorageProfile(ctx context.Context, profileID, key string) error
	ListProfiles(ctx context.Context) ([]storagedomain.StorageProfile, error)
	ListObjectKeysByStorageProfile(ctx context.Context, profileID, prefix string) ([]string, error)
}
