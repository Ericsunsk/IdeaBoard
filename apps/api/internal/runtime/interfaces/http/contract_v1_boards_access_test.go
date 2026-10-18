package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

func sharingRequest(t *testing.T, f *v1BoardsFixture, actor int64, key, version, role string, members []string) (int, http.Header, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"data": map[string]any{"workspace_access": role, "member_user_ids": members}})
	if err != nil {
		t.Fatal(err)
	}
	return doV1(t, f.engine, v1Call{method: http.MethodPut, path: f.boardPath("/access"), userID: actor, body: string(body), headers: map[string]string{contract.IdempotencyKeyHeader: key, contract.IfMatchHeader: version}})
}

func TestV1BoardsAccessResource(t *testing.T) {
	f := newV1BoardsFixture(t)
	viewer := tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))
	status, headers, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath("/access"), userID: f.owner})
	if status != 200 || headers.Get(contract.ETagHeader) != "access-2" {
		t.Fatalf("access=%d %v", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if len(data["member_user_ids"].([]any)) != 2 || data["access_state"] != "workspace" {
		t.Fatalf("access=%v", data)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "role-change", "access-2", "viewer", []string{viewer})
	if status != 200 {
		t.Fatalf("role change=%d %v", status, payload)
	}
	data = v1Data(t, payload).(map[string]any)
	if data["access_version"] != float64(3) {
		t.Fatalf("version=%v", data)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "role-change", "access-2", "viewer", []string{viewer})
	if status != 200 || v1Data(t, payload).(map[string]any)["access_version"] != float64(3) {
		t.Fatalf("replay=%d %v", status, payload)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "stale", "access-2", "editor", []string{viewer})
	if status != 412 {
		t.Fatalf("stale=%d %v", status, payload)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "off", "access-3", "none", []string{viewer})
	if status != 200 {
		t.Fatalf("off=%d %v", status, payload)
	}
	data = v1Data(t, payload).(map[string]any)
	if data["access_state"] != "private" || len(data["member_user_ids"].([]any)) != 0 {
		t.Fatalf("off state=%v", data)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "re-on", "access-4", "editor", []string{})
	if status != 200 || v1Data(t, payload).(map[string]any)["access_state"] != "private" {
		t.Fatalf("empty On=%d %v", status, payload)
	}
	for _, user := range []int64{f.editor, f.viewer, f.wsMember} {
		status, _, _ := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(""), userID: user})
		if status != 404 {
			t.Fatalf("unselected user %d read=%d", user, status)
		}
	}
}

func TestV1BoardsAccessValidationAndCreatorProtection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		actor         func(*v1BoardsFixture) int64
		version, role string
		members       func(*v1BoardsFixture) []string
		want          int
	}{
		{"missing version", func(f *v1BoardsFixture) int64 { return f.owner }, "", "viewer", func(f *v1BoardsFixture) []string { return []string{} }, 428},
		{"unknown role", func(f *v1BoardsFixture) int64 { return f.owner }, "access-2", "owner", func(f *v1BoardsFixture) []string { return []string{} }, 422},
		{"select creator", func(f *v1BoardsFixture) int64 { return f.owner }, "access-2", "viewer", func(f *v1BoardsFixture) []string { return []string{tenancy.FormatUserPublicID(f.dir.ensure(f.owner))} }, 422},
		{"nonmember", func(f *v1BoardsFixture) int64 { return f.owner }, "access-2", "viewer", func(f *v1BoardsFixture) []string {
			return []string{tenancy.FormatUserPublicID(f.dir.ensure(f.outsider))}
		}, 404},
		{"selected editor cannot manage", func(f *v1BoardsFixture) int64 { return f.editor }, "access-2", "viewer", func(f *v1BoardsFixture) []string { return []string{} }, 403},
		{"unselected owner cannot read", func(f *v1BoardsFixture) int64 {
			f.store.SeedWorkspaceMember(f.workspace.PublicID, f.wsMember, tenancy.RoleOwner)
			return f.wsMember
		}, "access-2", "viewer", func(f *v1BoardsFixture) []string { return []string{} }, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newV1BoardsFixture(t)
			status, _, payload := sharingRequest(t, f, tc.actor(f), tc.name, tc.version, tc.role, tc.members(f))
			if status != tc.want {
				t.Fatalf("response=%d %v want %d", status, payload, tc.want)
			}
			access, err := f.boards.GetBoardAccess(t.Context(), f.workspace.PublicID, f.boardID)
			if err != nil || access.AccessVersion != 2 {
				t.Fatalf("rejected write changed policy=%+v %v", access, err)
			}
		})
	}
}

func TestV1BoardsOnlySelectedMembersReadAtCommonRole(t *testing.T) {
	for _, role := range []boards.WorkspaceAccess{boards.WorkspaceAccessViewer, boards.WorkspaceAccessEditor} {
		t.Run(string(role), func(t *testing.T) {
			f := newV1BoardsFixture(t)
			members := []string{tenancy.FormatUserPublicID(f.dir.ensure(f.editor)), tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))}
			if status, _, payload := sharingRequest(t, f, f.owner, "configure", "access-2", string(role), members); status != 200 {
				t.Fatalf("configure=%d %v", status, payload)
			}
			for _, user := range []int64{f.editor, f.viewer} {
				for _, suffix := range []string{"", "/document", "/revisions", "/revisions/1"} {
					status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(suffix), userID: user})
					if status != 200 {
						t.Fatalf("read %d %s=%d %v", user, suffix, status, payload)
					}
				}
				status, payload := boardIDEMKey(t, f.engine, http.MethodPatch, f.boardPath(""), user, "", `{"data":{"name":"Updated"}}`)
				want := 200
				if role == boards.WorkspaceAccessViewer {
					want = 403
				}
				if status != want {
					t.Fatalf("common role rename=%d %v want %d", status, payload, want)
				}
				status, _, _ = doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath("/access"), userID: user})
				if status != 403 {
					t.Fatalf("noncreator access read=%d", status)
				}
			}
			status, _, _ := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(""), userID: f.wsMember})
			if status != 404 {
				t.Fatalf("unselected read=%d", status)
			}
		})
	}
}

func TestV1BoardsObsoleteSharingRoutesAbsent(t *testing.T) {
	f := newV1BoardsFixture(t)
	for _, tc := range []struct{ method, suffix string }{{http.MethodGet, "/members"}, {http.MethodPost, "/members"}, {http.MethodPatch, "/members/usr_fake"}, {http.MethodDelete, "/members/usr_fake"}, {http.MethodPost, "/access/make-only-me"}} {
		req := httptest.NewRequest(tc.method, f.boardPath(tc.suffix), nil)
		req.Header.Set("X-Test-User", strconv.FormatInt(f.owner, 10))
		rec := httptest.NewRecorder()
		f.engine.ServeHTTP(rec, req)
		if rec.Code != 404 {
			t.Fatalf("removed route %s %s=%d", tc.method, tc.suffix, rec.Code)
		}
	}
}
