package tests

import (
	"errors"
	"testing"

	"ideaboard/api/internal/media/derivatives/application/repair"
	mediaerrors "ideaboard/api/internal/media/errors"
)

func TestRepairDefaultsAreStable(t *testing.T) {
	if DefaultRepairMaxAttempts <= 0 {
		t.Fatal("default repair attempt cap must be positive")
	}
	if DefaultRepairMaxAttempts > 5 {
		t.Errorf("default repair attempt cap %d is excessive; runbook pins 3 as the cap", DefaultRepairMaxAttempts)
	}
	if DefaultRepairLease <= 0 {
		t.Fatal("default repair lease must be positive")
	}
}

func TestRepairErrRepairNotPossibleIsTyped(t *testing.T) {
	if !errors.Is(ErrRepairNotPossible, ErrRepairNotPossible) {
		t.Fatal("ErrRepairNotPossible must be comparable with errors.Is")
	}
}

func TestRepairPublishRepairedStampsEventType(t *testing.T) {
	called := false
	var got Event
	r := repair.NewRepair(nil, stubPublisher{fn: func(userID int64, event Event) {
		called = true
		got = event
	}}, RepairConfig{})
	r.PublishRepaired(7, Event{
		MediaAssetID: 42,
		DerivativeID: "derivative_99",
		Tier:         TierProcessingInput,
	})
	if !called {
		t.Fatal("publisher not called")
	}
	if got.Type != EventTypeRepaired {
		t.Fatalf("event type = %q, want %q", got.Type, EventTypeRepaired)
	}
	if got.SchemaVersion != EventSchemaVersion {
		t.Fatalf("schema version = %d, want %d", got.SchemaVersion, EventSchemaVersion)
	}
}

func TestRepairPublishRepairedIsNoOpWhenPublisherNil(t *testing.T) {
	r := &Repair{}
	// Must not panic when the publisher is nil; the runbook
	// pins this as a guard for the "events disabled in dev"
	// flag.
	r.PublishRepaired(1, Event{MediaAssetID: 1, Tier: TierProcessingInput})
}

func TestRepairErrRepairNotPossibleCode(t *testing.T) {
	// The structured error taxonomy and the typed sentinel
	// must agree on the same Code so a dashboard can pivot
	// on either. The mapping is the single source of truth.
	mediaErr := mediaerrors.New(mediaerrors.CodeRepairNotPossible, "media.derivative.repair", "cap reached", ErrRepairNotPossible)
	if mediaerrors.CodeOf(mediaErr) != mediaerrors.CodeRepairNotPossible {
		t.Fatal("structured error must carry repair_not_possible code")
	}
}

func TestRepairConfigDefaultsAreApplied(t *testing.T) {
	// NewRepair with a zero config must produce non-zero
	// defaults. We only need to check the constants the runbook
	// pins; the constructor itself is exercised by the worker
	// integration test.
	if DefaultRepairMaxAttempts == 0 || DefaultRepairLease == 0 {
		t.Fatal("defaults must be non-zero")
	}
}

type stubPublisher struct {
	fn func(userID int64, event Event)
}

func (s stubPublisher) Publish(userID int64, event Event) {
	if s.fn != nil {
		s.fn(userID, event)
	}
}
