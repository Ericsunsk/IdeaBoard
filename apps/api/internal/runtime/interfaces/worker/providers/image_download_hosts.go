package providers

import (
	"context"
	"net/url"
	"strings"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/outbound"
)

func trustedImageDownloadHosts(ctx context.Context, account *accounts.ApiAccount, publicOrigins config.PublicOriginsConfig) []string {
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
	for _, host := range trustedRuntimeAssetHosts(publicOrigins) {
		appendHost(host)
	}
	for _, host := range outbound.AllowHosts(ctx) {
		appendHost(host)
	}

	out := make([]string, 0, len(hosts))
	for host := range hosts {
		out = append(out, host)
	}
	return out
}
