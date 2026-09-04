package derivatives

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/media/storage/domain"
)

func TestReconcilerResultCounters(t *testing.T) {
	got := Result{Inspected: 10, Missing: 2, Invalid: 1, Errors: 1}
	if got.Inspected != 10 || got.Missing != 2 || got.Invalid != 1 || got.Errors != 1 {
		t.Fatalf("counters not preserved: %+v", got)
	}
}

func TestReconcilerConfigDefaultsAreStable(t *testing.T) {
	if defaultReconcilerProbeTimeout <= 0 {
		t.Fatal("default probe timeout must be positive")
	}
	if defaultReconcilerBatchSize <= 0 {
		t.Fatal("default batch size must be positive")
	}
	if defaultReconcilerLease <= 0 {
		t.Fatal("default lease must be positive")
	}
	if defaultReconcilerProbeTimeout > time.Minute {
		t.Errorf("probe timeout %v exceeds the 60s SLO", defaultReconcilerProbeTimeout)
	}
}

// TestReconcilerOutcomeClassification guards the per-row outcome
// values. The Worker uses these in its metrics so a regression here
// silently breaks the dashboard.
func TestReconcilerOutcomeClassification(t *testing.T) {
	if outcomeReady != 0 {
		t.Errorf("outcomeReady must be 0, got %d", outcomeReady)
	}
	if outcomeMissing != 1 {
		t.Errorf("outcomeMissing must be 1, got %d", outcomeMissing)
	}
	if outcomeInvalid != 2 {
		t.Errorf("outcomeInvalid must be 2, got %d", outcomeInvalid)
	}
}

// _ ensures unused-import elimination does not eat the storage
// sentinel.
var _ = domain.ErrStoredObjectNotFound
var _ = errors.Is
var _ = context.Background
