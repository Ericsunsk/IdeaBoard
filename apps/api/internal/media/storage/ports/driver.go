package ports

import (
	"context"
	"mime/multipart"
	"time"

	"ideaboard/api/internal/media/storage/domain"
)

type StorageDriver interface {
	Provider() domain.StorageProvider
	UsesObjectStorage() bool
	IsProfileReady(profile domain.StorageProfile) bool
	TestProfile(ctx context.Context, profile *domain.StorageProfile) error
	GetBucketInfo(ctx context.Context, profile *domain.StorageProfile) (map[string]interface{}, error)
	UploadRef(ctx context.Context, profile *domain.StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error)
	Delete(ctx context.Context, profile *domain.StorageProfile, objectKey string) error
	DeleteByPrefix(ctx context.Context, profile *domain.StorageProfile, objectKeyPrefix string) error
	ListObjectKeysByPrefix(ctx context.Context, profile *domain.StorageProfile, objectKeyPrefix string) ([]string, error)
	PutObject(ctx context.Context, profile *domain.StorageProfile, objectKey string, data []byte, contentType string) error
	StoreBytesRef(ctx context.Context, profile *domain.StorageProfile, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error)
	StoreRemoteRef(ctx context.Context, profile *domain.StorageProfile, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error)
	Read(ctx context.Context, profile *domain.StorageProfile, objectKey string, maxBytes int64) (*StoredUploadObject, error)
	SignedURL(ctx context.Context, profile *domain.StorageProfile, objectKey string, expiry time.Duration) (string, error)
	ProviderURL(profile *domain.StorageProfile, objectKey string) (string, error)
	VerifyObjectGeneration(ctx context.Context, profile *domain.StorageProfile, objectKey string, expected int64) error
}
