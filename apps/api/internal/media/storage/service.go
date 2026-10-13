package storage

import (
	"context"
	"errors"
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

// Backend-owned user-facing copy for this package. Each constant is the
// single source of truth for one rendered message; call sites reference
// these names so copy changes stay in one place. Values are byte-identical
// to the previously inline literals. The table lives in service.go (not a
// separate message.go) because the hexagonal-boundary architecture test
// only allows service.go at the storage root.
const (
	msgNewServiceWithLocalRoot = "不支持的存储类型" // NewServiceWithLocalRoot
)

type Service struct {
	s3CompatibleService *s3.S3CompatibleService

	profilesService *profiles.Service
	runtimeService  *runtime.Service
}

func NewService(profileRepo ports.ProfileRepository, publicAPIOrigin string) *Service {
	return NewServiceWithLocalRoot(profileRepo, publicAPIOrigin, "")
}

// NewServiceWithLocalRoot binds the builtin-local driver to a canonical
// filesystem root. An empty root resolves the process working directory once
// at construction; prefer passing the configured STORAGE_LOCAL_ROOT.
func NewServiceWithLocalRoot(profileRepo ports.ProfileRepository, publicAPIOrigin, localRoot string) *Service {
	origin := strings.TrimRight(strings.TrimSpace(publicAPIOrigin), "/")
	s3Svc := s3.NewS3CompatibleService()
	localDriver := local.NewLocalStorageDriverWithRoot(origin, localRoot)
	s3Driver := s3.NewS3CompatibleStorageDriver(s3Svc, remote.DownloadImage)

	drivers := profiles.DriverResolverFunc(func(provider domain.StorageProvider) (ports.StorageDriver, error) {
		switch provider {
		case domain.StorageProviderLocal:
			return localDriver, nil
		case domain.StorageProviderS3Compatible:
			return s3Driver, nil
		default:
			return nil, errors.New(msgNewServiceWithLocalRoot)
		}
	})

	profilesSvc := profiles.NewService(profileRepo, drivers)

	service := &Service{
		s3CompatibleService: s3Svc,
		profilesService:     profilesSvc,
	}

	runtimeSvc := runtime.NewService(profilesSvc, drivers, origin)
	service.runtimeService = runtimeSvc

	return service
}

func (s *Service) SetCapabilityProbe(fn func(context.Context, *s3.S3CompatibleConfig) error) {
	if s != nil && s.s3CompatibleService != nil {
		s.s3CompatibleService.SetCapabilityProbe(fn)
	}
}

func (s *Service) ListProviderMeta() []domain.StorageProviderMeta {
	return s.profilesService.ListProviderMeta()
}

func (s *Service) ListProfiles(ctx context.Context) ([]domain.StorageProfile, error) {
	return s.profilesService.ListProfiles(ctx)
}

func (s *Service) GetProfile(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return s.profilesService.GetProfile(ctx, id)
}

func (s *Service) SaveProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error) {
	return s.profilesService.SaveProfile(ctx, profile)
}

func (s *Service) SaveAndApplyProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error) {
	return s.profilesService.SaveAndApplyProfile(ctx, profile)
}

func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	return s.profilesService.DeleteProfile(ctx, id)
}

func (s *Service) ApplyProfile(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return s.profilesService.ApplyProfile(ctx, id)
}

func (s *Service) TestProfile(ctx context.Context, id string) error {
	return s.profilesService.TestProfile(ctx, id)
}

func (s *Service) GetBucketInfo(ctx context.Context, id string) (map[string]interface{}, error) {
	return s.profilesService.GetBucketInfo(ctx, id)
}

func (s *Service) EnsureProfileReadyForRuntime(ctx context.Context, profile domain.StorageProfile) error {
	return s.profilesService.EnsureProfileReadyForRuntime(ctx, profile)
}

func (s *Service) ResolveProfileByID(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return s.profilesService.ResolveProfileByID(ctx, id)
}

func (s *Service) ResolveActiveProfile(ctx context.Context) (*domain.StorageProfile, error) {
	return s.profilesService.ResolveActiveProfile(ctx)
}

func (s *Service) EnsureDefaultProfiles(ctx context.Context) error {
	return s.profilesService.EnsureDefaultProfiles(ctx)
}

func (s *Service) DefaultLocalStorageProfile(isCurrent bool) domain.StorageProfile {
	return s.profilesService.DefaultLocalStorageProfile(isCurrent)
}

func (s *Service) LoadProfilesWithSecrets(ctx context.Context) ([]domain.StorageProfile, error) {
	return s.profilesService.LoadProfilesWithSecrets(ctx)
}

func (s *Service) LoadProfileForReclamation(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return s.profilesService.LoadProfileForReclamation(ctx, id)
}

func (s *Service) Upload(ctx context.Context, file *multipart.FileHeader) (string, error) {
	return s.runtimeService.Upload(ctx, file)
}

func (s *Service) UploadWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (string, error) {
	return s.runtimeService.UploadWithOptions(ctx, file, opts)
}

func (s *Service) UploadRefWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.UploadRefWithOptions(ctx, file, opts)
}

func (s *Service) UploadRefAtProfile(ctx context.Context, profileID string, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.UploadRefAtProfile(ctx, profileID, file, opts)
}

func (s *Service) PrepareUploadRefFromFile(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.PrepareUploadRefFromFile(ctx, file, opts)
}

func (s *Service) PrepareUploadRef(ctx context.Context, filename string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.PrepareUploadRef(ctx, filename, opts)
}

func (s *Service) StoreRemoteRef(ctx context.Context, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreRemoteRef(ctx, remoteURL, opts)
}

func (s *Service) StoreMediaRef(ctx context.Context, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreMediaRef(ctx, sourceRef, opts)
}

func (s *Service) StoreMediaRefAtProfile(ctx context.Context, profileID, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreMediaRefAtProfile(ctx, profileID, sourceRef, opts)
}

func (s *Service) StoreMediaRefWithProfile(ctx context.Context, profile domain.StorageProfile, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreMediaRefWithProfile(ctx, profile, sourceRef, opts)
}

func (s *Service) StoreBytesRef(ctx context.Context, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreBytesRef(ctx, filename, data, opts)
}

func (s *Service) StoreBytesRefAtProfile(ctx context.Context, profileID, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreBytesRefAtProfile(ctx, profileID, filename, data, opts)
}

func (s *Service) StoreBytesRefWithProfile(ctx context.Context, profile domain.StorageProfile, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreBytesRefWithProfile(ctx, profile, filename, data, opts)
}

func (s *Service) StoreRemoteRefAtProfile(ctx context.Context, profileID, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreRemoteRefAtProfile(ctx, profileID, remoteURL, opts)
}

func (s *Service) StoreRemoteRefWithProfile(ctx context.Context, profile domain.StorageProfile, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.StoreRemoteRefWithProfile(ctx, profile, remoteURL, opts)
}

func (s *Service) UploadRefWithProfile(ctx context.Context, profile domain.StorageProfile, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	return s.runtimeService.UploadRefWithProfile(ctx, profile, file, opts)
}

func (s *Service) ReadUploadResourceByURL(ctx context.Context, rawURL string, maxBytes int64) (*ports.StoredUploadObject, error) {
	return s.runtimeService.ReadUploadResourceByURL(ctx, rawURL, maxBytes)
}

func (s *Service) PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error {
	return s.runtimeService.PutObjectAtProfile(ctx, profileID, objectKey, data, contentType)
}

func (s *Service) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error) {
	return s.runtimeService.ReadObjectAtProfile(ctx, profileID, objectKey, maxBytes)
}

func (s *Service) DeleteProfileAvatarByURL(ctx context.Context, storageUID string, rawURL string) error {
	return s.runtimeService.DeleteProfileAvatarByURL(ctx, storageUID, rawURL)
}

func (s *Service) DeleteUserUploads(ctx context.Context, userID int64, storageUID string) error {
	return s.runtimeService.DeleteUserUploads(ctx, userID, storageUID)
}

func (s *Service) DeleteObjectByKey(ctx context.Context, objectKey string) error {
	return s.runtimeService.DeleteObjectByKey(ctx, objectKey)
}

func (s *Service) DeleteObjectByStorageProfile(ctx context.Context, storageProfileID string, objectKey string) error {
	return s.runtimeService.DeleteObjectByStorageProfile(ctx, storageProfileID, objectKey)
}

func (s *Service) ListObjectKeysByStorageProfile(ctx context.Context, storageProfileID string, objectKeyPrefix string) ([]string, error) {
	return s.runtimeService.ListObjectKeysByStorageProfile(ctx, storageProfileID, objectKeyPrefix)
}

func (s *Service) DeleteUploadObjectPrefix(ctx context.Context, prefix string) error {
	return s.runtimeService.DeleteUploadObjectPrefix(ctx, prefix)
}

func (s *Service) DeleteUploadObjectPrefixes(ctx context.Context, prefixes []string) error {
	return s.runtimeService.DeleteUploadObjectPrefixes(ctx, prefixes)
}

func (s *Service) UsesObjectStorage(ctx context.Context) bool {
	return s.runtimeService.UsesObjectStorage(ctx)
}

func (s *Service) ResolveStoredUploadTarget(ctx context.Context, rawURL string) (*domain.StorageProfile, string, error) {
	return s.runtimeService.ResolveStoredUploadTarget(ctx, rawURL)
}

func (s *Service) PublicUploadURL(objectKey string) string {
	if s != nil && s.runtimeService != nil {
		return s.runtimeService.PublicUploadURL(objectKey)
	}
	return publicUploadURL(objectKey)
}

func (s *Service) Presign(ctx context.Context, req ports.PresignRequest) (ports.PresignedURL, error) {
	return s.runtimeService.Presign(ctx, req)
}

func publicUploadURL(objectKey string) string {
	return "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
}
