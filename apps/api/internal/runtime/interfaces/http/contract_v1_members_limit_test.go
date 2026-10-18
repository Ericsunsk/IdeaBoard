package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

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
	engine := newV1MembersEmailEngine(store, dir, nil)
	call := func(id int64, key string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method:  http.MethodPost,
			path:    ContractV1GroupPath + "/workspaces/" + workspace.PublicID + "/members",
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
