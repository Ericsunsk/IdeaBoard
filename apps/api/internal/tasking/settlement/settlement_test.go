package settlement_test

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/tasking/settlement"
	tasks "ideaboard/api/internal/tasking/domain"
)

// TestStateVocabularyPinsWireWords locks the durable settlement words.
// The database CHECK constraints are the final gate; this pins the Go
// side so a rename cannot drift from the persisted rows.
func TestStateVocabularyPinsWireWords(t *testing.T) {
	if settlement.StateFree != "free" || settlement.StateHeld != "held" ||
		settlement.StateCaptured != "captured" || settlement.StateReleased != "released" {
		t.Fatalf("state words = %q/%q/%q/%q, want free/held/captured/released",
			settlement.StateFree, settlement.StateHeld, settlement.StateCaptured, settlement.StateReleased)
	}
	if !settlement.IsInitialState("free") || !settlement.IsInitialState("held") {
		t.Fatal("free/held must be initial states")
	}
	if settlement.IsInitialState("captured") || settlement.IsInitialState("released") || settlement.IsInitialState("quoted") {
		t.Fatal("captured/released/quoted must not be initial states")
	}
	if !settlement.IsTerminalState("captured") || !settlement.IsTerminalState("released") {
		t.Fatal("captured/released must be terminal states")
	}
	if settlement.IsTerminalState("free") || settlement.IsTerminalState("held") {
		t.Fatal("free/held must not be terminal states")
	}
}

// TestTaskHoldKeyPinsDurableIdentity locks the exact ledger replay keys and
// ties their operation namespace to the durable submission claim.
func TestTaskHoldKeyPinsDurableIdentity(t *testing.T) {
	for _, tc := range []struct{ workspace, key, want string }{
		{"ws_alpha", "key-1", "task-hold:task:create:cb0350f91cf956a2c21ec8cc5cb3e2fb6d05c9108bd8d3dc32af0aa2f7447a7b"},
		{"ws_beta", "  key-2 ", "task-hold:task:create:4c2dd53cfc29f68918dcc3fc20a6b2103d529732ba0df5ecaa82aebe602543c9"},
		{"", "", "task-hold:task:create:b2d0381c69d54e887726267e06e709e3f7007b54b12cbb080116ff2ab4f7f3a2"},
	} {
		if got := settlement.TaskHoldKey(tc.workspace, tc.key); got != tc.want {
			t.Fatalf("TaskHoldKey(%q,%q) = %q, want %q", tc.workspace, tc.key, got, tc.want)
		}
	}
	if got := settlement.TaskHoldKey("ws_alpha", "key-1"); !strings.HasPrefix(got, "task-hold:"+tasks.TaskSubmitOperation+":") {
		t.Fatalf("TaskHoldKey namespace = %q, want durable claim operation %q", got, tasks.TaskSubmitOperation)
	}
}

// TestTaskLedgerKeysPinIdempotencyWords locks the capture/release key
// words the ledger replay depends on.
func TestTaskLedgerKeysPinIdempotencyWords(t *testing.T) {
	if got := settlement.TaskCaptureKey("task_abc"); got != "task-capture:task_abc" {
		t.Fatalf("TaskCaptureKey = %q", got)
	}
	if got := settlement.TaskReleaseKey("task_abc"); got != "task-release:task_abc" {
		t.Fatalf("TaskReleaseKey = %q", got)
	}
}

// TestVisionKeysPinOperationScope locks the vision idempotency words.
func TestVisionKeysPinOperationScope(t *testing.T) {
	if got := settlement.VisionHoldKey("op-1"); got != "ai-search:op-1:hold" {
		t.Fatalf("VisionHoldKey = %q", got)
	}
	if got := settlement.VisionCaptureKey("op-1"); got != "ai-search:op-1:capture" {
		t.Fatalf("VisionCaptureKey = %q", got)
	}
	if got := settlement.VisionReleaseKey("op-1"); got != "ai-search:op-1:release" {
		t.Fatalf("VisionReleaseKey = %q", got)
	}
	if got := settlement.VisionReconcileReleaseKey("op-1"); got != "ai-search:op-1:reconcile-release" {
		t.Fatalf("VisionReconcileReleaseKey = %q", got)
	}
}

// TestCaptureSplitMatchesHistoricalMath pins the per-output capping math
// the terminal settlement depends on.
func TestCaptureSplitMatchesHistoricalMath(t *testing.T) {
	for _, tc := range []struct {
		total, unit int64
		outputs     int
		capture     int64
		release     int64
	}{
		{20, 10, 1, 10, 10},
		{10, 10, 1, 10, 0},
		{10, 10, 0, 0, 10},
		{5, 10, 1, 5, 0},
		{0, 10, 1, 0, 0},
	} {
		if capture, release := settlement.CaptureSplit(tc.total, tc.unit, tc.outputs); capture != tc.capture || release != tc.release {
			t.Fatalf("CaptureSplit(%d,%d,%d) = (%d,%d), want (%d,%d)",
				tc.total, tc.unit, tc.outputs, capture, release, tc.capture, tc.release)
		}
	}
}

// TestBorrowOrBeginRejectsNilTransactorWithoutCarrier pins the fail-closed
// borrow-or-begin contract: no carrier and no transactor is an error,
// never an unfenced settlement.
func TestBorrowOrBeginRejectsNilTransactorWithoutCarrier(t *testing.T) {
	if _, err := settlement.BorrowOrBeginScope(context.Background(), nil); err == nil {
		t.Fatal("BorrowOrBeginScope without carrier or transactor must fail")
	}
}

// TestStageOrDeferRunsSynchronouslyWhenOwned pins the owned-leg staging:
// the event stages immediately so the caller can flush after its commit.
func TestStageOrDeferRunsSynchronouslyWhenOwned(t *testing.T) {
	ran := false
	settlement.StageOrDefer(context.Background(), true, func(context.Context) { ran = true })
	if !ran {
		t.Fatal("owned StageOrDefer must run the stage synchronously")
	}
	settlement.StageOrDefer(context.Background(), true, nil)
}
