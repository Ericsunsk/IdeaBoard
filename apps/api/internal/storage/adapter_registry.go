package storage

import (
	"context"
	"fmt"
	"mime/multipart"
)

type storageProviderAdapter interface {
	Provider() StorageProvider
	UsesObjectStorage() bool
	IsProfileReady(profile StorageProfile) bool
	IsDirectFetchEnabled(profile *StorageProfile) bool
	TestProfile(profile *StorageProfile) error
	GetBucketInfo(profile *StorageProfile) (map[string]interface{}, error)
	Upload(profile *StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (string, error)
	Delete(profile *StorageProfile, objectKey string) error
	StoreBytes(profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (string, error)
	StoreRemote(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (string, error)
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
