package ports

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/workerwire"
)

// WorkerDefaultTimeout bounds one Worker attempt. It nests inside the
// worker 360s execution budget, which nests inside the River 17min job
// timeout (ADR-0010). Single source for the 300s budget: Worker client
// default and provider execution share it.
const WorkerDefaultTimeout = 300 * time.Second

// Runtime failure classifiers use the shared Worker wire error codes.
const (
	WorkerErrorBadRequest       = workerwire.ErrorBadRequest
	WorkerErrorUnauthorized     = workerwire.ErrorUnauthorized
	WorkerErrorUpstream         = workerwire.ErrorUpstream
	WorkerErrorUnsupportedModel = workerwire.ErrorUnsupportedModel
	WorkerErrorInputFetchFailed = workerwire.ErrorInputFetchFailed
	WorkerErrorUploadFailed     = workerwire.ErrorUploadFailed
	WorkerErrorAdapterInternal  = workerwire.ErrorAdapterInternal
)

// WorkerRequest is one image generation call through the sole Worker egress
// (ai-proxy service). Field names mirror CanonicalImageRequest: the
// Worker owns shaping, provider routing, and retries. The API only sends
// canonical task facts plus presigned media URLs (explicit MaskIndex;
// single presigned PUT for the output).
type WorkerRequest struct {
	// TaskID is the workspace task public id.
	TaskID string
	// Provider is the bare Worker provider slug (workers-ai, fal, openai).
	Provider string
	// Model is the bare model name (no provider prefix, no path override).
	Model string
	// Prompt is the canonical compiled prompt.
	Prompt string
	// Width/Height are the planned output pixels; zero omits (Worker defaults).
	Width  int
	Height int
	// Seed pins deterministic generation; always set from the snapshot.
	Seed uint64
	// Images carries ordered presigned GET URLs; MaskIndex identifies an optional mask.
	Images []string
	// MaskIndex explicitly identifies the zero-based mask in Images; nil means references only.
	MaskIndex *int
	// ProviderOptions carries opaque operator custom JSON (object only).
	ProviderOptions map[string]interface{}
	// UploadURL is the presigned PUT for the single output (exact key,
	// content-type bound, short TTL).
	UploadURL string
	// Feature travels as Worker log metadata (searchable).
	Feature string
	// Timeout bounds this call; zero keeps the client default.
	Timeout time.Duration
}

// Validate rejects unusable requests before any network I/O.
func (r WorkerRequest) Validate() error {
	if strings.TrimSpace(r.TaskID) == "" {
		return fmt.Errorf("worker request requires a task id")
	}
	if strings.TrimSpace(r.Provider) == "" || strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("worker request requires a provider and model")
	}
	if strings.TrimSpace(r.UploadURL) == "" {
		return fmt.Errorf("worker request requires an upload URL")
	}
	references := len(r.Images)
	if r.MaskIndex != nil {
		if *r.MaskIndex < 0 || *r.MaskIndex >= len(r.Images) || len(r.Images) < 2 {
			return fmt.Errorf("worker mask index must identify a mask alongside an input image")
		}
		references--
	}
	if references > 4 {
		return fmt.Errorf("worker request carries at most 4 input images")
	}
	if r.Timeout < 0 {
		return fmt.Errorf("worker timeout must not be negative")
	}
	return nil
}

// WorkerReceipt is the parsed Worker acknowledgement for one call.
type WorkerReceipt struct {
	Model      string
	Provider   string
	Step       string
	CacheHit   bool
	LogID      string
	EventID    string
	StatusCode int
	// UpstreamStatus is the provider HTTP status behind the Worker.
	UpstreamStatus int
}

// WorkerResponse is the raw Worker outcome; the executor verifies the
// landed object by key and materializes its bytes through the existing
// finalize seam.
type WorkerResponse struct {
	StatusCode int
	Header     http.Header
	// ImageURL is the landed output URL (upload URL without query).
	ImageURL string
	// ContentType is the landed content type.
	ContentType string
	// Seed echoes the generation seed when reported.
	Seed       uint64
	HasSeed    bool
	Receipt    WorkerReceipt
	RawPreview string
}

// WorkerError is a structured Worker failure: the edge error code plus the
// upstream HTTP status when the Worker reached upstream. It carries the
// HTTP status so the existing failure classifiers keep working.
type WorkerError struct {
	Code           string
	Message        string
	TaskID         string
	Status         int
	UpstreamStatus int
	RetryAfter     time.Duration
}

func (e *WorkerError) Error() string {
	if e == nil {
		return "worker invocation failed"
	}
	if e.UpstreamStatus != 0 {
		return fmt.Sprintf("worker %s: %s (upstream %d)", e.Code, e.Message, e.UpstreamStatus)
	}
	return fmt.Sprintf("worker %s: %s", e.Code, e.Message)
}

// HTTPStatus reports the Worker HTTP status for failure classification.
func (e *WorkerError) HTTPStatus() int {
	if e == nil {
		return 0
	}
	return e.Status
}

// RetryAfterDuration reports the Retry-After for 429/503 mapping.
func (e *WorkerError) RetryAfterDuration() time.Duration {
	if e == nil {
		return 0
	}
	return e.RetryAfter
}

// NewWorkerError builds a structured Worker failure.
func NewWorkerError(code, message, taskID string, httpStatus, upstreamStatus int, retryAfter time.Duration) error {
	return &WorkerError{
		Code: code, Message: message, TaskID: taskID,
		Status: httpStatus, UpstreamStatus: upstreamStatus, RetryAfter: retryAfter,
	}
}

// WorkerErrorDetails extracts the edge error code from an error chain.
func WorkerErrorDetails(err error) (code string, ok bool) {
	var workerErr *WorkerError
	if !errors.As(err, &workerErr) {
		return "", false
	}
	return workerErr.Code, true
}

// WorkerClient is the sole AI image egress seam. Implementations own
// transport, auth, timeout, and single-shot sends; the Worker owns shaping,
// provider routing, and retries.
type WorkerClient interface {
	Do(ctx context.Context, req WorkerRequest) (WorkerResponse, error)
}

// WorkerClientFactory builds a WorkerClient per execution from current
// configuration. Resolving per execution keeps secret rotation effective
// without restarts; implementations fail closed when unconfigured.
type WorkerClientFactory func(ctx context.Context) (WorkerClient, error)

// ModelCustomParamsReader resolves catalog-stored custom params per
// structured pool model (provider plus bare model). Mapping @path
// overrides are rejected directly (single Worker exit): only opaque
// custom JSON travels to the Worker. Implementations fail the lookup (never silent
// defaults); a nil reader disables the lookup.
type ModelCustomParamsReader interface {
	LookupCustomParams(ctx context.Context, provider, model string) (customParams string, found bool, err error)
}
