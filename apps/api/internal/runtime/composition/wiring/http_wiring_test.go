package wiring

import (
	"context"
	"testing"

	"ideaboard/api/internal/generation/routing"
)

// Base pairing test for http_wiring.go.
func TestPoolRoutingSnapshotSaverRequiresTx(t *testing.T) {
	saver := newPoolRoutingSnapshotSaver()
	if saver == nil {
		t.Fatal("newPoolRoutingSnapshotSaver returned nil")
	}
	if err := saver(context.Background(), nil, "task-1", routing.RoutingSnapshot{}); err == nil {
		t.Fatal("snapshot saver with nil tx succeeded, want fail-closed error")
	}
}

func TestControlMonitorHandlerNilPoolStaysFailClosed(t *testing.T) {
	if h := newControlMonitorHandler(nil); h != nil {
		t.Fatalf("newControlMonitorHandler(nil) = %v, want nil fail-closed handler", h)
	}
}
