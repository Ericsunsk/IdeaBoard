// Package egress owns the closed-world host allowlist gate for outbound
// media fetches. It is the single owner of entry normalization and
// host:port matching previously duplicated in the library materializer
// (https-only) and the workspace task result fetcher (http+https).
// Transport SSRF guards (remote downloader, safehttp policy) still apply
// below; this gate pins egress to the caller-supplied allowlist BEFORE
// any network. Errors name only the host, never the full URL or payload.
package egress

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// CheckHostAllowlist enforces the closed-world egress gate for http(s)
// provider result downloads. An empty allowlist fails closed.
func CheckHostAllowlist(rawURL string, allowHosts []string) error {
	return checkHostAllowlist(rawURL, allowHosts, true)
}

// CheckHTTPSHostAllowlist enforces the same gate for https-only provider
// fetches (library materialization). An empty allowlist fails closed.
func CheckHTTPSHostAllowlist(rawURL string, allowHosts []string) error {
	return checkHostAllowlist(rawURL, allowHosts, false)
}

func checkHostAllowlist(rawURL string, allowHosts []string, allowHTTP bool) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return errors.New("egress: fetch URL is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return errors.New("egress: fetch URL is invalid")
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "https":
	case "http":
		if !allowHTTP {
			return fmt.Errorf("egress: fetch URL scheme %q is not allowed", scheme)
		}
	default:
		return fmt.Errorf("egress: fetch URL scheme %q is not allowed", scheme)
	}
	if parsed.User != nil {
		return errors.New("egress: fetch URL is invalid")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		if scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	matched, configured := matchAllowHost(host, port, allowHosts)
	if !configured {
		return errors.New("egress: download allowlist is not configured")
	}
	if !matched {
		return fmt.Errorf("egress: host %q is not allowlisted", host)
	}
	return nil
}

// matchAllowHost reports whether host matches the allowlist and whether
// the allowlist carries any usable entry. Bare-host entries match any
// port; host:port entries match exactly.
func matchAllowHost(host, port string, allowHosts []string) (matched, configured bool) {
	for _, raw := range allowHosts {
		entryHost, entryPort := normalizeAllowEntry(raw)
		if entryHost == "" {
			continue
		}
		configured = true
		if entryHost != host {
			continue
		}
		if entryPort == "" || entryPort == port {
			return true, true
		}
	}
	return false, configured
}

// normalizeAllowEntry splits one allowlist entry into its normalized
// (host, port) pair, mirroring the outbound policy and safehttp entry
// shapes. Unusable entries read as ("", "") and are skipped.
func normalizeAllowEntry(raw string) (string, string) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
		host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
		if host == "" {
			return "", ""
		}
		port := strings.TrimSpace(parsed.Port())
		if port == "" {
			switch strings.ToLower(parsed.Scheme) {
			case "http":
				port = "80"
			case "https":
				port = "443"
			}
		}
		return host, port
	}
	if strings.Contains(value, "/") {
		return "", ""
	}
	if entryHost, entryPort, err := net.SplitHostPort(value); err == nil {
		entryHost = strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(entryHost), "[]"), "."))
		if entryHost == "" {
			return "", ""
		}
		return entryHost, strings.TrimSpace(entryPort)
	}
	host := strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(value), "[]"), "."))
	if host == "" {
		return "", ""
	}
	return host, ""
}
