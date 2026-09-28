package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
var providerOutbound = outboundhttp.NewClient(outboundhttp.Config{})

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

type unsplashAdapter struct{}

// NewUnsplashAdapter serves Unsplash result pages through the shared
// provider contract.
func NewUnsplashAdapter() ports.SearchProviderAdapter { return &unsplashAdapter{} }

// NewPexelsAdapter serves Pexels result pages through the shared provider
// contract.
func NewPexelsAdapter() ports.SearchProviderAdapter { return &pexelsAdapter{} }

func (*unsplashAdapter) Provider() domain.Provider { return domain.ProviderUnsplash }

type unsplashPhoto struct {
	ID, Description, AltDescription string
	Width, Height                   int
	URLs                            struct {
		Thumb, Small, Regular, Raw string
	} `json:"urls"`
	User struct {
		Name  string
		Links struct {
			HTML string `json:"html"`
		} `json:"links"`
	} `json:"user"`
	Links struct {
		HTML string `json:"html"`
	} `json:"links"`
}

func (unsplashAdapter) Search(ctx context.Context, key string, search ports.SearchRequest) (ports.SearchProviderPage, error) {
	endpoint := "https://api.unsplash.com/photos"
	if search.Query != "" {
		endpoint = "https://api.unsplash.com/search/photos"
	}
	values := url.Values{"page": {strconv.Itoa(search.Page)}, "per_page": {strconv.Itoa(search.Limit)}}
	query := strings.TrimSpace(search.Query)
	if query != "" {
		values.Set("query", query)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	request.Header.Set("Authorization", "Client-ID "+key)
	response, err := providerOutbound.Do(ctx, request, outboundhttp.RequestOptions{
		Timeout:    providerSearchTimeout,
		AllowHosts: append(outbound.AllowHosts(ctx), "api.unsplash.com", "images.unsplash.com", "plus.unsplash.com", "unsplash.com"),
		Target:     unsplashTarget,
	})
	if err != nil {
		var statusErr *outboundhttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			return ports.SearchProviderPage{}, fmt.Errorf("unsplash status %d", statusErr.StatusCode)
		}
		return ports.SearchProviderPage{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderBody))
	if err != nil {
		return ports.SearchProviderPage{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ports.SearchProviderPage{}, fmt.Errorf("unsplash status %d", response.StatusCode)
	}
	var photos []unsplashPhoto
	hasMore := false
	if query == "" {
		if err := json.Unmarshal(body, &photos); err != nil {
			return ports.SearchProviderPage{}, err
		}
		hasMore = len(photos) == search.Limit
	} else {
		var payload struct {
			Results    []unsplashPhoto `json:"results"`
			TotalPages int             `json:"total_pages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return ports.SearchProviderPage{}, err
		}
		photos, hasMore = payload.Results, payload.TotalPages == 0 || search.Page < payload.TotalPages
	}
	results := make([]domain.Result, 0, len(photos))
	for _, item := range photos {
		results = append(results, domain.Result{ID: item.ID, Provider: domain.ProviderUnsplash, Title: firstNonEmpty(item.Description, item.AltDescription), Description: item.AltDescription, Author: item.User.Name, AuthorURL: item.User.Links.HTML, SourceURL: item.Links.HTML, ThumbnailURL: item.URLs.Thumb, PreviewURL: firstNonEmpty(item.URLs.Regular, item.URLs.Small), OriginalURL: item.URLs.Raw, Width: item.Width, Height: item.Height, MediaKind: "image"})
	}
	return ports.SearchProviderPage{Results: results, HasMore: hasMore}, nil
}

type pexelsAdapter struct{}

func (*pexelsAdapter) Provider() domain.Provider { return domain.ProviderPexels }
func (pexelsAdapter) Search(ctx context.Context, key string, search ports.SearchRequest) (ports.SearchProviderPage, error) {
	endpoint := "https://api.pexels.com/v1/curated"
	if search.Query != "" {
		endpoint = "https://api.pexels.com/v1/search"
	}
	values := url.Values{"page": {strconv.Itoa(search.Page)}, "per_page": {strconv.Itoa(search.Limit)}}
	query := strings.TrimSpace(search.Query)
	if query != "" {
		values.Set("query", query)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	request.Header.Set("Authorization", key)
	response, err := providerOutbound.Do(ctx, request, outboundhttp.RequestOptions{
		Timeout:    providerSearchTimeout,
		AllowHosts: append(outbound.AllowHosts(ctx), "api.pexels.com", "images.pexels.com", "pexels.com"),
		Target:     pexelsTarget,
	})
	if err != nil {
		var statusErr *outboundhttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			return ports.SearchProviderPage{}, fmt.Errorf("pexels status %d", statusErr.StatusCode)
		}
		return ports.SearchProviderPage{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderBody))
	if err != nil {
		return ports.SearchProviderPage{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ports.SearchProviderPage{}, fmt.Errorf("pexels status %d", response.StatusCode)
	}
	var payload struct {
		Photos []struct {
			ID              int    `json:"id"`
			Width           int    `json:"width"`
			Height          int    `json:"height"`
			Alt             string `json:"alt"`
			Photographer    string `json:"photographer"`
			PhotographerURL string `json:"photographer_url"`
			URL             string `json:"url"`
			Src             struct {
				Medium   string `json:"medium"`
				Large    string `json:"large"`
				Original string `json:"original"`
			} `json:"src"`
		} `json:"photos"`
		NextPage string `json:"next_page"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ports.SearchProviderPage{}, err
	}
	results := make([]domain.Result, 0, len(payload.Photos))
	for _, item := range payload.Photos {
		results = append(results, domain.Result{ID: strconv.Itoa(item.ID), Provider: domain.ProviderPexels, Title: item.Alt, Description: item.Alt, Author: item.Photographer, AuthorURL: item.PhotographerURL, SourceURL: item.URL, ThumbnailURL: item.Src.Medium, PreviewURL: item.Src.Large, OriginalURL: item.Src.Original, Width: item.Width, Height: item.Height, MediaKind: "image"})
	}
	return ports.SearchProviderPage{Results: results, HasMore: payload.NextPage != ""}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
