package v1

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"net/http"
	"testing"
)

// ---- merged from members_limit_test.go ----
func TestV1WorkspaceMemberLimit(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	workspace := v1WorkspaceFixture(t, ctx, store, "member limit", 1)
	for id := int64(1); id <= 6; id++ {
		dir.ensure(id)
	}
	for id := int64(2); id <= 5; id++ {
		if _, err := store.AddWorkspaceMember(ctx, workspace.PublicID, id, tenancy.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	engine := newV1MembersDisplayEngine(store, dir, nil)
	call := func(id int64, key string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method:  http.MethodPost,
			path:    runtimehttp.ContractV1GroupPath + "/workspaces/" + workspace.PublicID + "/members",
			userID:  1,
			body:    fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, tenancy.FormatUserPublicID(dir.ensure(id))),
			headers: map[string]string{contract.IdempotencyKeyHeader: key},
		})
		return status, payload
	}
	status, payload := call(6, "capacity-new")
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceMemberLimitReached) {
		t.Fatalf("sixth member = %d %v", status, payload)
	}
	status, payload = call(2, "capacity-existing")
	if status != http.StatusCreated {
		t.Fatalf("existing member at capacity = %d %v", status, payload)
	}
}

// TestV1DefaultWorkspaceInviteProtected proves Personal collaboration
// (#952) at the HTTP boundary without a database: the Personal Default
// workspace is private and rejects invites with 403
// workspace_default_protected (leaving no membership behind), while a
// non-Default Personal workspace invites freely with no account-default
// gate (absent quota reads as unlimited).
func TestV1DefaultWorkspaceInviteProtected(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const owner, target int64 = 1, 2
	dir.ensure(owner)
	dir.ensure(target)
	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	invite := func(workspacePublicID string, key string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost, path: runtimehttp.ContractV1GroupPath + "/workspaces/" + workspacePublicID + "/members",
			userID: owner,
			body:   fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, tenancy.FormatUserPublicID(dir.ensure(target))),
			headers: map[string]string{
				contract.IdempotencyKeyHeader: key,
			},
		})
		return status, payload
	}
	if status, payload := invite(def.PublicID, "default-invite-1"); status != http.StatusForbidden ||
		v1ErrorCode(t, payload) != string(contract.CodeWorkspaceDefaultProtected) {
		t.Fatalf("default invite = %d %v, want 403 workspace_default_protected", status, payload)
	}
	if _, member, _ := store.GetWorkspaceRole(ctx, def.PublicID, target); member {
		t.Fatal("blocked default invite must not create a membership")
	}
	if _, err := store.AddWorkspaceMember(ctx, def.PublicID, target, tenancy.RoleMember); !errors.Is(err, tenancy.ErrDefaultWorkspaceProtected) {
		t.Fatalf("store default invite = %v, want ErrDefaultWorkspaceProtected", err)
	}

	plain := v1WorkspaceFixture(t, ctx, store, "plain-collab", owner)
	if status, payload := invite(plain.PublicID, "plain-invite-1"); status != http.StatusCreated {
		t.Fatalf("plain invite = %d %v, want 201 (open, no default gate)", status, payload)
	}
	if _, member, _ := store.GetWorkspaceRole(ctx, plain.PublicID, target); !member {
		t.Fatal("open invite must create a membership")
	}
}
