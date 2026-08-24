package storage

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"
)

type storageProviderAdapter interface {
	Provider() StorageProvider
	UsesObjectStorage() bool
	IsProfileReady(profile StorageProfile) bool
	TestProfile(ctx context.Context, profile *StorageProfile) error
	GetBucketInfo(ctx context.Context, profile *StorageProfile) (map[string]interface{}, error)
	UploadRef(ctx context.Context, profile *StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error)
	Delete(ctx context.Context, profile *StorageProfile, objectKey string) error
	DeleteByPrefix(ctx context.Context, profile *StorageProfile, objectKeyPrefix string) error
	ListObjectKeysByPrefix(ctx context.Context, profile *StorageProfile, objectKeyPrefix string) ([]string, error)
	PutObject(ctx context.Context, profile *StorageProfile, objectKey string, data []byte, contentType string) error
	StoreBytesRef(ctx context.Context, profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error)
	StoreRemoteRef(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error)
	Read(ctx context.Context, profile *StorageProfile, objectKey string, maxBytes int64) (*StoredUploadObject, error)
	SignedURL(ctx context.Context, profile *StorageProfile, objectKey string, expiry time.Duration) (string, error)
	ProviderURL(profile *StorageProfile, objectKey string) (string, error)
}

type storageAdapterRegistry struct {
	adapters map[StorageProvider]storageProviderAdapter
}

func newStorageAdapterRegistry(adapters ...storageProviderAdapter) *storageAdapterRegistry {
	registry := &storageAdapterRegistry{
		adapters: make(map[StorageProvider]storageProviderAdapter, len(adapters)),
	}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		registry.adapters[adapter.Provider()] = adapter
	}
	return registry
}

func (r *storageAdapterRegistry) get(provider StorageProvider) (storageProviderAdapter, error) {
	if r == nil {
		return nil, fmt.Errorf("不支持的存储类型")
	}
	adapter, ok := r.adapters[canonicalizeStorageProvider(provider)]
	if !ok {
		return nil, fmt.Errorf("不支持的存储类型")
	}
	return adapter, nil
}
