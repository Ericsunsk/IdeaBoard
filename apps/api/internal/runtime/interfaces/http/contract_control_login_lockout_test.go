package httpapi

import (
	"net/http"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
)

func TestControlLoginLockoutIdempotency(t *testing.T) {
	db := testdb.Open(t)
	service := settings.NewRuntimeSettingsService(settings.NewSystemConfigRepository(db))
	h := newControlHarness(t, func(d *ContractControlDeps) { d.Settings = service })
	const path = "/api/control/v1/platform/settings/login-lockout"

	status, _, payload := h.serve(controlCall{method: http.MethodGet, path: path, userID: controlOperatorID, role: "admin"})
	if status != http.StatusOK {
		t.Fatalf("get login lockout = status %d payload %v", status, payload)
	}
	data, ok := controlData(t, payload).(map[string]any)
	if !ok {
		t.Fatalf("login lockout data is not an object: %v", payload)
	}
	if data["threshold"] != float64(10) || data["duration_seconds"] != float64(900) {
		t.Fatalf("login lockout defaults = %v, want threshold=10 duration=900", data)
	}

	const body = `{"threshold":5,"duration_seconds":300}`
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: map[string]string{contract.IdempotencyKeyHeader: "login-lockout-0"}})
	if status != http.StatusOK {
		t.Fatalf("key-only save = status %d payload %v, want 200", status, payload)
	}
	if last := h.sink.Records()[len(h.sink.Records())-1]; last.Operation != "platform.update_login_lockout" {
		t.Fatalf("key-only audit = %+v, want the recorded operation", last)
	}
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body})
	if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing idempotency key = status %d payload %v", status, payload)
	}
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: nil})
	if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing idempotency key = status %d payload %v", status, payload)
	}
	call := controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: controlMutationHeaders("login-lockout-1")}
	for i := 0; i < 2; i++ {
		status, _, payload = h.serve(call)
		if status != http.StatusOK {
			t.Fatalf("save/replay %d = status %d payload %v", i, status, payload)
		}
		controlData(t, payload)
	}
	records := h.sink.Records()
	last := records[len(records)-1]
	if last.Operation != "platform.update_login_lockout" || last.IdempotencyKey != "login-lockout-1" {
		t.Fatalf("control audit = %+v", last)
	}
	call.body = `{"threshold":6,"duration_seconds":300}`
	status, _, payload = h.serve(call)
	if status != http.StatusConflict || controlErrorCode(t, payload) != string(contract.CodeIdempotencyConflict) {
		t.Fatalf("key reuse conflict = status %d payload %v", status, payload)
	}
	got, err := service.GetLoginLockout(t.Context())
	if err != nil || got != (settings.LoginLockoutSetting{Threshold: 5, DurationSeconds: 300}) {
		t.Fatalf("login lockout after conflicting request = %+v err=%v", got, err)
	}

	badCall := controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: `{"threshold":2,"duration_seconds":300}`, headers: controlMutationHeaders("login-lockout-2")}
	status, _, payload = h.serve(badCall)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("out-of-range threshold = status %d payload %v, want 422", status, payload)
	}
}
