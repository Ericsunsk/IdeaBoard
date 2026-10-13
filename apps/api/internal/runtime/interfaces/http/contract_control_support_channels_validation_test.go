package httpapi

import (
	"fmt"
	"net/http"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
)

func TestControlSupportChannelsRequiresExplicitFields(t *testing.T) {
	h := newControlHarness(t, func(d *ContractControlDeps) {
		d.Settings = settings.NewRuntimeSettingsService(nil)
	})
	const path = "/api/control/v1/platform/settings/support-channels"
	tests := []struct {
		name string
		body string
		want int
	}{
		{"missing channels", `{}`, http.StatusBadRequest},
		{"null body", `null`, http.StatusBadRequest},
		{"null channels", `{"channels":null}`, http.StatusBadRequest},
		{"missing enabled", `{"channels":[{"id":"x","name":"X","kind":"custom","url":"https://example.com","sort_order":0}]}`, http.StatusBadRequest},
		{"missing sort order", `{"channels":[{"id":"x","name":"X","kind":"custom","url":"https://example.com","enabled":false}]}`, http.StatusBadRequest},
		{"unknown field", `{"channels":[],"extra":true}`, http.StatusBadRequest},
		{"unknown channel field", `{"channels":[{"id":"x","name":"X","kind":"custom","url":"https://example.com","enabled":false,"sort_order":0,"extra":true}]}`, http.StatusBadRequest},
		{"malformed body", `{"channels":`, http.StatusBadRequest},
		{"explicit zero values", `{"channels":[{"id":"x","name":"X","kind":"custom","url":"https://example.com","enabled":false,"sort_order":0}]}`, http.StatusInternalServerError},
		{"explicit clear", `{"channels":[]}`, http.StatusInternalServerError},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, payload := h.serve(controlCall{method: http.MethodPut, path: path, body: tt.body,
				userID: controlOperatorID, role: "admin", headers: controlMutationHeaders(fmt.Sprintf("support-validation-%d", i))})
			if status != tt.want {
				t.Fatalf("status = %d payload=%v, want %d", status, payload, tt.want)
			}
			wantCode := contract.CodeBadRequest
			if tt.want == http.StatusInternalServerError {
				wantCode = contract.CodeServiceUnavailable
			}
			if code := controlErrorCode(t, payload); code != string(wantCode) {
				t.Fatalf("code = %q, want %q", code, wantCode)
			}
		})
	}
}

func TestControlSupportChannelsDeniesUser(t *testing.T) {
	h := newControlHarness(t, nil)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		status, _, payload := h.serve(controlCall{method: method, path: "/api/control/v1/platform/settings/support-channels",
			userID: controlUserAID, role: "user", body: `{"channels":[]}`, headers: controlMutationHeaders("user-support-change")})
		if status != http.StatusForbidden || controlErrorCode(t, payload) != string(contract.CodeForbidden) {
			t.Fatalf("%s user access = status %d payload %v", method, status, payload)
		}
	}
}
