package safehttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 15 * time.Second

func ValidateURL(rawURL string) (*url.URL, error) {
	return ValidateURLWithAllowHosts(rawURL)
}

func ValidateURLWithAllowHosts(rawURL string, allowHosts ...string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("URL 格式错误")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("仅支持 http/https URL")
	}

	host := parsed.Hostname()
	if host == "" {
		return nil, fmt.Errorf("URL 缺少主机名")
	}

	resolveCtx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	normalizedAllowedHosts := normalizeAllowedHosts(allowHosts)
	hostAllowed := normalizedAllowedHosts[normalizeHost(host)]
	allowedIPs := resolveAllowedIPs(resolveCtx, normalizedAllowedHosts)
	allowLoopback := normalizedAllowedHosts["localhost"]

	if strings.EqualFold(host, "localhost") && !hostAllowed {
		return nil, fmt.Errorf("不允许访问本地地址")
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		return nil, fmt.Errorf("解析主机失败")
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("主机未解析到有效地址")
	}

	for _, addr := range addrs {
		if isLoopbackIP(addr.IP) && allowLoopback {
			continue
		}
		if isPrivateOrLocal(addr.IP) && !hostAllowed && !allowedIPs[ipKey(addr.IP)] {
			return nil, fmt.Errorf("不允许访问内网或本地地址")
		}
	}

	return parsed, nil
}

func Get(ctx context.Context, rawURL string, timeout time.Duration) (*http.Response, error) {
	return GetWithAllowHosts(ctx, rawURL, timeout)
}

func GetWithAllowHosts(ctx context.Context, rawURL string, timeout time.Duration, allowHosts ...string) (*http.Response, error) {
	parsed, err := ValidateURLWithAllowHosts(rawURL, allowHosts...)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: effectiveTimeout(timeout),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := ValidateURLWithAllowHosts(req.URL.String(), allowHosts...)
			return err
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败")
	}

	return client.Do(req)
}

func normalizeAllowedHosts(allowHosts []string) map[string]bool {
	normalized := make(map[string]bool, len(allowHosts))
	for _, raw := range allowHosts {
		host := normalizeHost(raw)
		if host == "" {
			continue
		}
		normalized[host] = true
	}
	return normalized
}

func normalizeHost(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return strings.ToLower(parsed.Hostname())
	}
	if strings.Contains(value, "/") {
		return ""
	}
	host := value
	if strings.Contains(host, ":") {
		if parsedHost, _, err := net.SplitHostPort(host); err == nil {
			host = parsedHost
		}
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

func resolveAllowedIPs(ctx context.Context, allowedHosts map[string]bool) map[string]bool {
	ips := make(map[string]bool)
	for host := range allowedHosts {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			key := ipKey(addr.IP)
			if key != "" {
				ips[key] = true
			}
		}
	}
	return ips
}

func ipKey(ip net.IP) string {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return ""
	}
	return addr.String()
}

func effectiveTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return defaultTimeout
}

func isPrivateOrLocal(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}

	return addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified()
}

func isLoopbackIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	return ok && addr.IsLoopback()
}
