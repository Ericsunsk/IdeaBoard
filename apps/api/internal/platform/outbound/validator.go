package outbound

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
)

var policyProvider struct {
	sync.RWMutex
	value settings.OutboundPolicySettingProvider
}

func ConfigurePolicyProvider(provider settings.OutboundPolicySettingProvider) func() {
	policyProvider.Lock()
	previous := policyProvider.value
	policyProvider.value = provider
	policyProvider.Unlock()
	return func() {
		policyProvider.Lock()
		policyProvider.value = previous
		policyProvider.Unlock()
	}
}

// AllowHosts loads the current policy for every outbound operation so Admin changes take effect without restart.
func AllowHosts(ctx context.Context) []string {
	policyProvider.RLock()
	provider := policyProvider.value
	policyProvider.RUnlock()
	if provider == nil {
		return nil
	}
	setting, err := provider.GetOutboundPolicy(ctx)
	if err != nil {
		slog.WarnContext(ctx, "outbound policy read failed; private egress remains blocked", "err", err)
		return nil
	}
	return append([]string(nil), setting.AllowHosts...)
}

func ValidateConfiguredEndpoint(raw string, allowHosts ...string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := safehttp.ValidateURLWithAllowHosts(trimmed, allowHosts...)
	if err != nil {
		return "", errors.New(msgValidateConfiguredEndpoint)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
