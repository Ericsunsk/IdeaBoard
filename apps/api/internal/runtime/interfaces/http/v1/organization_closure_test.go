package v1

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

// Organization closure lifecycle over HTTP (#955): only the Owner closes
// and restores; closure suspends member access and mutations while reads
// stay visible; restore resumes prior standing.
func TestV1OrganizationClosure(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const owner, member, outsider = 41, 42, 43
	status, _, created := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/organizations", userID: owner,
		body: `{"data":{"name":"acme"}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "org-close-create"}})
	if status != 201 {
		t.Fatalf("create org = %d %v", status, created)
	}
	orgID := created["data"].(map[string]any)["id"].(string)
	if got := created["data"].(map[string]any)["status"]; got != tenancy.OrganizationStatusActive {
		t.Fatalf("create org status = %v, want active", got)
	}
	store.SeedOrganizationMember(orgID, member, tenancy.RoleMember)
	closurePath := "/api/v1/organizations/" + orgID + "/closure"
	restorePath := "/api/v1/organizations/" + orgID + "/restore"
	closeCall := func(userID int64, key, body string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: closurePath, userID: userID, body: body}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, engine, call)
		return status, payload
	}

	if status, payload := closeCall(owner, "", `{"data":{"reason":"x"}}`); status != 400 ||
		v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("close without key = %d %v, want 400 idempotency_key_required", status, payload)
	}
	if status, payload := closeCall(member, "close-member", `{"data":{"reason":"x"}}`); status != 403 ||
		v1ErrorCode(t, payload) != string(contract.CodeOrganizationForbidden) {
		t.Fatalf("member close = %d %v, want 403 organization_forbidden", status, payload)
	}
	if status, _ := closeCall(outsider, "close-outsider", `{"data":{"reason":"x"}}`); status != 404 {
		t.Fatalf("outsider close = %d, want 404", status)
	}
	longReason := strings.Repeat("x", 281)
	if status, payload := closeCall(owner, "close-long", fmt.Sprintf(`{"data":{"reason":%q}}`, longReason)); status != 422 ||
		v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("long reason close = %d %v, want 422 validation_failed", status, payload)
	}

	status, closed := closeCall(owner, "close-1", `{"data":{"reason":"winding down"}}`)
	if status != 200 {
		t.Fatalf("owner close = %d %v", status, closed)
	}
	data := closed["data"].(map[string]any)
	if data["status"] != tenancy.OrganizationStatusClosed || data["closed_at"] == "" {
		t.Fatalf("closed org = %v, want status closed with closed_at", data)
	}
	if status, payload := closeCall(owner, "close-2", `{"data":{}}`); status != 409 ||
		v1ErrorCode(t, payload) != string(contract.CodeOrganizationClosed) {
		t.Fatalf("second close = %d %v, want 409 organization_closed", status, payload)
	}

	// Reads stay visible while closed; mutations fail closed.
	status, _, got := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/organizations/" + orgID, userID: member})
	if status != 200 || got["data"].(map[string]any)["status"] != tenancy.OrganizationStatusClosed {
		t.Fatalf("GET closed org = %d %v, want 200 closed", status, got)
	}
	status, _, renamed := doV1(t, engine, v1Call{method: http.MethodPatch, path: "/api/v1/organizations/" + orgID, userID: owner, body: `{"data":{"name":"late"}}`})
	if status != 409 || v1ErrorCode(t, renamed) != string(contract.CodeOrganizationClosed) {
		t.Fatalf("rename while closed = %d %v, want 409 organization_closed", status, renamed)
	}
	status, _, wsPayload := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/organizations/" + orgID + "/workspaces", userID: owner,
		body: `{"data":{"name":"late-ws"}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "org-ws-closed"}})
	if status != 409 || v1ErrorCode(t, wsPayload) != string(contract.CodeOrganizationClosed) {
		t.Fatalf("workspace create while closed = %d %v, want 409 organization_closed", status, wsPayload)
	}

	restoreCall := func(userID int64, key string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: restorePath, userID: userID}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, engine, call)
		return status, payload
	}
	if status, payload := restoreCall(owner, ""); status != 400 ||
		v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("restore without key = %d %v, want 400", status, payload)
	}
	if status, payload := restoreCall(member, "restore-member"); status != 403 ||
		v1ErrorCode(t, payload) != string(contract.CodeOrganizationForbidden) {
		t.Fatalf("member restore = %d %v, want 403", status, payload)
	}
	status, restored := restoreCall(owner, "restore-1")
	if status != 200 {
		t.Fatalf("owner restore = %d %v", status, restored)
	}
	if got := restored["data"].(map[string]any)["status"]; got != tenancy.OrganizationStatusActive {
		t.Fatalf("restored status = %v, want active", got)
	}
	if status, payload := restoreCall(owner, "restore-2"); status != 409 ||
		v1ErrorCode(t, payload) != string(contract.CodeOrganizationNotClosed) {
		t.Fatalf("second restore = %d %v, want 409 organization_not_closed", status, payload)
	}
}

// Closed Organizations suspend workspace content access over HTTP (#955):
// member workspace reads converge to 404 until restore.
func TestV1ClosedOrganizationSuspendsWorkspaceAccess(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const owner, member = 51, 52
	status, _, created := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/organizations", userID: owner,
		body: `{"data":{"name":"acme"}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "org-suspend-create"}})
	if status != 201 {
		t.Fatalf("create org = %d %v", status, created)
	}
	orgID := created["data"].(map[string]any)["id"].(string)
	store.SeedOrganizationMember(orgID, member, tenancy.RoleMember)
	status, _, wsPayload := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/organizations/" + orgID + "/workspaces", userID: owner,
		body: `{"data":{"name":"org-ws"}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "org-suspend-ws"}})
	if status != 201 {
		t.Fatalf("create org workspace = %d %v", status, wsPayload)
	}
	wsID := wsPayload["data"].(map[string]any)["id"].(string)
	wsPath := "/api/v1/workspaces/" + wsID
	if status, _, _ := doV1(t, engine, v1Call{method: http.MethodGet, path: wsPath, userID: owner}); status != 200 {
		t.Fatalf("GET workspace while open = %d, want 200", status)
	}

	status, _, closed := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/organizations/" + orgID + "/closure", userID: owner,
		body: `{"data":{}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "org-suspend-close"}})
	if status != 200 {
		t.Fatalf("close = %d %v", status, closed)
	}
	for _, userID := range []int64{owner, member} {
		if status, _, _ := doV1(t, engine, v1Call{method: http.MethodGet, path: wsPath, userID: userID}); status != 404 {
			t.Fatalf("GET workspace while closed user=%d = %d, want 404", userID, status)
		}
	}
	status, _, listed := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces", userID: owner})
	if status != 200 {
		t.Fatalf("list workspaces while closed = %d", status)
	}
	for _, item := range listed["data"].([]any) {
		if item.(map[string]any)["id"] == wsID {
			t.Fatalf("closed org workspace still listed")
		}
	}

	status, _, restored := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/organizations/" + orgID + "/restore", userID: owner,
		headers: map[string]string{contract.IdempotencyKeyHeader: "org-suspend-restore"}})
	if status != 200 {
		t.Fatalf("restore = %d %v", status, restored)
	}
	if status, _, _ := doV1(t, engine, v1Call{method: http.MethodGet, path: wsPath, userID: owner}); status != 200 {
		t.Fatalf("GET workspace after restore = %d, want 200", status)
	}
}
