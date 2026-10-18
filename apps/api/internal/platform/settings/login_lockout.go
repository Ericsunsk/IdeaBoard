package settings

import (
	"context"
	"strconv"

	"ideaboard/api/internal/platform/errorrender"
)

type LoginLockoutSetting struct {
	Threshold       int `json:"threshold"`
	DurationSeconds int `json:"duration_seconds"`
}

const (
	loginLockoutThresholdKey       = "auth_login_lockout_threshold"
	loginLockoutDurationSecondsKey = "auth_login_lockout_duration_seconds"
	defaultLoginLockoutThreshold   = 10
	minLoginLockoutThreshold       = 3
	maxLoginLockoutThreshold       = 100
	defaultLoginLockoutDuration    = 900
	minLoginLockoutDuration        = 60
	maxLoginLockoutDuration        = 86400
)

func DefaultLoginLockoutSetting() LoginLockoutSetting {
	return LoginLockoutSetting{Threshold: defaultLoginLockoutThreshold, DurationSeconds: defaultLoginLockoutDuration}
}

func (s *RuntimeSettingsService) GetLoginLockout(ctx context.Context) (LoginLockoutSetting, error) {
	setting := DefaultLoginLockoutSetting()
	values, err := s.getMany(ctx, loginLockoutThresholdKey, loginLockoutDurationSecondsKey)
	if err != nil {
		return setting, err
	}
	if parsed, ok := parseBoundedInt(values[loginLockoutThresholdKey], minLoginLockoutThreshold, maxLoginLockoutThreshold); ok {
		setting.Threshold = parsed
	}
	if parsed, ok := parseBoundedInt(values[loginLockoutDurationSecondsKey], minLoginLockoutDuration, maxLoginLockoutDuration); ok {
		setting.DurationSeconds = parsed
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateLoginLockout(ctx context.Context, setting LoginLockoutSetting) (LoginLockoutSetting, error) {
	if setting.Threshold < minLoginLockoutThreshold || setting.Threshold > maxLoginLockoutThreshold {
		return LoginLockoutSetting{}, errorrender.NewValidationError(errorrender.MsgUpdateLoginLockout)
	}
	if setting.DurationSeconds < minLoginLockoutDuration || setting.DurationSeconds > maxLoginLockoutDuration {
		return LoginLockoutSetting{}, errorrender.NewValidationError(errorrender.MsgUpdateLoginLockout2)
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: loginLockoutThresholdKey, Value: strconv.Itoa(setting.Threshold)},
		{Key: loginLockoutDurationSecondsKey, Value: strconv.Itoa(setting.DurationSeconds)},
	}); err != nil {
		return LoginLockoutSetting{}, err
	}
	return setting, nil
}
