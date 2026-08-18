package storage

import (
	"context"
	"errors"
)

var errProviderHTTPSUnavailable = errors.New("storage profile does not expose a provider HTTPS URL")

var errStoredObjectNotFound = errors.New("stored object not found")

type StorageStoreOptions struct {
	DirPrefix  string
	AllowHosts []string
}

type StoredUploadRef struct {
	URL            string
	StorageKey     string
	StorageProfile string
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
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

type StoredUploadLocation struct {
	StorageProfileID string
	StorageKey       string
}

type StoredUploadLocator interface {
	LocateStoredUpload(context.Context, StoredUploadLookup) (StoredUploadLocation, bool, error)
}

type StorageService struct {
	profileRepo         *StorageProfileRepository
	assetLocator        StoredUploadLocator
	s3CompatibleService *S3CompatibleService
	adapterRegistry     *storageAdapterRegistry
	providerMeta        map[StorageProvider]StorageProviderMeta
}

func NewStorageService(profileRepo *StorageProfileRepository, assetLocator StoredUploadLocator) *StorageService {
	service := &StorageService{
		profileRepo:         profileRepo,
		assetLocator:        assetLocator,
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
