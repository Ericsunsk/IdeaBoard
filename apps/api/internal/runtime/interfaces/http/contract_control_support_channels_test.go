package httpapi

import (
	"net/http"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
)

func TestControlSupportChannelsIdempotency(t *testing.T) {
	db := testdb.Open(t)
	service := settings.NewRuntimeSettingsService(settings.NewSystemConfigRepository(db))
	h := newControlHarness(t, func(d *ContractControlDeps) { d.Settings = service })
	const path = "/api/control/v1/platform/settings/support-channels"
	const body = `{"channels":[{"id":"discord","name":"Discord","kind":"discord","url":"https://discord.gg/example","enabled":true,"sort_order":0}]}`

	status, _, payload := h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: map[string]string{contract.IdempotencyKeyHeader: "support-settings-0"}})
	if status != http.StatusOK {
		t.Fatalf("key-only save = status %d payload %v, want 200", status, payload)
	}
	if last := h.sink.Records()[len(h.sink.Records())-1]; last.Operation != "platform.update_support_channels" {
		t.Fatalf("key-only audit = %+v, want the recorded operation", last)
	}
	// The idempotency key stays required for mutations.
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body})
	if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing idempotency key = status %d payload %v", status, payload)
	}
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: nil})
	if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing idempotency key = status %d payload %v", status, payload)
	}
	call := controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: controlMutationHeaders("support-settings-1")}
	for i := 0; i < 2; i++ {
		status, _, payload = h.serve(call)
		if status != http.StatusOK {
			t.Fatalf("save/replay %d = status %d payload %v", i, status, payload)
		}
		controlData(t, payload)
	}
	records := h.sink.Records()
	last := records[len(records)-1]
	if last.Operation != "platform.update_support_channels" || last.IdempotencyKey != "support-settings-1" {
		t.Fatalf("control audit = %+v", last)
	}
	call.body = `{"channels":[]}`
	status, _, payload = h.serve(call)
	if status != http.StatusConflict || controlErrorCode(t, payload) != string(contract.CodeIdempotencyConflict) {
		t.Fatalf("key reuse conflict = status %d payload %v", status, payload)
	}
	got, err := service.GetSupportChannels(t.Context())
	if err != nil || len(got.Channels) != 1 {
		t.Fatalf("channels after conflicting request = %+v err=%v", got, err)
	}
}
