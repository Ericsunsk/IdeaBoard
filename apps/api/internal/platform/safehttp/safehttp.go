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

type resolvedPolicy struct {
	parsed       *url.URL
	allowedHosts map[string]bool
	allowedIPs   map[string]bool
}

func ValidateURL(rawURL string) (*url.URL, error) {
	return ValidateURLWithAllowHosts(rawURL)
}

func ValidateURLWithAllowHosts(rawURL string, allowHosts ...string) (*url.URL, error) {
	policy, err := resolvePolicy(rawURL, allowHosts...)
	if err != nil {
		return nil, err
	}
	return policy.parsed, nil
}

func resolvePolicy(rawURL string, allowHosts ...string) (*resolvedPolicy, error) {
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
	allowedIPs := resolveAllowedIPs(resolveCtx, normalizedAllowedHosts)

	addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		return nil, fmt.Errorf("解析主机失败")
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("主机未解析到有效地址")
	}

	if _, err := filterAllowedIPs(host, addrs, normalizedAllowedHosts, allowedIPs); err != nil {
		return nil, err
	}

	return &resolvedPolicy{
		parsed:       parsed,
		allowedHosts: normalizedAllowedHosts,
		allowedIPs:   allowedIPs,
	}, nil
}

func Get(ctx context.Context, rawURL string, timeout time.Duration) (*http.Response, error) {
	return GetWithAllowHosts(ctx, rawURL, timeout)
}

func GetWithAllowHosts(ctx context.Context, rawURL string, timeout time.Duration, allowHosts ...string) (*http.Response, error) {
	return DoWithAllowHosts(ctx, http.MethodGet, rawURL, nil, timeout, allowHosts...)
}

func DoWithAllowHosts(
	ctx context.Context,
	method string,
	rawURL string,
	buildRequest func(*http.Request),
	timeout time.Duration,
	allowHosts ...string,
) (*http.Response, error) {
	policy, err := resolvePolicy(rawURL, allowHosts...)
	if err != nil {
		return nil, err
	}

	transport := cloneDefaultTransport()
	transport.DialContext = restrictedDialContext(policy.allowedHosts, policy.allowedIPs)

	client := &http.Client{
		Timeout:   effectiveTimeout(timeout),
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := ValidateURLWithAllowHosts(req.URL.String(), allowHosts...)
			return err
		},
	}

	req, err := http.NewRequestWithContext(ctx, method, policy.parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败")
	}
	if buildRequest != nil {
		buildRequest(req)
	}

	return client.Do(req)
}

func cloneDefaultTransport() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		return base.Clone()
	}
	return &http.Transport{}
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

func restrictedDialContext(allowedHosts map[string]bool, allowedIPs map[string]bool) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   defaultTimeout,
		KeepAlive: 30 * time.Second,
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		addrs, err := lookupDialIPs(ctx, host)
		if err != nil {
			return nil, err
		}

		allowedAddrs, err := filterAllowedIPs(host, addrs, allowedHosts, allowedIPs)
		if err != nil {
			return nil, err
		}

		var lastErr error
		for _, ipAddr := range allowedAddrs {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("连接目标主机失败")
	}
}

func lookupDialIPs(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("解析主机失败")
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("主机未解析到有效地址")
	}
	return addrs, nil
}

func filterAllowedIPs(host string, addrs []net.IPAddr, allowedHosts map[string]bool, allowedIPs map[string]bool) ([]net.IPAddr, error) {
	hostAllowed := allowedHosts[normalizeHost(host)]
	allowLoopback := allowedHosts["localhost"]

	if strings.EqualFold(host, "localhost") && !hostAllowed {
		return nil, fmt.Errorf("不允许访问本地地址")
	}

	allowed := make([]net.IPAddr, 0, len(addrs))
	for _, addr := range addrs {
		if isLoopbackIP(addr.IP) && allowLoopback {
			allowed = append(allowed, addr)
			continue
		}
		if isPrivateOrLocal(addr.IP) && !hostAllowed && !allowedIPs[ipKey(addr.IP)] {
			continue
		}
		allowed = append(allowed, addr)
	}

	if len(allowed) == 0 {
		return nil, fmt.Errorf("不允许访问内网或本地地址")
	}

	return allowed, nil
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
