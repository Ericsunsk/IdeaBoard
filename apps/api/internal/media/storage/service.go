package storage

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"

	"ideaboard/api/internal/media/ingestion"
)

// LegacyStorageKeyFromURL resolves the object key used by older ingestions
// that persisted a planned extension while the downloaded object used the
// extension from its final URL.
func LegacyStorageKeyFromURL(storageKey, rawURL string) string {
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	urlExt := strings.ToLower(path.Ext(parsed.Path))
	if len(urlExt) <= 1 || urlExt == "." {
		return ""
	}
	keyExt := strings.ToLower(path.Ext(storageKey))
	if keyExt == urlExt {
		return ""
	}
	if keyExt == "" || keyExt == "." {
		return storageKey + urlExt
	}
	return strings.TrimSuffix(storageKey, path.Ext(storageKey)) + urlExt
}

var errProviderHTTPSUnavailable = errors.New("storage profile does not expose a provider HTTPS URL")

var errStoredObjectNotFound = errors.New("stored object not found")

type StorageStoreOptions struct {
	DirPrefix  string
	AllowHosts []string
	ObjectKey  string
}

type StoredUploadRef struct {
	URL            string
	StorageKey     string
	StorageProfile string
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
	ChecksumSHA256 string
	Animated       bool
}

func (r StoredUploadRef) IngestionCompletion(sourceURL string) ingestion.Completion {
	return ingestion.Completion{
		StorageKey: r.StorageKey,
		URL:        r.URL, SourceURL: sourceURL, MIMEType: r.MIMEType,
		Width: r.Width, Height: r.Height, SizeBytes: r.SizeBytes,
		ChecksumSHA256: r.ChecksumSHA256, Animated: r.Animated,
	}
}

type StoredUploadObject struct {
	ObjectKey   string
	Filename    string
	ContentType string
	Data        []byte
}

type StoredUploadLookup struct {
	AssetID   int64
	URL       string
	ObjectKey string
}

type StoredDerivativeLookup struct {
	AssetID int64
	Tier    string
}

type StoredUploadLocation struct {
	StorageProfileID string
	StorageKey       string
}

type StoredUploadLocator interface {
	LocateStoredUpload(context.Context, StoredUploadLookup) (StoredUploadLocation, bool, error)
}

type StoredDerivativeLocator interface {
	LocateStoredDerivative(context.Context, StoredDerivativeLookup) (StoredUploadLocation, bool, error)
}

type StorageService struct {
	profileRepo         *StorageProfileRepository
	assetLocator        StoredUploadLocator
	publicAPIOrigin     string
	s3CompatibleService *S3CompatibleService
	adapterRegistry     *storageAdapterRegistry
	providerMeta        map[StorageProvider]StorageProviderMeta
}

func NewStorageService(profileRepo *StorageProfileRepository, assetLocator StoredUploadLocator) *StorageService {
	return newStorageService(profileRepo, assetLocator, "")
}

// NewStorageServiceWithPublicAPI binds local upload URLs to the API origin so
// browsers on a separate web origin do not resolve /uploads against the web host.
func NewStorageServiceWithPublicAPI(profileRepo *StorageProfileRepository, assetLocator StoredUploadLocator, publicAPIOrigin string) *StorageService {
	return newStorageService(profileRepo, assetLocator, publicAPIOrigin)
}

func newStorageService(profileRepo *StorageProfileRepository, assetLocator StoredUploadLocator, publicAPIOrigin string) *StorageService {
	service := &StorageService{
		profileRepo:         profileRepo,
		assetLocator:        assetLocator,
		publicAPIOrigin:     strings.TrimRight(strings.TrimSpace(publicAPIOrigin), "/"),
		s3CompatibleService: NewS3CompatibleService(),
		providerMeta:        make(map[StorageProvider]StorageProviderMeta),
	}
	for _, meta := range []StorageProviderMeta{
		{
			Provider:      StorageProviderLocal,
			Label:         "本地存储",
			Description:   "使用服务器本地 /uploads 目录保存文件。",
			Builtin:       true,
			CreateAllowed: false,
		},
		{
			Provider:           StorageProviderS3Compatible,
			Label:              "S3 兼容存储",
			Description:        "统一接入 Amazon S3、阿里云 OSS、腾讯云 COS、Cloudflare R2、MinIO 等 S3 兼容对象存储。",
			Builtin:            false,
			CreateAllowed:      true,
			SupportsTest:       true,
			SupportsBucketInfo: true,
			Fields: []StorageProviderField{
				{Key: "access_key_id", Label: "Access Key ID", Type: StorageFieldTypeText, Placeholder: "请输入 Access Key ID", Required: true},
				{Key: "secret_access_key", Label: "Secret Access Key", Type: StorageFieldTypePassword, Placeholder: "请输入 Secret Access Key", Required: true, Secret: true},
				{Key: "bucket_name", Label: "Bucket 名称", Type: StorageFieldTypeText, Placeholder: "请输入 Bucket 名称", Required: true},
				{Key: "endpoint", Label: "Endpoint", Type: StorageFieldTypeText, Placeholder: "例如：s3.us-east-1.amazonaws.com / <account>.r2.cloudflarestorage.com", Required: true},
				{Key: "region_id", Label: "地域 / Region", Type: StorageFieldTypeText, Placeholder: "例如：us-east-1，可留空"},
				{Key: "access_domain", Label: "公开访问域名", Type: StorageFieldTypeText, Placeholder: "例如：https://bucket.s3.us-east-1.amazonaws.com", Hint: "直接拼接对象 Key；若服务需要 Bucket 路径，请在域名中包含 /bucket。", Required: true},
				{Key: "force_path_style", Label: "Path Style", Type: StorageFieldTypeSwitch, Hint: "部分兼容存储需要使用 path-style 访问。"},
			},
		},
	} {
		service.providerMeta[meta.Provider] = meta
	}
	service.adapterRegistry = newStorageAdapterRegistry(
		newLocalStorageAdapter(service),
		newS3CompatibleStorageAdapter(service),
	)
	return service
}

func (s *StorageService) ListProviderMeta() []StorageProviderMeta {
	providers := []StorageProviderMeta{
		s.providerMeta[StorageProviderLocal],
		s.providerMeta[StorageProviderS3Compatible],
	}
	for i := range providers {
		if providers[i].Fields == nil {
			providers[i].Fields = []StorageProviderField{}
		}
	}
	return providers
}
