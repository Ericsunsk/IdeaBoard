package storage

import (
	"context"
	"mime/multipart"
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

func (a *s3CompatibleStorageAdapter) Upload(ctx context.Context, profile *StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	return a.service.s3CompatibleService.UploadWithConfig(ctx, file, a.service.s3CompatibleConfigFromProfile(profile), opts)
}

func (a *s3CompatibleStorageAdapter) Delete(ctx context.Context, profile *StorageProfile, objectKey string) error {
	return a.service.s3CompatibleService.DeleteWithConfig(ctx, objectKey, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) DeleteByPrefix(ctx context.Context, profile *StorageProfile, objectKeyPrefix string) error {
	return a.service.s3CompatibleService.DeletePrefixWithConfig(ctx, objectKeyPrefix, a.service.s3CompatibleConfigFromProfile(profile))
}

func (a *s3CompatibleStorageAdapter) StoreBytes(ctx context.Context, profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (string, error) {
	return a.service.s3CompatibleService.uploadBytesWithConfig(ctx, filename, data, a.service.s3CompatibleConfigFromProfile(profile), opts)
}

func (a *s3CompatibleStorageAdapter) StoreRemote(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (string, error) {
	data, filename, err := a.service.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return "", err
	}
	return a.service.s3CompatibleService.uploadBytesWithConfig(ctx, filename, data, a.service.s3CompatibleConfigFromProfile(profile), opts)
}
