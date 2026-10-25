package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSendWorkspaceInvitationContract(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		assertContractRequest(t, r, http.MethodPost, "/internal/mail/workspace-invitation")
		assertJSONContentType(t, r)
		body := decodeContractBody(t, r)
		for key, value := range map[string]string{
			"recipient": "member@example.com", "invite_id": "wsi_test",
			"workspace_name": "Studio", "inviter_name": "Alex",
			"role": "member", "token": strings.Repeat("a", 64),
		} {
			assertContractField(t, body, key, value)
		}
		writeContractJSON(w, http.StatusOK, map[string]any{
			"code": 0, "msg": "success", "data": map[string]any{"ok": true},
		})
	})
	defer server.Close()
	if err := newContractClient(server).SendWorkspaceInvitation(context.Background(), WorkspaceInvitationMail{
		Recipient: "member@example.com", InviteID: "wsi_test", WorkspaceName: "Studio",
		InviterName: "Alex", Role: "member", Token: strings.Repeat("a", 64),
	}); err != nil {
		t.Fatalf("SendWorkspaceInvitation: %v", err)
	}
}

func TestSendWorkspaceInvitationRejectsFailedAndInvalidSuccess(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unready", 503, `{"code":503,"msg":"workspace invitation delivery failed"}`},
		{"provider failure", 502, `{"code":502,"msg":"workspace invitation delivery failed"}`},
		{"false success", 200, `{"code":0,"msg":"success","data":{"ok":false}}`},
		{"missing success", 200, `{"code":0,"msg":"success","data":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			defer server.Close()
			if err := newContractClient(server).SendWorkspaceInvitation(context.Background(), WorkspaceInvitationMail{}); err == nil {
				t.Fatal("delivery must not succeed for a failed or malformed response")
			}
		})
	}
}
