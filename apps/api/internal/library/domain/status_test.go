package domain

import "testing"

// TestCanTransitionMirrorsSQLGuards pins the durable materialization
// transition table to the guards in
// internal/db/queries/workspace_library.sql: StartLibraryMaterialization
// fences pending -> processing (public running); Complete/Fail flip a
// live (pending/processing) job to completed/failed, so direct
// pending -> completed|failed is allowed; terminals have no outgoing
// transitions. Every true entry was already possible before typing.
func TestCanTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]V1MaterializationStatus{
		{V1MaterializationPending, V1MaterializationRunning},
		{V1MaterializationPending, V1MaterializationCompleted},
		{V1MaterializationPending, V1MaterializationFailed},
		{V1MaterializationRunning, V1MaterializationCompleted},
		{V1MaterializationRunning, V1MaterializationFailed},
	}
	for _, transition := range allowed {
		if !CanTransition(transition[0], transition[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	terminals := []V1MaterializationStatus{V1MaterializationCompleted, V1MaterializationFailed}
	live := []V1MaterializationStatus{
		V1MaterializationPending, V1MaterializationRunning,
		V1MaterializationCompleted, V1MaterializationFailed,
	}
	for _, from := range terminals {
		for _, to := range live {
			if CanTransition(from, to) {
				t.Errorf("CanTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
	if CanTransition(V1MaterializationPending, V1MaterializationPending) {
		t.Error("CanTransition(pending, pending) = true, want false (claim converges without a flip, never pending -> pending)")
	}
	if CanTransition(V1MaterializationRunning, V1MaterializationRunning) {
		t.Error("CanTransition(running, running) = true, want false (no self-transition guard exists)")
	}
	if CanTransition(V1MaterializationRunning, V1MaterializationPending) {
		t.Error("CanTransition(running, pending) = true, want false (failed jobs retry by claiming a new job, never a backward flip)")
	}
}

// TestMaterializationStatusWireWords pins the wire/DB words: typing must
// not change the strings on the wire or in the database. The durable
// word for running stays 'processing' (mapped at the store boundary).
func TestMaterializationStatusWireWords(t *testing.T) {
	for status, want := range map[V1MaterializationStatus]string{
		V1MaterializationPending:   "pending",
		V1MaterializationRunning:   "running",
		V1MaterializationCompleted: "completed",
		V1MaterializationFailed:    "failed",
	} {
		if string(status) != want {
			t.Errorf("V1MaterializationStatus = %q, want %q", string(status), want)
		}
	}
	for dbWord, want := range map[string]V1MaterializationStatus{
		"pending":    V1MaterializationPending,
		"processing": V1MaterializationRunning,
		"completed":  V1MaterializationCompleted,
		"failed":     V1MaterializationFailed,
	} {
		got, err := V1MaterializationStatusFromDB(dbWord)
		if err != nil || got != want {
			t.Errorf("V1MaterializationStatusFromDB(%q) = (%q, %v), want (%q, nil)", dbWord, got, err, want)
		}
	}
	if _, err := V1MaterializationStatusFromDB("exploding"); err == nil {
		t.Error("V1MaterializationStatusFromDB(exploding) = nil, want error (unknown statuses fail closed)")
	}
}

// TestResolveStatusWireWords pins the resolve outcome vocabulary: typing
// must not change the strings on the wire.
func TestResolveStatusWireWords(t *testing.T) {
	if string(V1ResolveResolved) != "resolved" {
		t.Errorf("V1ResolveResolved = %q, want %q", string(V1ResolveResolved), "resolved")
	}
	if string(V1ResolveUnresolved) != "unresolved" {
		t.Errorf("V1ResolveUnresolved = %q, want %q", string(V1ResolveUnresolved), "unresolved")
	}
}
