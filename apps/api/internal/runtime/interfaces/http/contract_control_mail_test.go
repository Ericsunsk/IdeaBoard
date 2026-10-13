package httpapi

// Control mail-setting tests: the mail handlers delegate to the
// Auth-owned mail seam under the standard control guard (capability,
// audit with operator identity, header-only idempotency, contract
// envelope), fail closed without the seam, and map Auth failures onto
// stable control codes.

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/platform/contract"
)

type fakeControlMail struct {
	mu         sync.Mutex
	summary    *auth.MailConfigSummary
	getErr     error
	updateErr  error
	clearErr   error
	testErr    error
	updates    []auth.MailConfigUpdate
	actors     []int64
	recipients []string
	clears     int
}

func newFakeControlMail() *fakeControlMail {
	return &fakeControlMail{
		summary: &auth.MailConfigSummary{
			Configured: true, APIKeyConfigured: true,
			FromName: "IdeaBoard", FromEmail: "noreply@example.com", ReplyTo: "support@example.com",
			ConfigVersion: 2, Readiness: "ready",
		},
	}
}

func (f *fakeControlMail) GetMailConfig(_ context.Context) (*auth.MailConfigSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.summary, nil
}

func (f *fakeControlMail) UpdateMailConfig(_ context.Context, input auth.MailConfigUpdate, actorID int64) (*auth.MailConfigSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, input)
	f.actors = append(f.actors, actorID)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.summary, nil
}

func (f *fakeControlMail) ClearMailConfig(_ context.Context, actorID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clears++
	f.actors = append(f.actors, actorID)
	return f.clearErr
}

func (f *fakeControlMail) SendMailConfigTest(_ context.Context, recipient string, actorID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recipients = append(f.recipients, recipient)
	f.actors = append(f.actors, actorID)
	return f.testErr
}

func newMailHarness(t *testing.T, mail *fakeControlMail) *controlHarness {
	t.Helper()
	return newControlHarness(t, func(d *ContractControlDeps) {
		d.Mail = mail
	})
}

func mailMutationHeaders(key string) map[string]string {
	return map[string]string{
		contract.IdempotencyKeyHeader: key,
	}
}

func TestControlMailSettingGet(t *testing.T) {
	h := newMailHarness(t, newFakeControlMail())

	t.Run("read returns the contract envelope", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/platform/settings/mail",
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%v)", status, payload)
		}
		data, ok := controlData(t, payload).(map[string]any)
		if !ok {
			t.Fatalf("data envelope = %v", payload)
		}
		if data["from_email"] != "noreply@example.com" || data["readiness"] != "ready" {
			t.Fatalf("mail summary = %v", data)
		}
		if data["configured"] != true || data["api_key_configured"] != true {
			t.Fatalf("mail flags = %v", data)
		}
	})

	t.Run("anonymous read is 401", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/platform/settings/mail",
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeUnauthorized) {
			t.Fatalf("code = %q, want unauthorized", code)
		}
	})

	t.Run("unwired seam fails closed with 503", func(t *testing.T) {
		plain := newControlHarness(t, nil)
		status, _, payload := plain.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/platform/settings/mail",
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503 (%v)", status, payload)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeServiceUnavailable) {
			t.Fatalf("code = %q, want service_unavailable", code)
		}
	})
}

func TestControlMailSettingUpdate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
	}{
		{name: "omitted"},
		{name: "disabled", field: `,"enabled":false`},
		{name: "enabled", field: `,"enabled":true`},
	} {
		t.Run("preserves enabled "+tc.name, func(t *testing.T) {
			mail := newFakeControlMail()
			h := newMailHarness(t, mail)
			status, _, payload := h.serve(controlCall{
				method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
				body:   `{"from_name":"IdeaBoard","from_email":"auth@example.com"` + tc.field + `}`,
				userID: controlOperatorID, role: "admin",
				headers: mailMutationHeaders("mail-enabled-" + tc.name),
			})
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%v)", status, payload)
			}
			mail.mu.Lock()
			defer mail.mu.Unlock()
			if len(mail.updates) != 1 {
				t.Fatalf("updates = %d, want 1", len(mail.updates))
			}
			got := mail.updates[0].Enabled
			if tc.name == "omitted" {
				if got != nil {
					t.Fatal("omitted enabled must remain nil")
				}
			} else if got == nil || *got != (tc.name == "enabled") {
				t.Fatalf("enabled = %v, want %s", got, tc.name)
			}
		})
	}

	t.Run("update delegates with the operator as actor and audits the outcome", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
			body:   `{"from_name":"IdeaBoard Ops","from_email":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-put-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%v)", status, payload)
		}
		data, ok := controlData(t, payload).(map[string]any)
		if !ok || data["from_email"] != "noreply@example.com" {
			t.Fatalf("mail summary = %v", payload)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.updates) != 1 {
			t.Fatalf("updates = %d, want 1", len(mail.updates))
		}
		if mail.updates[0].FromName != "IdeaBoard Ops" || mail.updates[0].FromEmail != "ops@example.com" {
			t.Fatalf("update input = %+v", mail.updates[0])
		}
		if len(mail.actors) != 1 || mail.actors[0] != controlOperatorID {
			t.Fatalf("actors = %v, want [%d]", mail.actors, controlOperatorID)
		}
		records := h.sink.Records()
		last := records[len(records)-1]
		if last.Capability != ControlCapabilityPlatform || last.Operation != "platform.update_mail_setting" {
			t.Fatalf("audit = %+v, want control:platform platform.update_mail_setting", last)
		}
		if last.Outcome != ControlAuditOK {
			t.Fatalf("audit outcome = %+v", last)
		}
	})

	t.Run("idempotency key is required", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
			body:   `{"from_name":"IdeaBoard","from_email":"noreply@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: map[string]string{contract.IdempotencyKeyHeader: "mail-put-3"},
		})
		if status != http.StatusOK {
			t.Fatalf("key-only status = %d, want 200 (%v)", status, payload)
		}
		if last := h.sink.Records()[len(h.sink.Records())-1]; last.Operation != "platform.update_mail_setting" {
			t.Fatalf("key-only audit = %+v, want the recorded operation", last)
		}
		status, _, payload = h.serve(controlCall{
			method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
			body:   `{"from_name":"IdeaBoard","from_email":"noreply@example.com"}`,
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("code = %q, want idempotency_key_required", code)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.updates) != 1 {
			t.Fatalf("guarded updates = %d, want 1 (only the key-only call)", len(mail.updates))
		}
	})

	t.Run("unknown fields are rejected", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
			body:   `{"from_name":"IdeaBoard","from_email":"noreply@example.com","bogus":1}`,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-put-4"),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%v)", status, payload)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeBadRequest) {
			t.Fatalf("code = %q, want bad_request", code)
		}
	})

	t.Run("auth failures map onto stable codes", func(t *testing.T) {
		cases := []struct {
			name   string
			seam   error
			status int
			code   contract.Code
		}{
			{"validation", &auth.MailConfigError{Status: http.StatusBadRequest, Message: "发件人无效"}, http.StatusBadRequest, contract.CodeBadRequest},
			{"conflict", &auth.MailConfigError{Status: http.StatusConflict, Message: "not configured"}, http.StatusConflict, contract.CodeConflict},
			{"auth down", &auth.MailConfigError{Status: http.StatusBadGateway, Message: "boom"}, http.StatusServiceUnavailable, contract.CodeServiceUnavailable},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				mail := newFakeControlMail()
				mail.updateErr = tc.seam
				h := newMailHarness(t, mail)
				status, _, payload := h.serve(controlCall{
					method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
					body:   `{"from_name":"IdeaBoard","from_email":"noreply@example.com"}`,
					userID: controlOperatorID, role: "admin",
					headers: mailMutationHeaders("mail-put-err-" + tc.name),
				})
				if status != tc.status {
					t.Fatalf("status = %d, want %d (%v)", status, tc.status, payload)
				}
				if code := controlErrorCode(t, payload); code != string(tc.code) {
					t.Fatalf("code = %q, want %q", code, tc.code)
				}
			})
		}
	})
}

func TestControlMailSettingDelete(t *testing.T) {
	mail := newFakeControlMail()
	h := newMailHarness(t, mail)

	status, _, payload := h.serve(controlCall{
		method: http.MethodDelete, path: "/api/control/v1/platform/settings/mail",
		userID: controlOperatorID, role: "admin",
		headers: mailMutationHeaders("mail-del-1"),
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%v)", status, payload)
	}
	data, ok := controlData(t, payload).(map[string]any)
	if !ok || data["deleted"] != true {
		t.Fatalf("clear ack = %v", payload)
	}
	mail.mu.Lock()
	defer mail.mu.Unlock()
	if mail.clears != 1 {
		t.Fatalf("clears = %d, want 1", mail.clears)
	}
	if len(mail.actors) != 1 || mail.actors[0] != controlOperatorID {
		t.Fatalf("actors = %v, want [%d]", mail.actors, controlOperatorID)
	}
}

func TestControlMailSettingTest(t *testing.T) {
	t.Run("test sends and acks with key and audit", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-test-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%v)", status, payload)
		}
		data, ok := controlData(t, payload).(map[string]any)
		if !ok || data["sent"] != true {
			t.Fatalf("test ack = %v", payload)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.recipients) != 1 || mail.recipients[0] != "ops@example.com" {
			t.Fatalf("recipients = %v", mail.recipients)
		}
		if len(mail.actors) != 1 || mail.actors[0] != controlOperatorID {
			t.Fatalf("actors = %v, want [%d]", mail.actors, controlOperatorID)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Capability != ControlCapabilityPlatform || last.Operation != "platform.test_mail_setting" {
			t.Fatalf("audit = %+v, want control:platform platform.test_mail_setting", last)
		}
		if last.IdempotencyKey != "mail-test-1" || last.Outcome != ControlAuditOK {
			t.Fatalf("audit key/outcome = %+v", last)
		}
	})

	t.Run("idempotency key is required", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: map[string]string{contract.IdempotencyKeyHeader: "mail-test-key"},
		})
		if status != http.StatusOK {
			t.Fatalf("key-only status = %d, want 200 (%v)", status, payload)
		}
		if last := h.sink.Records()[len(h.sink.Records())-1]; last.Operation != "platform.test_mail_setting" {
			t.Fatalf("key-only audit = %+v, want the recorded operation", last)
		}
		status, _, payload = h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("code = %q, want idempotency_key_required", code)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.recipients) != 1 {
			t.Fatalf("guarded tests = %d, want 1 (only the key-only call)", len(mail.recipients))
		}
	})

	t.Run("capability gate holds for anonymous and non-admin callers", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:    `{"recipient":"ops@example.com"}`,
			headers: mailMutationHeaders("mail-test-cap-1"),
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("anon status = %d, want 401", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeUnauthorized) {
			t.Fatalf("anon code = %q, want unauthorized", code)
		}
		status, _, _ = h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlUserAID, role: "user",
			headers: mailMutationHeaders("mail-test-cap-2"),
		})
		if status != http.StatusForbidden {
			t.Fatalf("member status = %d, want 403", status)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.recipients) != 0 {
			t.Fatalf("denied tests = %d, want 0", len(mail.recipients))
		}
	})

	t.Run("same key same request replays without resending the probe", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		body := `{"recipient":"ops@example.com"}`
		first := controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   body,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-test-replay-1"),
		}
		status, _, _ := h.serve(first)
		if status != http.StatusOK {
			t.Fatalf("first status = %d, want 200", status)
		}
		mail.mu.Lock()
		if len(mail.recipients) != 1 {
			mail.mu.Unlock()
			t.Fatalf("recipients = %d, want 1", len(mail.recipients))
		}
		mail.mu.Unlock()
		second := first
		second.headers = mailMutationHeaders("mail-test-replay-1")
		status, _, payload := h.serve(second)
		if status != http.StatusOK {
			t.Fatalf("replay status = %d, want 200 (%v)", status, payload)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.recipients) != 1 {
			t.Fatalf("replay must not resend: recipients = %v", mail.recipients)
		}
		records := h.sink.Records()
		if last := records[len(records)-1]; !last.Replayed || last.Outcome != ControlAuditOK {
			t.Fatalf("replay audit = %+v, want replayed success", last)
		}
	})

	t.Run("same key different request conflicts before the seam", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		first := controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-test-conflict-1"),
		}
		if status, _, _ := h.serve(first); status != http.StatusOK {
			t.Fatalf("first status = %d, want 200", status)
		}
		divergent := first
		divergent.body = `{"recipient":"other@example.com"}`
		divergent.headers = mailMutationHeaders("mail-test-conflict-1")
		status, _, payload := h.serve(divergent)
		if status != http.StatusConflict {
			t.Fatalf("divergent status = %d, want 409", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyConflict) {
			t.Fatalf("code = %q, want idempotency_conflict", code)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.recipients) != 1 || mail.recipients[0] != "ops@example.com" {
			t.Fatalf("conflict must not send: recipients = %v", mail.recipients)
		}
	})

	t.Run("blank recipient is 400 before the seam", func(t *testing.T) {
		mail := newFakeControlMail()
		h := newMailHarness(t, mail)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"  "}`,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-test-2"),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%v)", status, payload)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeBadRequest) {
			t.Fatalf("code = %q, want bad_request", code)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.recipients) != 0 {
			t.Fatalf("guarded tests = %d, want 0", len(mail.recipients))
		}
	})

	t.Run("unconfigured auth maps to 409", func(t *testing.T) {
		mail := newFakeControlMail()
		mail.testErr = &auth.MailConfigError{Status: http.StatusConflict, Message: "not configured"}
		h := newMailHarness(t, mail)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-test-3"),
		})
		if status != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (%v)", status, payload)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeConflict) {
			t.Fatalf("code = %q, want conflict", code)
		}
	})

	t.Run("delivery failure never reports success and never poisons the key", func(t *testing.T) {
		mail := newFakeControlMail()
		mail.testErr = &auth.MailConfigError{Status: http.StatusBadGateway, Message: "provider down"}
		h := newMailHarness(t, mail)
		call := controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-test-fail-1"),
		}
		status, _, payload := h.serve(call)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("failure status = %d, want 503 (%v)", status, payload)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeServiceUnavailable) {
			t.Fatalf("code = %q, want service_unavailable", code)
		}
		if _, hasData := payload["data"]; hasData {
			t.Fatalf("failure must not carry data (never success): %v", payload)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Outcome != ControlAuditFailed {
			t.Fatalf("failure audit = %+v, want failed", last)
		}
		// The failure is not cached: clearing the downstream fault and
		// retrying the same key re-executes the probe instead of
		// replaying the failure. A rollback elsewhere never implies
		// the sent probe was revoked.
		mail.mu.Lock()
		mail.testErr = nil
		mail.mu.Unlock()
		status, _, payload = h.serve(call)
		if status != http.StatusOK {
			t.Fatalf("retry status = %d, want 200 (%v)", status, payload)
		}
		if ok := controlData(t, payload).(map[string]any); ok["sent"] != true {
			t.Fatalf("retry ack = %v", payload)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.recipients) != 2 {
			t.Fatalf("retry must re-execute: recipients = %v", mail.recipients)
		}
	})

	t.Run("unknown envelope failure maps to 503 and stays retryable", func(t *testing.T) {
		mail := newFakeControlMail()
		mail.testErr = &auth.MailConfigError{Status: http.StatusInternalServerError, Message: "unknown"}
		h := newMailHarness(t, mail)
		call := controlCall{
			method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
			body:   `{"recipient":"ops@example.com"}`,
			userID: controlOperatorID, role: "admin",
			headers: mailMutationHeaders("mail-test-fail-2"),
		}
		status, _, payload := h.serve(call)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503 (%v)", status, payload)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeServiceUnavailable) {
			t.Fatalf("code = %q, want service_unavailable", code)
		}
		mail.mu.Lock()
		mail.testErr = nil
		mail.mu.Unlock()
		status, _, _ = h.serve(call)
		if status != http.StatusOK {
			t.Fatalf("retry status = %d, want 200", status)
		}
		mail.mu.Lock()
		defer mail.mu.Unlock()
		if len(mail.recipients) != 2 {
			t.Fatalf("unknown failure must not poison the key: recipients = %v", mail.recipients)
		}
	})
}
