package worker

import (
	"testing"

	"ideaboard/api/internal/accounts"
)

func TestTrustedImageDownloadHostsIncludesProxyEnvAndTrustedAssets(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "https://assets.example.com")
	t.Setenv("API_OUTBOUND_ALLOW_HOSTS", "cdn.example.com, https://media.example.org:8443")

	got := trustedImageDownloadHosts(&accounts.ApiAccount{ProxyURL: "https://proxy.example.com/v1"})
	if len(got) != 4 {
		t.Fatalf("got %d hosts, want 4: %#v", len(got), got)
	}

	want := map[string]bool{
		"proxy.example.com":  true,
		"assets.example.com": true,
		"cdn.example.com":    true,
		"media.example.org":  true,
	}
	for _, host := range got {
		delete(want, host)
	}
	if len(want) != 0 {
		t.Fatalf("missing hosts: %#v", want)
	}
}
