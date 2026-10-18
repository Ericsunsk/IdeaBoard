package ports

import "errors"

// Service error sentinels. Application/domain errors originate here as
// typed sentinels; fmt.Errorf is only a %w wrapper, never an origin.
// The v1 usage edge (v1/billing.go renderV1UsageError) maps
// invalid input to 4xx (ErrInvalidUserID to 400 bad_request,
// ErrInvalidRange to 422 validation_failed); every other GetCurrent
// failure stays 503 service_unavailable.
var (
	ErrReadersNotConfigured = errors.New("usage service: readers not configured")
	ErrInvalidUserID        = errors.New("usage service: invalid user id")
	ErrCacheValueType       = errors.New("usage service: unexpected cached value type")
	ErrLoadTimezone         = errors.New("usage service: load timezone")
	ErrReadSettledCaptures  = errors.New("usage service: read settled captures")
	ErrReadLogicalTasks     = errors.New("usage service: read logical tasks")
	ErrReadDailyCaptures    = errors.New("usage service: read daily settled captures")
	ErrUnsupportedTimezone  = errors.New("usage response: unsupported timezone")
	ErrTrendBucketCount     = errors.New("usage response: trend bucket count mismatch")
	ErrUnsafeNumber         = errors.New("usage value exceeds JavaScript safe integer range")
)
