package database_test

import (
	"testing"

	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/infrastructure/database"
)

func TestPoolsNilClose(t *testing.T) {
	var p *database.Pools
	if err := p.Close(); err != nil {
		t.Fatalf("nil Pools.Close returned error: %v", err)
	}
}

func TestHealthCheckerNil(t *testing.T) {
	var h *database.HealthChecker
	status := h.CheckHealth(t.Context())
	if status.Ready {
		t.Fatal("expected nil health checker to report not ready")
	}
	if status.Kind != domain.ComponentKindDatabase {
		t.Fatalf("expected component kind database, got %v", status.Kind)
	}
}

func TestHealthCheckerNewNil(t *testing.T) {
	h := database.NewHealthChecker(nil)
	status := h.CheckHealth(t.Context())
	if status.Ready {
		t.Fatal("expected uninitialized health checker to report not ready")
	}
}
