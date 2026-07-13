package outbound

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/settings"
)

type mutablePolicyProvider struct {
	setting settings.OutboundPolicySetting
	err     error
}

func (p *mutablePolicyProvider) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return p.setting, p.err
}

func TestAllowHostsReadsCurrentPolicyAndFailsClosed(t *testing.T) {
	provider := &mutablePolicyProvider{setting: settings.OutboundPolicySetting{AllowHosts: []string{"first.internal"}}}
	restore := ConfigurePolicyProvider(provider)
	t.Cleanup(restore)

	if got := AllowHosts(context.Background()); len(got) != 1 || got[0] != "first.internal" {
		t.Fatalf("initial hosts = %#v", got)
	}
	provider.setting = settings.OutboundPolicySetting{AllowHosts: []string{"second.internal"}}
	if got := AllowHosts(context.Background()); len(got) != 1 || got[0] != "second.internal" {
		t.Fatalf("updated hosts = %#v, want dynamic read", got)
	}
	provider.err = errors.New("database unavailable")
	if got := AllowHosts(context.Background()); len(got) != 0 {
		t.Fatalf("hosts on provider failure = %#v, want fail closed", got)
	}
}
