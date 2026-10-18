package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
)

const (
	// DefaultSearchLimit/MaxSearchLimit alias the contract kernel bounds
	// (P0-5): the search edge validates through contract.ParseSearchLimit;
	// the service keeps clamping semantics on the already-parsed value.
	DefaultSearchLimit  = contract.DefaultSearchLimit
	MaxSearchLimit      = contract.MaxSearchLimit
	MaxSearchQueryRunes = 200
	searchTimeout       = 12 * time.Second
)

type searchCursor struct {
	Unsplash     int  `json:"unsplash"`
	Pexels       int  `json:"pexels"`
	UnsplashDone bool `json:"unsplash_done,omitempty"`
	PexelsDone   bool `json:"pexels_done,omitempty"`
}

// providerConfigSource is the narrow settings seam the search service
// needs: provider listing plus key resolution. The settings-backed
// repository satisfies it structurally.
type providerConfigSource interface {
	List(context.Context) ([]domain.ProviderConfig, error)
	APIKey(context.Context, domain.Provider) (string, error)
}

type SearchService struct {
	configs  providerConfigSource
	adapters map[domain.Provider]ports.SearchProviderAdapter
}

type providerCall struct {
	provider domain.Provider
	page     ports.SearchProviderPage
	status   domain.ProviderStatus
	err      error
}

func NewSearchService(configs providerConfigSource, adapters []ports.SearchProviderAdapter) *SearchService {
	byProvider := make(map[domain.Provider]ports.SearchProviderAdapter, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			byProvider[adapter.Provider()] = adapter
		}
	}
	return &SearchService{configs: configs, adapters: byProvider}
}

func (s *SearchService) Search(ctx context.Context, rawQuery, rawCursor string, limit int) (response domain.SearchResponse, err error) {
	query, limit, cursor, err := normalizeSearchInput(rawQuery, rawCursor, limit)
	if err != nil {
		return domain.SearchResponse{}, err
	}
	if s == nil || s.configs == nil {
		return domain.SearchResponse{Query: query, Results: []domain.Result{}, HasMore: false}, nil
	}
	active, keys, err := s.resolveProviders(ctx, cursor)
	if err != nil {
		return domain.SearchResponse{}, err
	}
	if len(active) == 0 {
		return domain.SearchResponse{Query: query, Results: []domain.Result{}, HasMore: false}, nil
	}
	resultsByProvider := s.fetchProviderPages(ctx, query, limit, cursor, active, keys)
	allResults, statuses, pages, hasMore, failures := aggregateProviderResults(active, resultsByProvider, limit)
	if failures == len(active) {
		return domain.SearchResponse{}, fmt.Errorf("all search providers failed")
	}
	allResults = dedupeResults(allResults)
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}
	next := ""
	if hasMore {
		next = encodeCursor(nextCursor(cursor, active, statuses, pages))
	}
	return domain.SearchResponse{Query: query, Results: allResults, NextCursor: next, HasMore: hasMore, Partial: failures > 0, ProviderStatus: statuses}, nil
}

func normalizeSearchInput(rawQuery, rawCursor string, limit int) (string, int, searchCursor, error) {
	query, err := normalizeQuery(rawQuery)
	if err != nil {
		return "", 0, searchCursor{}, err
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	cursor, err := decodeCursor(rawCursor)
	if err != nil {
		return "", 0, searchCursor{}, err
	}
	return query, limit, cursor, nil
}

func (s *SearchService) resolveProviders(ctx context.Context, cursor searchCursor) ([]domain.Provider, map[domain.Provider]string, error) {
	configs, err := s.configs.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	active := make([]domain.Provider, 0, len(configs))
	keys := make(map[domain.Provider]string, len(configs))
	availableProviders := 0
	for _, config := range configs {
		if !domain.IsProvider(config.Provider) || !config.Enabled || config.TestState != domain.TestStatePassed {
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
		return nil, nil, nil
	}
	return active, keys, nil
}

func (s *SearchService) fetchProviderPages(ctx context.Context, query string, limit int, cursor searchCursor, active []domain.Provider, keys map[domain.Provider]string) map[domain.Provider]providerCall {
	calls := make(chan providerCall, len(active))
	var wait sync.WaitGroup
	for _, provider := range active {
		wait.Add(1)
		go func() {
			defer wait.Done()
			calls <- s.fetchProviderPage(ctx, query, limit, cursor, provider, keys[provider])
		}()
	}
	wait.Wait()
	close(calls)
	results := make(map[domain.Provider]providerCall, len(active))
	for call := range calls {
		results[call.provider] = call
	}
	return results
}

func (s *SearchService) fetchProviderPage(ctx context.Context, query string, limit int, cursor searchCursor, provider domain.Provider, key string) providerCall {
	started := time.Now()
	adapter := s.adapters[provider]
	if adapter == nil {
		return providerCall{provider: provider, status: domain.ProviderStatus{Provider: provider, Status: "failed", FailureCategory: domain.FailureUnknown, LatencyMS: time.Since(started).Milliseconds()}, err: errors.New("provider adapter unavailable")}
	}
	callCtx, cancel := context.WithTimeout(ctx, searchTimeout)
	page, callErr := adapter.Search(callCtx, key, ports.SearchRequest{Query: query, Page: cursor.page(provider), Limit: limit})
	cancel()
	status := domain.ProviderStatus{Provider: provider, Status: "ok", LatencyMS: time.Since(started).Milliseconds()}
	if callErr != nil {
		status.Status = "failed"
		status.FailureCategory = classifyProviderError(callErr)
	}
	return providerCall{provider: provider, page: page, status: status, err: callErr}
}

func aggregateProviderResults(active []domain.Provider, resultsByProvider map[domain.Provider]providerCall, limit int) ([]domain.Result, []domain.ProviderStatus, map[domain.Provider]ports.SearchProviderPage, bool, int) {
	allResults := make([]domain.Result, 0, limit*len(active))
	statuses := make([]domain.ProviderStatus, 0, len(active))
	pages := make(map[domain.Provider]ports.SearchProviderPage, len(active))
	hasMore := false
	failures := 0
	for _, provider := range active {
		call := resultsByProvider[provider]
		statuses = append(statuses, call.status)
		pages[provider] = call.page
		if call.err != nil {
			failures++
			continue
		}
		call.page.Results = filterProviderResults(provider, call.page.Results)
		allResults = append(allResults, call.page.Results...)
		hasMore = hasMore || call.page.HasMore
	}
	return allResults, statuses, pages, hasMore, failures
}

func normalizeQuery(raw string) (string, error) {
	query := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if utf8.RuneCountInString(query) > MaxSearchQueryRunes {
		return "", fmt.Errorf(errorrender.MsgNormalizeQuery, MaxSearchQueryRunes)
	}
	return query, nil
}

func decodeCursor(raw string) (searchCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return searchCursor{Unsplash: 1, Pexels: 1}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return searchCursor{}, errors.New(errorrender.MsgDecodeCursor)
	}
	var cursor searchCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Unsplash < 1 || cursor.Pexels < 1 {
		return searchCursor{}, errors.New(errorrender.MsgDecodeCursor)
	}
	return cursor, nil
}

func encodeCursor(cursor searchCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func nextCursor(cursor searchCursor, active []domain.Provider, statuses []domain.ProviderStatus, pages map[domain.Provider]ports.SearchProviderPage) searchCursor {
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
		if provider == domain.ProviderUnsplash {
			cursor.Unsplash++
		} else {
			cursor.Pexels++
		}
	}
	return cursor
}

func (c searchCursor) done(provider domain.Provider) bool {
	if provider == domain.ProviderPexels {
		return c.PexelsDone
	}
	return c.UnsplashDone
}

func (c *searchCursor) setDone(provider domain.Provider) {
	if provider == domain.ProviderPexels {
		c.PexelsDone = true
		return
	}
	c.UnsplashDone = true
}

func (c searchCursor) page(provider domain.Provider) int {
	if provider == domain.ProviderPexels {
		return c.Pexels
	}
	return c.Unsplash
}

func dedupeResults(results []domain.Result) []domain.Result {
	seen := make(map[string]struct{}, len(results))
	output := make([]domain.Result, 0, len(results))
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

func filterProviderResults(provider domain.Provider, results []domain.Result) []domain.Result {
	allowedHosts := providerResultHosts(provider)
	filtered := make([]domain.Result, 0, len(results))
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

func providerResultHosts(provider domain.Provider) []string {
	if provider == domain.ProviderPexels {
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

func classifyProviderError(err error) domain.FailureCategory {
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.FailureNetwork
	}
	return domain.FailureUnknown
}
