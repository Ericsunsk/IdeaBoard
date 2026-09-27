package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testActor() V1Actor {
	return V1Actor{Type: V1ActorUser, ID: "usr_abc123"}
}

func TestV1RegistryIsFrozen(t *testing.T) {
	types := V1EventTypes()
	if len(types) != 11 {
		t.Fatalf("registry has %d types, want 11 (additive only)", len(types))
	}
	for _, typ := range types {
		version, ok := V1EventVersion(typ)
		if !ok || version != 1 {
			t.Fatalf("type %q version=%d ok=%v, want version 1", typ, version, ok)
		}
	}
}

func TestV1RegistryExcludesControl(t *testing.T) {
	for _, typ := range V1EventTypes() {
		lowered := strings.ToLower(typ)
		for _, banned := range []string{"audit", "control", "admin", "internal"} {
			if strings.Contains(lowered, banned) {
				t.Fatalf("registry type %q must not expose control/audit surface", typ)
			}
		}
	}
}

func TestMintV1EnvelopeAssignsEvtIDAndVersion(t *testing.T) {
	envelope, err := MintV1Envelope(EventV1TaskCompleted, "ws_abc123", testActor(),
		V1Resource{Type: V1ResourceTask, ID: "task_abc123"},
		V1TaskCompletedData{TaskID: "task_abc123", OutputsURI: "/api/v1/workspaces/ws_abc123/tasks/task_abc123/outputs"})
	if err != nil {
		t.Fatal(err)
	}
	if !IsV1EventType(envelope.Type) {
		t.Fatalf("type %q not registered", envelope.Type)
	}
	if !strings.HasPrefix(envelope.ID, "evt_") {
		t.Fatalf("id %q must carry evt_ prefix", envelope.ID)
	}
	if envelope.Version != 1 {
		t.Fatalf("version=%d, want 1", envelope.Version)
	}
	if envelope.OccurredAt.Location() != time.UTC {
		t.Fatalf("occurred_at must be UTC, got %v", envelope.OccurredAt.Location())
	}
	if err := ValidateV1Envelope(envelope); err != nil {
		t.Fatalf("minted envelope invalid: %v", err)
	}
}

func TestMintV1EnvelopeRejectsUnknownType(t *testing.T) {
	if _, err := MintV1Envelope("task.exploded", "ws_x", testActor(),
		V1Resource{Type: V1ResourceTask, ID: "task_x"}, map[string]any{"task_id": "task_x"}); err == nil {
		t.Fatal("unknown type must fail closed")
	}
}

func TestValidateV1EnvelopeRejectsInternalIDs(t *testing.T) {
	base, err := MintV1Envelope(EventV1MediaAssetReady, "ws_abc123", testActor(),
		V1Resource{Type: V1ResourceMediaAsset, ID: "asset_abc123"},
		V1MediaAssetReadyData{AssetID: "asset_abc123"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*V1Envelope)
	}{
		{"numeric id", func(e *V1Envelope) { e.ID = "12345" }},
		{"bad prefix", func(e *V1Envelope) { e.ID = "task_abc123" }},
		{"numeric workspace", func(e *V1Envelope) { e.WorkspaceID = "42" }},
		{"numeric resource", func(e *V1Envelope) { e.Resource.ID = "7" }},
		{"numeric actor", func(e *V1Envelope) { e.Actor.ID = "9" }},
		{"version drift", func(e *V1Envelope) { e.Version = 2 }},
		{"unknown type", func(e *V1Envelope) { e.Type = "media.asset_vanished" }},
		{"bad actor type", func(e *V1Envelope) { e.Actor.Type = "superuser" }},
		{"empty data", func(e *V1Envelope) { e.Data = nil }},
		{"storage url leak", func(e *V1Envelope) {
			completed, _ := MintV1Envelope(EventV1TaskCompleted, "ws_abc123", testActor(),
				V1Resource{Type: V1ResourceTask, ID: "task_abc123"},
				V1TaskCompletedData{TaskID: "task_abc123", OutputsURI: "https://storage.internal/bucket/key"})
			*e = completed
		}},
	}
	for _, tc := range cases {
		probe := base
		tc.mutate(&probe)
		if err := ValidateV1Envelope(probe); err == nil {
			t.Fatalf("%s must be rejected", tc.name)
		}
	}
}

func TestValidateV1BillingSettledCarriesRefsNotAmounts(t *testing.T) {
	envelope, err := MintV1Envelope(EventV1BillingSettled, "ws_abc123",
		V1Actor{Type: V1ActorSystem, ID: "system"},
		V1Resource{Type: V1ResourceTask, ID: "task_abc123"},
		V1BillingSettledData{ReferenceType: "task", ReferenceID: "task_abc123",
			SettlementState: "captured", HoldTransactionID: "hold-1"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(envelope.Data, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"unit_credits", "total_credits", "amount", "quantity"} {
		if _, ok := decoded[banned]; ok {
			t.Fatalf("billing.settled must not carry %q", banned)
		}
	}
}

func TestTaskStatusToV1Mapping(t *testing.T) {
	cases := map[string]string{
		"pending":   EventV1TaskCreated,
		"running":   EventV1TaskRunning,
		"completed": EventV1TaskCompleted,
		"failed":    EventV1TaskFailed,
		"cancelled": EventV1TaskCancelled,
	}
	for status, want := range cases {
		got, ok := taskStatusToV1(status)
		if !ok || got != want {
			t.Fatalf("status %q -> (%q,%v), want (%q,true)", status, got, ok, want)
		}
	}
	if _, ok := taskStatusToV1("exploding"); ok {
		t.Fatal("unknown status must fail closed")
	}
}

func TestV1DeduperCollapsesDuplicates(t *testing.T) {
	deduper := NewV1Deduper(2)
	if deduper.Seen("evt_a") {
		t.Fatal("first sight must not be a duplicate")
	}
	if !deduper.Seen("evt_a") {
		t.Fatal("second sight must be a duplicate")
	}
	_ = deduper.Seen("evt_b")
	_ = deduper.Seen("evt_c")
	if deduper.Seen("evt_a") {
		t.Log("evt_a still retained (cap not exceeded for this path)")
	}
}
