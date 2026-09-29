package ports

import (
	"context"

	"ideaboard/api/internal/runtime/domain"
)

// Runner represents a background or network service lifecycle.
type Runner interface {
	Name() string
	Kind() domain.ComponentKind
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// HealthChecker inspects runtime component health and readiness.
type HealthChecker interface {
	CheckHealth(ctx context.Context) domain.ComponentStatus
}

// ResourceCloser manages explicit resource closing at process termination.
type ResourceCloser interface {
	Close() error
}
