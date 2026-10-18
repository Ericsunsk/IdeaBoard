package settings

import (
	"context"
	"strconv"
	"time"

	"ideaboard/api/internal/platform/errorrender"
)

type RateLimitSetting struct {
	WindowSeconds int `json:"window_seconds"`
	MaxRequests   int `json:"max_requests"`
}

type RateLimitSettingProvider interface {
	GetRateLimit(ctx context.Context) (RateLimitSetting, error)
}

const (
	rateLimitWindowSecondsKey = "api_rate_limit_window_seconds"
	rateLimitMaxRequestsKey   = "api_rate_limit_max_requests"
	defaultRateLimitWindow    = 2
	defaultRateLimitMax       = 120
	maxRateLimitWindow        = 3600
	maxRateLimitRequests      = 10000
)

func DefaultRateLimitSetting() RateLimitSetting {
	return RateLimitSetting{WindowSeconds: defaultRateLimitWindow, MaxRequests: defaultRateLimitMax}
}

func (s *RuntimeSettingsService) GetRateLimit(ctx context.Context) (RateLimitSetting, error) {
	if cached, ok := s.cachedRateLimit(time.Now()); ok {
		return cached, nil
	}

	setting := DefaultRateLimitSetting()
	values, err := s.getMany(ctx, rateLimitWindowSecondsKey, rateLimitMaxRequestsKey)
	if err != nil {
		return setting, err
	}
	if parsed, ok := parseBoundedInt(values[rateLimitWindowSecondsKey], 1, maxRateLimitWindow); ok {
		setting.WindowSeconds = parsed
	}
	if parsed, ok := parseBoundedInt(values[rateLimitMaxRequestsKey], 1, maxRateLimitRequests); ok {
		setting.MaxRequests = parsed
	}
	s.writeCachedRateLimit(setting)
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateRateLimit(ctx context.Context, setting RateLimitSetting) (RateLimitSetting, error) {
	if setting.WindowSeconds < 1 || setting.WindowSeconds > maxRateLimitWindow {
		return RateLimitSetting{}, errorrender.NewValidationError(errorrender.MsgUpdateRateLimit)
	}
	if setting.MaxRequests < 1 || setting.MaxRequests > maxRateLimitRequests {
		return RateLimitSetting{}, errorrender.NewValidationError(errorrender.MsgUpdateRateLimit2)
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: rateLimitWindowSecondsKey, Value: strconv.Itoa(setting.WindowSeconds)},
		{Key: rateLimitMaxRequestsKey, Value: strconv.Itoa(setting.MaxRequests)},
	}); err != nil {
		return RateLimitSetting{}, err
	}
	s.publishRateLimit(ctx, setting)
	return setting, nil
}
