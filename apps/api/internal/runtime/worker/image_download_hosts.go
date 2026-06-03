package worker

import (
	"net/url"
	"strings"

	"api/internal/accounts"
	"api/internal/platform/outbound"
)

func trustedImageDownloadHosts(account *accounts.ApiAccount) []string {
	hosts := map[string]struct{}{}
	appendHost := func(raw string) {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
			hosts[strings.TrimSpace(parsed.Hostname())] = struct{}{}
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
	for _, host := range trustedOpenAIAssetHosts() {
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
