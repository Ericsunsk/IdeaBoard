package providers

import (
	"context"
	"testing"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/platform/config"
)

func TestTrustedImageDownloadHostsIncludesProxyEnvAndTrustedAssets(t *testing.T) {
	setTestOutboundAllowHosts(t, "cdn.example.com", "media.example.org:8443")

	got := trustedImageDownloadHosts(
		context.Background(),
		&accounts.ApiAccount{ProxyURL: "https://proxy.example.com/v1"},
		config.PublicOriginsConfig{API: "https://assets.example.com"},
	)
	if len(got) != 4 {
		t.Fatalf("got %d hosts, want 4: %#v", len(got), got)
	}

	want := map[string]bool{
		"proxy.example.com":      true,
		"assets.example.com":     true,
		"cdn.example.com":        true,
		"media.example.org:8443": true,
	}
	for _, host := range got {
		delete(want, host)
	}
	if len(want) != 0 {
		t.Fatalf("missing hosts: %#v", want)
	}
}
