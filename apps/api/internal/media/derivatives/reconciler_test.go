package derivatives

import (
	"context"
	"errors"
	"testing"
	"time"

	mediaerrors "ideaboard/api/internal/media/errors"
	"ideaboard/api/internal/media/storage"
)

// TestStructuredErrorCodeMappingContract locks the inline-to-typed
// mapping. The mapping is the single source of truth that the
// runbook, the Admin UI, and the front-end align on. New inline
// strings must round-trip through this test before the build is
// considered green.
func TestStructuredErrorCodeMappingContract(t *testing.T) {
	cases := map[string]mediaerrors.Code{
		"asset_not_found":            mediaerrors.CodeMediaNotFound,
		"source_read_failed":         mediaerrors.CodeObjectMissing,
		"destination_write_failed":   mediaerrors.CodeObjectMissing,
		"key_failed":                 mediaerrors.CodeObjectMissing,
		"source_limits_exceeded":     mediaerrors.CodeObjectInvalid,
		"output_limits_exceeded":     mediaerrors.CodeObjectInvalid,
		"render_failed":              mediaerrors.CodeStorageTransient,
		"renderer_unavailable":       mediaerrors.CodeStorageTransient,
		"invalid_tier":               mediaerrors.CodeMediaNotFound,
		"stale_processing_recovered": mediaerrors.CodeStorageTransient,
		"unknown":                    mediaerrors.CodeMediaNotFound,
	}
	for inline, want := range cases {
		if got := structuredErrorCode(inline); got != want {
			t.Errorf("structuredErrorCode(%q) = %q, want %q", inline, got, want)
		}
	}
}

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
var _ = storage.ErrStoredObjectNotFound
var _ = errors.Is
var _ = context.Background
