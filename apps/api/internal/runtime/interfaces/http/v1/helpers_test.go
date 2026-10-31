package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	v1testsupport "ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"
	taskdomain "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var testInvitations = v1testsupport.TestInvitations

type fakeTaskQuoter = v1testsupport.FakeTaskQuoter
type fakeTaskQueue = v1testsupport.FakeTaskQueue
type fakeTaskHolder = v1testsupport.FakeTaskHolder
var newFakeTaskHolder = v1testsupport.NewFakeTaskHolder
type v1TxTaskStore = v1testsupport.TxTaskStore
var newV1TxTaskStore = v1testsupport.NewTxTaskStore
var allowTestPoolResolver = v1testsupport.AllowTestPoolResolver
var v1TxSnapshotSaver = v1testsupport.TxSnapshotSaver
type v1FakeBillingAccounts = v1testsupport.FakeBillingAccounts
type v1FakeQuoteWallets = v1testsupport.FakeQuoteWallets
var newV1FakeQuoteWallets = v1testsupport.NewFakeQuoteWallets

type v1TxSubmissionAcceptance struct {
	boards taskports.BoardAuthorizer
	assets taskports.AssetReader
}

func (f *v1TxSubmissionAcceptance) AuthorizeSubmission(ctx context.Context, tx pgx.Tx, input taskdomain.CreateInput) (taskdomain.TaskBoardBinding, error) {
	t := &v1testsupport.TxSubmissionAcceptance{Boards: f.boards, Assets: f.assets}
	return t.AuthorizeSubmission(ctx, tx, input)
}

func newV1MediaTestEngine(store tenancy.Store, dir *staticUserDirectory, deps ContractV1Deps, withGlobalMiddleware ...bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestRole))
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
	if len(withGlobalMiddleware) > 0 && withGlobalMiddleware[0] {
		runtimehttp.RegisterRoutes(router, runtimehttp.Dependencies{
			CORSMiddleware: func(c *gin.Context) { c.Next() },
		}, nil, runtimehttp.FrontendRouteOptions{})
	}
	deps.Store = store
	deps.Users = dir
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, deps)
	return router
}

type v1MediaAccessPGFixture struct {
	engine *gin.Engine
	ws     *tenancy.Workspace
	ws2    *tenancy.Workspace
	owner  int64
	member int64
	boards *boards.PostgresWorkspaceBoardStore
	media  *workspacemedia.Service
}

func newV1MediaAccessPGFixture(t *testing.T) *v1MediaAccessPGFixture {
	t.Helper()
	db := testdb.Open(t)
	ctx := context.Background()
	tstore := tenancypg.NewStore(db, nil)
	dir := newStaticUserDirectory()
	const owner, member, other = 1, 10, 8
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-access", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	v1testsupport.ConfigureWorkspaceDefaultQuota(t, db, ws)
	if _, err := tstore.AddWorkspaceMember(ctx, ws.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	ws2, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-access-2", OwnerUserID: other})
	if err != nil {
		t.Fatal(err)
	}
	media := workspacemedia.NewService(db)
	objects := v1testsupport.NewFakeMediaObjects()
	boardsStore := boards.NewPostgresWorkspaceBoardStore(db, workspacemedia.ValidateDocumentAssets)
	engine := newV1MediaTestEngine(tstore, dir, ContractV1Deps{
		Media: ContractV1MediaDeps{
			Media:         media,
			MediaObjects:  objects,
			MediaRenderer: imgproxy.NewRenderer(v1testsupport.TestImgproxyConfig(t), nil),
		},
		Boards: boardsStore,
	})
	return &v1MediaAccessPGFixture{engine: engine, ws: ws, ws2: ws2, owner: owner, member: member, boards: boardsStore, media: media}
}

func (f *v1MediaAccessPGFixture) uploadsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/uploads"
}

func (f *v1MediaAccessPGFixture) assetsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets"
}

func pgAccessUpload(t *testing.T, f *v1MediaAccessPGFixture, creator int64, boardID string, key string) string {
	t.Helper()
	staged := v1testsupport.GapsPNGBytes(512)
	checksum := v1testsupport.GapsChecksum(staged)
	body := fmt.Sprintf(`{"data":{"filename":"photo.png","mime_type":"image/png","size_bytes":%d,"checksum_sha256":%q,"media_kind":"image"%s}}`,
		len(staged), checksum, func() string {
			if boardID == "" {
				return ""
			}
			return fmt.Sprintf(`,"board_id":%q`, boardID)
		}())
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(),
		body: body, userID: creator, headers: map[string]string{"Idempotency-Key": key + "-create"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	uploadID := v1testsupport.MediaUploadID(t, v1testsupport.Data(t, payload))
	v1testsupport.GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, creator, key+"-stage")
	finish := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":%d,"width":100,"height":80}}`, checksum, len(staged))
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete",
		body: finish, userID: creator, headers: map[string]string{"Idempotency-Key": key + "-complete"}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	return v1testsupport.MediaAssetID(t, v1testsupport.Data(t, payload))
}

// ---- merged from (base helpers_test.go) ----
// staticUserDirectory is a deterministic runtimehttp.V1UserDirectory for tests: user N
// maps to the same UUID testsupport derives, so injected principals and
// member identifiers agree. Only ensured ids resolve; unknown uuids 404.
type staticUserDirectory struct {
	mu                 sync.Mutex
	byID               map[int64]uuid.UUID
	byUUID             map[uuid.UUID]int64
	failPublicIDLookup atomic.Bool
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
	if d.failPublicIDLookup.Load() {
		return uuid.Nil, tenancy.ErrNotFound
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.byID[userID]; ok {
		return id, nil
	}
	return uuid.Nil, tenancy.ErrNotFound
}

func (d *staticUserDirectory) GetUserPublicIDs(ctx context.Context, userIDs []int64) (map[int64]uuid.UUID, error) {
	if d.failPublicIDLookup.Load() {
		return nil, tenancy.ErrNotFound
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[int64]uuid.UUID, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if id, ok := d.byID[userID]; ok {
			out[userID] = id
		}
	}
	return out, nil
}

func newV1TestEngine(store tenancy.Store, dir *staticUserDirectory) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	// Test principal injector: testsupport.HeaderTestUser carries the numeric
	// user id, testsupport.HeaderTestRole the legacy role. Absent header
	// means anonymous (401).
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestRole))
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
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	var boardStore boards.WorkspaceBoardStore
	if membershipScope, ok := store.(boards.WorkspaceMembershipScope); ok {
		boardStore = boards.NewInMemoryWorkspaceBoardStoreForTests(membershipScope, dir)
	}
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{Store: store, Users: dir, Boards: boardStore, Invitations: testInvitations(store, nil, nil, NewInvitationIdentityReader(nil, nil, dir))})
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
		testsupport.SetTestUser(req, call.userID)
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

// Organization authorization fixtures keep every candidate in the Organization;
// Workspace visibility still requires its own explicit grant.
func v1OrganizationWorkspaceFixture(t *testing.T, ctx context.Context, store *tenancy.InMemoryStore, name string, owner int64, candidates ...int64) *tenancy.Workspace {
	t.Helper()
	org, err := store.CreateOrganization(ctx, owner, name)
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range candidates {
		if userID == owner {
			continue
		}
		if _, err := store.AddOrganizationMember(ctx, org.PublicID, userID, tenancy.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: name, OwnerUserID: owner, OrganizationID: &org.PublicID})
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestV1UserProvisionsDefaultWorkspace(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/user", userID: 42})
	if status != http.StatusOK {
		t.Fatalf("GET /user status = %d, body=%v", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["id"] != tenancy.FormatUserPublicID(testUUIDForUser(42)) {
		t.Fatalf("user.id = %v", data["id"])
	}
	personal, _ := data["personal_workspace_id"].(string)
	if !contract.IsWorkspaceID(personal) {
		t.Fatalf("personal_workspace_id = %v, want ws_...", data["personal_workspace_id"])
	}
	// Second call converges to the same personal workspace.
	_, _, second := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/user", userID: 42})
	if second["data"].(map[string]any)["personal_workspace_id"] != personal {
		t.Fatal("personal workspace not stable across calls")
	}

	// Anonymous callers fail closed with 401 + contract error code.
	status, _, anon := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/user"})
	if status != http.StatusUnauthorized || v1ErrorCode(t, anon) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon /user = %d %v, want 401 unauthorized", status, anon)
	}
}

// TestV1LegacyMePathsStayDeleted pins the /me → /user hard cut: the old
// singleton paths must 404 (no dual routes, no compat shims), even for
// authenticated callers.
func TestV1LegacyMePathsStayDeleted(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	engine := newV1TestEngine(store, newStaticUserDirectory())
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/me"},
		{http.MethodPut, "/api/v1/me/avatar"},
	} {
		req := httptest.NewRequest(call.method, call.path, nil)
		testsupport.SetTestUser(req, 1)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404 (legacy /me path must stay deleted)", call.method, call.path, recorder.Code)
		}
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

	t.Run("delete", func(t *testing.T) {
		deleteTargets := map[int64]int{owner: 200, admin: 200, member: 403, outsider: 404}
		for userID, want := range deleteTargets {
			target := v1WorkspaceFixture(t, ctx, store, fmt.Sprintf("delete-%d", userID), owner)
			store.SeedWorkspaceMember(target.PublicID, admin, tenancy.RoleAdmin)
			store.SeedWorkspaceMember(target.PublicID, member, tenancy.RoleMember)
			status, _, payload := doV1(t, engine, v1Call{
				method: http.MethodDelete, path: "/api/v1/workspaces/" + target.PublicID, userID: userID,
				headers: map[string]string{contract.IdempotencyKeyHeader: fmt.Sprintf("matrix-delete-%d", userID)},
			})
			if status != want {
				t.Fatalf("delete user=%d status = %d, want %d (%v)", userID, status, want, payload)
			}
			if want == 403 && v1ErrorCode(t, payload) != string(contract.CodeWorkspaceForbidden) {
				t.Fatalf("member delete code = %v, want workspace_forbidden", payload)
			}
			if want == 404 && v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
				t.Fatalf("outsider delete code = %v, want workspace_not_found", payload)
			}
		}
	})

	t.Run("delete requires idempotency key", func(t *testing.T) {
		target := v1WorkspaceFixture(t, ctx, store, "delete-nokey", owner)
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodDelete, path: "/api/v1/workspaces/" + target.PublicID, userID: owner,
		})
		if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("delete without key = %d %v, want 400 idempotency_key_required", status, payload)
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
	ws := v1OrganizationWorkspaceFixture(t, ctx, store, "members", owner, admin, member, outsider, 5, 6, 7)
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
		addAs := func(actor, target int64, role, key string) (int, map[string]any) {
			call := v1Call{
				method: http.MethodPost, path: membersPath, userID: actor,
				body: fmt.Sprintf(`{"data":{"user_id":%q,"role":%q}}`, usr(target), role),
			}
			if key != "" {
				call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
			}
			status, _, payload := doV1(t, engine, call)
			return status, payload
		}
		if status, payload := addAs(owner, 5, "member", "add-owner-member"); status != 201 {
			t.Fatalf("owner add = %d %v, want 201", status, payload)
		}
		if status, payload := addAs(admin, 6, "admin", "add-admin-admin"); status != 201 {
			t.Fatalf("admin add admin = %d %v, want 201", status, payload)
		}
		if status, payload := addAs(admin, 7, "owner", "add-admin-owner"); status != 403 ||
			v1ErrorCode(t, payload) != string(contract.CodeMembershipTransitionForbidden) {
			t.Fatalf("admin add owner = %d %v, want 403 transition forbidden", status, payload)
		}
		if status, payload := addAs(member, 7, "member", "add-member-member"); status != 403 {
			t.Fatalf("member add = %d %v, want 403", status, payload)
		}
		if status, payload := addAs(outsider, 7, "member", "add-outsider-member"); status != 404 {
			t.Fatalf("outsider add = %d %v, want 404", status, payload)
		}
		// Missing Idempotency-Key fails closed even for authorized adds.
		if status, payload := addAs(owner, 5, "member", ""); status != 400 ||
			v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("add without key = %d %v, want 400 idempotency_key_required", status, payload)
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
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodDelete, path: membersPath + "/" + usr(owner), userID: owner,
			headers: map[string]string{contract.IdempotencyKeyHeader: "remove-last-owner"}})
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
		status, _, payload = doV1(t, engine, v1Call{method: http.MethodDelete, path: membersPath + "/" + usr(5), userID: 5,
			headers: map[string]string{contract.IdempotencyKeyHeader: "self-leave-5"}})
		if status != 200 {
			t.Fatalf("self leave = %d %v, want 200", status, payload)
		}
		// Missing Idempotency-Key fails closed on remove.
		status, _, payload = doV1(t, engine, v1Call{method: http.MethodDelete, path: membersPath + "/" + usr(member), userID: owner})
		if status != 400 || v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("remove without key = %d %v, want 400 idempotency_key_required", status, payload)
		}
		// Unknown and malformed member ids are invisible (404).
		unknown := tenancy.FormatUserPublicID(uuid.New())
		status, _, payload = doV1(t, engine, v1Call{method: http.MethodPatch, path: membersPath + "/" + unknown, userID: owner, body: `{"data":{"role":"member"}}`})
		if status != 404 || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceMemberNotFound) {
			t.Fatalf("unknown member = %d %v, want 404 member not found", status, payload)
		}
		status, _, _ = doV1(t, engine, v1Call{method: http.MethodDelete, path: membersPath + "/nope", userID: owner})
		if status != 404 {
			t.Fatalf("malformed member = %d, want 404", status)
		}
	})
}

func TestV1WorkspaceMemberIdempotency(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const owner int64 = 1
	ws := v1OrganizationWorkspaceFixture(t, ctx, store, "member-idem", owner, 5, 6, 7)
	for _, id := range []int64{owner, 5, 6, 7} {
		dir.ensure(id)
	}
	membersPath := "/api/v1/workspaces/" + ws.PublicID + "/members"
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(dir.ensure(userID)) }

	add := func(key string, target int64, role string) (int, map[string]any) {
		call := v1Call{method: http.MethodPost, path: membersPath, userID: owner,
			body: fmt.Sprintf(`{"data":{"user_id":%q,"role":%q}}`, usr(target), role)}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, engine, call)
		return status, payload
	}
	remove := func(key string, target int64) (int, map[string]any) {
		call := v1Call{method: http.MethodDelete, path: membersPath + "/" + usr(target), userID: owner}
		if key != "" {
			call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
		}
		status, _, payload := doV1(t, engine, call)
		return status, payload
	}

	// Missing keys fail closed on both ops.
	if status, payload := add("", 5, "member"); status != 400 ||
		v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("add without key = %d %v, want 400 idempotency_key_required", status, payload)
	}
	if status, payload := remove("", 5); status != 400 ||
		v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("remove without key = %d %v, want 400 idempotency_key_required", status, payload)
	}

	// Same key+request replays the first add; same user with a
	// different role under the same key conflicts instead of
	// silently returning the old row.
	status, first := add("member-add-1", 5, "member")
	if status != 201 {
		t.Fatalf("add = %d %v, want 201", status, first)
	}
	firstDTO := v1Data(t, first).(map[string]any)
	status, replay := add("member-add-1", 5, "member")
	if status != 201 {
		t.Fatalf("add replay = %d %v, want 201", status, replay)
	}
	replayDTO := v1Data(t, replay).(map[string]any)
	if replayDTO["user_id"] != firstDTO["user_id"] || replayDTO["role"] != "member" {
		t.Fatalf("add replay = %v, want same member %v", replayDTO, firstDTO)
	}
	status, conflict := add("member-add-1", 5, "admin")
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeWorkspaceIdempotencyConflict) {
		t.Fatalf("add divergent = %d %v, want 409 workspace_idempotency_conflict", status, conflict)
	}

	// Remove converges on replay; a different target under the same
	// key conflicts, and the untouched member stays removable.
	if status, _ := add("member-add-6", 6, "member"); status != 201 {
		t.Fatalf("seed 6 = %d, want 201", status)
	}
	if status, _ := add("member-add-7", 7, "member"); status != 201 {
		t.Fatalf("seed 7 = %d, want 201", status)
	}
	if status, payload := remove("member-remove-1", 6); status != 200 {
		t.Fatalf("remove = %d %v, want 200", status, payload)
	}
	dir.failPublicIDLookup.Store(true)
	if status, payload := remove("member-remove-1", 6); status != 200 {
		t.Fatalf("remove replay with public ID lookup unavailable = %d %v, want 200 replayed", status, payload)
	}
	status, conflict = remove("member-remove-1", 7)
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeWorkspaceIdempotencyConflict) {
		t.Fatalf("remove divergent = %d %v, want 409 workspace_idempotency_conflict", status, conflict)
	}
	if status, payload := remove("member-remove-2", 7); status != 200 {
		t.Fatalf("remove untouched = %d %v, want 200", status, payload)
	}
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

func TestV1UserConcurrentDefaultProvision(t *testing.T) {
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
			status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/user", userID: 77})
			if status != http.StatusOK {
				t.Errorf("GET /user status = %d", status)
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
	// Provision the Default workspace first. The 3 fixtures are plain
	// non-Default rows, so /user mints the Default and the list holds 4:
	// pages of 2 + 2.
	if status, _, _ := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/user", userID: userID}); status != 200 {
		t.Fatalf("GET /user = %d", status)
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
	if status != 200 || len(second["data"].([]any)) != 2 || second["meta"].(map[string]any)["has_more"] != false {
		t.Fatalf("second page = %d %v", status, second)
	}
	defaults := 0
	for _, page := range []map[string]any{first, second} {
		for _, item := range page["data"].([]any) {
			if item.(map[string]any)["is_default"] == true {
				defaults++
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("default rows across pages = %d, want exactly 1", defaults)
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

// TestV1CreateMultipleNonDefaultWorkspaces proves plain POST creation
// without an organization is unlimited and never Default.
func TestV1CreateMultipleNonDefaultWorkspaces(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const userID int64 = 61
	post := func(key, name string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost, path: "/api/v1/workspaces", userID: userID,
			body:    fmt.Sprintf(`{"data":{"name":%q}}`, name),
			headers: map[string]string{contract.IdempotencyKeyHeader: key},
		})
		return status, payload
	}
	ids := map[string]struct{}{}
	for i, name := range []string{"plain-a", "plain-b", "plain-c"} {
		status, payload := post(fmt.Sprintf("plain-%d", i), name)
		if status != 201 {
			t.Fatalf("create %q = %d %v, want 201", name, status, payload)
		}
		data := v1Data(t, payload).(map[string]any)
		if data["is_default"] == true {
			t.Fatalf("POST create %q must not be Default: %v", name, data)
		}
		id, _ := data["id"].(string)
		ids[id] = struct{}{}
	}
	if len(ids) != 3 {
		t.Fatalf("created ids = %v, want 3 distinct rows", ids)
	}
}

// TestV1DeleteDefaultWorkspaceProtected proves the Default workspace
// cannot be hard-deleted (403 workspace_default_protected) while plain
// rows delete immediately and irreversibly.
func TestV1DeleteDefaultWorkspaceProtected(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const userID int64 = 62
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/user", userID: userID})
	if status != 200 {
		t.Fatalf("GET /user = %d %v, want 200", status, payload)
	}
	defID, _ := v1Data(t, payload).(map[string]any)["personal_workspace_id"].(string)
	if defID == "" {
		t.Fatal("personal_workspace_id missing after provision")
	}
	status, _, deleted := doV1(t, engine, v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + defID, userID: userID,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-default-1"}})
	if status != http.StatusForbidden || v1ErrorCode(t, deleted) != string(contract.CodeWorkspaceDefaultProtected) {
		t.Fatalf("delete default = %d %v, want 403 workspace_default_protected", status, deleted)
	}

	plain := v1WorkspaceFixture(t, context.Background(), store, "plain", userID)
	status, _, _ = doV1(t, engine, v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + plain.PublicID, userID: userID,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-plain-1"}})
	if status != 200 {
		t.Fatalf("delete plain = %d, want 200", status)
	}
	// Deleted workspaces read as 404 (irreversible).
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + plain.PublicID, userID: userID})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("get deleted = %d %v, want 404 workspace_not_found", status, payload)
	}
}

// TestV1DeleteWorkspaceIdempotency proves key reuse with the same request
// replays the first result while reuse with a different request renders
// 409 workspace_idempotency_conflict.
func TestV1DeleteWorkspaceIdempotency(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const userID int64 = 66
	first := v1WorkspaceFixture(t, context.Background(), store, "idem-a", userID)
	status, _, _ := doV1(t, engine, v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + first.PublicID, userID: userID,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-idem-1"}})
	if status != 200 {
		t.Fatalf("first delete = %d, want 200", status)
	}
	// The workspace and its membership are physically gone, but the same
	// authenticated request must still replay the successful response.
	status, _, replay := doV1(t, engine, v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + first.PublicID, userID: userID,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-idem-1"}})
	if status != http.StatusOK {
		t.Fatalf("replayed delete = %d %v, want 200", status, replay)
	}
	data := v1Data(t, replay).(map[string]any)
	if data["deleted"] != true || data["id"] != first.PublicID {
		t.Fatalf("replayed delete data = %v, want original deleted workspace", data)
	}
	for _, caller := range []int64{0, userID + 1} {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + first.PublicID, userID: caller,
			headers: map[string]string{contract.IdempotencyKeyHeader: "delete-idem-1"}})
		want := http.StatusNotFound
		if caller == 0 {
			want = http.StatusUnauthorized
		}
		if status != want {
			t.Fatalf("replay caller=%d = %d %v, want %d", caller, status, payload, want)
		}
	}
	// A new key has no successful deletion to replay.
	status, _, missing := doV1(t, engine, v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + first.PublicID, userID: userID,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-idem-new"}})
	if status != http.StatusNotFound || v1ErrorCode(t, missing) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("new key on deleted workspace = %d %v, want 404 workspace_not_found", status, missing)
	}
	// Same key on a different workspace is a different request hash: 409.
	second := v1WorkspaceFixture(t, context.Background(), store, "idem-b", userID)
	status, _, conflict := doV1(t, engine, v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + second.PublicID, userID: userID,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-idem-1"}})
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeWorkspaceIdempotencyConflict) {
		t.Fatalf("conflicting delete = %d %v, want 409 workspace_idempotency_conflict", status, conflict)
	}
}

// workspaceDeleteFailureStore exercises tenancy failures at the HTTP boundary;
// PostgreSQL tests separately cover the transactional task/hold invariant.
type workspaceDeleteFailureStore struct {
	tenancy.Store
	deleteErr error
}

func (s *workspaceDeleteFailureStore) DeleteWorkspace(ctx context.Context, publicID string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.Store.DeleteWorkspace(ctx, publicID)
}

func TestV1DeleteWorkspaceActiveTasksConflictDoesNotClaimKey(t *testing.T) {
	base := tenancy.NewInMemoryStoreForTests()
	store := &workspaceDeleteFailureStore{Store: base, deleteErr: tenancy.ErrWorkspaceActiveTasks}
	engine := newV1TestEngine(store, newStaticUserDirectory())
	const owner int64 = 69
	workspace := v1WorkspaceFixture(t, context.Background(), base, "delete-after-tasks", owner)
	call := v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + workspace.PublicID, userID: owner,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-after-tasks"}}
	status, _, payload := doV1(t, engine, call)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceActiveTasks) {
		t.Fatalf("active task delete = %d %v, want 409 workspace_active_tasks", status, payload)
	}
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: call.path, userID: owner})
	if status != http.StatusOK {
		t.Fatalf("workspace after conflict = %d %v, want 200", status, payload)
	}
	store.deleteErr = nil
	for i := 0; i < 2; i++ {
		status, _, payload = doV1(t, engine, call)
		if status != http.StatusOK {
			t.Fatalf("settled delete attempt %d = %d %v, want 200", i, status, payload)
		}
	}
}

func TestV1DeleteWorkspaceDatabaseFailureDoesNotClaimKey(t *testing.T) {
	base := tenancy.NewInMemoryStoreForTests()
	store := &workspaceDeleteFailureStore{Store: base, deleteErr: errors.New("database permission denied")}
	engine := newV1TestEngine(store, newStaticUserDirectory())
	const owner int64 = 70
	workspace := v1WorkspaceFixture(t, context.Background(), base, "delete-after-database-recovers", owner)
	call := v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + workspace.PublicID, userID: owner,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-after-database-recovers"}}
	status, _, payload := doV1(t, engine, call)
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
		t.Fatalf("database failure delete = %d %v, want 503 service_unavailable", status, payload)
	}
	if _, err := base.GetWorkspace(context.Background(), workspace.PublicID); err != nil {
		t.Fatalf("workspace after database failure: %v", err)
	}
	store.deleteErr = nil
	for attempt := 0; attempt < 2; attempt++ {
		status, _, payload = doV1(t, engine, call)
		if status != http.StatusOK {
			t.Fatalf("recovered delete attempt %d = %d %v, want 200", attempt, status, payload)
		}
	}
}

func TestV1DeleteWorkspaceDoesNotCachePermissionFailure(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	engine := newV1TestEngine(store, newStaticUserDirectory())
	const owner, member int64 = 67, 68
	workspace := v1WorkspaceFixture(t, context.Background(), store, "delete-after-promotion", owner)
	store.SeedWorkspaceMember(workspace.PublicID, member, tenancy.RoleMember)
	call := v1Call{method: http.MethodDelete, path: "/api/v1/workspaces/" + workspace.PublicID, userID: member,
		headers: map[string]string{contract.IdempotencyKeyHeader: "delete-after-promotion"}}
	status, _, payload := doV1(t, engine, call)
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceForbidden) {
		t.Fatalf("member delete = %d %v, want 403 workspace_forbidden", status, payload)
	}
	store.SeedWorkspaceMember(workspace.PublicID, member, tenancy.RoleAdmin)
	status, _, payload = doV1(t, engine, call)
	if status != http.StatusOK {
		t.Fatalf("promoted admin delete = %d %v, want 200", status, payload)
	}
}

// TestV1RenameDefaultWorkspaceProtected proves the Default workspace
// cannot be renamed (403 workspace_default_protected) while plain rows
// rename normally.
func TestV1RenameDefaultWorkspaceProtected(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const userID int64 = 65
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/user", userID: userID})
	if status != 200 {
		t.Fatalf("GET /user = %d %v, want 200", status, payload)
	}
	defID, _ := v1Data(t, payload).(map[string]any)["personal_workspace_id"].(string)
	if defID == "" {
		t.Fatal("personal_workspace_id missing after provision")
	}
	status, _, renamed := doV1(t, engine, v1Call{method: http.MethodPatch, path: "/api/v1/workspaces/" + defID, userID: userID, body: `{"data":{"name":"Renamed Default"}}`})
	if status != http.StatusForbidden || v1ErrorCode(t, renamed) != string(contract.CodeWorkspaceDefaultProtected) {
		t.Fatalf("rename default = %d %v, want 403 workspace_default_protected", status, renamed)
	}

	plain := v1WorkspaceFixture(t, context.Background(), store, "plain", userID)
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodPatch, path: "/api/v1/workspaces/" + plain.PublicID, userID: userID, body: `{"data":{"name":"Renamed Plain"}}`})
	if status != 200 {
		t.Fatalf("rename plain = %d %v, want 200", status, payload)
	}
	if v1Data(t, payload).(map[string]any)["name"] != "Renamed Plain" {
		t.Fatalf("renamed plain name = %v, want Renamed Plain", payload)
	}
}

// TestV1ListBackfillsMissingDefault proves first-list auto-provisioning is
// keyed on the missing Default, not on an empty list: a user holding only
// plain rows still gets a Default row.
func TestV1ListBackfillsMissingDefault(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const userID int64 = 63
	v1WorkspaceFixture(t, ctx, store, "plain-a", userID)
	v1WorkspaceFixture(t, ctx, store, "plain-b", userID)

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces?page_size=50", userID: userID})
	if status != 200 {
		t.Fatalf("list = %d %v, want 200", status, payload)
	}
	items := v1Data(t, payload).([]any)
	if len(items) != 3 {
		t.Fatalf("list items = %d, want 2 plain + backfilled Default", len(items))
	}
	defaults := 0
	for _, item := range items {
		if item.(map[string]any)["is_default"] == true {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("default rows = %d, want exactly 1", defaults)
	}
}

// TestV1CreateBoardBackfillsDefault proves board creation mints a missing
// Default workspace as a side effect while the board itself lands in the
// requested workspace.
func TestV1CreateBoardBackfillsDefault(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const userID int64 = 64
	ws := v1WorkspaceFixture(t, ctx, store, "plain", userID)
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + ws.PublicID + "/boards", userID: userID,
		body:    `{"data":{"name":"Atlas"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "board-backfill-1"},
	})
	if status != 201 {
		t.Fatalf("create board = %d %v, want 201", status, payload)
	}
	visible, err := store.ListWorkspacesForUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	defaults := 0
	for _, w := range visible {
		if w.IsDefaultWorkspace() {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("default rows after board create = %d, want exactly 1 (%v)", defaults, visible)
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

func TestV1OrganizationMembers(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	engine := newV1TestEngine(store, dir)

	const owner, admin, member, outsider = 31, 32, 33, 34
	for _, userID := range []int64{owner, admin, member, outsider} {
		dir.ensure(userID)
	}
	usr := func(userID int64) string {
		return tenancy.FormatUserPublicID(testUUIDForUser(userID))
	}
	key := func(parts ...string) map[string]string {
		return map[string]string{contract.IdempotencyKeyHeader: strings.Join(parts, "-")}
	}

	status, _, created := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/organizations", userID: owner,
		body: `{"data":{"name":"acme"}}`, headers: key("org", "create")})
	if status != 201 {
		t.Fatalf("create org = %d %v", status, created)
	}
	orgID := created["data"].(map[string]any)["id"].(string)
	memberPath := "/api/v1/organizations/" + orgID + "/members"

	// At most one organization: creating a second one fails closed.
	status, _, second := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/organizations", userID: owner,
		body: `{"data":{"name":"second"}}`, headers: key("org", "create", "2")})
	if status != 409 || v1ErrorCode(t, second) != string(contract.CodeOrganizationLimitReached) {
		t.Fatalf("second org = %d %v, want 409 organization_limit_reached", status, second)
	}

	// Organization membership can no longer bypass recipient confirmation.
	status, _, _ = doV1RoundTrip(t, engine, v1Call{method: http.MethodPost, path: memberPath, userID: owner,
		body: fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, usr(outsider)), headers: key("retired-add")}, "retired-")
	if status != http.StatusNotFound {
		t.Fatalf("direct Organization add = %d, want 404", status)
	}
	if _, err := store.AddOrganizationMember(ctx, orgID, admin, tenancy.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddOrganizationMember(ctx, orgID, member, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}

	// Only owner/admin create organization workspaces on the shared pool.
	wsPath := "/api/v1/organizations/" + orgID + "/workspaces"
	status, _, wsPayload := doV1(t, engine, v1Call{method: http.MethodPost, path: wsPath, userID: owner,
		body: `{"data":{"name":"org-ws"}}`, headers: key("org", "ws", "1")})
	if status != 201 {
		t.Fatalf("owner creates org workspace = %d %v", status, wsPayload)
	}
	wsID := wsPayload["data"].(map[string]any)["id"].(string)
	wsDetail := "/api/v1/workspaces/" + wsID
	if status, _, _ := doV1(t, engine, v1Call{method: http.MethodPost, path: wsPath, userID: member,
		body: `{"data":{"name":"nope"}}`, headers: key("org", "ws", "2")}); status != 403 {
		t.Fatalf("member creates org workspace = %d, want 403", status)
	}
	if status, _, _ := doV1(t, engine, v1Call{method: http.MethodPost, path: wsPath, userID: outsider,
		body: `{"data":{"name":"nope"}}`, headers: key("org", "ws", "3")}); status != 404 {
		t.Fatalf("outsider creates org workspace = %d, want 404", status)
	}

	getWS := func(userID int64) int {
		status, _, _ := doV1(t, engine, v1Call{method: http.MethodGet, path: wsDetail, userID: userID})
		return status
	}
	// Owner/admin hold automatic access; members need an explicit grant;
	// non-members stay hidden until they join.
	if got := getWS(owner); got != 200 {
		t.Fatalf("owner reads org workspace = %d, want 200", got)
	}
	if got := getWS(admin); got != 200 {
		t.Fatalf("admin reads org workspace = %d, want 200", got)
	}
	if got := getWS(member); got != 404 {
		t.Fatalf("member reads org workspace = %d, want 404 before grant", got)
	}
	if got := getWS(outsider); got != 404 {
		t.Fatalf("outsider reads org workspace = %d, want 404", got)
	}
	patchWS := func(userID int64) int {
		status, _, _ := doV1(t, engine, v1Call{method: http.MethodPatch, path: wsDetail, userID: userID,
			body: `{"data":{"name":"renamed"}}`})
		return status
	}
	if got := patchWS(owner); got != 200 {
		t.Fatalf("owner renames org workspace = %d, want 200", got)
	}
	if got := patchWS(admin); got != 200 {
		t.Fatalf("admin renames org workspace = %d, want 200", got)
	}

	// Owner/admin authorize member workspace access; the grant can read but
	// workspace rename stays owner/admin-only server-side.
	grant := func(actor, target int64, role string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodPost, path: wsDetail + "/members", userID: actor,
			body:    fmt.Sprintf(`{"data":{"user_id":%q,"role":%q}}`, usr(target), role),
			headers: key("ws", "grant", fmt.Sprint(actor), fmt.Sprint(target))})
		return status, payload
	}
	if status, payload := grant(owner, member, tenancy.RoleMember); status != 201 {
		t.Fatalf("owner grants member workspace = %d %v, want 201", status, payload)
	}
	if got := getWS(member); got != 200 {
		t.Fatalf("member reads org workspace = %d, want 200 after grant", got)
	}
	if got := patchWS(member); got != 403 {
		t.Fatalf("member renames org workspace = %d, want 403", got)
	}
	// Grants require organization membership first (join-first fail closed).
	if status, _ := grant(owner, outsider, tenancy.RoleMember); status != 404 {
		t.Fatalf("grant to non-org-member = %d, want 404", status)
	}

	// Only owners change admin roles. Demotion drops automatic all-workspace
	// access and keeps only explicit grants (none here, so hidden).
	patchMember := func(actor, target int64, role string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodPatch,
			path: memberPath + "/" + usr(target), userID: actor,
			body: fmt.Sprintf(`{"data":{"role":%q}}`, role)})
		return status, payload
	}
	if status, _ := patchMember(admin, member, tenancy.RoleAdmin); status != 403 {
		t.Fatalf("admin promotes member = %d, want 403", status)
	}
	if status, _ := patchMember(owner, admin, tenancy.RoleMember); status != 200 {
		t.Fatalf("owner demotes admin = %d, want 200", status)
	}
	if got := getWS(admin); got != 404 {
		t.Fatalf("demoted admin reads org workspace = %d, want 404", got)
	}

	// Leaving removes every workspace grant at once; content stays
	// organization-owned and visible to remaining members.
	remove := func(actor, target int64) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodDelete,
			path: memberPath + "/" + usr(target), userID: actor,
			headers: key("org", "remove", fmt.Sprint(actor), fmt.Sprint(target))})
		return status, payload
	}
	if status, _ := remove(member, member); status != 200 {
		t.Fatalf("member self-leave = %d, want 200", status)
	}
	if got := getWS(member); got != 404 {
		t.Fatalf("left member reads org workspace = %d, want 404", got)
	}
	if got := getWS(owner); got != 200 {
		t.Fatalf("owner reads org workspace after leave = %d, want 200 (content retained)", got)
	}
	// Owner rows are immutable outside explicit handover.
	if status, payload := remove(owner, owner); status != 403 ||
		v1ErrorCode(t, payload) != string(contract.CodeMembershipTransitionForbidden) {
		t.Fatalf("owner self-leave = %d %v, want 403 membership_transition_forbidden", status, payload)
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
		runtimehttp.ResolvePrincipal()(c)
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
		runtimehttp.ResolvePrincipal()(c)
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
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user", nil)
		runtimehttp.ResolvePrincipal()(c)
		if !c.IsAborted() || recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
	})
}

func TestV1NilStoreFailsClosed(t *testing.T) {
	engine := newV1TestEngine(nil, newStaticUserDirectory())
	status, _, me := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/user", userID: 1})
	if status != 200 {
		t.Fatalf("GET /user without store = %d %v, want 200 (no DB needed)", status, me)
	}
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces", userID: 1})
	if status != 501 || v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("list without store = %d %v, want 501", status, payload)
	}
}

// ---- merged from idempotency_test.go ----
func TestRequireV1IdempotencyKey(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantKey    string
		wantOK     bool
		wantStatus int
	}{
		{name: "missing", wantStatus: http.StatusBadRequest},
		{name: "blank", header: "  \t", wantStatus: http.StatusBadRequest},
		{name: "overlong", header: strings.Repeat("k", contract.IdempotencyMaxKeyLength+1), wantStatus: http.StatusBadRequest},
		{name: "trimmed valid key", header: "  board-create-1  ", wantKey: "board-create-1", wantOK: true, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
			if tt.header != "" {
				c.Request.Header.Set(contract.IdempotencyKeyHeader, tt.header)
			}

			gotKey, gotOK := shared.RequireIdempotencyKey(c)
			if gotKey != tt.wantKey || gotOK != tt.wantOK {
				t.Fatalf("shared.RequireIdempotencyKey() = (%q, %t), want (%q, %t)", gotKey, gotOK, tt.wantKey, tt.wantOK)
			}
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if !tt.wantOK {
				var payload contract.ErrorEnvelope
				if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
					t.Fatalf("decode error envelope: %v; body=%s", err, recorder.Body.String())
				}
				if payload.Error.Code != string(contract.CodeIdempotencyKeyRequired) {
					t.Fatalf("error code = %q, want %q", payload.Error.Code, contract.CodeIdempotencyKeyRequired)
				}
				if !c.IsAborted() {
					t.Fatal("invalid key must abort the request")
				}
			}
		})
	}
}
