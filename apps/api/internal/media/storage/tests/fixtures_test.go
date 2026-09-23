package tests

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/media/storage/application/authorization"
	"ideaboard/api/internal/media/storage/application/integrity"
	"ideaboard/api/internal/media/storage/application/taskmedia"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/infrastructure/local"
	"ideaboard/api/internal/media/storage/infrastructure/postgres"
	"ideaboard/api/internal/media/storage/infrastructure/s3"
	"ideaboard/api/internal/media/storage/ports"
	_ "ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

type (
	StorageService                = storage.Service
	StorageProfile                = domain.StorageProfile
	StorageProvider               = domain.StorageProvider
	StorageFieldType              = domain.StorageFieldType
	StorageProviderField          = domain.StorageProviderField
	StorageProviderMeta           = domain.StorageProviderMeta
	StorageSecretVersion          = domain.StorageSecretVersion
	StorageDriver                 = ports.StorageDriver
	StorageStoreOptions           = ports.StorageStoreOptions
	StoredUploadRef               = ports.StoredUploadRef
	StoredUploadObject            = ports.StoredUploadObject
	StoredUploadLookup            = ports.StoredUploadLookup
	StoredDerivativeLookup        = ports.StoredDerivativeLookup
	StoredUploadLocation          = ports.StoredUploadLocation
	StoredUploadLocator           = ports.StoredUploadLocator
	StoredDerivativeLocator       = ports.StoredDerivativeLocator
	ObjectStore                   = ports.ObjectStore
	PresignedURL                  = ports.PresignedURL
	PresignRequest                = ports.PresignRequest
	UploadResourceKind            = domain.UploadResourceKind
	UploadResource                = domain.UploadResource
	UploadCanvasResource          = domain.UploadCanvasResource
	UploadProfileAvatarResource   = domain.UploadProfileAvatarResource
	UploadTaskOutputResource      = domain.UploadTaskOutputResource
	UploadLibraryResource         = domain.UploadLibraryResource
	UploadSiteAssetResource       = domain.UploadSiteAssetResource
	UploadBoardAccessResolver     = ports.UploadBoardAccessResolver
	UploadOwnerStorageUIDResolver = ports.UploadOwnerStorageUIDResolver
	ReadableUploadAccess          = ports.ReadableUploadAccess
	ReadableUploadAccessResult    = ports.ReadableUploadAccessResult
	TaskMediaAuthorizer           = authorization.TaskMediaAuthorizer
	RuntimeTaskMediaStore         = taskmedia.RuntimeTaskMediaStore
	IntegrityVerifier             = integrity.IntegrityVerifier
	IntegrityConfig               = integrity.IntegrityConfig
	S3CompatibleService           = s3.S3CompatibleService
	S3CompatibleConfig            = s3.S3CompatibleConfig
	S3CompatibleProfileConfig     = s3.S3CompatibleProfileConfig
	LocalStorageProfileConfig     = local.LocalStorageProfileConfig
	StorageProfileRepository      struct {
		*postgres.Repository
		DB *pgxpool.Pool
	}
)

const (
	StorageProviderLocal              = domain.StorageProviderLocal
	StorageProviderS3Compatible       = domain.StorageProviderS3Compatible
	StorageSecretVersionStatusActive  = domain.StorageSecretVersionStatusActive
	StorageSecretVersionStatusRetired = domain.StorageSecretVersionStatusRetired
	StorageSecretVersionStatusRevoked = domain.StorageSecretVersionStatusRevoked
	DefaultPresignTTL                 = ports.DefaultPresignTTL
	MaxPresignTTL                     = ports.MaxPresignTTL
	UploadObjectRoot                  = domain.UploadObjectRoot
	storageSecretAccessKeyName        = "secret_access_key"

	UploadResourceKindSiteAsset     = domain.UploadResourceKindSiteAsset
	UploadResourceKindCanvas        = domain.UploadResourceKindCanvas
	UploadResourceKindProfileAvatar = domain.UploadResourceKindProfileAvatar
	UploadResourceKindTaskOutput    = domain.UploadResourceKindTaskOutput
	UploadResourceKindLibrary       = domain.UploadResourceKindLibrary
)

var (
	ErrStoredObjectNotFound           = domain.ErrStoredObjectNotFound
	ErrObjectGenerationUnavailable    = domain.ErrObjectGenerationUnavailable
	ErrStorageSecretVersionConflict   = domain.ErrStorageSecretVersionConflict
	ErrStorageProfileLocatorImmutable = domain.ErrStorageProfileLocatorImmutable
	ErrPresignInvalidProfile          = domain.ErrPresignInvalidProfile
	ErrPresignInvalidObjectKey        = domain.ErrPresignInvalidObjectKey
	ErrPresignTTLTooLong              = domain.ErrPresignTTLTooLong
	ErrPresignUnsupportedProvider     = domain.ErrPresignUnsupportedProvider
	ErrUploadAccessInvalidPath        = domain.ErrUploadAccessInvalidPath
	ErrUploadAccessDenied             = domain.ErrUploadAccessDenied
	ErrUploadAccessOwnerMismatch      = domain.ErrUploadAccessOwnerMismatch
	ErrUploadAccessDependencyMissing  = domain.ErrUploadAccessDependencyMissing
	ErrProviderHTTPSUnavailable       = domain.ErrProviderHTTPSUnavailable

	NewStorageService                 = storage.NewService
	NewStorageProfileRepository       = newStorageProfileRepository
	NewTaskMediaAuthorizer            = authorization.NewTaskMediaAuthorizer
	NewReadableUploadAccess           = authorization.NewReadableUploadAccess
	NewRuntimeTaskMediaStore          = taskmedia.NewRuntimeTaskMediaStore
	NewIntegrityVerifier              = integrity.NewIntegrityVerifier
	ParseUploadResource               = domain.ParseUploadResource
	NormalizeUploadObjectKey          = domain.NormalizeUploadObjectKey
	NormalizeCanvasStorageUserSegment = domain.NormalizeCanvasStorageUserSegment
	UserUploadObjectPrefixes          = domain.UserUploadObjectPrefixes
	ImageDerivativeObjectKey          = domain.ImageDerivativeObjectKey
	IsImageDerivativeObjectKey        = domain.IsImageDerivativeObjectKey
	PublicUploadURL                   = publicUploadURL
	S3CompatibleProfileConfigFromMap  = s3.S3CompatibleProfileConfigFromMap
)

func publicUploadURL(objectKey string) string {
	return "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
}

func newStorageProfileRepository(db *pgxpool.Pool) *StorageProfileRepository {
	return &StorageProfileRepository{Repository: postgres.NewRepository(db), DB: db}
}

func newStorageServiceTest(t *testing.T) (*StorageService, *StorageProfileRepository) {
	t.Helper()
	secretstest.EnableEncryption(t)

	db := testdb.Open(t)
	repo := NewStorageProfileRepository(db)
	service := NewStorageService(repo, nil, "")
	service.SetCapabilityProbe(func(context.Context, *S3CompatibleConfig) error { return nil })
	if err := service.EnsureDefaultProfiles(context.Background()); err != nil {
		t.Fatalf("EnsureDefaultProfiles returned error: %v", err)
	}
	return service, repo
}

func newStorageServiceTestWithoutDefault(t *testing.T) (*StorageService, *StorageProfileRepository) {
	t.Helper()
	secretstest.EnableEncryption(t)

	db := testdb.Open(t)
	repo := NewStorageProfileRepository(db)
	service := NewStorageService(repo, nil, "")
	service.SetCapabilityProbe(func(context.Context, *S3CompatibleConfig) error { return nil })
	return service, repo
}

type testStorageOutboundPolicyProvider struct{ hosts []string }

func (p testStorageOutboundPolicyProvider) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})
}

func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aCwkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode tiny png: %v", err)
	}
	return data
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp dir failed: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("expected temp dir to be empty, found %v", names)
	}
}

func builtinLocalProfileID() string {
	return "builtin-local"
}

func canonicalizeStorageProvider(provider StorageProvider) StorageProvider {
	return provider
}

func storageProfileConfigMap(provider StorageProvider, cfg map[string]interface{}) map[string]interface{} {
	switch canonicalizeStorageProvider(provider) {
	case StorageProviderLocal:
		return LocalStorageProfileConfig{}.Map()
	case StorageProviderS3Compatible:
		return S3CompatibleProfileConfigFromMap(cfg).Map()
	default:
		return cloneConfig(cfg)
	}
}

func configString(cfg map[string]interface{}, key string) string {
	if cfg == nil {
		return ""
	}
	if val, ok := cfg[key]; ok {
		switch typed := val.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
	}
	return ""
}

func configBool(cfg map[string]interface{}, key string) bool {
	if cfg == nil {
		return false
	}
	if val, ok := cfg[key]; ok {
		switch typed := val.(type) {
		case bool:
			return typed
		case string:
			return typed == "true" || typed == "1"
		case float64:
			return typed != 0
		}
	}
	return false
}

func cloneConfig(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(cfg))
	for key, value := range cfg {
		cloned[key] = value
	}
	return cloned
}
