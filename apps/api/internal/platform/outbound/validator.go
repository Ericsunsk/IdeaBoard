package outbound

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"ideaboard/api/internal/platform/safehttp"
)

const allowHostsEnvKey = "API_OUTBOUND_ALLOW_HOSTS"
const mediaValidationTimeout = 1200 * time.Millisecond

func AllowHostsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv(allowHostsEnvKey))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		host := strings.TrimSpace(item)
		if host == "" {
			continue
		}
		out = append(out, host)
	}
	return out
}

func ValidateConfiguredEndpoint(raw string, allowHosts ...string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := safehttp.ValidateURLWithAllowHosts(trimmed, allowHosts...)
	if err != nil {
		return "", fmt.Errorf("无效的服务地址")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func ValidateMediaHTTPURL(raw string, allowHosts ...string) (*url.URL, error) {
	parsed, err := safehttp.ValidateURLWithAllowHostsAndTimeout(strings.TrimSpace(raw), mediaValidationTimeout, allowHosts...)
	if err != nil {
		return nil, fmt.Errorf("媒体地址不可用")
	}
	return parsed, nil
}
