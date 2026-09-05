package providers

import (
	"context"
	"errors"
	"net/http"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
)

type upstreamHTTPError = runtimeports.UpstreamHTTPError

func markProviderInvocationOutcomeUnknown(err error) error {
	return runtimeports.MarkProviderInvocationOutcomeUnknown(err)
}

func newUpstreamHTTPError(statusCode int, body string) error {
	return runtimeports.NewUpstreamHTTPError(statusCode, body, 0)
}

func newUpstreamHTTPErrorFromResponse(resp *http.Response, body string) error {
	return runtimeports.NewUpstreamHTTPErrorFromResponse(resp, body)
}

func parseRetryAfter(raw string) time.Duration {
	return runtimeports.ParseRetryAfter(raw)
}

func providerHTTPStatus(err error) (int, bool) {
	return runtimeports.ProviderHTTPStatus(err)
}

func newExecutionFailureError(stage string, category string, err error) error {
	return runtimeports.NewExecutionFailureError(stage, category, err)
}

func executionFailureDetails(err error) (string, string, bool) {
	return runtimeports.ExecutionFailureDetails(err)
}

func shouldFailoverProviderError(err error) bool {
	return runtimeports.ShouldFailoverProviderError(err)
}

func providerFailureCategory(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return tasks.FailureCategoryTimeout
	}
	if status, ok := providerHTTPStatus(err); ok {
		switch status {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return tasks.FailureCategoryInputInvalid
		case http.StatusUnauthorized, http.StatusForbidden:
			return tasks.FailureCategoryAuth
		case http.StatusPaymentRequired:
			return tasks.FailureCategoryQuota
		case http.StatusRequestTimeout, http.StatusTooManyRequests:
			return tasks.FailureCategoryTimeout
		default:
			return tasks.FailureCategoryProviderError
		}
	}
	return tasks.FailureCategoryProviderError
}

func newProviderOutputParseError(err error) error {
	return runtimeports.NewProviderOutputParseError(err)
}
