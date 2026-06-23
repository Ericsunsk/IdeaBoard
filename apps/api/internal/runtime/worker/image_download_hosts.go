package worker

import (
	"context"
	"net/url"
	"strings"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
)

func trustedImageDownloadHosts(ctx context.Context, account *accounts.ApiAccount, provider settings.PublicURLSettingProvider) []string {
	hosts := map[string]struct{}{}
	appendHost := func(raw string) {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err == nil && strings.TrimSpace(parsed.Host) != "" {
			hosts[strings.TrimSpace(parsed.Host)] = struct{}{}
			return
		}
		host := strings.TrimSpace(raw)
		if host != "" {
			hosts[host] = struct{}{}
		}
	}

	if account != nil {
		appendHost(account.ProxyURL)
	}
	for _, host := range trustedRuntimeAssetHosts(ctx, provider) {
		appendHost(host)
	}
	for _, host := range outbound.AllowHostsFromEnv() {
		appendHost(host)
	}

	out := make([]string, 0, len(hosts))
	for host := range hosts {
		out = append(out, host)
	}
	return out
}
