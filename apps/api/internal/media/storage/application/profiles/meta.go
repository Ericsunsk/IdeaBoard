package profiles

import "ideaboard/api/internal/media/storage/domain"

func DefaultProviderMeta() map[domain.StorageProvider]domain.StorageProviderMeta {
	meta := make(map[domain.StorageProvider]domain.StorageProviderMeta)
	for _, m := range []domain.StorageProviderMeta{
		{
			Provider:      domain.StorageProviderLocal,
			Label:         msgDefaultProviderMeta,
			Description:   msgDefaultProviderMeta2,
			Builtin:       true,
			CreateAllowed: false,
		},
		{
			Provider:           domain.StorageProviderS3Compatible,
			Label:              msgDefaultProviderMeta3,
			Description:        msgDefaultProviderMeta4,
			Builtin:            false,
			CreateAllowed:      true,
			SupportsTest:       true,
			SupportsBucketInfo: true,
			Fields: []domain.StorageProviderField{
				{Key: "access_key_id", Label: "Access Key ID", Type: domain.StorageFieldTypeText, Placeholder: msgDefaultProviderMeta5, Required: true},
				{Key: "secret_access_key", Label: "Secret Access Key", Type: domain.StorageFieldTypePassword, Placeholder: msgDefaultProviderMeta6, Required: true, Secret: true},
				{Key: "bucket_name", Label: msgDefaultProviderMeta7, Type: domain.StorageFieldTypeText, Placeholder: msgDefaultProviderMeta8, Required: true},
				{Key: "endpoint", Label: "Endpoint", Type: domain.StorageFieldTypeText, Placeholder: msgDefaultProviderMeta9, Required: true},
				{Key: "region_id", Label: msgDefaultProviderMeta10, Type: domain.StorageFieldTypeText, Placeholder: msgDefaultProviderMeta11},
				{Key: "access_domain", Label: msgDefaultProviderMeta12, Type: domain.StorageFieldTypeText, Placeholder: msgDefaultProviderMeta13, Hint: msgDefaultProviderMeta14, Required: true},
				{Key: "force_path_style", Label: "Path Style", Type: domain.StorageFieldTypeSwitch, Hint: msgDefaultProviderMeta15},
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
