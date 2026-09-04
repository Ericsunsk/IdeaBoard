package domain

import "time"

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

const (
	StorageSecretVersionStatusActive  = "active"
	StorageSecretVersionStatusRetired = "retired"
	StorageSecretVersionStatusRevoked = "revoked"
)

type StorageSecretVersion struct {
	ID             int64      `json:"id"`
	SecretName     string     `json:"secret_name"`
	VersionNumber  int32      `json:"version_number"`
	Status         string     `json:"status"`
	RotatedAt      time.Time  `json:"rotated_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	RetiredAt      *time.Time `json:"retired_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	HasSecretValue bool       `json:"has_secret_value"`
}

type StorageProfile struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Provider         StorageProvider        `json:"provider"`
	Enabled          bool                   `json:"enabled"`
	IsCurrent        bool                   `json:"is_current"`
	Builtin          bool                   `json:"builtin"`
	SecretConfigured bool                   `json:"secret_configured,omitempty"`
	SecretVersions   []StorageSecretVersion `json:"-"`
	Config           map[string]interface{} `json:"config"`
}
