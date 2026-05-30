package storage

// S3CompatibleConfig 通用 S3 兼容对象存储配置
type S3CompatibleConfig struct {
	Enabled         bool   `json:"enabled"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	BucketName      string `json:"bucket_name"`
	Endpoint        string `json:"endpoint"`      // 例如 s3.us-east-1.amazonaws.com / <account>.r2.cloudflarestorage.com
	RegionID        string `json:"region_id"`     // 例如 us-east-1，可选
	AccessDomain    string `json:"access_domain"` // 例如 https://bucket.s3.us-east-1.amazonaws.com
	ForcePathStyle  bool   `json:"force_path_style"`
}

type StorageProvider string

const (
	StorageProviderLocal        StorageProvider = "local"
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
