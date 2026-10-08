package infrastructure

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
	"ideaboard/api/internal/platform/outboundhttp"
	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
)

const providerTestTimeout = 8 * time.Second

// providerOutbound is the shared standard outbound client for supplier
// calls: timeout cap, bounded retries and Retry-After respect match the
// gateway migration (#766). Per-call Timeout/Target stay explicit so
// Unsplash and Pexels remain separately observable.
type providerHTTPClient interface {
	Do(context.Context, *http.Request, outboundhttp.RequestOptions) (*http.Response, error)
}

var providerOutbound providerHTTPClient = outboundhttp.NewClient(outboundhttp.Config{})

// providerTarget names supplier egress in outbound logs/metrics.
const (
	unsplashTarget = "unsplash"
	pexelsTarget   = "pexels"
)

type httpProviderAdapter struct {
	provider domain.Provider
	request  func(context.Context, string) (*http.Request, error)
}

// NewProviderTestAdapters returns one connection-test adapter per web search
// supplier, in display order.
func NewProviderTestAdapters() []ports.ProviderTestAdapter {
	return []ports.ProviderTestAdapter{
		&httpProviderAdapter{provider: domain.ProviderUnsplash, request: unsplashTestRequest},
		&httpProviderAdapter{provider: domain.ProviderPexels, request: pexelsTestRequest},
	}
}

func (a *httpProviderAdapter) Provider() domain.Provider { return a.provider }

func (a *httpProviderAdapter) Test(ctx context.Context, apiKey string) (domain.FailureCategory, string, error) {
	request, err := a.request(ctx, apiKey)
	if err != nil {
		return domain.FailureUnknown, msgTest, err
	}
	allowHosts := append(outbound.AllowHosts(ctx), a.providerHost())
	response, err := providerOutbound.Do(ctx, request, outboundhttp.RequestOptions{
		Timeout:    providerTestTimeout,
		AllowHosts: allowHosts,
		Target:     a.providerTarget(),
	})
	if err != nil {
		var statusErr *outboundhttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			return providerTestStatusFailure(statusErr.StatusCode)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.FailureNetwork, msgTest2, err
		}
		return domain.FailureNetwork, msgTest3, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if readErr != nil {
		return domain.FailureResponse, msgTest4, readErr
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return domain.FailureCredentials, msgTest5, fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return domain.FailureQuota, msgTest6, fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.FailureUnknown, msgTest7, fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	if len(body) == 0 {
		return domain.FailureResponse, msgTest8, errors.New("empty provider response")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return domain.FailureResponse, msgTest9, err
	}
	return "", msgTest10, nil
}

func (a *httpProviderAdapter) providerHost() string {
	if a.provider == domain.ProviderPexels {
		return "api.pexels.com"
	}
	return "api.unsplash.com"
}

func (a *httpProviderAdapter) providerTarget() string {
	if a.provider == domain.ProviderPexels {
		return pexelsTarget
	}
	return unsplashTarget
}

// providerTestStatusFailure maps a terminal supplier status to the same
// failure categories as a single-shot response, so bounded retries never
// change connection-test verdicts: exhausted 429/5xx answers classify
// exactly like the responses they replace.
func providerTestStatusFailure(status int) (domain.FailureCategory, string, error) {
	err := fmt.Errorf("provider returned status %d", status)
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.FailureCredentials, msgTest5, err
	case http.StatusTooManyRequests:
		return domain.FailureQuota, msgTest6, err
	default:
		return domain.FailureUnknown, msgTest7, err
	}
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

// providerSearchTimeout bounds one supplier page fetch; the application
// holds its own orchestration timeout separately.
const providerSearchTimeout = 12 * time.Second

// maxProviderBody caps one supplier page payload.
const maxProviderBody = 4 << 20

func fetchProviderBody(ctx context.Context, request *http.Request, target string, hosts []string) ([]byte, error) {
	response, err := providerOutbound.Do(ctx, request, outboundhttp.RequestOptions{
		Timeout:    providerSearchTimeout,
		AllowHosts: append(outbound.AllowHosts(ctx), hosts...),
		Target:     target,
	})
	if err != nil {
		var statusErr *outboundhttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			return nil, fmt.Errorf("%s status %d", target, statusErr.StatusCode)
		}
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderBody))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s status %d", target, response.StatusCode)
	}
	return body, nil
}
