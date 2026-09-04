package profiles

import "ideaboard/api/internal/media/storage/domain"

func DefaultProviderMeta() map[domain.StorageProvider]domain.StorageProviderMeta {
	meta := make(map[domain.StorageProvider]domain.StorageProviderMeta)
	for _, m := range []domain.StorageProviderMeta{
		{
			Provider:      domain.StorageProviderLocal,
			Label:         "本地存储",
			Description:   "使用服务器本地 /uploads 目录保存文件。",
			Builtin:       true,
			CreateAllowed: false,
		},
		{
			Provider:           domain.StorageProviderS3Compatible,
			Label:              "S3 兼容存储",
			Description:        "统一接入 Amazon S3、阿里云 OSS、腾讯云 COS、Cloudflare R2、MinIO 等 S3 兼容对象存储。",
			Builtin:            false,
			CreateAllowed:      true,
			SupportsTest:       true,
			SupportsBucketInfo: true,
			Fields: []domain.StorageProviderField{
				{Key: "access_key_id", Label: "Access Key ID", Type: domain.StorageFieldTypeText, Placeholder: "请输入 Access Key ID", Required: true},
				{Key: "secret_access_key", Label: "Secret Access Key", Type: domain.StorageFieldTypePassword, Placeholder: "请输入 Secret Access Key", Required: true, Secret: true},
				{Key: "bucket_name", Label: "Bucket 名称", Type: domain.StorageFieldTypeText, Placeholder: "请输入 Bucket 名称", Required: true},
				{Key: "endpoint", Label: "Endpoint", Type: domain.StorageFieldTypeText, Placeholder: "例如：s3.us-east-1.amazonaws.com / <account>.r2.cloudflarestorage.com", Required: true},
				{Key: "region_id", Label: "地域 / Region", Type: domain.StorageFieldTypeText, Placeholder: "例如：us-east-1，可留空"},
				{Key: "access_domain", Label: "公开访问域名", Type: domain.StorageFieldTypeText, Placeholder: "例如：https://bucket.s3.us-east-1.amazonaws.com", Hint: "直接拼接对象 Key；若服务需要 Bucket 路径，请在域名中包含 /bucket。", Required: true},
				{Key: "force_path_style", Label: "Path Style", Type: domain.StorageFieldTypeSwitch, Hint: "部分兼容存储需要使用 path-style 访问。"},
			},
		},
	} {
		meta[m.Provider] = m
	}
	return meta
}

func ListProviderMeta(meta map[domain.StorageProvider]domain.StorageProviderMeta) []domain.StorageProviderMeta {
	providers := []domain.StorageProviderMeta{
		meta[domain.StorageProviderLocal],
		meta[domain.StorageProviderS3Compatible],
	}
	for i := range providers {
		if providers[i].Fields == nil {
			providers[i].Fields = []domain.StorageProviderField{}
		}
	}
	return providers
}
