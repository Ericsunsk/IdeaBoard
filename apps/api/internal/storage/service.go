package storage

import (
	"api/internal/platform/settings"
	"sync"
)

type StorageStoreOptions struct {
	DirPrefix  string
	AllowHosts []string
}

type StorageService struct {
	configRepo          *settings.SystemConfigRepository
	kodoService         *KodoService
	s3CompatibleService *S3CompatibleService
	adapterRegistry     *storageAdapterRegistry
	providerMeta        map[StorageProvider]StorageProviderMeta
	previewMu           sync.RWMutex
	previewTransformer  func(string) string
	previewDirty        bool
	previewResolver     func() (*StorageProfile, error)
}

func NewStorageService(configRepo *settings.SystemConfigRepository) *StorageService {
	service := &StorageService{
		configRepo:          configRepo,
		kodoService:         NewKodoService(configRepo),
		s3CompatibleService: NewS3CompatibleService(),
		providerMeta:        make(map[StorageProvider]StorageProviderMeta),
		previewDirty:        true,
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
			Provider:           StorageProviderKodo,
			Label:              "Kodo",
			Description:        "七牛云 Kodo 对象存储，支持直传抓取与 Bucket 域名读取。",
			Builtin:            true,
			CreateAllowed:      true,
			SupportsTest:       true,
			SupportsBucketInfo: true,
			Fields: []StorageProviderField{
				{Key: "access_key", Label: "Access Key", Type: StorageFieldTypeText, Placeholder: "请输入 Access Key", Required: true},
				{Key: "secret_key", Label: "Secret Key", Type: StorageFieldTypePassword, Placeholder: "请输入 Secret Key", Required: true, Secret: true},
				{Key: "bucket_name", Label: "Bucket 名称", Type: StorageFieldTypeText, Placeholder: "请输入 Bucket 名称", Required: true},
				{Key: "upload_domain", Label: "上传域名", Type: StorageFieldTypeText, Placeholder: "留空则自动检测，例如 up-z2.qiniup.com"},
				{Key: "access_domain", Label: "访问域名", Type: StorageFieldTypeText, Placeholder: "例如：https://xxx.clouddn.com", Required: true},
				{Key: "thumbnail_mode", Label: "缩略图模式", Type: StorageFieldTypeSwitch, Hint: "列表和画廊优先走缩略图域名参数。"},
				{Key: "direct_fetch", Label: "直传模式", Type: StorageFieldTypeSwitch, Hint: "让 Kodo 直接抓取上游 URL，减少业务服务器中转。"},
			},
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
				{Key: "access_domain", Label: "访问域名", Type: StorageFieldTypeText, Placeholder: "例如：https://bucket.s3.us-east-1.amazonaws.com", Required: true},
				{Key: "force_path_style", Label: "Path Style", Type: StorageFieldTypeSwitch, Hint: "部分兼容存储需要使用 path-style 访问。"},
			},
		},
	} {
		service.providerMeta[meta.Provider] = meta
	}
	service.adapterRegistry = newStorageAdapterRegistry(
		newLocalStorageAdapter(service),
		newKodoStorageAdapter(service),
		newS3CompatibleStorageAdapter(service),
	)
	return service
}

func (s *StorageService) ListProviderMeta() []StorageProviderMeta {
	return []StorageProviderMeta{
		s.providerMeta[StorageProviderLocal],
		s.providerMeta[StorageProviderKodo],
		s.providerMeta[StorageProviderS3Compatible],
	}
}
