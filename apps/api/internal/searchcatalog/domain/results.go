package domain

// Result is one normalized web image search hit.
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

// ProviderStatus is one provider's per-call outcome for admin observability.
type ProviderStatus struct {
	Provider        Provider        `json:"provider"`
	Status          string          `json:"status"`
	FailureCategory FailureCategory `json:"failure_category,omitempty"`
	LatencyMS       int64           `json:"latency_ms"`
}

// SearchResponse is the paged web search projection served to clients.
type SearchResponse struct {
	Query          string           `json:"query"`
	Results        []Result         `json:"results"`
	NextCursor     string           `json:"next_cursor,omitempty"`
	HasMore        bool             `json:"has_more"`
	Partial        bool             `json:"-"`
	ProviderStatus []ProviderStatus `json:"-"`
}
