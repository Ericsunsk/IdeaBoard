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

// NewPexelsAdapter serves Pexels result pages through the shared provider
// contract.
func NewPexelsAdapter() ports.SearchProviderAdapter { return &pexelsAdapter{} }

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
	body, err := fetchProviderBody(ctx, request, pexelsTarget, []string{"api.pexels.com", "images.pexels.com", "pexels.com"})
	if err != nil {
		return ports.SearchProviderPage{}, err
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
