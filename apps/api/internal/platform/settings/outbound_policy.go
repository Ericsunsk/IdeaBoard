package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/validation"
)

const (
	outboundPolicyAllowHostsKey = "api_outbound_allow_hosts"
	maxOutboundAllowHosts       = 128
	maxOutboundAllowHostLength  = 255
)

type OutboundPolicySetting struct {
	AllowHosts []string `json:"allow_hosts"`
}

type OutboundPolicySettingProvider interface {
	GetOutboundPolicy(ctx context.Context) (OutboundPolicySetting, error)
}

func DefaultOutboundPolicySetting() OutboundPolicySetting {
	return OutboundPolicySetting{AllowHosts: []string{}}
}

func (s *RuntimeSettingsService) GetOutboundPolicy(ctx context.Context) (OutboundPolicySetting, error) {
	fallback := DefaultOutboundPolicySetting()
	values, err := s.getMany(ctx, outboundPolicyAllowHostsKey)
	if err != nil {
		return fallback, err
	}
	if raw, ok := values[outboundPolicyAllowHostsKey]; ok {
		var hosts []string
		if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
			return fallback, fmt.Errorf("decode outbound policy setting: %w", err)
		}
		normalized, err := NormalizeOutboundPolicySetting(OutboundPolicySetting{AllowHosts: hosts})
		if err != nil {
			return fallback, fmt.Errorf("validate outbound policy setting: %w", err)
		}
		return normalized, nil
	}
	return fallback, nil
}

func (s *RuntimeSettingsService) UpdateOutboundPolicy(ctx context.Context, setting OutboundPolicySetting, actorID int64) (OutboundPolicySetting, error) {
	normalized, err := NormalizeOutboundPolicySetting(setting)
	if err != nil {
		return OutboundPolicySetting{}, err
	}
	if actorID <= 0 {
		return OutboundPolicySetting{}, validation.New("管理员身份无效")
	}
	metadata, err := json.Marshal(normalized)
	if err != nil {
		return OutboundPolicySetting{}, err
	}
	if s == nil || s.repo == nil {
		return OutboundPolicySetting{}, ErrRuntimeSettingsRepositoryUnavailable
	}
	allowHosts, err := json.Marshal(normalized.AllowHosts)
	if err != nil {
		return OutboundPolicySetting{}, err
	}
	if err := s.repo.SetManyWithAdminAudit(ctx, []SystemConfigValue{
		{Key: outboundPolicyAllowHostsKey, Value: string(allowHosts)},
	}, SystemConfigAdminAudit{
		ActionType: "update_outbound_policy",
		ActorID:    actorID,
		Metadata:   metadata,
	}); err != nil {
		return OutboundPolicySetting{}, err
	}
	return normalized, nil
}

func NormalizeOutboundPolicySetting(setting OutboundPolicySetting) (OutboundPolicySetting, error) {
	if len(setting.AllowHosts) > maxOutboundAllowHosts {
		return OutboundPolicySetting{}, validation.New("allow_hosts 最多支持 128 条")
	}
	normalized := make([]string, 0, len(setting.AllowHosts))
	seen := make(map[string]struct{}, len(setting.AllowHosts))
	for _, raw := range setting.AllowHosts {
		host, err := normalizeOutboundAllowHost(raw)
		if err != nil {
			return OutboundPolicySetting{}, err
		}
		if _, exists := seen[host]; exists {
			continue
		}
		seen[host] = struct{}{}
		normalized = append(normalized, host)
	}
	return OutboundPolicySetting{AllowHosts: normalized}, nil
}

func normalizeOutboundAllowHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxOutboundAllowHostLength || strings.Contains(raw, "://") || strings.ContainsAny(raw, " /?#@\\\\") {
		return "", validation.New("allow_hosts 仅支持 hostname、IP 或 host:port")
	}

	host := raw
	port := ""
	portValue := 0
	if parsedHost, parsedPort, err := net.SplitHostPort(raw); err == nil {
		host, port = parsedHost, parsedPort
		parsedPortValue, parseErr := strconv.Atoi(port)
		portValue = parsedPortValue
		if parseErr != nil || portValue < 1 || portValue > 65535 {
			return "", validation.New("allow_hosts 端口必须在 1 到 65535 之间")
		}
	} else if strings.HasPrefix(raw, "[") || strings.Count(raw, ":") > 1 {
		if net.ParseIP(raw) == nil {
			return "", validation.New("allow_hosts 包含无效 IP 地址")
		}
	}

	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return "", validation.New("allow_hosts 包含空主机")
	}
	if net.ParseIP(host) == nil && !validOutboundHostname(host) {
		return "", validation.New("allow_hosts 包含无效 hostname")
	}
	if port != "" {
		return net.JoinHostPort(host, strconv.Itoa(portValue)), nil
	}
	return host, nil
}

func validOutboundHostname(host string) bool {
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}
