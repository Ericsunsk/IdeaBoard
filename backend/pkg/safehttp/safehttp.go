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

	if strings.EqualFold(host, "localhost") {
		return nil, fmt.Errorf("不允许访问本地地址")
	}

	resolveCtx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		return nil, fmt.Errorf("解析主机失败")
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("主机未解析到有效地址")
	}

	for _, addr := range addrs {
		if isPrivateOrLocal(addr.IP) {
			return nil, fmt.Errorf("不允许访问内网或本地地址")
		}
	}

	return parsed, nil
}

func Get(ctx context.Context, rawURL string, timeout time.Duration) (*http.Response, error) {
	parsed, err := ValidateURL(rawURL)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: effectiveTimeout(timeout),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := ValidateURL(req.URL.String())
			return err
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败")
	}

	return client.Do(req)
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
