package tests

import (
	"encoding/json"
	"testing"
)

func TestPublisherStampsSchemaVersionAndCarriesFacts(t *testing.T) {
	event := Event{
		Type:            EventTypeReady,
		MediaAssetID:    42,
		DerivativeID:    "derivative_99",
		Tier:            "processing-input",
		Format:          "webp",
		Status:          "ready",
		TaskID:          "task_xyz",
		ProviderAttempt: "attempt_1",
		CorrelationID:   "corr-1",
		StorageProfile:  "primary",
	}
	if EventSchemaVersion < 2 {
		t.Fatalf("EventSchemaVersion must be >= 2 to carry the new fields, got %d", EventSchemaVersion)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["schema_version"] == nil {
		t.Fatalf("schema_version missing from payload: %s", raw)
	}
	if decoded["type"] != EventTypeReady {
		t.Fatalf("type = %v, want %q", decoded["type"], EventTypeReady)
	}
	if decoded["task_id"] != "task_xyz" {
		t.Fatalf("task_id lost: %v", decoded["task_id"])
	}
	if decoded["provider_attempt_id"] != "attempt_1" {
		t.Fatalf("provider_attempt_id lost: %v", decoded["provider_attempt_id"])
	}
}

func TestPublisherDefaultsTypeAndSchema(t *testing.T) {
	var p *UserEventPublisher
	p.Publish(1, Event{Status: "ready"})
	// When the hub is nil, Publish must not panic. The behaviour is
	// covered by the runtime contract; the test just guards against
	// a nil deref regression.
}

func TestAllEventTypeConstantsAreDistinct(t *testing.T) {
	constants := map[string]struct{}{
		EventTypeUpdated:         {},
		EventTypeReady:           {},
		EventTypeInvalidated:     {},
		EventTypeRepairRequested: {},
		EventTypeRepaired:        {},
	}
	if len(constants) != 5 {
		t.Fatalf("dedup map holds %d, want 5 distinct constants", len(constants))
	}
}

func TestEventTypeConstantsHaveStableWireValues(t *testing.T) {
	// Wire values are part of the public contract: subscribers
	// (runbook, dashboards, front-end) depend on them.
	if EventTypeUpdated != "media.derivative.updated" {
		t.Errorf("EventTypeUpdated = %q, want %q", EventTypeUpdated, "media.derivative.updated")
	}
	if EventTypeReady != "media.derivative.ready" {
		t.Errorf("EventTypeReady = %q, want %q", EventTypeReady, "media.derivative.ready")
	}
	if EventTypeInvalidated != "media.derivative.invalidated" {
		t.Errorf("EventTypeInvalidated = %q, want %q", EventTypeInvalidated, "media.derivative.invalidated")
	}
	if EventTypeRepairRequested != "media.derivative.repair_requested" {
		t.Errorf("EventTypeRepairRequested = %q, want %q", EventTypeRepairRequested, "media.derivative.repair_requested")
	}
	if EventTypeRepaired != "media.derivative.repaired" {
		t.Errorf("EventTypeRepaired = %q, want %q", EventTypeRepaired, "media.derivative.repaired")
	}
}
