package infrastructure

import (
	"context"
	"encoding/json"
	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type unsplashAdapter struct{}

// NewUnsplashAdapter serves Unsplash result pages through the shared
// provider contract.
func NewUnsplashAdapter() ports.SearchProviderAdapter { return &unsplashAdapter{} }

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
	body, err := fetchProviderBody(ctx, request, unsplashTarget, []string{"api.unsplash.com", "images.unsplash.com", "plus.unsplash.com", "unsplash.com"})
	if err != nil {
		return ports.SearchProviderPage{}, err
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
