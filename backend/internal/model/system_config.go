package model

import "time"

// SystemConfig 系统配置（key-value 单行存储）
type SystemConfig struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Key       string    `gorm:"type:varchar(100);not null;uniqueIndex" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// AliyunSegConfig 阿里云视觉AI分割抠图配置
type AliyunSegConfig struct {
	Enabled         bool   `json:"enabled"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	RegionID        string `json:"region_id"` // 默认 cn-shanghai
}

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
