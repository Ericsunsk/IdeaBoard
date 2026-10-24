package v1

// Organization read expansion tests (?expand=): the detail endpoint
// collapses the organization panel (role, members, workspaces) and the
// list endpoint optionally carries roles and workspace counts. Default
// shapes stay unchanged; unknown expansions fail closed.

import (
	"context"
	"net/http"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

func expandOrgFixture(t *testing.T, ctx context.Context) (*tenancy.InMemoryStore, *staticUserDirectory, *tenancy.Organization, *tenancy.Workspace) {
	t.Helper()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	org, err := store.CreateOrganization(ctx, 11, "expand-org")
	if err != nil {
		t.Fatal(err)
	}
	dir.ensure(11)
	dir.ensure(12)
	if _, err := store.AddOrganizationMember(ctx, org.PublicID, 12, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	ws, err := store.CreateOrganizationWorkspace(ctx, org.PublicID, 11, "expand-ws")
	if err != nil {
		t.Fatal(err)
	}
	return store, dir, org, ws
}

func TestV1GetOrganizationDefaultShapeUnchanged(t *testing.T) {
	ctx := context.Background()
	store, dir, org, _ := expandOrgFixture(t, ctx)
	engine := newV1TestEngine(store, dir)

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/organizations/" + org.PublicID, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET organization status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	for _, key := range []string{"members", "workspaces", "role", "workspace_count"} {
		if _, present := data[key]; present {
			t.Fatalf("default detail carries %q: %v (expand must stay opt-in)", key, data)
		}
	}
}

func TestV1GetOrganizationExpandCollapsesPanel(t *testing.T) {
	ctx := context.Background()
	store, dir, org, ws := expandOrgFixture(t, ctx)
	engine := newV1TestEngine(store, dir)

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodGet,
		path:   "/api/v1/organizations/" + org.PublicID + "?expand=members,workspaces,role",
		userID: 11,
	})
	if status != http.StatusOK {
		t.Fatalf("GET organization?expand status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["role"] != tenancy.RoleOwner {
		t.Fatalf("role = %v, want owner (single resolution, no extra round trip)", data["role"])
	}
	members, ok := data["members"].([]any)
	if !ok || len(members) != 2 {
		t.Fatalf("members = %v, want 2 expanded members", data["members"])
	}
	first, ok := members[0].(map[string]any)
	if !ok || first["user_id"] == nil {
		t.Fatalf("expanded member = %v, want identity batch", members[0])
	}
	workspaces, ok := data["workspaces"].([]any)
	if !ok || len(workspaces) != 1 {
		t.Fatalf("workspaces = %v, want 1 expanded workspace", data["workspaces"])
	}
	if workspaces[0].(map[string]any)["id"] != ws.PublicID {
		t.Fatalf("expanded workspace = %v, want %s", workspaces[0], ws.PublicID)
	}
}

func TestV1GetOrganizationExpandUnknownFails(t *testing.T) {
	ctx := context.Background()
	store, dir, org, _ := expandOrgFixture(t, ctx)
	engine := newV1TestEngine(store, dir)

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodGet,
		path:   "/api/v1/organizations/" + org.PublicID + "?expand=members,billing",
		userID: 11,
	})
	if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeBadRequest) {
		t.Fatalf("unknown expand = %d %v, want 400 bad_request", status, payload)
	}
}

func TestV1ListOrganizationsExpandRoleAndCounts(t *testing.T) {
	ctx := context.Background()
	store, dir, org, ws := expandOrgFixture(t, ctx)
	engine := newV1TestEngine(store, dir)

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodGet,
		path:   "/api/v1/organizations?expand=role,workspace_counts",
		userID: 11,
	})
	if status != http.StatusOK {
		t.Fatalf("GET organizations?expand status = %d (%v)", status, payload)
	}
	page, ok := v1Data(t, payload).([]any)
	if !ok || len(page) != 1 {
		t.Fatalf("organization page = %v, want 1 row", v1Data(t, payload))
	}
	row, ok := page[0].(map[string]any)
	if !ok {
		t.Fatalf("organization row = %v, want object", page[0])
	}
	if row["id"] != org.PublicID {
		t.Fatalf("row id = %v, want %s", row["id"], org.PublicID)
	}
	if row["role"] != tenancy.RoleOwner {
		t.Fatalf("row role = %v, want owner", row["role"])
	}
	if row["workspace_count"] != float64(1) {
		t.Fatalf("row workspace_count = %v, want 1", row["workspace_count"])
	}
	_ = ws
}

func TestV1ListOrganizationsDefaultShapeUnchanged(t *testing.T) {
	ctx := context.Background()
	store, dir, _, _ := expandOrgFixture(t, ctx)
	engine := newV1TestEngine(store, dir)

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/organizations", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET organizations status = %d (%v)", status, payload)
	}
	page, ok := v1Data(t, payload).([]any)
	if !ok || len(page) != 1 {
		t.Fatalf("organization page = %v, want 1 row", v1Data(t, payload))
	}
	row := page[0].(map[string]any)
	for _, key := range []string{"role", "workspace_count", "members", "workspaces"} {
		if _, present := row[key]; present {
			t.Fatalf("default list row carries %q: %v (expand must stay opt-in)", key, row)
		}
	}
}
