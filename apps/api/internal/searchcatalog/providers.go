package searchcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
)

const providerTestTimeout = 8 * time.Second

type providerAdapter interface {
	Provider() Provider
	Test(context.Context, string) (FailureCategory, string, error)
}

type httpProviderAdapter struct {
	provider Provider
	request  func(context.Context, string) (*http.Request, error)
}

func newProviderAdapters() []providerAdapter {
	return []providerAdapter{
		&httpProviderAdapter{provider: ProviderUnsplash, request: unsplashTestRequest},
		&httpProviderAdapter{provider: ProviderPexels, request: pexelsTestRequest},
	}
}

func (a *httpProviderAdapter) Provider() Provider { return a.provider }

func (a *httpProviderAdapter) Test(ctx context.Context, apiKey string) (FailureCategory, string, error) {
	request, err := a.request(ctx, apiKey)
	if err != nil {
		return FailureUnknown, "请求构造失败", err
	}
	allowHosts := append(outbound.AllowHosts(ctx), a.providerHost())
	response, err := safehttp.DoRequestWithAllowHosts(ctx, request, providerTestTimeout, allowHosts...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return FailureNetwork, "请求超时", err
		}
		return FailureNetwork, "网络不可达", err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if readErr != nil {
		return FailureResponse, "响应读取失败", readErr
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return FailureCredentials, "凭证无效或无权访问", fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return FailureQuota, "请求频率或配额受限", fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return FailureUnknown, "供应商拒绝了测试请求", fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	if len(body) == 0 {
		return FailureResponse, "供应商返回为空响应", errors.New("empty provider response")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return FailureResponse, "供应商响应格式无效", err
	}
	return "", "连接测试通过", nil
}

func (a *httpProviderAdapter) providerHost() string {
	if a.provider == ProviderPexels {
		return "api.pexels.com"
	}
	return "api.unsplash.com"
}

func unsplashTestRequest(ctx context.Context, apiKey string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.unsplash.com/photos/random?client_id="+urlQueryEscape(apiKey), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept-Version", "v1")
	return request, nil
}

func pexelsTestRequest(ctx context.Context, apiKey string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.pexels.com/v1/search?query=texture&per_page=1", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", apiKey)
	return request, nil
}

func urlQueryEscape(value string) string {
	return strings.NewReplacer("%", "%25", "&", "%26", "?", "%3F", "#", "%23", "+", "%2B", " ", "%20").Replace(value)
}
