package safehttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultTimeout = 15 * time.Second

var safeTransportPool sync.Map

type resolvedPolicy struct {
	parsed           *url.URL
	allowedHosts     map[string]bool
	allowedHostPorts map[string]bool
	allowedIPs       map[string]bool
	allowedIPPorts   map[string]bool
}

func ValidateURL(rawURL string) (*url.URL, error) {
	return ValidateURLWithAllowHosts(rawURL)
}

func ValidateURLWithAllowHosts(rawURL string, allowHosts ...string) (*url.URL, error) {
	policy, err := resolvePolicyWithTimeout(context.Background(), rawURL, defaultTimeout, allowHosts...)
	if err != nil {
		return nil, err
	}
	return policy.parsed, nil
}

func ValidateURLWithAllowHostsAndTimeout(rawURL string, timeout time.Duration, allowHosts ...string) (*url.URL, error) {
	policy, err := resolvePolicyWithTimeout(context.Background(), rawURL, timeout, allowHosts...)
	if err != nil {
		return nil, err
	}
	return policy.parsed, nil
}

func resolvePolicy(ctx context.Context, rawURL string, allowHosts ...string) (*resolvedPolicy, error) {
	return resolvePolicyWithTimeout(ctx, rawURL, defaultTimeout, allowHosts...)
}

func resolvePolicyWithTimeout(ctx context.Context, rawURL string, resolveTimeout time.Duration, allowHosts ...string) (*resolvedPolicy, error) {
	ctx = nonNilContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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

	if resolveTimeout <= 0 {
		resolveTimeout = defaultTimeout
	}
	resolveCtx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	normalizedAllowedHosts, normalizedAllowedHostPorts := normalizeAllowedHosts(allowHosts)
	allowedIPs, allowedIPPorts := resolveAllowedIPs(resolveCtx, normalizedAllowedHosts, normalizedAllowedHostPorts)

	addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		if ctxErr := resolveCtx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("解析主机失败")
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("主机未解析到有效地址")
	}

	if _, err := filterAllowedIPs(host, defaultPort(parsed), addrs, normalizedAllowedHosts, normalizedAllowedHostPorts, allowedIPs, allowedIPPorts); err != nil {
		return nil, err
	}

	return &resolvedPolicy{
		parsed:           parsed,
		allowedHosts:     normalizedAllowedHosts,
		allowedHostPorts: normalizedAllowedHostPorts,
		allowedIPs:       allowedIPs,
		allowedIPPorts:   allowedIPPorts,
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
	ctx = nonNilContext(ctx)
	policy, err := resolvePolicy(ctx, rawURL, allowHosts...)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, policy.parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败")
	}
	if buildRequest != nil {
		buildRequest(req)
	}

	transport, err := transportForRequest(ctx, req, policy)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout:   effectiveTimeout(timeout),
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := validateURLWithContext(req.Context(), req.URL.String(), effectiveTimeout(timeout), allowHosts...)
			return err
		},
	}

	return client.Do(req)
}

func DoRequestWithAllowHosts(
	ctx context.Context,
	req *http.Request,
	timeout time.Duration,
	allowHosts ...string,
) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("请求参数错误")
	}
	ctx = nonNilContext(ctx)
	policy, err := resolvePolicy(ctx, req.URL.String(), allowHosts...)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	transport, err := transportForRequest(ctx, req, policy)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout:   effectiveTimeout(timeout),
		Transport: transport,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := validateURLWithContext(next.Context(), next.URL.String(), effectiveTimeout(timeout), allowHosts...)
			return err
		},
	}
	return client.Do(req)
}

func validateURLWithContext(ctx context.Context, rawURL string, timeout time.Duration, allowHosts ...string) (*url.URL, error) {
	policy, err := resolvePolicyWithTimeout(ctx, rawURL, timeout, allowHosts...)
	if err != nil {
		return nil, err
	}
	return policy.parsed, nil
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func transportForRequest(ctx context.Context, req *http.Request, policy *resolvedPolicy) (*http.Transport, error) {
	transport := cloneDefaultTransport()
	allowRequestProxyDialTarget(ctx, transport, req, policy)

	key := transportPolicyKey(policy)
	if cached, ok := safeTransportPool.Load(key); ok {
		if existing, ok := cached.(*http.Transport); ok {
			return existing, nil
		}
	}

	transport.DialContext = restrictedDialContext(policy.allowedHosts, policy.allowedHostPorts, policy.allowedIPs, policy.allowedIPPorts)
	actual, _ := safeTransportPool.LoadOrStore(key, transport)
	stored, ok := actual.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("HTTP 传输配置错误")
	}
	return stored, nil
}

func transportPolicyKey(policy *resolvedPolicy) string {
	if policy == nil {
		return ""
	}
	return strings.Join([]string{
		"hosts=" + sortedMapKey(policy.allowedHosts),
		"hostports=" + sortedMapKey(policy.allowedHostPorts),
		"ips=" + sortedMapKey(policy.allowedIPs),
		"ipports=" + sortedMapKey(policy.allowedIPPorts),
	}, "|")
}

func sortedMapKey(values map[string]bool) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key, allowed := range values {
		if allowed {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func allowRequestProxyDialTarget(ctx context.Context, transport *http.Transport, req *http.Request, policy *resolvedPolicy) {
	if transport == nil || transport.Proxy == nil || req == nil || policy == nil {
		return
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil || proxyURL == nil {
		return
	}
	addProxyDialAllowance(ctx, policy, proxyURL)
}

func addProxyDialAllowance(ctx context.Context, policy *resolvedPolicy, proxyURL *url.URL) {
	if policy == nil || proxyURL == nil {
		return
	}
	host := normalizeHost(proxyURL.Hostname())
	if host == "" {
		return
	}
	if policy.allowedHosts == nil {
		policy.allowedHosts = map[string]bool{}
	}
	if policy.allowedHostPorts == nil {
		policy.allowedHostPorts = map[string]bool{}
	}
	port := defaultPort(proxyURL)
	if port == "" {
		policy.allowedHosts[host] = true
	} else {
		policy.allowedHostPorts[hostPortKey(host, port)] = true
	}

	if policy.allowedIPs == nil {
		policy.allowedIPs = map[string]bool{}
	}
	if policy.allowedIPPorts == nil {
		policy.allowedIPPorts = map[string]bool{}
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return
	}
	for _, addr := range addrs {
		if key := ipKey(addr.IP); key != "" {
			if port == "" {
				policy.allowedIPs[key] = true
			} else {
				policy.allowedIPPorts[hostPortKey(key, port)] = true
			}
		}
	}
}

func cloneDefaultTransport() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		return base.Clone()
	}
	return &http.Transport{}
}

func normalizeAllowedHosts(allowHosts []string) (map[string]bool, map[string]bool) {
	normalizedHosts := make(map[string]bool, len(allowHosts))
	normalizedHostPorts := make(map[string]bool, len(allowHosts))
	for _, raw := range allowHosts {
		host, port := normalizeAllowedTarget(raw)
		if host == "" {
			continue
		}
		if port == "" {
			normalizedHosts[host] = true
		} else {
			normalizedHostPorts[hostPortKey(host, port)] = true
		}
	}
	return normalizedHosts, normalizedHostPorts
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

func normalizeAllowedTarget(raw string) (string, string) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return normalizeHost(parsed.Hostname()), defaultPort(parsed)
	}
	if strings.Contains(value, "/") {
		return "", ""
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		return normalizeHost(host), strings.TrimSpace(port)
	}
	return normalizeHost(value), ""
}

func resolveAllowedIPs(ctx context.Context, allowedHosts map[string]bool, allowedHostPorts map[string]bool) (map[string]bool, map[string]bool) {
	ips := make(map[string]bool)
	ipPorts := make(map[string]bool)
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
	for hostPort := range allowedHostPorts {
		host, port := splitHostPortKey(hostPort)
		if host == "" || port == "" {
			continue
		}
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			key := ipKey(addr.IP)
			if key != "" {
				ipPorts[hostPortKey(key, port)] = true
			}
		}
	}
	return ips, ipPorts
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

func restrictedDialContext(allowedHosts map[string]bool, allowedHostPorts map[string]bool, allowedIPs map[string]bool, allowedIPPorts map[string]bool) func(context.Context, string, string) (net.Conn, error) {
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

		allowedAddrs, err := filterAllowedIPs(host, port, addrs, allowedHosts, allowedHostPorts, allowedIPs, allowedIPPorts)
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

func filterAllowedIPs(host string, port string, addrs []net.IPAddr, allowedHosts map[string]bool, allowedHostPorts map[string]bool, allowedIPs map[string]bool, allowedIPPorts map[string]bool) ([]net.IPAddr, error) {
	normalizedHost := normalizeHost(host)
	hostAllowed := allowedHosts[normalizedHost] || allowedHostPorts[hostPortKey(normalizedHost, port)]
	allowLoopback := allowedHosts["localhost"] || allowedHostPorts[hostPortKey("localhost", port)]

	if strings.EqualFold(host, "localhost") && !hostAllowed {
		return nil, fmt.Errorf("不允许访问本地地址")
	}

	allowed := make([]net.IPAddr, 0, len(addrs))
	for _, addr := range addrs {
		addrKey := ipKey(addr.IP)
		if isLoopbackIP(addr.IP) && allowLoopback {
			allowed = append(allowed, addr)
			continue
		}
		if isPrivateOrLocal(addr.IP) && !hostAllowed && !allowedIPs[addrKey] && !allowedIPPorts[hostPortKey(addrKey, port)] {
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

func defaultPort(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	if port := strings.TrimSpace(parsed.Port()); port != "" {
		return port
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func hostPortKey(host string, port string) string {
	host = normalizeHost(host)
	port = strings.TrimSpace(port)
	if host == "" || port == "" {
		return ""
	}
	return host + "\x00" + port
}

func splitHostPortKey(key string) (string, string) {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
