package providers

import (
	"context"
	"os"
	"testing"

	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
)

type testOutboundPolicyProvider struct{ hosts []string }

func (p testOutboundPolicyProvider) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

func TestMain(m *testing.M) {
	safehttp.ConfigurePolicyProvider(testOutboundPolicyProvider{hosts: []string{"localhost", "127.0.0.1", "::1"}})
	os.Exit(m.Run())
}

func setTestOutboundAllowHosts(t *testing.T, hosts ...string) {
	t.Helper()
	restore := safehttp.ConfigurePolicyProvider(testOutboundPolicyProvider{hosts: hosts})
	t.Cleanup(restore)
}
