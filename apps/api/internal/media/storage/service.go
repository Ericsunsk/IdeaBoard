package storage

import (
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"

	"ideaboard/api/internal/media/storage/application/profiles"
	"ideaboard/api/internal/media/storage/application/runtime"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/infrastructure/local"
	"ideaboard/api/internal/media/storage/infrastructure/remote"
	"ideaboard/api/internal/media/storage/infrastructure/s3"
	"ideaboard/api/internal/media/storage/ports"
)

type dynamicAssetLocator struct {
	svc *StorageService
}

func (d dynamicAssetLocator) LocateStoredUpload(ctx context.Context, lookup ports.StoredUploadLookup) (ports.StoredUploadLocation, bool, error) {
	if d.svc == nil || d.svc.assetLocator == nil {
		return ports.StoredUploadLocation{}, false, nil
	}
	return d.svc.assetLocator.LocateStoredUpload(ctx, lookup)
}

type StorageService struct {
	assetLocator        ports.StoredUploadLocator
	s3CompatibleService *s3.S3CompatibleService

	profilesService *profiles.Service
	runtimeService  *runtime.Service
}

func NewStorageService(profileRepo ports.StorageProfileRepository, assetLocator ports.StoredUploadLocator, publicAPIOrigin ...string) *StorageService {
	origin := ""
	if len(publicAPIOrigin) > 0 {
		origin = publicAPIOrigin[0]
	}
	return newStorageService(profileRepo, assetLocator, origin)
}

func newStorageService(profileRepo ports.StorageProfileRepository, assetLocator ports.StoredUploadLocator, publicAPIOrigin string) *StorageService {
	origin := strings.TrimRight(strings.TrimSpace(publicAPIOrigin), "/")
	s3Svc := s3.NewS3CompatibleService()
	localDriver := local.NewLocalStorageDriver(origin)
	s3Driver := s3.NewS3CompatibleStorageDriver(s3Svc, remote.DownloadImage)

	drivers := profiles.DriverResolverFunc(func(provider domain.StorageProvider) (ports.StorageDriver, error) {
		switch provider {
		case domain.StorageProviderLocal:
			return localDriver, nil
		case domain.StorageProviderS3Compatible:
			return s3Driver, nil
		default:
			return nil, fmt.Errorf("不支持的存储类型")
		}
	})

	profilesSvc := profiles.NewService(profileRepo, drivers)

	service := &StorageService{
		assetLocator:        assetLocator,
		s3CompatibleService: s3Svc,
		profilesService:     profilesSvc,
	}

	runtimeSvc := runtime.NewService(profilesSvc, drivers, dynamicAssetLocator{svc: service}, origin)
	service.runtimeService = runtimeSvc

	return service
}

func (s *StorageService) SetAssetLocator(locator ports.StoredUploadLocator) {
	s.assetLocator = locator
	if s.runtimeService != nil {
		s.runtimeService.SetAssetLocator(locator)
	}
}

func (s *StorageService) SetCapabilityProbe(fn func(context.Context, *s3.S3CompatibleConfig) error) {
	if s != nil && s.s3CompatibleService != nil {
		s.s3CompatibleService.SetCapabilityProbe(fn)
	}
}

func (s *StorageService) ListProviderMeta() []domain.StorageProviderMeta {
	return s.profilesService.ListProviderMeta()
}

func (s *StorageService) ListProfiles(ctx context.Context) ([]domain.StorageProfile, error) {
	return s.profilesService.ListProfiles(ctx)
}

func (s *StorageService) GetProfile(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return s.profilesService.GetProfile(ctx, id)
}

func (s *StorageService) SaveProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error) {
	return s.profilesService.SaveProfile(ctx, profile)
}

func (s *StorageService) SaveAndApplyProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error) {
	return s.profilesService.SaveAndApplyProfile(ctx, profile)
}

func (s *StorageService) DeleteProfile(ctx context.Context, id string) error {
	return s.profilesService.DeleteProfile(ctx, id)
}

func (s *StorageService) ApplyProfile(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return s.profilesService.ApplyProfile(ctx, id)
}

func (s *StorageService) TestProfile(ctx context.Context, id string) error {
	return s.profilesService.TestProfile(ctx, id)
}

func (s *StorageService) GetBucketInfo(ctx context.Context, id string) (map[string]interface{}, error) {
	return s.profilesService.GetBucketInfo(ctx, id)
}

func (s *StorageService) EnsureProfileReadyForRuntime(ctx context.Context, profile domain.StorageProfile) error {
	return s.profilesService.EnsureProfileReadyForRuntime(ctx, profile)
}

func (s *StorageService) ResolveProfileByID(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return s.profilesService.ResolveProfileByID(ctx, id)
}

func (s *StorageService) ResolveActiveProfile(ctx context.Context) (*domain.StorageProfile, error) {
	return s.profilesService.ResolveActiveProfile(ctx)
}

func (s *StorageService) EnsureDefaultProfiles(ctx context.Context) error {
	return s.profilesService.EnsureDefaultProfiles(ctx)
}

func (s *StorageService) DefaultLocalStorageProfile(isCurrent bool) domain.StorageProfile {
	return s.profilesService.DefaultLocalStorageProfile(isCurrent)
}

func (s *StorageService) LoadProfilesWithSecrets(ctx context.Context) ([]domain.StorageProfile, error) {
	return s.profilesService.LoadProfilesWithSecrets(ctx)
}

func (s *StorageService) LoadProfileForReclamation(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return s.profilesService.LoadProfileForReclamation(ctx, id)
}

func (s *StorageService) Upload(ctx context.Context, file *multipart.FileHeader) (string, error) {
	return s.runtimeService.Upload(ctx, file)
}

func (s *StorageService) UploadWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (string, error) {
	return s.runtimeService.UploadWithOptions(ctx, file, opts)
}

func (s *StorageService) UploadRefWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.UploadRefWithOptions(ctx, file, opts)
}

func (s *StorageService) UploadRefAtProfile(ctx context.Context, profileID string, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.UploadRefAtProfile(ctx, profileID, file, opts)
}

func (s *StorageService) PrepareUploadRefFromFile(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.PrepareUploadRefFromFile(ctx, file, opts)
}

func (s *StorageService) PrepareUploadRef(ctx context.Context, filename string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.PrepareUploadRef(ctx, filename, opts)
}

func (s *StorageService) StoreRemoteRef(ctx context.Context, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreRemoteRef(ctx, remoteURL, opts)
}

func (s *StorageService) StoreMediaRef(ctx context.Context, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreMediaRef(ctx, sourceRef, opts)
}

func (s *StorageService) StoreMediaRefAtProfile(ctx context.Context, profileID, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreMediaRefAtProfile(ctx, profileID, sourceRef, opts)
}

func (s *StorageService) StoreMediaRefWithProfile(ctx context.Context, profile domain.StorageProfile, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreMediaRefWithProfile(ctx, profile, sourceRef, opts)
}

func (s *StorageService) StoreBytesRef(ctx context.Context, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreBytesRef(ctx, filename, data, opts)
}

func (s *StorageService) StoreBytesRefAtProfile(ctx context.Context, profileID, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreBytesRefAtProfile(ctx, profileID, filename, data, opts)
}

func (s *StorageService) StoreBytesRefWithProfile(ctx context.Context, profile domain.StorageProfile, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreBytesRefWithProfile(ctx, profile, filename, data, opts)
}

func (s *StorageService) StoreRemoteRefAtProfile(ctx context.Context, profileID, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreRemoteRefAtProfile(ctx, profileID, remoteURL, opts)
}

func (s *StorageService) StoreRemoteRefWithProfile(ctx context.Context, profile domain.StorageProfile, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreRemoteRefWithProfile(ctx, profile, remoteURL, opts)
}

func (s *StorageService) UploadRefWithProfile(ctx context.Context, profile domain.StorageProfile, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.UploadRefWithProfile(ctx, profile, file, opts)
}

func (s *StorageService) ReadUploadResourceByURL(ctx context.Context, rawURL string, maxBytes int64) (*ports.StoredUploadObject, error) {
	return s.runtimeService.ReadUploadResourceByURL(ctx, rawURL, maxBytes)
}

func (s *StorageService) PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error {
	return s.runtimeService.PutObjectAtProfile(ctx, profileID, objectKey, data, contentType)
}

func (s *StorageService) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error) {
	return s.runtimeService.ReadObjectAtProfile(ctx, profileID, objectKey, maxBytes)
}

func (s *StorageService) ResolveProviderDerivativeURL(ctx context.Context, assetID int64, tier string) (string, error) {
	return s.runtimeService.ResolveProviderDerivativeURL(ctx, assetID, tier)
}

func (s *StorageService) DeleteProfileAvatarByURL(ctx context.Context, storageUID string, rawURL string) error {
	return s.runtimeService.DeleteProfileAvatarByURL(ctx, storageUID, rawURL)
}

func (s *StorageService) DeleteCanvasByStorageUID(ctx context.Context, storageUID string) error {
	return s.runtimeService.DeleteCanvasByStorageUID(ctx, storageUID)
}

func (s *StorageService) DeleteUserUploads(ctx context.Context, userID int64, storageUID string) error {
	return s.runtimeService.DeleteUserUploads(ctx, userID, storageUID)
}

func (s *StorageService) DeleteObjectByKey(ctx context.Context, objectKey string) error {
	return s.runtimeService.DeleteObjectByKey(ctx, objectKey)
}

func (s *StorageService) DeleteObjectByStorageProfile(ctx context.Context, storageProfileID string, objectKey string) error {
	return s.runtimeService.DeleteObjectByStorageProfile(ctx, storageProfileID, objectKey)
}

func (s *StorageService) ListObjectKeysByStorageProfile(ctx context.Context, storageProfileID string, objectKeyPrefix string) ([]string, error) {
	return s.runtimeService.ListObjectKeysByStorageProfile(ctx, storageProfileID, objectKeyPrefix)
}

func (s *StorageService) DeleteUploadObjectPrefix(ctx context.Context, prefix string) error {
	return s.runtimeService.DeleteUploadObjectPrefix(ctx, prefix)
}

func (s *StorageService) DeleteUploadObjectPrefixes(ctx context.Context, prefixes []string) error {
	return s.runtimeService.DeleteUploadObjectPrefixes(ctx, prefixes)
}

func (s *StorageService) DeleteCanvasBoardByStorageUID(ctx context.Context, storageUID string, boardID string) error {
	return s.runtimeService.DeleteCanvasBoardByStorageUID(ctx, storageUID, boardID)
}

func (s *StorageService) UsesObjectStorage(ctx context.Context) bool {
	return s.runtimeService.UsesObjectStorage(ctx)
}

func (s *StorageService) ResolveStoredUploadTarget(ctx context.Context, rawURL string) (*domain.StorageProfile, string, error) {
	return s.runtimeService.ResolveStoredUploadTarget(ctx, rawURL)
}

func (s *StorageService) PublicUploadURL(objectKey string) string {
	if s != nil && s.runtimeService != nil {
		return s.runtimeService.PublicUploadURL(objectKey)
	}
	return publicUploadURL(objectKey)
}

func (s *StorageService) Presign(ctx context.Context, req ports.PresignRequest) (ports.PresignedURL, error) {
	return s.runtimeService.Presign(ctx, req)
}

func publicUploadURL(objectKey string) string {
	return "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
}
