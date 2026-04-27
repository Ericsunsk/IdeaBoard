package worker

import (
	"context"
	"errors"
	"fmt"
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

func shouldFailoverProviderError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
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
