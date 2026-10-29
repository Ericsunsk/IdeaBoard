package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/errorrender"
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
		return OutboundPolicySetting{}, errorrender.NewValidationError(errorrender.MsgPlatformSettingsUpdateOutboundPolicy)
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
		return OutboundPolicySetting{}, errorrender.NewValidationError(errorrender.MsgNormalizeOutboundPolicySetting)
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
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errorrender.NewValidationError(errorrender.MsgNormalizeOutboundAllowHost)
	}
	// Accept pasted URLs/origins: strip one scheme and any path, query or
	// fragment, then normalize the host[:port] remainder. Only genuinely
	// unusable entries (userinfo, wildcards, whitespace, bad ports/IPs)
	// still fail.
	if i := strings.Index(value, "://"); i >= 0 {
		parsed, err := url.Parse(value)
		if err != nil || parsed.User != nil || strings.TrimSpace(parsed.Hostname()) == "" {
			return "", errorrender.NewValidationError(errorrender.MsgNormalizeOutboundAllowHost)
		}
		value = strings.TrimSpace(parsed.Hostname())
		if port := strings.TrimSpace(parsed.Port()); port != "" {
			value = net.JoinHostPort(value, port)
		}
	} else if i := strings.IndexAny(value, "/?#"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > maxOutboundAllowHostLength || strings.ContainsAny(value, " \t\n\r@\\") {
		return "", errorrender.NewValidationError(errorrender.MsgNormalizeOutboundAllowHost)
	}

	host := value
	port := ""
	portValue := 0
	if parsedHost, parsedPort, err := net.SplitHostPort(value); err == nil {
		host, port = parsedHost, parsedPort
		parsedPortValue, parseErr := strconv.Atoi(port)
		portValue = parsedPortValue
		if parseErr != nil || portValue < 1 || portValue > 65535 {
			return "", errorrender.NewValidationError(errorrender.MsgNormalizeOutboundAllowHost2)
		}
	} else if strings.HasPrefix(value, "[") || strings.Count(value, ":") > 1 {
		if net.ParseIP(value) == nil {
			return "", errorrender.NewValidationError(errorrender.MsgNormalizeOutboundAllowHost3)
		}
	}

	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return "", errorrender.NewValidationError(errorrender.MsgNormalizeOutboundAllowHost4)
	}
	if net.ParseIP(host) == nil && !validOutboundHostname(host) {
		return "", errorrender.NewValidationError(errorrender.MsgNormalizeOutboundAllowHost5)
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
