package profiles

import (
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/platform/errorrender"
)

func DefaultProviderMeta() map[domain.StorageProvider]domain.StorageProviderMeta {
	meta := make(map[domain.StorageProvider]domain.StorageProviderMeta)
	for _, m := range []domain.StorageProviderMeta{
		{
			Provider:      domain.StorageProviderLocal,
			Label:         errorrender.MsgDefaultProviderMeta,
			Description:   errorrender.MsgDefaultProviderMeta2,
			Builtin:       true,
			CreateAllowed: false,
		},
		{
			Provider:           domain.StorageProviderS3Compatible,
			Label:              errorrender.MsgDefaultProviderMeta3,
			Description:        errorrender.MsgDefaultProviderMeta4,
			Builtin:            false,
			CreateAllowed:      true,
			SupportsTest:       true,
			SupportsBucketInfo: true,
			Fields: []domain.StorageProviderField{
				{Key: "access_key_id", Label: "Access Key ID", Type: domain.StorageFieldTypeText, Placeholder: errorrender.MsgDefaultProviderMeta5, Required: true},
				{Key: "secret_access_key", Label: "Secret Access Key", Type: domain.StorageFieldTypePassword, Placeholder: errorrender.MsgDefaultProviderMeta6, Required: true, Secret: true},
				{Key: "bucket_name", Label: errorrender.MsgDefaultProviderMeta7, Type: domain.StorageFieldTypeText, Placeholder: errorrender.MsgDefaultProviderMeta8, Required: true},
				{Key: "endpoint", Label: "Endpoint", Type: domain.StorageFieldTypeText, Placeholder: errorrender.MsgDefaultProviderMeta9, Required: true},
				{Key: "region_id", Label: errorrender.MsgDefaultProviderMeta10, Type: domain.StorageFieldTypeText, Placeholder: errorrender.MsgDefaultProviderMeta11},
				{Key: "access_domain", Label: errorrender.MsgDefaultProviderMeta12, Type: domain.StorageFieldTypeText, Placeholder: errorrender.MsgDefaultProviderMeta13, Hint: errorrender.MsgDefaultProviderMeta14, Required: true},
				{Key: "force_path_style", Label: "Path Style", Type: domain.StorageFieldTypeSwitch, Hint: errorrender.MsgDefaultProviderMeta15},
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
