package wiring

import (
	"testing"

	"ideaboard/api/internal/platform/riverpolicy"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
)

// Base pairing test for task_wiring.go.
// riverQueueStatsByQueue must expose exactly the unique queues from the
// retry matrix: a missing queue loses its RED signals, a duplicate
// wastes a gauge series.
func TestRiverQueueStatsByQueueMirrorsRetryMatrix(t *testing.T) {
	monitor := taskqueue.NewRiverQueue(nil, nil, riverpolicy.RiverMaxAttemptsDefault)
	stats := riverQueueStatsByQueue(monitor)
	if len(stats) == 0 {
		t.Fatal("riverQueueStatsByQueue returned empty map, want one entry per matrix queue")
	}
	seen := map[string]struct{}{}
	for _, policy := range riverpolicy.RiverRetryMatrix() {
		if _, dup := seen[policy.Queue]; dup {
			continue
		}
		seen[policy.Queue] = struct{}{}
		if _, ok := stats[policy.Queue]; !ok {
			t.Errorf("queue %q from retry matrix missing in stats map", policy.Queue)
		}
	}
	if len(stats) != len(seen) {
		t.Errorf("stats map has %d queues, want %d unique matrix queues", len(stats), len(seen))
	}
}
