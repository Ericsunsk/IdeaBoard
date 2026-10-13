package httpapi

import (
	"net/http"
	"testing"

	"ideaboard/api/internal/identity/principal"
)

// Control durable request identity unit tests:
// the canonical hash pins method + route template + the real target +
// body, so the same key against a different target or body is a
// different request (409).

func testDurablePrincipal(t *testing.T) principal.Principal {
	t.Helper()
	p, err := principal.New(11, testUUIDForUser(11), "auth-11", "test-operator", "admin")
	if err != nil {
		t.Fatalf("principal.New: %v", err)
	}
	return p
}

func TestControlDurableHashPinsTarget(t *testing.T) {
	p := testDurablePrincipal(t)
	record := &ControlAuditRecord{TargetType: "user", TargetID: "usr_aaa"}
	same := controlDurableClaim(p, "users.ban", "hash-key-1", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":7}`))

	otherTarget := &ControlAuditRecord{TargetType: "user", TargetID: "usr_bbb"}
	divergent := controlDurableClaim(p, "users.ban", "hash-key-1", otherTarget, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":7}`))
	if divergent.RequestHash == same.RequestHash {
		t.Fatal("same key + same body + different target must hash differently (target conflict 409s)")
	}
	if divergent.Operator != same.Operator || divergent.Operation != same.Operation || divergent.Key != same.Key {
		t.Fatalf("scope moved with the target: %+v vs %+v", divergent, same)
	}

	otherBody := controlDurableClaim(p, "users.ban", "hash-key-1", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":30}`))
	if otherBody.RequestHash == same.RequestHash {
		t.Fatal("same key + same target + different body must hash differently (body conflict 409s)")
	}

	repeat := controlDurableClaim(p, "users.ban", "hash-key-1", &ControlAuditRecord{TargetType: "user", TargetID: "usr_aaa"}, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":7}`))
	if repeat.RequestHash != same.RequestHash {
		t.Fatal("same request must hash identically (retry replays)")
	}
	if repeat.TargetType != "user" || repeat.TargetID != "usr_aaa" {
		t.Fatalf("claim target = %q/%q, want the real audit target", repeat.TargetType, repeat.TargetID)
	}
}

func TestControlDurableHashTracksBody(t *testing.T) {
	p := testDurablePrincipal(t)
	record := &ControlAuditRecord{TargetType: "user", TargetID: "usr_aaa"}
	body := []byte(`{"expires_in_days":7}`)
	first := controlDurableClaim(p, "users.ban", "hash-key-2", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", body)
	again := controlDurableClaim(p, "users.ban", "hash-key-2", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", body)
	if again.RequestHash != first.RequestHash {
		t.Fatal("identical requests must hash identically ")
	}
	changed := controlDurableClaim(p, "users.ban", "hash-key-2", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":8}`))
	if changed.RequestHash == first.RequestHash {
		t.Fatal("different bodies must hash differently")
	}
}
