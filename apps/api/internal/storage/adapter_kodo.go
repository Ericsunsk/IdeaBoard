package storage

import (
	"context"
	"mime/multipart"
	"strings"
)

type kodoStorageAdapter struct {
	service *StorageService
}

func newKodoStorageAdapter(service *StorageService) *kodoStorageAdapter {
	return &kodoStorageAdapter{service: service}
}

func (a *kodoStorageAdapter) Provider() StorageProvider {
	return StorageProviderKodo
}

func (a *kodoStorageAdapter) UsesObjectStorage() bool {
	return true
}

func (a *kodoStorageAdapter) IsProfileReady(profile StorageProfile) bool {
	cfg := a.service.kodoConfigFromProfile(&profile)
	return a.service.kodoService.IsConfigEnabled(cfg) && strings.TrimSpace(cfg.AccessDomain) != ""
}

func (a *kodoStorageAdapter) IsDirectFetchEnabled(profile *StorageProfile) bool {
	cfg := a.service.kodoConfigFromProfile(profile)
	return cfg.DirectFetch
}

func (a *kodoStorageAdapter) TestProfile(profile *StorageProfile) error {
	return a.service.kodoService.TestConnectionWithConfig(a.service.kodoConfigFromProfile(profile))
}

func (a *kodoStorageAdapter) GetBucketInfo(profile *StorageProfile) (map[string]interface{}, error) {
	return a.service.kodoService.GetBucketInfoWithConfig(a.service.kodoConfigFromProfile(profile))
}

func (a *kodoStorageAdapter) Upload(profile *StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	return a.service.kodoService.UploadWithConfig(file, a.service.kodoConfigFromProfile(profile), opts)
}

func (a *kodoStorageAdapter) Delete(profile *StorageProfile, objectKey string) error {
	return a.service.kodoService.DeleteWithConfig(objectKey, a.service.kodoConfigFromProfile(profile))
}

func (a *kodoStorageAdapter) StoreBytes(profile *StorageProfile, filename string, data []byte, opts StorageStoreOptions) (string, error) {
	return a.service.kodoService.UploadWithConfig(createMemoryFileHeader(filename, data), a.service.kodoConfigFromProfile(profile), opts)
}

func (a *kodoStorageAdapter) StoreRemote(ctx context.Context, profile *StorageProfile, remoteURL string, opts StorageStoreOptions) (string, error) {
	cfg := a.service.kodoConfigFromProfile(profile)
	if cfg.DirectFetch {
		if url, err := a.service.kodoService.FetchURLWithConfig(remoteURL, cfg); err == nil {
			return url, nil
		}
	}

	data, filename, err := a.service.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return "", err
	}

	return a.service.kodoService.UploadWithConfig(createMemoryFileHeader(filename, data), cfg, opts)
}
