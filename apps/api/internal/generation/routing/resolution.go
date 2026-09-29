package routing

import (
	"fmt"
	"strings"
)

// Resolution is a canonical pool resolution (ADR-0012 rule 3). The pool
// speaks exactly 1k, 2k, and 4k.
type Resolution string

// Canonical resolutions. Values are lowercase on the wire and in pool
// capability sets.
const (
	Resolution1K Resolution = "1k"
	Resolution2K Resolution = "2k"
	Resolution4K Resolution = "4k"
)

// NormalizeResolution canonicalizes one raw resolution value. It accepts
// only 1k, 2k, and 4k case-insensitively with surrounding whitespace.
// Ingress applies a Service policy to omitted input before calling this seam.
func NormalizeResolution(raw string) (Resolution, error) {
	switch res := Resolution(strings.ToLower(strings.TrimSpace(raw))); res {
	case Resolution1K, Resolution2K, Resolution4K:
		return res, nil
	default:
		return "", &InvalidRoutingInputError{Field: "resolution", Value: raw, Reason: "unsupported resolution"}
	}
}

// NormalizeService canonicalizes one raw service slug. Services are
// catalog-owned product capabilities (ADR-0012 rule 1); the resolver only
// folds case and whitespace so snapshot facts and the selection hash see
// one spelling.
func NormalizeService(raw string) (string, error) {
	service := strings.ToLower(strings.TrimSpace(raw))
	if service == "" {
		return "", &InvalidRoutingInputError{Field: "service", Value: raw, Reason: "service must not be empty"}
	}
	return service, nil
}

// InvalidRoutingInputError reports a deterministic caller bug: an empty
// service or a non-canonical resolution. It is never an availability
// outcome, never maps to service_resolution_unavailable, and never
// triggers model fallback.
type InvalidRoutingInputError struct {
	Field  string
	Value  string
	Reason string
}

func (e *InvalidRoutingInputError) Error() string {
	if e == nil {
		return "routing: invalid routing input"
	}
	return fmt.Sprintf("routing: invalid %s %q: %s", e.Field, e.Value, e.Reason)
}
