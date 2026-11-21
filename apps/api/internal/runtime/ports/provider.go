package ports

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"net"
	"net/http"
	"time"

	"ideaboard/api/internal/platform/retrypolicy"
	tasks "ideaboard/api/internal/tasking/domain"
)

// ProviderCallTimeline records the externally visible phases of one provider attempt.
type ProviderCallTimeline struct {
	ProviderStartedAt  *time.Time
	ProviderFinishedAt *time.Time
	FinalizeStartedAt  *time.Time
	FinalizeFinishedAt *time.Time
}

// ProviderCallOutput is the stable result contract between the Worker
// caller and execution orchestration.
type ProviderCallOutput struct {
	Result   string
	Outputs  []tasks.TaskOutput
	Timeline ProviderCallTimeline
	// Worker receipt facts recorded on the closed attempt. Field names
	// stay DB-mapped (gateway_step et al. are frozen sqlc columns).
	ResolvedModel  string
	GatewayStep    string
	CacheHit       bool
	ReceiptLogID   string
	ReceiptEventID string
}

// WorkerCallInput carries one Worker execution's resolved inputs. The
// model comes from the pinned RoutingSnapshot. The execution token stays
// in the tasking layer (claim fence on terminal writes) and never enters
// this seam: the edge contract carries no idempotency field, and local
// settlement never depended on upstream dedup.
type WorkerCallInput struct {
	Task      *tasks.TaskPayload
	Model     string
	AttemptNo int32
	Client    WorkerClient
	// ExecutionContext bounds provider work separately from materialization.
	// Nil uses CallWorkerAPI's context for callers without a separate budget.
	ExecutionContext context.Context
}

// WorkerCaller hides the Worker transport from task execution
// orchestration. The Worker owns shaping and provider routing; this seam
// only sends canonical task facts and materializes landed outputs.
type WorkerCaller interface {
	CallWorkerAPI(context.Context, WorkerCallInput) (ProviderCallOutput, error)
}

// WorkerCallerFunc adapts a function to WorkerCaller for focused orchestration tests.
type WorkerCallerFunc func(context.Context, WorkerCallInput) (ProviderCallOutput, error)

func (f WorkerCallerFunc) CallWorkerAPI(ctx context.Context, input WorkerCallInput) (ProviderCallOutput, error) {
	return f(ctx, input)
}

// UpstreamHTTPError records status code and body from provider HTTP responses.
type UpstreamHTTPError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *UpstreamHTTPError) Error() string {
	return fmt.Sprintf(errorrender.MsgRuntimePortsError, e.StatusCode, e.Body)
}

func (e *UpstreamHTTPError) HTTPStatus() int {
	return e.StatusCode
}

func (e *UpstreamHTTPError) RetryAfterDuration() time.Duration {
	return e.RetryAfter
}

func NewUpstreamHTTPError(statusCode int, body string, retryAfter time.Duration) error {
	return &UpstreamHTTPError{
		StatusCode: statusCode,
		Body:       body,
		RetryAfter: retryAfter,
	}
}

func NewUpstreamHTTPErrorFromResponse(resp *http.Response, body string) error {
	if resp == nil {
		return NewUpstreamHTTPError(0, body, 0)
	}
	return &UpstreamHTTPError{
		StatusCode: resp.StatusCode,
		Body:       body,
		RetryAfter: ParseRetryAfter(resp.Header.Get("Retry-After")),
	}
}

func ParseRetryAfter(raw string) time.Duration {
	return retrypolicy.ParseRetryAfter(raw)
}

type ProviderInvocationOutcomeUnknownError struct {
	Err error
}

func (e *ProviderInvocationOutcomeUnknownError) Error() string {
	if e == nil || e.Err == nil {
		return "provider invocation outcome is unknown"
	}
	return e.Err.Error()
}

func (e *ProviderInvocationOutcomeUnknownError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type ExecutionFailureError struct {
	Stage    string
	Category string
	Err      error
}

func (e *ExecutionFailureError) Error() string {
	if e == nil || e.Err == nil {
		return "task execution failed"
	}
	return e.Err.Error()
}

func (e *ExecutionFailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewExecutionFailureError(stage string, category string, err error) error {
	return &ExecutionFailureError{
		Stage:    stage,
		Category: category,
		Err:      err,
	}
}

func ExecutionFailureDetails(err error) (string, string, bool) {
	var failureErr *ExecutionFailureError
	if !errors.As(err, &failureErr) {
		return "", "", false
	}
	return failureErr.Stage, failureErr.Category, true
}

func ProviderHTTPStatus(err error) (int, bool) {
	var workerErr *WorkerError
	if errors.As(err, &workerErr) {
		return workerErr.Status, true
	}
	var upstreamErr *UpstreamHTTPError
	if !errors.As(err, &upstreamErr) {
		return 0, false
	}
	return upstreamErr.StatusCode, true
}

type ProviderOutputParseError struct {
	Err error
}

func (e *ProviderOutputParseError) Error() string {
	if e == nil || e.Err == nil {
		return "provider output parse failed"
	}
	return e.Err.Error()
}

func (e *ProviderOutputParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewProviderOutputParseError(err error) error {
	return &ProviderOutputParseError{Err: err}
}

func ShouldFailoverProviderError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var outcomeUnknown *ProviderInvocationOutcomeUnknownError
	if errors.As(err, &outcomeUnknown) {
		return false
	}
	if !isFailoverableExecutionFailure(err) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if status, ok := ProviderHTTPStatus(err); ok {
		return isFailoverableHTTPStatus(status)
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func isFailoverableExecutionFailure(err error) bool {
	if _, category, ok := ExecutionFailureDetails(err); ok {
		switch category {
		case tasks.FailureCategoryTimeout, tasks.FailureCategoryProviderError:
			return true
		default:
			return false
		}
	}
	return true
}

func isFailoverableHTTPStatus(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusPaymentRequired,
		http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity:
		return false
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	default:
		return status >= http.StatusInternalServerError
	}
}

func ClassifyProviderFailure(err error, timeline ProviderCallTimeline) (string, string) {
	if errors.Is(err, context.Canceled) {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryCancelled
	}
	if stage, category, ok := ExecutionFailureDetails(err); ok {
		return stage, category
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryTimeout
	}
	if status, ok := ProviderHTTPStatus(err); ok {
		return classifyHTTPStatusFailure(status)
	}
	if timeline.FinalizeStartedAt != nil {
		return tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError
	}
	if timeline.ProviderStartedAt != nil {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryProviderError
	}
	return tasks.ExecutionStageAssigned, tasks.FailureCategoryUnknown
}

func classifyHTTPStatusFailure(status int) (string, string) {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryInputInvalid
	case http.StatusUnauthorized, http.StatusForbidden:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryAuth
	case http.StatusPaymentRequired:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryQuota
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryTimeout
	default:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryProviderError
	}
}

const DefaultMaxErrorBodyLen = 500

func TruncateErrorBody(body string, maxLen ...int) string {
	limit := DefaultMaxErrorBodyLen
	if len(maxLen) > 0 && maxLen[0] > 0 {
		limit = maxLen[0]
	}
	if len(body) > limit {
		return body[:limit] + "...(truncated)"
	}
	return body
}
