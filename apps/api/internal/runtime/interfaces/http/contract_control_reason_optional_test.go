package httpapi

// Control reason-tier tests: routine config mutations (ReasonOptional)
// succeed without an operator reason, while fund/safety-affecting
// mutations stay enforced (secure by default) and fail closed with 400.
// The Idempotency-Key stays required on both tiers, and exempt calls
// still emit their audit record with the empty reason as-is.

import (
	"fmt"
	"net/http"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

func TestControlReasonOptionalExempt(t *testing.T) {
	mail := newFakeControlMail()
	h := newMailHarness(t, mail)

	t.Run("exempt update succeeds without reason and audits empty", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
			body:   `{"from_name":"IdeaBoard","from_email":"noreply@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: map[string]string{contract.IdempotencyKeyHeader: "reason-opt-1"},
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%v)", status, payload)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Operation != "platform.update_mail_setting" || last.Reason != "" || last.Outcome != ControlAuditOK {
			t.Fatalf("audit = %+v, want empty reason with success outcome", last)
		}
	})

	t.Run("exempt test probe succeeds without reason", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: map[string]string{contract.IdempotencyKeyHeader: "reason-opt-2"},
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%v)", status, payload)
		}
	})

	t.Run("exempt proxied route passes the guard without reason", func(t *testing.T) {
		// Nil gateway dep fails closed past the guard with 501: the
		// absence of control_reason_required proves the exemption.
		h := newControlHarness(t, nil)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPut, path: "/api/control/v1/platform/ai-gateway",
			body:   `{}`,
			userID: controlOperatorID, role: "admin",
			headers: map[string]string{contract.IdempotencyKeyHeader: "reason-opt-3"},
		})
		if status != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501 (nil dep, guard passed) (%v)", status, payload)
		}
		if code := controlErrorCode(t, payload); code == string(contract.CodeControlReasonRequired) {
			t.Fatalf("code = %q, exempt route must not require a reason", code)
		}
	})

	t.Run("exempt route still requires the idempotency key", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
			body:   `{"from_name":"IdeaBoard","from_email":"noreply@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: map[string]string{OperatorReasonHeader: "has reason, no key"},
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("code = %q, want idempotency_key_required", code)
		}
	})
}

func TestControlReasonRequiredEnforced(t *testing.T) {
	h := newControlHarness(t, nil)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
	keyOnly := func(key string) map[string]string {
		return map[string]string{contract.IdempotencyKeyHeader: key}
	}

	t.Run("ban without reason is 400", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/ban", targetUser),
			body:   `{"expires_in_days":7}`,
			userID: controlOperatorID, role: "admin",
			headers: keyOnly("reason-enf-1"),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeControlReasonRequired) {
			t.Fatalf("code = %q, want control_reason_required", code)
		}
	})

	t.Run("grant without reason is 400", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/grants",
			body:   fmt.Sprintf(`{"user_id":%q,"amount_credits":10,"business_reference_id":"reason-enf-2"}`, targetUser),
			userID: controlOperatorID, role: "admin",
			headers: keyOnly("reason-enf-2"),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeControlReasonRequired) {
			t.Fatalf("code = %q, want control_reason_required", code)
		}
	})

	t.Run("delete mail setting without reason is 400", func(t *testing.T) {
		mailHarness := newMailHarness(t, newFakeControlMail())
		status, _, payload := mailHarness.serve(controlCall{
			method: http.MethodDelete, path: "/api/control/v1/platform/settings/mail",
			userID: controlOperatorID, role: "admin",
			headers: keyOnly("reason-enf-3"),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeControlReasonRequired) {
			t.Fatalf("code = %q, want control_reason_required", code)
		}
	})
}
