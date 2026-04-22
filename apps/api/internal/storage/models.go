package storage

// KodoConfig 七牛云 Kodo 对象存储配置
type KodoConfig struct {
	Enabled       bool   `json:"enabled"`
	AccessKey     string `json:"access_key"`
	SecretKey     string `json:"secret_key"`
	BucketName    string `json:"bucket_name"`    // 存储空间名称
	UploadDomain  string `json:"upload_domain"`  // 源站上传域名 e.g. up-z2.qiniup.com
	AccessDomain  string `json:"access_domain"`  // 存储空间访问域名 e.g. https://xxx.clouddn.com
	ThumbnailMode bool   `json:"thumbnail_mode"` // 缩略图模式支持
	DirectFetch   bool   `json:"direct_fetch"`   // 直传模式：七牛云直接抓取远程URL，不经过业务服务器中转
}

// S3CompatibleConfig 通用 S3 兼容对象存储配置
type S3CompatibleConfig struct {
	Enabled          bool   `json:"enabled"`
	AccessKeyID      string `json:"access_key_id"`
	SecretAccessKey  string `json:"secret_access_key"`
	BucketName       string `json:"bucket_name"`
	Endpoint         string `json:"endpoint"`       // 例如 s3.us-east-1.amazonaws.com / <account>.r2.cloudflarestorage.com
	RegionID         string `json:"region_id"`      // 例如 us-east-1，可选
	AccessDomain     string `json:"access_domain"`  // 例如 https://bucket.s3.us-east-1.amazonaws.com
	ForcePathStyle   bool   `json:"force_path_style"`
}

type StorageProvider string

const (
	StorageProviderLocal        StorageProvider = "local"
	StorageProviderKodo         StorageProvider = "kodo"
	StorageProviderS3Compatible StorageProvider = "s3-compatible"
)

type StorageFieldType string

const (
	StorageFieldTypeText     StorageFieldType = "text"
	StorageFieldTypePassword StorageFieldType = "password"
	StorageFieldTypeSwitch   StorageFieldType = "switch"
)

type StorageProviderField struct {
	Key         string           `json:"key"`
	Label       string           `json:"label"`
	Type        StorageFieldType `json:"type"`
	Placeholder string           `json:"placeholder,omitempty"`
	Hint        string           `json:"hint,omitempty"`
	Required    bool             `json:"required,omitempty"`
	Secret      bool             `json:"secret,omitempty"`
}

type StorageProviderMeta struct {
	Provider           StorageProvider        `json:"provider"`
	Label              string                 `json:"label"`
	Description        string                 `json:"description"`
	Builtin            bool                   `json:"builtin"`
	CreateAllowed      bool                   `json:"create_allowed"`
	SupportsTest       bool                   `json:"supports_test"`
	SupportsBucketInfo bool                   `json:"supports_bucket_info"`
	Fields             []StorageProviderField `json:"fields"`
}

type StorageProfile struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Provider         StorageProvider        `json:"provider"`
	Enabled          bool                   `json:"enabled"`
	IsCurrent        bool                   `json:"is_current"`
	Builtin          bool                   `json:"builtin"`
	SecretConfigured bool                   `json:"secret_configured,omitempty"`
	Config           map[string]interface{} `json:"config"`
}

type StorageProfilesConfig struct {
	Profiles []StorageProfile `json:"profiles"`
}
