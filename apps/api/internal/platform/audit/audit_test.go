package audit

import (
	"context"
	"testing"
)

func TestMapOutcomeConvergesToCheckConstraint(t *testing.T) {
	cases := map[Outcome]string{
		OutcomeRead:    "success",
		OutcomeSuccess: "success",
		OutcomeDenied:  "denied",
		OutcomeFailure: "failure",
		"":             "success",
	}
	for in, want := range cases {
		if got := MapOutcome(in); got != want {
			t.Errorf("MapOutcome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppendRecordSkipsWithoutStore(t *testing.T) {
	var nilStore *Store
	if err := nilStore.AppendRecord(context.Background(), RecordParams{OperatorUserID: 1}); err != nil {
		t.Fatalf("nil store must skip, got %v", err)
	}
	empty := &Store{}
	if err := empty.AppendRecord(context.Background(), RecordParams{OperatorUserID: 1}); err != nil {
		t.Fatalf("empty store must skip, got %v", err)
	}
}

func TestAppendRecordSkipsServiceSubjects(t *testing.T) {
	store := NewStore(nil)
	if err := store.AppendRecord(context.Background(), RecordParams{OperatorUserID: 0}); err != nil {
		t.Fatalf("zero operator must skip without a users row, got %v", err)
	}
}
