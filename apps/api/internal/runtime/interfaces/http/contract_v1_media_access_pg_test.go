package httpapi

// Media asset authorization over real PostgreSQL + HTTP (issue #936,
// criterion 7).
//
// End to end on PG-backed tenancy/boards/media: same-tenant unauthorized
// members, foreign workspace ids, the bound-upload pre-save window,
// prewarmed-cache/304 behavior, batch, multiple sources, and reference
// GC. Binary error contracts stay plain-status throughout.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
	"ideaboard/api/internal/testdb"
)

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
	tstore := tenancypg.NewStore(db)
	dir := newStaticUserDirectory()
	const owner, member, other = 1, 10, 8
	for _, id := range []int64{owner, member, other} {
		dir.ensure(id)
	}
	ws, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-access", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tstore.AddWorkspaceMember(ctx, ws.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	ws2, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "media-access-2", OwnerUserID: other})
	if err != nil {
		t.Fatal(err)
	}
	media := workspacemedia.NewService(db)
	objects := &fakeV1MediaObjects{objects: make(map[string][]byte)}
	boardsStore := boards.NewPostgresWorkspaceBoardStore(db, workspacemedia.ValidateDocumentAssets)
	engine := newV1MediaTestEngine(tstore, dir, ContractV1Deps{
		Media: media, MediaObjects: objects,
		MediaRenderer: imgproxy.NewRenderer(v1TestImgproxyConfig(t), nil),
		Boards:        boardsStore,
	})
	return &v1MediaAccessPGFixture{engine: engine, ws: ws, ws2: ws2, owner: owner, member: member, boards: boardsStore, media: media}
}

func (f *v1MediaAccessPGFixture) uploadsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/uploads"
}

func (f *v1MediaAccessPGFixture) assetsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/media-assets"
}

// pgAccessUpload declares (optionally board-bound), stages real bytes, and
// completes one asset, returning its id. Board binding requires the
// creator's write access and is verified by the handler, not the client.
func pgAccessUpload(t *testing.T, f *v1MediaAccessPGFixture, creator int64, boardID string, key string) string {
	t.Helper()
	staged := v1GapsPNGBytes(512)
	checksum := v1GapsChecksum(staged)
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
	uploadID := v1MediaUploadID(t, v1Data(t, payload))
	v1GapsStage(t, f.engine, f.uploadsPath()+"/"+uploadID+"/bytes", staged, creator, key+"-stage")
	finish := fmt.Sprintf(`{"data":{"checksum_sha256":%q,"size_bytes":%d,"width":100,"height":80}}`, checksum, len(staged))
	status, _, payload = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath() + "/" + uploadID + "/complete",
		body: finish, userID: creator, headers: map[string]string{"Idempotency-Key": key + "-complete"}})
	if status != http.StatusOK {
		t.Fatalf("complete = %d %v, want 200", status, payload)
	}
	return v1MediaAssetID(t, v1Data(t, payload).(map[string]any)["asset"])
}

func pgAccessBoard(t *testing.T, f *v1MediaAccessPGFixture, name string) string {
	t.Helper()
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.boards.CreateBoard(context.Background(), f.ws.PublicID, f.owner, name, boardID); err != nil {
		t.Fatalf("create board: %v", err)
	}
	return boardID
}

func pgAccessDoc(t *testing.T, f *v1MediaAccessPGFixture, boardID string, revision int64, assetIDs ...string) {
	t.Helper()
	nodes := make([]string, 0, len(assetIDs))
	for i, id := range assetIDs {
		nodes = append(nodes, fmt.Sprintf(`{"id":%d,"x":0,"y":0,"width":280,"sourceImageUrl":"https://x/y.png","sourceMediaRef":{"assetId":%q},"outputImages":[]}`, i+1, id))
	}
	envelope := fmt.Sprintf(`{"schema_version":1,"payload":{"schemaVersion":4,"imageNodes":[%s],"promptNodes":[],"fusionNodes":[]}}`, strings.Join(nodes, ","))
	if _, err := f.boards.ReplaceDocument(context.Background(), f.ws.PublicID, boardID, f.owner, revision, 1, []byte(envelope)); err != nil {
		t.Fatalf("replace document: %v", err)
	}
}

// Same-tenant unauthorized members read 404 on every surface; lists hide
// the asset; foreign workspace ids converge to 404 too.
func TestV1MediaAccessPGDeniesUnauthorizedMember(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	assetID := pgAccessUpload(t, f, f.owner, "", "deny")

	for name, call := range map[string]v1Call{
		"metadata": {method: "GET", path: f.assetsPath() + "/" + assetID, userID: f.member},
		"batch": {method: "POST", path: f.assetsPath() + "/batch",
			body:   fmt.Sprintf(`{"data":{"asset_ids":[%q]}}`, assetID),
			userID: f.member},
	} {
		status, _, payload := doV1(t, f.engine, call)
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeMediaNotFound) {
			t.Fatalf("%s = %d %v, want 404 media_not_found", name, status, payload)
		}
	}
	for _, path := range []string{
		f.assetsPath() + "/" + assetID + "/content",
		f.assetsPath() + "/" + assetID + "/derivatives/thumb",
	} {
		status, _, _ := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: path, userID: f.member}, "pg-acc-")
		if status != http.StatusNotFound {
			t.Fatalf("binary %s = %d, want plain 404", path, status)
		}
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath(), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("list = %d %v, want 200", status, payload)
	}
	for _, item := range v1Data(t, payload).([]any) {
		if item.(map[string]any)["id"] == assetID {
			t.Fatalf("list leaked unauthorized asset %s", assetID)
		}
	}
	foreign := "/api/v1/workspaces/" + f.ws2.PublicID + "/media-assets/" + assetID + "/content"
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: foreign, userID: f.owner}, "pg-deny-")
	if status != http.StatusNotFound {
		t.Fatalf("foreign workspace content = %d, want plain 404", status)
	}
}

// The bound-upload pre-save window: a board reader reads the bytes before
// any document accepts the asset; unbound uploads stay owner-only even
// though the board is not autosaved/accepted yet.
func TestV1MediaAccessPGTargetWindow(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	ctx := context.Background()
	boardID := pgAccessBoard(t, f, "target")
	assetID := pgAccessUpload(t, f, f.owner, boardID, "window-bound")
	if _, err := f.boards.ReplaceBoardAccess(ctx, f.ws.PublicID, boardID, f.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{f.member}); err != nil {
		t.Fatal(err)
	}
	status, _, _ := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.member}, "pg-window-")
	if status != http.StatusOK {
		t.Fatalf("target-reader content = %d, want 200 before any document accept", status)
	}
	plain := pgAccessUpload(t, f, f.owner, "", "window-plain")
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + plain + "/content", userID: f.member}, "pg-window-")
	if status != http.StatusNotFound {
		t.Fatalf("unbound content = %d, want 404 (no workspace-readable window)", status)
	}
	// Served DTOs carry same-origin API URLs only: the internal imgproxy
	// signed URL never appears on the wire as a public bypass.
	status, _, payload := doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID, userID: f.owner})
	if status != http.StatusOK {
		t.Fatalf("owner metadata = %d %v, want 200", status, payload)
	}
	dto := v1Data(t, payload).(map[string]any)
	contentURL, _ := dto["content_url"].(string)
	if !strings.HasPrefix(contentURL, "/api/v1/workspaces/") || strings.Contains(contentURL, "?") {
		t.Fatalf("content_url = %q, want same-origin API path without signature", contentURL)
	}
	derivatives, _ := dto["derivatives"].(map[string]any)
	for tier, raw := range derivatives {
		entry, _ := raw.(map[string]any)
		u, _ := entry["url"].(string)
		if entry["status"] == "ready" && (!strings.HasPrefix(u, "/api/v1/workspaces/") || strings.Contains(u, "?")) {
			t.Fatalf("derivative %s url = %q, want same-origin API path", tier, u)
		}
	}
	// Forged bindings fail closed at create (404, never a grant).
	staged := v1GapsPNGBytes(512)
	body := fmt.Sprintf(`{"data":{"filename":"evil.png","mime_type":"image/png","size_bytes":%d,"checksum_sha256":%q,"media_kind":"image","board_id":%q}}`,
		len(staged), v1GapsChecksum(staged), "board_forged000000001")
	status, _, _ = doV1(t, f.engine, v1Call{method: "POST", path: f.uploadsPath(), body: body,
		userID: f.member, headers: map[string]string{"Idempotency-Key": "window-forge"}})
	if status != http.StatusNotFound {
		t.Fatalf("forged binding create = %d, want 404", status)
	}
}

// Authorization precedes cache behavior: an unauthorized caller with a
// stolen ETag reads 404 (never 304), and prewarming as the owner never
// makes the derivative serve the member.
func TestV1MediaAccessPGAuthBeforeCache(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	ctx := context.Background()
	boardID := pgAccessBoard(t, f, "cache")
	assetID := pgAccessUpload(t, f, f.owner, boardID, "cache")
	if _, err := f.boards.ReplaceBoardAccess(ctx, f.ws.PublicID, boardID, f.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{f.member}); err != nil {
		t.Fatal(err)
	}
	// Prewarm as the authorized reader: content (captures ETag) + derivative.
	status, headers, _ := doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/content", userID: f.member}, "pg-cache-")
	if status != http.StatusOK {
		t.Fatalf("prewarm content = %d, want 200", status)
	}
	etag := headers.Get("ETag")
	if etag == "" {
		t.Fatal("prewarm content carried no ETag")
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/thumb", userID: f.member}, "pg-cache-")
	if status != http.StatusOK {
		t.Fatalf("prewarm derivative = %d, want 200", status)
	}
	// Lose the grant: the same URLs now 404 — no cross-identity cache reuse.
	if err := func() error {
		_, err := f.boards.ReplaceBoardAccess(ctx, f.ws.PublicID, boardID, f.owner, 2, boards.WorkspaceAccessNone, nil)
		return err
	}(); err != nil {
		t.Fatal(err)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{
		method: "GET", path: f.assetsPath() + "/" + assetID + "/content",
		userID: f.member, headers: map[string]string{"If-None-Match": etag},
	}, "pg-cache-")
	if status != http.StatusNotFound {
		t.Fatalf("stolen-ETag content = %d, want 404 (auth before 304)", status)
	}
	status, _, _ = doV1RoundTrip(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID + "/derivatives/thumb", userID: f.member}, "pg-cache-")
	if status != http.StatusNotFound {
		t.Fatalf("prewarmed derivative = %d, want 404 (no shared-cache reuse)", status)
	}
}

// Document lifecycle over HTTP: accept grants, removal revokes (GC), the
// owner source survives, and batch follows the same rule per id.
func TestV1MediaAccessPGDocumentLifecycle(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	ctx := context.Background()
	boardID := pgAccessBoard(t, f, "doc")
	assetID := pgAccessUpload(t, f, f.owner, "", "doclife")
	if _, err := f.boards.ReplaceBoardAccess(ctx, f.ws.PublicID, boardID, f.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{f.member}); err != nil {
		t.Fatal(err)
	}
	metadata := func(user int64) int {
		status, _, _ := doV1(t, f.engine, v1Call{method: "GET", path: f.assetsPath() + "/" + assetID, userID: user})
		return status
	}
	if got := metadata(f.member); got != http.StatusNotFound {
		t.Fatalf("pre-accept metadata = %d, want 404", got)
	}
	pgAccessDoc(t, f, boardID, 1, assetID)
	if got := metadata(f.member); got != http.StatusOK {
		t.Fatalf("post-accept metadata = %d, want 200", got)
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/batch",
		body: fmt.Sprintf(`{"data":{"asset_ids":[%q]}}`, assetID), userID: f.member})
	if status != http.StatusOK {
		t.Fatalf("batch post-accept = %d %v, want 200", status, payload)
	}
	// GC: the accept without the asset revokes; owner still reads.
	pgAccessDoc(t, f, boardID, 2)
	if got := metadata(f.member); got != http.StatusNotFound {
		t.Fatalf("post-removal metadata = %d, want 404", got)
	}
	if got := metadata(f.owner); got != http.StatusOK {
		t.Fatalf("owner metadata = %d, want 200 (independent source)", got)
	}
	status, _, _ = doV1(t, f.engine, v1Call{method: "POST", path: f.assetsPath() + "/batch",
		body: fmt.Sprintf(`{"data":{"asset_ids":[%q]}}`, assetID), userID: f.member})
	if status != http.StatusNotFound {
		t.Fatalf("batch post-removal = %d, want 404", status)
	}
}
