package settings

import (
	"context"
	"strconv"

	"ideaboard/api/internal/platform/errorrender"
)

type MediaUploadSetting struct {
	UploadMaxConcurrent int `json:"upload_max_concurrent"`
}

type MediaUploadSettingProvider interface {
	GetMediaUpload(ctx context.Context) (MediaUploadSetting, error)
}

const (
	mediaUploadMaxConcurrentKey = "media_upload_max_concurrent"
	defaultUploadMaxConcurrent  = 8
	minUploadMaxConcurrent      = 1
	maxUploadMaxConcurrent      = 64
)

func DefaultMediaUploadSetting() MediaUploadSetting {
	return MediaUploadSetting{
		UploadMaxConcurrent: defaultUploadMaxConcurrent,
	}
}

func (s *RuntimeSettingsService) GetMediaUpload(ctx context.Context) (MediaUploadSetting, error) {
	setting := DefaultMediaUploadSetting()
	values, err := s.getMany(ctx, mediaUploadMaxConcurrentKey)
	if err != nil {
		return setting, err
	}
	if parsed, ok := parseBoundedInt(values[mediaUploadMaxConcurrentKey], minUploadMaxConcurrent, maxUploadMaxConcurrent); ok {
		setting.UploadMaxConcurrent = parsed
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateMediaUpload(ctx context.Context, setting MediaUploadSetting) (MediaUploadSetting, error) {
	normalized, err := NormalizeMediaUploadSetting(setting)
	if err != nil {
		return MediaUploadSetting{}, err
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: mediaUploadMaxConcurrentKey, Value: strconv.Itoa(normalized.UploadMaxConcurrent)},
	}); err != nil {
		return MediaUploadSetting{}, err
	}
	return normalized, nil
}

func NormalizeMediaUploadSetting(setting MediaUploadSetting) (MediaUploadSetting, error) {
	if setting.UploadMaxConcurrent < minUploadMaxConcurrent || setting.UploadMaxConcurrent > maxUploadMaxConcurrent {
		return MediaUploadSetting{}, errorrender.NewValidationError(errorrender.MsgNormalizeMediaUploadSetting)
	}
	return setting, nil
}
