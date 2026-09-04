package s3

import (
	"context"
	"mime/multipart"
	"time"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/infrastructure/remote"
	"ideaboard/api/internal/media/storage/ports"
)

type RemoteDownloader interface {
	DownloadRemoteFile(ctx context.Context, remoteURL string, allowHosts []string) (RemoteFile, error)
}

type RemoteFile interface {
	Filename() string
	ContentType() string
	Width() int
	Height() int
	Animated() bool
	Size() int64
	Reader() multipart.File
	Cleanup() error
}

type S3CompatibleStorageDriver struct {
	service    *S3CompatibleService
	downloader func(ctx context.Context, remoteURL string, allowHosts []string) (*remote.ImageFile, error)
}

var _ ports.StorageDriver = (*S3CompatibleStorageDriver)(nil)

func NewS3CompatibleStorageDriver(
	service *S3CompatibleService,
	downloader func(ctx context.Context, remoteURL string, allowHosts []string) (*remote.ImageFile, error),
) *S3CompatibleStorageDriver {
	return &S3CompatibleStorageDriver{
		service:    service,
		downloader: downloader,
	}
}

func (d *S3CompatibleStorageDriver) Service() *S3CompatibleService {
	return d.service
}

func (d *S3CompatibleStorageDriver) Provider() domain.StorageProvider {
	return domain.StorageProviderS3Compatible
}

func (d *S3CompatibleStorageDriver) UsesObjectStorage() bool {
	return true
}

func (d *S3CompatibleStorageDriver) IsProfileReady(profile domain.StorageProfile) bool {
	return d.service.IsConfigEnabled(S3CompatibleConfigFromProfile(&profile))
}

func (d *S3CompatibleStorageDriver) TestProfile(ctx context.Context, profile *domain.StorageProfile) error {
	return d.service.TestConnectionWithConfig(ctx, S3CompatibleConfigFromProfile(profile))
}

func (d *S3CompatibleStorageDriver) GetBucketInfo(ctx context.Context, profile *domain.StorageProfile) (map[string]interface{}, error) {
	return d.service.GetBucketInfoWithConfig(ctx, S3CompatibleConfigFromProfile(profile))
}

func (d *S3CompatibleStorageDriver) UploadRef(ctx context.Context, profile *domain.StorageProfile, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return d.service.UploadRefWithConfig(ctx, file, S3CompatibleConfigFromProfile(profile), opts)
}

func (d *S3CompatibleStorageDriver) Delete(ctx context.Context, profile *domain.StorageProfile, objectKey string) error {
	return d.service.DeleteWithConfig(ctx, objectKey, S3CompatibleConfigFromProfile(profile))
}

func (d *S3CompatibleStorageDriver) DeleteByPrefix(ctx context.Context, profile *domain.StorageProfile, objectKeyPrefix string) error {
	return d.service.DeletePrefixWithConfig(ctx, objectKeyPrefix, S3CompatibleConfigFromProfile(profile))
}

func (d *S3CompatibleStorageDriver) ListObjectKeysByPrefix(ctx context.Context, profile *domain.StorageProfile, objectKeyPrefix string) ([]string, error) {
	return d.service.ListObjectKeysWithConfig(ctx, objectKeyPrefix, S3CompatibleConfigFromProfile(profile))
}

func (d *S3CompatibleStorageDriver) PutObject(ctx context.Context, profile *domain.StorageProfile, objectKey string, data []byte, contentType string) error {
	return d.service.PutBytesAtKeyWithConfig(ctx, objectKey, data, contentType, S3CompatibleConfigFromProfile(profile))
}

func (d *S3CompatibleStorageDriver) StoreBytesRef(ctx context.Context, profile *domain.StorageProfile, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return d.service.UploadBytesWithConfigRef(ctx, filename, data, S3CompatibleConfigFromProfile(profile), opts)
}

func (d *S3CompatibleStorageDriver) StoreRemoteRef(ctx context.Context, profile *domain.StorageProfile, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	if d.downloader == nil {
		return nil, domain.ErrUploadAccessDependencyMissing
	}
	remoteFile, err := d.downloader(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return nil, err
	}
	defer remoteFile.Cleanup()

	opts.ObjectKey = canonicalObjectKeyExtension(opts.ObjectKey, filepathExt(remoteFile.Filename))
	ref, err := d.service.UploadReaderWithConfigRef(ctx, remoteFile.Filename, remoteFile.ContentType, remoteFile.File, remoteFile.Size, S3CompatibleConfigFromProfile(profile), opts)
	if err != nil {
		return nil, err
	}
	ref.Width = remoteFile.Width
	ref.Height = remoteFile.Height
	ref.Animated = remoteFile.Animated
	return ref, nil
}

func (d *S3CompatibleStorageDriver) Read(ctx context.Context, profile *domain.StorageProfile, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error) {
	return d.service.ReadWithConfig(ctx, objectKey, S3CompatibleConfigFromProfile(profile), maxBytes)
}

func (d *S3CompatibleStorageDriver) SignedURL(ctx context.Context, profile *domain.StorageProfile, objectKey string, expiry time.Duration) (string, error) {
	return d.service.SignedURLWithConfig(ctx, objectKey, expiry, S3CompatibleConfigFromProfile(profile))
}

func (d *S3CompatibleStorageDriver) ProviderURL(profile *domain.StorageProfile, objectKey string) (string, error) {
	return d.service.PublicObjectURL(S3CompatibleConfigFromProfile(profile), objectKey)
}

func (d *S3CompatibleStorageDriver) VerifyObjectGeneration(ctx context.Context, profile *domain.StorageProfile, objectKey string, expected int64) error {
	return d.service.VerifyObjectGeneration(ctx, objectKey, expected, S3CompatibleConfigFromProfile(profile))
}
