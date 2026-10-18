package settings

import (
	"context"
	"strconv"
	"strings"
)

type RegistrationSetting struct {
	Enabled bool `json:"enabled"`
}

const registrationEnabledKey = "auth_registration_enabled"

func DefaultRegistrationSetting() RegistrationSetting {
	return RegistrationSetting{Enabled: false}
}

func (s *RuntimeSettingsService) GetRegistration(ctx context.Context) (RegistrationSetting, error) {
	setting := DefaultRegistrationSetting()
	values, err := s.getMany(ctx, registrationEnabledKey)
	if err != nil {
		return setting, err
	}
	if raw, ok := values[registrationEnabledKey]; ok {
		setting.Enabled = strings.TrimSpace(raw) == "true"
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateRegistration(ctx context.Context, setting RegistrationSetting) (RegistrationSetting, error) {
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: registrationEnabledKey, Value: strconv.FormatBool(setting.Enabled)},
	}); err != nil {
		return RegistrationSetting{}, err
	}
	return setting, nil
}
