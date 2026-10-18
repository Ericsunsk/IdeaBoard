package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"ideaboard/api/internal/platform/errorrender"
)

const (
	supportChannelsKey = "support_channels"
	maxSupportChannels = 20
)

type SupportChannel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sort_order"`
}

type SupportChannelsSetting struct {
	Channels []SupportChannel `json:"channels"`
}

type SupportChannelsSettingProvider interface {
	GetSupportChannels(context.Context) (SupportChannelsSetting, error)
}

func DefaultSupportChannelsSetting() SupportChannelsSetting {
	return SupportChannelsSetting{Channels: []SupportChannel{}}
}

func (s *RuntimeSettingsService) GetSupportChannels(ctx context.Context) (SupportChannelsSetting, error) {
	setting := DefaultSupportChannelsSetting()
	values, err := s.getMany(ctx, supportChannelsKey)
	if err != nil {
		return setting, err
	}
	raw, ok := values[supportChannelsKey]
	if !ok {
		return setting, nil
	}
	if err := json.Unmarshal([]byte(raw), &setting); err != nil {
		return DefaultSupportChannelsSetting(), fmt.Errorf("decode support channels setting: %w", err)
	}
	normalized, err := NormalizeSupportChannelsSetting(setting)
	if err != nil {
		return DefaultSupportChannelsSetting(), fmt.Errorf("validate support channels setting: %w", err)
	}
	return normalized, nil
}

func (s *RuntimeSettingsService) UpdateSupportChannels(ctx context.Context, setting SupportChannelsSetting) (SupportChannelsSetting, error) {
	normalized, err := NormalizeSupportChannelsSetting(setting)
	if err != nil {
		return SupportChannelsSetting{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return SupportChannelsSetting{}, err
	}
	if s == nil || s.repo == nil {
		return SupportChannelsSetting{}, ErrRuntimeSettingsRepositoryUnavailable
	}
	if err := s.repo.SetMany(ctx, []SystemConfigValue{{Key: supportChannelsKey, Value: string(payload)}}); err != nil {
		return SupportChannelsSetting{}, err
	}
	return normalized, nil
}

func NormalizeSupportChannelsSetting(setting SupportChannelsSetting) (SupportChannelsSetting, error) {
	if len(setting.Channels) > maxSupportChannels {
		return SupportChannelsSetting{}, errorrender.NewValidationError(errorrender.MsgNormalizeSupportChannels)
	}
	channels := make([]SupportChannel, 0, len(setting.Channels))
	ids := make(map[string]struct{}, len(setting.Channels))
	for _, channel := range setting.Channels {
		channel.ID = strings.TrimSpace(channel.ID)
		channel.Name = strings.TrimSpace(channel.Name)
		channel.Kind = strings.TrimSpace(channel.Kind)
		channel.URL = strings.TrimSpace(channel.URL)
		if !validSupportChannelID(channel.ID) || channel.Name == "" || utf8.RuneCountInString(channel.Name) > 80 || !validSupportChannelKind(channel.Kind) || !validSupportChannelURL(channel.URL) || channel.SortOrder < 0 {
			return SupportChannelsSetting{}, errorrender.NewValidationError(errorrender.MsgNormalizeSupportChannel)
		}
		if _, exists := ids[channel.ID]; exists {
			return SupportChannelsSetting{}, errorrender.NewValidationError(errorrender.MsgNormalizeSupportChannel2)
		}
		ids[channel.ID] = struct{}{}
		channels = append(channels, channel)
	}
	sort.SliceStable(channels, func(i, j int) bool { return channels[i].SortOrder < channels[j].SortOrder })
	return SupportChannelsSetting{Channels: channels}, nil
}

func EnabledSupportChannels(setting SupportChannelsSetting) SupportChannelsSetting {
	channels := make([]SupportChannel, 0, len(setting.Channels))
	for _, channel := range setting.Channels {
		if channel.Enabled {
			channels = append(channels, channel)
		}
	}
	return SupportChannelsSetting{Channels: channels}
}

func validSupportChannelKind(kind string) bool {
	switch kind {
	case "discord", "telegram", "whatsapp", "line", "wechat", "custom":
		return true
	default:
		return false
	}
}

func validSupportChannelURL(raw string) bool {
	if len(raw) > 2048 || strings.ContainsAny(raw, "\\\t\r\n ") {
		return false
	}
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Hostname() != "" && parsed.User == nil
}

func validSupportChannelID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, char := range id {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}
