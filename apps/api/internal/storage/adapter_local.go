package storage

import (
	"context"
	"fmt"
	"mime/multipart"
)

type localStorageAdapter struct {
	service *StorageService
}

func newLocalStorageAdapter(service *StorageService) *localStorageAdapter {
	return &localStorageAdapter{service: service}
}

func (a *localStorageAdapter) Provider() StorageProvider {
	return StorageProviderLocal
}

func (a *localStorageAdapter) UsesObjectStorage() bool {
	return false
}

func (a *localStorageAdapter) IsProfileReady(profile StorageProfile) bool {
	return true
}

func (a *localStorageAdapter) IsDirectFetchEnabled(profile *StorageProfile) bool {
	return false
}

func (a *localStorageAdapter) TestProfile(profile *StorageProfile) error {
	return nil
}

func (a *localStorageAdapter) GetBucketInfo(profile *StorageProfile) (map[string]interface{}, error) {
	return nil, fmt.Errorf("当前存储类型不支持读取 Bucket 信息")
}

func (a *localStorageAdapter) Upload(profile *StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	return a.service.saveUploadedFileLocal(file, opts)
}

func (a *localStorageAdapter) Delete(profile *StorageProfile, objectKey string) error {
	return a.service.deleteLocalByObjectKey(objectKey)
}

func (a *localStorageAdapter) DeleteByPrefix(profile *StorageProfile, objectKeyPrefix string) error {
	return a.service.deleteLocalByPrefix(objectKeyPrefix)
}

func (a *localStorageAdapter) StoreBytes(profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (string, error) {
	return a.service.saveBytesLocal(filename, data, opts)
}

func (a *localStorageAdapter) StoreRemote(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (string, error) {
	data, filename, err := a.service.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return "", err
	}
	return a.service.saveBytesLocal(filename, data, opts)
}
