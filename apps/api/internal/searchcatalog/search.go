package searchcatalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
)

const (
	DefaultSearchLimit  = 24
	MaxSearchLimit      = 48
	MaxSearchQueryRunes = 200
	searchTimeout       = 12 * time.Second
	maxProviderBody     = 4 << 20
)

var ErrNoSearchProviders = errors.New("no search providers are enabled")

type Result struct {
	ID           string   `json:"id"`
	Provider     Provider `json:"provider"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Author       string   `json:"author"`
	AuthorURL    string   `json:"author_url"`
	SourceURL    string   `json:"source_url"`
	ThumbnailURL string   `json:"thumbnail_url"`
	PreviewURL   string   `json:"preview_url"`
	OriginalURL  string   `json:"original_url"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	MediaKind    string   `json:"media_kind"`
}

type ProviderStatus struct {
	Provider        Provider        `json:"provider"`
	Status          string          `json:"status"`
	FailureCategory FailureCategory `json:"failure_category,omitempty"`
	LatencyMS       int64           `json:"latency_ms"`
}

type SearchResponse struct {
	Query          string           `json:"query"`
	Results        []Result         `json:"results"`
	NextCursor     string           `json:"next_cursor,omitempty"`
	HasMore        bool             `json:"has_more"`
	Partial        bool             `json:"-"`
	ProviderStatus []ProviderStatus `json:"-"`
}

type searchCursor struct {
	Unsplash     int  `json:"unsplash"`
	Pexels       int  `json:"pexels"`
	UnsplashDone bool `json:"unsplash_done,omitempty"`
	PexelsDone   bool `json:"pexels_done,omitempty"`
}

type SearchService struct {
	configs  *ConfigRepository
	adapters map[Provider]searchAdapter
	metrics  SearchMetrics
}

type searchAdapter interface {
	Provider() Provider
	Search(context.Context, string, int, int, string) (adapterPage, error)
}

type adapterPage struct {
	Results []Result
	HasMore bool
}

func NewSearchService(configs *ConfigRepository, metrics ...SearchMetrics) *SearchService {
	adapters := map[Provider]searchAdapter{
		ProviderUnsplash: &unsplashAdapter{},
		ProviderPexels:   &pexelsAdapter{},
	}
	recorder := SearchMetrics(noopSearchMetrics{})
	if len(metrics) > 0 && metrics[0] != nil {
		recorder = metrics[0]
	}
	return &SearchService{configs: configs, adapters: adapters, metrics: recorder}
}

func (s *SearchService) Search(ctx context.Context, rawQuery, rawCursor string, limit int) (response SearchResponse, err error) {
	started := time.Now()
	page := "append"
	metricStatuses := []ProviderStatus(nil)
	if strings.TrimSpace(rawCursor) == "" {
		page = "first"
	}
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = "failure"
		} else if response.Partial {
			outcome = "partial"
		}
		if s != nil && s.metrics != nil {
			statuses := response.ProviderStatus
			if len(statuses) == 0 {
				statuses = metricStatuses
			}
			s.metrics.RecordSearch(SearchObservation{Duration: time.Since(started), Page: page, Outcome: outcome, ResultCount: len(response.Results), ProviderStatus: statuses})
		}
	}()
	query, err := normalizeQuery(rawQuery)
	if err != nil {
		return SearchResponse{}, err
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	cursor, err := decodeCursor(rawCursor)
	if err != nil {
		return SearchResponse{}, err
	}
	if s == nil || s.configs == nil {
		return SearchResponse{}, ErrNoSearchProviders
	}
	configs, err := s.configs.List(ctx)
	if err != nil {
		return SearchResponse{}, err
	}
	type providerCall struct {
		provider Provider
		page     adapterPage
		status   ProviderStatus
		err      error
	}
	active := make([]Provider, 0, len(configs))
	keys := make(map[Provider]string, len(configs))
	availableProviders := 0
	for _, config := range configs {
		if !config.Enabled || config.TestState != TestStatePassed {
			continue
		}
		key, keyErr := s.configs.APIKey(ctx, config.Provider)
		if keyErr != nil {
			continue
		}
		availableProviders++
		if cursor.done(config.Provider) {
			continue
		}
		active = append(active, config.Provider)
		keys[config.Provider] = key
	}
	if availableProviders == 0 {
		return SearchResponse{}, ErrNoSearchProviders
	}
	if len(active) == 0 {
		return SearchResponse{Query: query, Results: []Result{}, HasMore: false}, nil
	}
	calls := make(chan providerCall, len(active))
	var wait sync.WaitGroup
	for _, provider := range active {
		provider := provider
		wait.Add(1)
		go func() {
			defer wait.Done()
			started := time.Now()
			adapter := s.adapters[provider]
			pageNumber := cursor.page(provider)
			if adapter == nil {
				calls <- providerCall{provider: provider, status: ProviderStatus{Provider: provider, Status: "failed", FailureCategory: FailureUnknown, LatencyMS: time.Since(started).Milliseconds()}, err: errors.New("provider adapter unavailable")}
				return
			}
			callCtx, cancel := context.WithTimeout(ctx, searchTimeout)
			page, callErr := adapter.Search(callCtx, keys[provider], pageNumber, limit, query)
			cancel()
			status := ProviderStatus{Provider: provider, Status: "ok", LatencyMS: time.Since(started).Milliseconds()}
			if callErr != nil {
				status.Status = "failed"
				status.FailureCategory = classifyProviderError(callErr)
			}
			calls <- providerCall{provider: provider, page: page, status: status, err: callErr}
		}()
	}
	wait.Wait()
	close(calls)
	allResults := make([]Result, 0, limit*len(active))
	statuses := make([]ProviderStatus, 0, len(active))
	pages := make(map[Provider]adapterPage, len(active))
	resultsByProvider := make(map[Provider]providerCall, len(active))
	hasMore := false
	failures := 0
	for call := range calls {
		resultsByProvider[call.provider] = call
		pages[call.provider] = call.page
	}
	for _, provider := range active {
		call := resultsByProvider[provider]
		statuses = append(statuses, call.status)
		if call.err != nil {
			failures++
			continue
		}
		call.page.Results = filterProviderResults(provider, call.page.Results)
		allResults = append(allResults, call.page.Results...)
		hasMore = hasMore || call.page.HasMore
	}
	metricStatuses = statuses
	if failures == len(active) {
		return SearchResponse{}, fmt.Errorf("all search providers failed")
	}
	allResults = dedupeResults(allResults)
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}
	next := ""
	if hasMore {
		next = encodeCursor(nextCursor(cursor, active, statuses, pages))
	}
	return SearchResponse{Query: query, Results: allResults, NextCursor: next, HasMore: hasMore, Partial: failures > 0, ProviderStatus: statuses}, nil
}

func normalizeQuery(raw string) (string, error) {
	query := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if utf8.RuneCountInString(query) > MaxSearchQueryRunes {
		return "", fmt.Errorf("搜索关键词不能超过 %d 个字符", MaxSearchQueryRunes)
	}
	return query, nil
}

func decodeCursor(raw string) (searchCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return searchCursor{Unsplash: 1, Pexels: 1}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return searchCursor{}, errors.New("分页游标无效")
	}
	var cursor searchCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Unsplash < 1 || cursor.Pexels < 1 {
		return searchCursor{}, errors.New("分页游标无效")
	}
	return cursor, nil
}

func encodeCursor(cursor searchCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func nextCursor(cursor searchCursor, active []Provider, statuses []ProviderStatus, pages map[Provider]adapterPage) searchCursor {
	for _, provider := range active {
		failed := false
		for _, status := range statuses {
			if status.Provider == provider && status.Status == "failed" {
				failed = true
			}
		}
		if failed {
			continue
		}
		page := pages[provider]
		if !page.HasMore {
			cursor.setDone(provider)
			continue
		}
		if provider == ProviderUnsplash {
			cursor.Unsplash++
		} else {
			cursor.Pexels++
		}
	}
	return cursor
}

func (c searchCursor) done(provider Provider) bool {
	if provider == ProviderPexels {
		return c.PexelsDone
	}
	return c.UnsplashDone
}

func (c *searchCursor) setDone(provider Provider) {
	if provider == ProviderPexels {
		c.PexelsDone = true
		return
	}
	c.UnsplashDone = true
}

func (c searchCursor) page(provider Provider) int {
	if provider == ProviderPexels {
		return c.Pexels
	}
	return c.Unsplash
}

func dedupeResults(results []Result) []Result {
	seen := make(map[string]struct{}, len(results))
	output := make([]Result, 0, len(results))
	for _, result := range results {
		key := string(result.Provider) + ":" + result.ID
		if result.ID == "" || result.ThumbnailURL == "" || result.PreviewURL == "" || result.SourceURL == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		output = append(output, result)
	}
	return output
}

func filterProviderResults(provider Provider, results []Result) []Result {
	allowedHosts := providerResultHosts(provider)
	filtered := make([]Result, 0, len(results))
	for _, result := range results {
		if !isProviderURL(result.ThumbnailURL, allowedHosts) ||
			!isProviderURL(result.PreviewURL, allowedHosts) ||
			!isProviderURL(result.SourceURL, allowedHosts) {
			continue
		}
		if result.OriginalURL != "" && !isProviderURL(result.OriginalURL, allowedHosts) {
			result.OriginalURL = ""
		}
		if result.AuthorURL != "" && !isProviderURL(result.AuthorURL, allowedHosts) {
			result.AuthorURL = ""
		}
		if result.Width <= 0 || result.Height <= 0 {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}

func providerResultHosts(provider Provider) []string {
	if provider == ProviderPexels {
		return []string{"images.pexels.com", "pexels.com", "www.pexels.com"}
	}
	return []string{"images.unsplash.com", "plus.unsplash.com", "unsplash.com", "www.unsplash.com"}
}

func isProviderURL(raw string, allowedHosts []string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.Port() != "" && parsed.Port() != "443" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	for _, allowed := range allowedHosts {
		if host == allowed {
			return true
		}
	}
	return false
}

type unsplashAdapter struct{}

func (*unsplashAdapter) Provider() Provider { return ProviderUnsplash }

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

func (a *unsplashAdapter) Search(ctx context.Context, key string, page, limit int, query string) (adapterPage, error) {
	endpoint := "https://api.unsplash.com/photos"
	if query != "" {
		endpoint = "https://api.unsplash.com/search/photos"
	}
	values := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(limit)}}
	if query != "" {
		values.Set("query", query)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	request.Header.Set("Authorization", "Client-ID "+key)
	response, err := safehttp.DoRequestWithAllowHosts(ctx, request, searchTimeout, append(outbound.AllowHosts(ctx), "api.unsplash.com", "images.unsplash.com", "plus.unsplash.com", "unsplash.com")...)
	if err != nil {
		return adapterPage{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderBody))
	if err != nil {
		return adapterPage{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return adapterPage{}, fmt.Errorf("unsplash status %d", response.StatusCode)
	}
	var photos []unsplashPhoto
	hasMore := false
	if query == "" {
		if err := json.Unmarshal(body, &photos); err != nil {
			return adapterPage{}, err
		}
		hasMore = len(photos) == limit
	} else {
		var payload struct {
			Results    []unsplashPhoto `json:"results"`
			TotalPages int             `json:"total_pages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return adapterPage{}, err
		}
		photos, hasMore = payload.Results, payload.TotalPages == 0 || page < payload.TotalPages
	}
	results := make([]Result, 0, len(photos))
	for _, item := range photos {
		results = append(results, Result{ID: item.ID, Provider: ProviderUnsplash, Title: firstNonEmpty(item.Description, item.AltDescription), Description: item.AltDescription, Author: item.User.Name, AuthorURL: item.User.Links.HTML, SourceURL: item.Links.HTML, ThumbnailURL: item.URLs.Thumb, PreviewURL: firstNonEmpty(item.URLs.Regular, item.URLs.Small), OriginalURL: item.URLs.Raw, Width: item.Width, Height: item.Height, MediaKind: "image"})
	}
	return adapterPage{Results: results, HasMore: hasMore}, nil
}

type pexelsAdapter struct{}

func (*pexelsAdapter) Provider() Provider { return ProviderPexels }
func (a *pexelsAdapter) Search(ctx context.Context, key string, page, limit int, query string) (adapterPage, error) {
	endpoint := "https://api.pexels.com/v1/curated"
	if query != "" {
		endpoint = "https://api.pexels.com/v1/search"
	}
	values := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(limit)}}
	if query != "" {
		values.Set("query", query)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	request.Header.Set("Authorization", key)
	response, err := safehttp.DoRequestWithAllowHosts(ctx, request, searchTimeout, append(outbound.AllowHosts(ctx), "api.pexels.com", "images.pexels.com", "pexels.com")...)
	if err != nil {
		return adapterPage{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderBody))
	if err != nil {
		return adapterPage{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return adapterPage{}, fmt.Errorf("pexels status %d", response.StatusCode)
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
		return adapterPage{}, err
	}
	results := make([]Result, 0, len(payload.Photos))
	for _, item := range payload.Photos {
		results = append(results, Result{ID: strconv.Itoa(item.ID), Provider: ProviderPexels, Title: item.Alt, Description: item.Alt, Author: item.Photographer, AuthorURL: item.PhotographerURL, SourceURL: item.URL, ThumbnailURL: item.Src.Medium, PreviewURL: item.Src.Large, OriginalURL: item.Src.Original, Width: item.Width, Height: item.Height, MediaKind: "image"})
	}
	return adapterPage{Results: results, HasMore: payload.NextPage != ""}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func classifyProviderError(err error) FailureCategory {
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureNetwork
	}
	return FailureUnknown
}
