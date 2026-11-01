// Package workerwire owns the Go JSON contract with the ai-proxy Worker.
// It contains no Runtime execution policy or ModelRegistry diagnostics.
package workerwire

import "encoding/json"

const (
	GeneratePath = "/v1/generate"
	DryRunPath   = "/v1/dry-run"

	ErrorBadRequest       = "BAD_REQUEST"
	ErrorUnauthorized     = "UNAUTHORIZED"
	ErrorUpstream         = "UPSTREAM_ERROR"
	ErrorUnsupportedModel = "UNSUPPORTED_MODEL"
	ErrorInputFetchFailed = "INPUT_FETCH_FAILED"
	ErrorUploadFailed     = "UPLOAD_FAILED"
	ErrorAdapterInternal  = "ADAPTER_INTERNAL_ERROR"
)

// ImageRequest is the common CanonicalImageRequest vocabulary. Pointers
// preserve absent vs explicit zero for optional numeric values.
// UploadURL belongs to the endpoint-specific envelopes below.
type ImageRequest struct {
	TaskID          string         `json:"task_id"`
	Provider        string         `json:"provider"`
	Model           string         `json:"model"`
	Prompt          string         `json:"prompt"`
	NegativePrompt  *string        `json:"negative_prompt,omitempty"`
	Width           *int           `json:"width,omitempty"`
	Height          *int           `json:"height,omitempty"`
	Steps           *int           `json:"steps,omitempty"`
	GuidanceScale   *float64       `json:"guidance_scale,omitempty"`
	Seed            *uint64        `json:"seed,omitempty"`
	Images          []string       `json:"images,omitempty"`
	MaskIndex       *int           `json:"mask_index,omitempty"`
	ProviderOptions map[string]any `json:"provider_options,omitempty"`
	PublicURL       string         `json:"public_url,omitempty"`
}

// GenerateRequest always encodes upload_url; generation requires a slot.
type GenerateRequest struct {
	ImageRequest
	UploadURL string `json:"upload_url"`
}

// DryRunRequest does not require media URLs. ModelRegistry probes only
// route facts and provider_options, never images or output slots.
type DryRunRequest struct {
	ImageRequest
	UploadURL string `json:"upload_url,omitempty"`
}

type Receipt struct {
	UpstreamStatus int     `json:"upstream_status"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	LogID          *string `json:"log_id,omitempty"`
	CacheStatus    *string `json:"cache_status,omitempty"`
}

type Success struct {
	TaskID      string  `json:"task_id"`
	Status      string  `json:"status"`
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	ImageURL    string  `json:"image_url"`
	ContentType string  `json:"content_type"`
	Seed        *uint64 `json:"seed,omitempty"`
	DurationMS  int64   `json:"duration_ms"`
	Receipt     Receipt `json:"receipt"`
}

type Failure struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
	Error  struct {
		Code           string `json:"code"`
		Message        string `json:"message"`
		UpstreamStatus *int   `json:"upstream_status,omitempty"`
	} `json:"error"`
}

type DryRunResponse struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Target   struct {
		URL    string          `json:"url"`
		Method string          `json:"method"`
		Body   json.RawMessage `json:"body"`
		// Older Worker responses placed the diagnostic label under target.
		PathSource string `json:"path_source,omitempty"`
	} `json:"target"`
	PathSource string `json:"path_source"`
}
