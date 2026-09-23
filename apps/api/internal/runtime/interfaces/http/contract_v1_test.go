package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// staticUserDirectory is a deterministic V1UserDirectory for tests: user N
// maps to the same UUID testsupport derives, so injected principals and
// member identifiers agree. Only ensured ids resolve; unknown uuids 404.
type staticUserDirectory struct {
	mu     sync.Mutex
	byID   map[int64]uuid.UUID
	byUUID map[uuid.UUID]int64
}

func newStaticUserDirectory() *staticUserDirectory {
	return &staticUserDirectory{byID: map[int64]uuid.UUID{}, byUUID: map[uuid.UUID]int64{}}
}

func testUUIDForUser(userID int64) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("ideaboard-test-user-%d", userID)))
}

func (d *staticUserDirectory) ensure(userID int64) uuid.UUID {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.byID[userID]; ok {
		return id
	}
	id := testUUIDForUser(userID)
	d.byID[userID] = id
	d.byUUID[id] = userID
	return id
}

func (d *staticUserDirectory) ResolveUserID(ctx context.Context, publicID uuid.UUID) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if userID, ok := d.byUUID[publicID]; ok {
		return userID, nil
	}
	return 0, tenancy.ErrNotFound
}

func (d *staticUserDirectory) GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.byID[userID]; ok {
		return id, nil
	}
	return uuid.Nil, tenancy.ErrNotFound
}

func newV1TestEngine(store tenancy.Store, dir *staticUserDirectory) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ContractRequestIDMiddleware())
	// Test principal injector: X-Test-User carries the numeric user id,
	// X-Test-Role the legacy role. Absent header means anonymous (401).
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("X-Test-User"))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader("X-Test-Role"))
		if role == "" {
			role = "user"
		}
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{Store: store, Users: dir})
	return router
}

var v1TestRequestSeq atomic.Int64

type v1Call struct {
	method  string
	path    string
	body    string
	userID  int64
	headers map[string]string
}

func doV1RoundTrip(t *testing.T, engine *gin.Engine, call v1Call, reqIDPrefix string) (int, http.Header, string) {
	t.Helper()
	reader := strings.NewReader(call.body)
	req := httptest.NewRequest(call.method, call.path, reader)
	if call.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	requestID := fmt.Sprintf("%s%d", reqIDPrefix, v1TestRequestSeq.Add(1))
	req.Header.Set(contract.RequestIDHeader, requestID)
	if call.userID > 0 {
		req.Header.Set("X-Test-User", strconv.FormatInt(call.userID, 10))
	}
	for key, value := range call.headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if got := recorder.Header().Get(contract.RequestIDHeader); got != requestID {
		t.Fatalf("%s %s: Request-Id = %q, want %q", call.method, call.path, got, requestID)
	}
	return recorder.Code, recorder.Header(), recorder.Body.String()
}

func doV1(t *testing.T, engine *gin.Engine, call v1Call) (int, http.Header, map[string]any) {
	t.Helper()
	status, header, body := doV1RoundTrip(t, engine, call, "test-req-")
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("%s %s: response is not JSON: %v; body=%s", call.method, call.path, err, body)
	}
	return status, header, payload
}

func v1Data(t *testing.T, payload map[string]any) any {
	t.Helper()
	data, ok := payload["data"]
	if !ok {
		t.Fatalf("success envelope requires data: %v", payload)
	}
	if _, hasError := payload["error"]; hasError {
		t.Fatalf("success envelope must not carry error: %v", payload)
	}
	return data
}

func v1ErrorCode(t *testing.T, payload map[string]any) string {
	t.Helper()
	errBody, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope requires error object: %v", payload)
	}
	code, _ := errBody["code"].(string)
	if code == "" {
		t.Fatalf("error envelope requires code: %v", payload)
	}
	requestID, _ := errBody["request_id"].(string)
	if requestID == "" {
		t.Fatalf("error envelope requires request_id: %v", payload)
	}
	if _, hasData := payload["data"]; hasData {
		t.Fatalf("error envelope must not carry data: %v", payload)
	}
	return code
}

func v1WorkspaceFixture(t *testing.T, ctx context.Context, store *tenancy.InMemoryStore, name string, owner int64) *tenancy.Workspace {
	t.Helper()
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: name, OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestV1MeProvisionsPersonalWorkspace(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/me", userID: 42})
	if status != http.StatusOK {
		t.Fatalf("GET /me status = %d, body=%v", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["id"] != tenancy.FormatUserPublicID(testUUIDForUser(42)) {
		t.Fatalf("me.id = %v", data["id"])
	}
	personal, _ := data["personal_workspace_id"].(string)
	if !strings.HasPrefix(personal, "ws_") {
		t.Fatalf("personal_workspace_id = %v, want ws_...", data["personal_workspace_id"])
	}
	// Second call converges to the same personal workspace.
	_, _, second := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/me", userID: 42})
	if second["data"].(map[string]any)["personal_workspace_id"] != personal {
		t.Fatal("personal workspace not stable across calls")
	}

	// Anonymous callers fail closed with 401 + contract error code.
	status, _, anon := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/me"})
	if status != http.StatusUnauthorized || v1ErrorCode(t, anon) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon /me = %d %v, want 401 unauthorized", status, anon)
	}
}

func TestV1WorkspaceAuthorizationMatrix(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const owner, admin, member, outsider = 1, 2, 3, 4
	ws := v1WorkspaceFixture(t, ctx, store, "matrix", owner)
	store.SeedWorkspaceMember(ws.PublicID, admin, tenancy.RoleAdmin)
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	wsPath := "/api/v1/workspaces/" + ws.PublicID

	t.Run("get", func(t *testing.T) {
		for userID, want := range map[int64]int{owner: 200, admin: 200, member: 200, outsider: 404, 0: 401} {
			status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: wsPath, userID: userID})
			if status != want {
				t.Fatalf("GET user=%d status = %d, want %d (%v)", userID, status, want, payload)
			}
			if want == 404 && v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
				t.Fatalf("outsider code = %v, want workspace_not_found", payload)
			}
		}
	})

	t.Run("patch", func(t *testing.T) {
		cases := []struct {
			userID   int64
			want     int
			wantCode string
		}{
			{owner, 200, ""}, {admin, 200, ""},
			{member, 403, string(contract.CodeWorkspaceForbidden)},
			{outsider, 404, string(contract.CodeWorkspaceNotFound)},
			{0, 401, string(contract.CodeUnauthorized)},
		}
		for _, tc := range cases {
			status, _, payload := doV1(t, engine, v1Call{
				method: http.MethodPatch, path: wsPath, userID: tc.userID,
				body: fmt.Sprintf(`{"data":{"name":"renamed-by-%d"}}`, tc.userID),
			})
			if status != tc.want {
				t.Fatalf("PATCH user=%d status = %d, want %d (%v)", tc.userID, status, tc.want, payload)
			}
			if tc.wantCode != "" && v1ErrorCode(t, payload) != tc.wantCode {
				t.Fatalf("PATCH user=%d code = %v, want %s", tc.userID, payload, tc.wantCode)
			}
		}
	})

	t.Run("archive", func(t *testing.T) {
		archiveTargets := map[int64]int{owner: 200, admin: 200, member: 403, outsider: 404}
		for userID, want := range archiveTargets {
			target := v1WorkspaceFixture(t, ctx, store, fmt.Sprintf("archive-%d", userID), owner)
			store.SeedWorkspaceMember(target.PublicID, admin, tenancy.RoleAdmin)
			store.SeedWorkspaceMember(target.PublicID, member, tenancy.RoleMember)
			status, _, payload := doV1(t, engine, v1Call{
				method: http.MethodPost, path: "/api/v1/workspaces/" + target.PublicID + "/archive", userID: userID,
			})
			if status != want {
				t.Fatalf("archive user=%d status = %d, want %d (%v)", userID, status, want, payload)
			}
		}
	})

	t.Run("malformed id is invisible", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/nope", userID: owner})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("malformed id = %d %v, want 404 workspace_not_found", status, payload)
		}
	})
}

func TestV1MemberAuthorizationAndTransitions(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const owner, admin, member, outsider = 1, 2, 3, 4
	ws := v1WorkspaceFixture(t, ctx, store, "members", owner)
	store.SeedWorkspaceMember(ws.PublicID, admin, tenancy.RoleAdmin)
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	// The static directory only resolves ensured users (production resolves
	// via the users table); ensure every fixture/actor id upfront so member
	// projection never 503s on setup data.
	for _, id := range []int64{owner, admin, member, outsider, 5, 6, 7} {
		dir.ensure(id)
	}
	membersPath := "/api/v1/workspaces/" + ws.PublicID + "/members"
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(dir.ensure(userID)) }

	t.Run("list matrix", func(t *testing.T) {
		for userID, want := range map[int64]int{owner: 200, admin: 200, member: 200, outsider: 404, 0: 401} {
			status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: userID})
			if status != want {
				t.Fatalf("list user=%d status = %d, want %d (%v)", userID, status, want, payload)
			}
			if want == 200 {
				items := v1Data(t, payload).([]any)
				if len(items) != 3 {
					t.Fatalf("members = %d, want 3", len(items))
				}
			}
		}
	})

	t.Run("add matrix", func(t *testing.T) {
		addAs := func(actor, target int64, role string) (int, map[string]any) {
			status, _, payload := doV1(t, engine, v1Call{
				method: http.MethodPost, path: membersPath, userID: actor,
				body: fmt.Sprintf(`{"data":{"user_id":%q,"role":%q}}`, usr(target), role),
			})
			return status, payload
		}
		if status, payload := addAs(owner, 5, "member"); status != 201 {
			t.Fatalf("owner add = %d %v, want 201", status, payload)
		}
		if status, payload := addAs(admin, 6, "admin"); status != 201 {
			t.Fatalf("admin add admin = %d %v, want 201", status, payload)
		}
		if status, payload := addAs(admin, 7, "owner"); status != 403 ||
			v1ErrorCode(t, payload) != string(contract.CodeMembershipTransitionForbidden) {
			t.Fatalf("admin add owner = %d %v, want 403 transition forbidden", status, payload)
		}
		if status, payload := addAs(member, 7, "member"); status != 403 {
			t.Fatalf("member add = %d %v, want 403", status, payload)
		}
		if status, payload := addAs(outsider, 7, "member"); status != 404 {
			t.Fatalf("outsider add = %d %v, want 404", status, payload)
		}
	})

	t.Run("transitions", func(t *testing.T) {
		patchRole := func(actor, target int64, role string) (int, map[string]any) {
			status, _, payload := doV1(t, engine, v1Call{
				method: http.MethodPatch, path: membersPath + "/" + usr(target), userID: actor,
				body: fmt.Sprintf(`{"data":{"role":%q}}`, role),
			})
			return status, payload
		}
		// Owner-only promotion: admin cannot promote.
		if status, payload := patchRole(admin, 5, "admin"); status != 403 ||
			v1ErrorCode(t, payload) != string(contract.CodeMembershipTransitionForbidden) {
			t.Fatalf("admin promote = %d %v, want 403", status, payload)
		}
		if status, _ := patchRole(owner, 5, "admin"); status != 200 {
			t.Fatalf("owner promote = %d, want 200", status)
		}
		// Last owner cannot be demoted or removed.
		if status, payload := patchRole(owner, owner, "admin"); status != 403 ||
			v1ErrorCode(t, payload) != string(contract.CodeMembershipTransitionForbidden) {
			t.Fatalf("demote last owner = %d %v, want 403", status, payload)
		}
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodDelete, path: membersPath + "/" + usr(owner), userID: owner})
		if status != 403 || v1ErrorCode(t, payload) != string(contract.CodeMembershipTransitionForbidden) {
			t.Fatalf("remove last owner = %d %v, want 403", status, payload)
		}
		// Second owner unlocks demotion; member self-leave works.
		if status, _ := patchRole(owner, 5, "owner"); status != 200 {
			t.Fatalf("second owner promote = %d, want 200", status)
		}
		if status, _ := patchRole(owner, 5, "member"); status != 200 {
			t.Fatalf("demote with backup owner = %d, want 200", status)
		}
		status, _, payload = doV1(t, engine, v1Call{method: http.MethodDelete, path: membersPath + "/" + usr(5), userID: 5})
		if status != 200 {
			t.Fatalf("self leave = %d %v, want 200", status, payload)
		}
		// Unknown and malformed member ids are invisible (404).
		unknown := tenancy.FormatUserPublicID(uuid.New())
		status, _, payload = doV1(t, engine, v1Call{method: http.MethodPatch, path: membersPath + "/" + unknown, userID: owner, body: `{"data":{"role":"member"}}`})
		if status != 404 || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceMemberNotFound) {
			t.Fatalf("unknown member = %d %v, want 404 member not found", status, payload)
		}
		status, _, payload = doV1(t, engine, v1Call{method: http.MethodDelete, path: membersPath + "/nope", userID: owner})
		if status != 404 {
			t.Fatalf("malformed member = %d, want 404", status)
		}
	})
}

func TestV1WorkspaceIdempotency(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	post := func(key, name string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: "/api/v1/workspaces", userID: 11, body: fmt.Sprintf(`{"data":{"name":%q}}`, name)}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, engine, call)
		return status, payload
	}

	status, first := post("ws-key-1", "idem")
	if status != 201 {
		t.Fatalf("create = %d %v, want 201", status, first)
	}
	firstID := first["data"].(map[string]any)["id"].(string)

	status, replay := post("ws-key-1", "idem")
	if status != 201 || replay["data"].(map[string]any)["id"] != firstID {
		t.Fatalf("replay = %d %v, want same public_id %s", status, replay, firstID)
	}
	status, conflict := post("ws-key-1", "different")
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeWorkspaceIdempotencyConflict) {
		t.Fatalf("conflict = %d %v, want 409 workspace_idempotency_conflict", status, conflict)
	}
	status, missing := post("", "nokey")
	if status != 400 || v1ErrorCode(t, missing) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, missing)
	}
}

func TestV1MeConcurrentProvision(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const callers = 16
	ids := make([]string, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/me", userID: 77})
			if status != http.StatusOK {
				t.Errorf("GET /me status = %d", status)
				return
			}
			ids[slot], _ = payload["data"].(map[string]any)["personal_workspace_id"].(string)
		}(i)
	}
	wg.Wait()
	for i := 1; i < callers; i++ {
		if ids[i] != ids[0] || ids[0] == "" {
			t.Fatalf("concurrent provision diverged: %q vs %q", ids[i], ids[0])
		}
	}
	visible, err := store.ListWorkspacesForUser(context.Background(), 77)
	if err != nil || len(visible) != 1 {
		t.Fatalf("visible = %v, %v; want single workspace", visible, err)
	}
}

func TestV1WorkspacePagination(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const userID = 50
	dir.ensure(userID)
	for _, name := range []string{"p1", "p2", "p3"} {
		v1WorkspaceFixture(t, ctx, store, name, userID)
	}
	get := func(query string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces" + query, userID: userID})
		return status, payload
	}
	// Provision the personal workspace first. The 3 fixtures are already
	// personal (no organization), so /me converges onto the first one and
	// the list stays at 3: pages of 2 + 1.
	if status, _, _ := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/me", userID: userID}); status != 200 {
		t.Fatalf("GET /me = %d", status)
	}
	status, first := get("?page_size=2")
	if status != 200 {
		t.Fatalf("list = %d %v", status, first)
	}
	meta := first["meta"].(map[string]any)
	if meta["has_more"] != true || meta["next_cursor"] == "" {
		t.Fatalf("meta = %v, want has_more with cursor", meta)
	}
	if len(first["data"].([]any)) != 2 {
		t.Fatalf("page = %d items, want 2", len(first["data"].([]any)))
	}
	cursor := meta["next_cursor"].(string)
	status, second := get("?page_size=2&cursor=" + cursor)
	if status != 200 || len(second["data"].([]any)) != 1 || second["meta"].(map[string]any)["has_more"] != false {
		t.Fatalf("second page = %d %v", status, second)
	}
	status, bad := get("?cursor=!!!")
	if status != 400 || v1ErrorCode(t, bad) != string(contract.CodePaginationCursorInvalid) {
		t.Fatalf("bad cursor = %d %v, want 400 pagination_cursor_invalid", status, bad)
	}
	status, badSize := get("?page_size=101")
	if status != 400 {
		t.Fatalf("bad page_size = %d %v, want 400", status, badSize)
	}
}

func TestV1Organizations(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const owner, member, outsider = 21, 22, 23
	createOrg := func(userID int64, key, name string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: "/api/v1/organizations", userID: userID, body: fmt.Sprintf(`{"data":{"name":%q}}`, name)}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, engine, call)
		return status, payload
	}
	status, created := createOrg(owner, "org-key-1", "acme")
	if status != 201 {
		t.Fatalf("create org = %d %v", status, created)
	}
	orgID := created["data"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(orgID, "org_") {
		t.Fatalf("org id = %q, want org_...", orgID)
	}
	status, replay := createOrg(owner, "org-key-1", "acme")
	if status != 201 || replay["data"].(map[string]any)["id"] != orgID {
		t.Fatalf("org replay = %d %v, want same id", status, replay)
	}
	status, conflict := createOrg(owner, "org-key-1", "other")
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeOrganizationIdempotencyConflict) {
		t.Fatalf("org conflict = %d %v, want 409", status, conflict)
	}
	orgPath := "/api/v1/organizations/" + orgID

	// Seed a plain member for the matrix (store has no org invite endpoint).
	ctx := context.Background()
	org, err := store.GetOrganization(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	_ = org
	store.SeedOrganizationMember(orgID, member, tenancy.RoleMember)

	for userID, want := range map[int64]int{owner: 200, member: 200, outsider: 404, 0: 401} {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: orgPath, userID: userID})
		if status != want {
			t.Fatalf("GET org user=%d = %d, want %d (%v)", userID, status, want, payload)
		}
	}
	patch := func(userID int64) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodPatch, path: orgPath, userID: userID, body: `{"data":{"name":"acme-2"}}`})
		return status, payload
	}
	if status, _ := patch(owner); status != 200 {
		t.Fatalf("owner patch org = %d", status)
	}
	if status, payload := patch(member); status != 403 ||
		v1ErrorCode(t, payload) != string(contract.CodeOrganizationForbidden) {
		t.Fatalf("member patch org = %d %v, want 403", status, payload)
	}

	// Organization workspaces: list visible to members, create owner/admin.
	status, _, listPayload := doV1(t, engine, v1Call{method: http.MethodGet, path: orgPath + "/workspaces", userID: member})
	if status != 200 {
		t.Fatalf("list org workspaces = %d %v", status, listPayload)
	}
	createWS := func(userID int64) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost, path: orgPath + "/workspaces", userID: userID,
			body:    `{"data":{"name":"org-ws"}}`,
			headers: map[string]string{contract.IdempotencyKeyHeader: fmt.Sprintf("org-ws-%d", userID)},
		})
		return status, payload
	}
	if status, payload := createWS(owner); status != 201 {
		t.Fatalf("create org ws = %d %v", status, payload)
	}
	if status, payload := createWS(member); status != 403 {
		t.Fatalf("member create org ws = %d %v, want 403", status, payload)
	}
	if status, _ := createWS(outsider); status != 404 {
		t.Fatalf("outsider create org ws = %d, want 404", status)
	}

	// POST /workspaces with organization_id honors org visibility.
	orgScoped := func(userID int64, orgRef string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost, path: "/api/v1/workspaces", userID: userID,
			body: fmt.Sprintf(`{"data":{"name":"scoped","organization_id":%q}}`, orgRef),
			headers: map[string]string{contract.IdempotencyKeyHeader: fmt.Sprintf("scoped-%d-%s", userID,
				strings.ReplaceAll(orgRef, "_", ""))},
		})
		return status, payload
	}
	if status, payload := orgScoped(owner, orgID); status != 201 ||
		payload["data"].(map[string]any)["organization_id"] != orgID {
		t.Fatalf("scoped create = %d %v, want org echoed", status, payload)
	}
	if status, _ := orgScoped(outsider, orgID); status != 404 {
		t.Fatalf("outsider scoped create = %d, want 404", status)
	}
}

func TestResolvePrincipalTenantScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func(userID int64, workspaceParam string) (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspaceParam, nil)
		c.Params = gin.Params{{Key: "workspace_id", Value: workspaceParam}}
		identityPrincipal, err := principal.New(userID, testUUIDForUser(userID),
			fmt.Sprintf("auth-%d", userID), "Test User", "user")
		if err != nil {
			t.Fatal(err)
		}
		if err := principal.Set(c, identityPrincipal); err != nil {
			t.Fatal(err)
		}
		return c, recorder
	}

	t.Run("valid scope resolved", func(t *testing.T) {
		c, _ := newContext(1, "ws_abc123")
		ResolvePrincipal()(c)
		if c.IsAborted() {
			t.Fatal("valid scope must not abort")
		}
		got, ok := principal.From(c)
		if !ok || got.TenantScope() != "ws_abc123" {
			t.Fatalf("tenant scope = %q, %v", got.TenantScope(), ok)
		}
	})

	t.Run("malformed scope leaves empty without failure", func(t *testing.T) {
		c, _ := newContext(1, "nope")
		ResolvePrincipal()(c)
		if c.IsAborted() {
			t.Fatal("malformed scope must not abort")
		}
		got, ok := principal.From(c)
		if !ok || got.TenantScope() != "" {
			t.Fatalf("tenant scope = %q, want empty", got.TenantScope())
		}
	})

	t.Run("missing principal fails closed", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		ResolvePrincipal()(c)
		if !c.IsAborted() || recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
	})
}

func TestV1NilStoreFailsClosed(t *testing.T) {
	engine := newV1TestEngine(nil, newStaticUserDirectory())
	status, _, me := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/me", userID: 1})
	if status != 200 {
		t.Fatalf("GET /me without store = %d %v, want 200 (no DB needed)", status, me)
	}
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces", userID: 1})
	if status != 501 || v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("list without store = %d %v, want 501", status, payload)
	}
}
