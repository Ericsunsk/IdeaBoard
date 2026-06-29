package worker

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/tasking/tasks"
	"net"
	"net/http"
)

type upstreamHTTPError struct {
	statusCode int
	body       string
}

func newUpstreamHTTPError(statusCode int, body string) error {
	return &upstreamHTTPError{
		statusCode: statusCode,
		body:       body,
	}
}

func (e *upstreamHTTPError) Error() string {
	return fmt.Sprintf("API 返回 %d: %s", e.statusCode, e.body)
}

func providerHTTPStatus(err error) (int, bool) {
	var upstreamErr *upstreamHTTPError
	if !errors.As(err, &upstreamErr) {
		return 0, false
	}
	return upstreamErr.statusCode, true
}

type executionFailureError struct {
	stage    string
	category string
	err      error
}

func newExecutionFailureError(stage string, category string, err error) error {
	return &executionFailureError{
		stage:    stage,
		category: category,
		err:      err,
	}
}

func (e *executionFailureError) Error() string {
	if e == nil || e.err == nil {
		return "task execution failed"
	}
	return e.err.Error()
}

func (e *executionFailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func executionFailureDetails(err error) (string, string, bool) {
	var failureErr *executionFailureError
	if !errors.As(err, &failureErr) {
		return "", "", false
	}
	return failureErr.stage, failureErr.category, true
}

func shouldFailoverProviderError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if _, category, ok := executionFailureDetails(err); ok {
		switch category {
		case tasks.FailureCategoryTimeout, tasks.FailureCategoryProviderError:
		default:
			return false
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if status, ok := providerHTTPStatus(err); ok {
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

	var networkErr net.Error
	return errors.As(err, &networkErr)
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
