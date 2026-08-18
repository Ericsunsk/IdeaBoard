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

func (a *localStorageAdapter) TestProfile(ctx context.Context, profile *StorageProfile) error {
	return ctx.Err()
}

func (a *localStorageAdapter) GetBucketInfo(ctx context.Context, profile *StorageProfile) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("当前存储类型不支持读取 Bucket 信息")
}

func (a *localStorageAdapter) UploadRef(ctx context.Context, profile *StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.service.saveUploadedFileLocalRef(ctx, file, opts)
}

func (a *localStorageAdapter) Delete(ctx context.Context, profile *StorageProfile, objectKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return a.service.deleteLocalByObjectKey(objectKey)
}

func (a *localStorageAdapter) DeleteByPrefix(ctx context.Context, profile *StorageProfile, objectKeyPrefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return a.service.deleteLocalByPrefix(objectKeyPrefix)
}

func (a *localStorageAdapter) PutObject(ctx context.Context, profile *StorageProfile, objectKey string, data []byte, contentType string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return a.service.putLocalObject(objectKey, data)
}

func (a *localStorageAdapter) StoreBytes(ctx context.Context, profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return a.service.saveBytesLocal(ctx, filename, data, opts)
}

func (a *localStorageAdapter) StoreBytesRef(ctx context.Context, profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.service.saveBytesLocalRef(ctx, filename, data, opts)
}

func (a *localStorageAdapter) StoreRemote(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (string, error) {
	remoteFile, err := a.service.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return "", err
	}
	defer remoteFile.Cleanup()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return a.service.saveReaderLocal(ctx, remoteFile.Filename, remoteFile, remoteFile.Size, opts)
}

func (a *localStorageAdapter) StoreRemoteRef(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	remoteFile, err := a.service.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return nil, err
	}
	defer remoteFile.Cleanup()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ref, err := a.service.saveReaderLocalRef(ctx, remoteFile.Filename, remoteFile.ContentType, remoteFile, remoteFile.Size, opts)
	if err != nil {
		return nil, err
	}
	ref.Width = remoteFile.Width
	ref.Height = remoteFile.Height
	return ref, nil
}

func (a *localStorageAdapter) Read(ctx context.Context, profile *StorageProfile, objectKey string, maxBytes int64) (*StoredUploadObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.service.readLocalByObjectKey(objectKey, maxBytes)
}

func (a *localStorageAdapter) ProviderURL(_ *StorageProfile, _ string) (string, error) {
	return "", errProviderHTTPSUnavailable
}
