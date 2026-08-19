package storage

import (
	"context"
	"mime/multipart"
	"time"
)

type s3CompatibleStorageAdapter struct {
	service *StorageService
}

func newS3CompatibleStorageAdapter(service *StorageService) *s3CompatibleStorageAdapter {
	return &s3CompatibleStorageAdapter{service: service}
}

func (a *s3CompatibleStorageAdapter) Provider() StorageProvider {
	return StorageProviderS3Compatible
}

func (a *s3CompatibleStorageAdapter) UsesObjectStorage() bool {
	return true
}

func (a *s3CompatibleStorageAdapter) IsProfileReady(profile StorageProfile) bool {
	return a.service.s3CompatibleService.IsConfigEnabled(a.service.s3CompatibleConfigFromProfile(&profile))
}

func (a *s3CompatibleStorageAdapter) TestProfile(ctx context.Context, profile *StorageProfile) error {
	return a.service.s3CompatibleService.TestConnectionWithConfig(ctx, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) GetBucketInfo(ctx context.Context, profile *StorageProfile) (map[string]interface{}, error) {
	return a.service.s3CompatibleService.GetBucketInfoWithConfig(ctx, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) UploadRef(ctx context.Context, profile *StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error) {
	return a.service.s3CompatibleService.UploadRefWithConfig(ctx, file, a.service.s3CompatibleConfigFromProfile(profile), opts)
}

func (a *s3CompatibleStorageAdapter) Delete(ctx context.Context, profile *StorageProfile, objectKey string) error {
	return a.service.s3CompatibleService.DeleteWithConfig(ctx, objectKey, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) DeleteByPrefix(ctx context.Context, profile *StorageProfile, objectKeyPrefix string) error {
	return a.service.s3CompatibleService.DeletePrefixWithConfig(ctx, objectKeyPrefix, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) ListObjectKeysByPrefix(ctx context.Context, profile *StorageProfile, objectKeyPrefix string) ([]string, error) {
	return a.service.s3CompatibleService.ListObjectKeysWithConfig(ctx, objectKeyPrefix, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) PutObject(ctx context.Context, profile *StorageProfile, objectKey string, data []byte, contentType string) error {
	return a.service.s3CompatibleService.putBytesAtKeyWithConfig(ctx, objectKey, data, contentType, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) StoreBytes(ctx context.Context, profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (string, error) {
	return a.service.s3CompatibleService.uploadBytesWithConfig(ctx, filename, data, a.service.s3CompatibleConfigFromProfile(profile), opts)
}

func (a *s3CompatibleStorageAdapter) StoreBytesRef(ctx context.Context, profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error) {
	return a.service.s3CompatibleService.uploadBytesWithConfigRef(ctx, filename, data, a.service.s3CompatibleConfigFromProfile(profile), opts)
}

func (a *s3CompatibleStorageAdapter) StoreRemote(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (string, error) {
	remoteFile, err := a.service.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return "", err
	}
	defer remoteFile.Cleanup()
	return a.service.s3CompatibleService.uploadReaderWithConfig(ctx, remoteFile.Filename, remoteFile.ContentType, remoteFile, remoteFile.Size, a.service.s3CompatibleConfigFromProfile(profile), opts)
}

func (a *s3CompatibleStorageAdapter) StoreRemoteRef(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error) {
	remoteFile, err := a.service.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return nil, err
	}
	defer remoteFile.Cleanup()
	ref, err := a.service.s3CompatibleService.uploadReaderWithConfigRef(ctx, remoteFile.Filename, remoteFile.ContentType, remoteFile, remoteFile.Size, a.service.s3CompatibleConfigFromProfile(profile), opts)
	if err != nil {
		return nil, err
	}
	ref.Width = remoteFile.Width
	ref.Height = remoteFile.Height
	ref.Animated = remoteFile.Animated
	return ref, nil
}

func (a *s3CompatibleStorageAdapter) Read(ctx context.Context, profile *StorageProfile, objectKey string, maxBytes int64) (*StoredUploadObject, error) {
	return a.service.s3CompatibleService.readWithConfig(ctx, objectKey, a.service.s3CompatibleConfigFromProfile(profile), maxBytes)
}

func (a *s3CompatibleStorageAdapter) SignedURL(ctx context.Context, profile *StorageProfile, objectKey string, expiry time.Duration) (string, error) {
	return a.service.s3CompatibleService.SignedURLWithConfig(ctx, objectKey, expiry, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) ProviderURL(profile *StorageProfile, objectKey string) (string, error) {
	return a.service.s3CompatibleService.publicObjectURL(a.service.s3CompatibleConfigFromProfile(profile), objectKey)
}
