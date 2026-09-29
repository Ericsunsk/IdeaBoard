package routing

import (
	"errors"
	"fmt"
)

// ErrServiceResolutionUnavailable is the errors.Is sentinel for an empty
// eligible set. It converges to one canonical error before task
// creation, billing hold, or enqueue (ADR-0012 rule 8): HTTP 422, code
// service_resolution_unavailable, identifying the normalized request as
// details.fields.service and details.fields.resolution. HTTP rendering
// stays in wave 2 (the edge maps this sentinel to
// contract.CodeServiceResolutionUnavailable); this package only defines
// the domain fact.
//
// The registry entry already exists in
// internal/platform/contract/errorcodes.go
// (CodeServiceResolutionUnavailable, StatusOf maps it to 422), so no new
// code registration is needed: wave 2 references the kernel code.
var ErrServiceResolutionUnavailable = errors.New("routing: service resolution unavailable")

// ResolutionUnavailableError carries the normalized request values for
// an empty eligible set. Match it with errors.Is against
// ErrServiceResolutionUnavailable at the edge; extract Service and
// Resolution with errors.As for the details.fields rendering.
type ResolutionUnavailableError struct {
	Service    string
	Resolution Resolution
}

// Is makes errors.Is(err, ErrServiceResolutionUnavailable) true for
// this outcome while keeping the normalized values available via
// errors.As.
func (e *ResolutionUnavailableError) Is(target error) bool {
	return target == ErrServiceResolutionUnavailable
}

func (e *ResolutionUnavailableError) Error() string {
	if e == nil {
		return ErrServiceResolutionUnavailable.Error()
	}
	return fmt.Sprintf("routing: service %q has no enabled model for resolution %q", e.Service, string(e.Resolution))
}
