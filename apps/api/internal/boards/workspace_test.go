package boards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/boards/previewscene"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestV1BoardLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})

	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if !contract.IsBoardID(boardID) {
		t.Fatalf("board id = %q, want board_ prefix", boardID)
	}
	created, err := store.CreateBoard(ctx, "ws_test", 1, "Launch", boardID)
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 {
		t.Fatalf("revision = %d, want 1", created.Revision)
	}
	if FormatV1ETag(created.Revision) != "1" {
		t.Fatal("ETag must be the revision counter")
	}

	boards, err := store.ListBoardCatalog(ctx, "ws_test", false, 1, tenancy.RoleOwner, V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(boards.Entries) != 1 {
		t.Fatalf("list = %v, %v; want 1 board", boards, err)
	}

	renamed, err := store.RenameBoard(ctx, "ws_test", boardID, "Launch v2", 1)
	if err != nil || renamed.Name != "Launch v2" || renamed.Revision != 1 {
		t.Fatalf("rename = %v, %v", renamed, err)
	}

	archived, err := store.ArchiveBoard(ctx, "ws_test", boardID, 1)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archive = %v, %v", archived, err)
	}
	visible, err := store.ListBoardCatalog(ctx, "ws_test", false, 1, tenancy.RoleOwner, V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(visible.Entries) != 0 {
		t.Fatalf("archived boards must be excluded, got %v", visible)
	}
	if _, err := store.ReplaceDocument(ctx, "ws_test", boardID, 1, 1, 1, V1InitialDocument()); err != ErrV1Archived {
		t.Fatalf("archived write = %v, want ErrV1Archived", err)
	}
	restored, err := store.RestoreBoard(ctx, "ws_test", boardID, 1)
	if err != nil || restored.ArchivedAt != nil {
		t.Fatalf("restore = %v, %v", restored, err)
	}
}

func TestV1DocumentConcurrency(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	boardID, _ := NewV1BoardPublicID()
	if _, err := store.CreateBoard(ctx, "ws_c", 1, "Doc", boardID); err != nil {
		t.Fatal(err)
	}
	snapshot := `{"schema_version":2,"payload":{"nodes":[]}}`
	schemaVersion, _, err := ValidateV1DocumentStrict([]byte(snapshot))
	if err != nil || schemaVersion != 2 {
		t.Fatalf("validate = %d, %v", schemaVersion, err)
	}
	// Stale If-Match fails.
	if _, err := store.ReplaceDocument(ctx, "ws_c", boardID, 1, 99, 2, jsonRaw(snapshot)); err != ErrRevisionConflict {
		t.Fatalf("stale write = %v, want ErrRevisionConflict", err)
	}
	// Fresh ETag succeeds and appends an immutable revision.
	updated, err := store.ReplaceDocument(ctx, "ws_c", boardID, 1, 1, 2, jsonRaw(snapshot))
	if err != nil || updated.Revision != 2 {
		t.Fatalf("write = %v, %v; want revision 2", updated, err)
	}
	revisions, err := store.ListRevisions(ctx, "ws_c", boardID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions = %d, %v; want 2 entries", len(revisions), err)
	}
	if revisions[1].SourceRevision != 1 || revisions[1].Revision != 2 {
		t.Fatalf("revision linkage = %+v", revisions[1])
	}
	got, err := store.GetRevision(ctx, "ws_c", boardID, 1)
	if err != nil || got.Revision != 1 {
		t.Fatalf("get revision 1 = %v, %v", got, err)
	}
	// Entries are copies: mutating the returned snapshot must not corrupt history.
	got.Snapshot[0] = 'X'
	again, _ := store.GetRevision(ctx, "ws_c", boardID, 1)
	if again.Snapshot[0] == 'X' {
		t.Fatal("revision entries must be immutable copies")
	}
}

func TestV1DocumentStrictRejection(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"unknown field", `{"schema_version":1,"payload":{},"bogus":1}`},
		{"missing payload", `{"schema_version":1}`},
		{"zero schema", `{"schema_version":0,"payload":{}}`},
		{"array payload", `{"schema_version":1,"payload":[]}`},
		{"trailing payload", `{"schema_version":1,"payload":{}} {}`},
		{"empty", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ValidateV1DocumentStrict([]byte(tc.raw)); err == nil {
				t.Fatalf("ValidateV1DocumentStrict(%q) = nil, want error", tc.raw)
			}
		})
	}
	if _, _, err := ValidateV1DocumentStrict([]byte(`{"schema_version":3,"payload":{"a":1}}`)); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
}

func TestWorkspaceMembershipPrecondition(t *testing.T) {
	for _, tc := range []struct {
		workspace string
		want      bool
	}{
		{"owner", true},
		{"admin", true},
		{"member", true},
		{" member ", true},
		{"", false},
		{"bogus", false},
	} {
		if got := IsWorkspaceMemberRole(tc.workspace); got != tc.want {
			t.Fatalf("IsWorkspaceMemberRole(%q) = %v; want %v", tc.workspace, got, tc.want)
		}
	}
	for _, tc := range []struct {
		access WorkspaceAccess
		want   bool
	}{
		{WorkspaceAccessNone, true},
		{WorkspaceAccessViewer, true},
		{WorkspaceAccessEditor, true},
		{"", false},
		{"superadmin", false},
	} {
		if got := ValidateWorkspaceAccess(tc.access); got != tc.want {
			t.Fatalf("ValidateWorkspaceAccess(%q) = %v; want %v", tc.access, got, tc.want)
		}
	}
}

func TestV1ETagParsing(t *testing.T) {
	for _, raw := range []string{"3", `"3"`, `W/"3"`, "  3  "} {
		if revision, err := ParseV1IfMatch(raw); err != nil || revision != 3 {
			t.Fatalf("ParseV1IfMatch(%q) = %d, %v", raw, revision, err)
		}
	}
	for _, raw := range []string{"", "*", "0", "-1", "abc"} {
		if _, err := ParseV1IfMatch(raw); err == nil {
			t.Fatalf("ParseV1IfMatch(%q) = nil, want error", raw)
		}
	}
}

func jsonRaw(s string) []byte { return []byte(s) }

// merged from board_media_refs_test.go (access/preview aggregation over the in-memory store).
// Board document -> media reference reconciliation (#936, in-memory twin).
//
// Pins that ReplaceDocument reconciles the accepted reference set: newly
// referenced asset ids appear through the narrow port, removed ids stop,
// and non-identities never become references.

func refsTestEnvelope(t *testing.T, assetIDs ...string) json.RawMessage {
	t.Helper()
	type mediaRef struct {
		AssetID string `json:"assetId"`
	}
	type output struct {
		URL      string    `json:"url"`
		MediaRef *mediaRef `json:"mediaRef,omitempty"`
	}
	type imageNode struct {
		ID             float64   `json:"id"`
		X              float64   `json:"x"`
		Y              float64   `json:"y"`
		Width          float64   `json:"width"`
		SourceImageURL string    `json:"sourceImageUrl"`
		SourceMediaRef *mediaRef `json:"sourceMediaRef,omitempty"`
		Outputs        []output  `json:"outputImages"`
	}
	nodes := make([]imageNode, 0, len(assetIDs))
	for i, id := range assetIDs {
		ref := &mediaRef{AssetID: id}
		nodes = append(nodes, imageNode{
			ID: float64(i + 1), Width: 280,
			SourceImageURL: "https://example.com/y.png",
			SourceMediaRef: ref,
			Outputs:        []output{{URL: "https://example.com/o.png", MediaRef: ref}},
		})
	}
	payload, err := json.Marshal(map[string]any{
		"schemaVersion": 4,
		"imageNodes":    nodes,
		"promptNodes":   []any{},
		"fusionNodes":   []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(map[string]any{"schema_version": 1, "payload": json.RawMessage(payload)})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func exerciseBoardMediaRefs(t *testing.T, ctx context.Context, store WorkspaceBoardStore, workspaceID string, owner int64) {
	t.Helper()
	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBoard(ctx, workspaceID, owner, "refs", boardID); err != nil {
		t.Fatalf("create board: %v", err)
	}
	list := func(asset string) []string {
		t.Helper()
		rows, err := store.ListBoardsReferencingAsset(ctx, workspaceID, asset)
		if err != nil {
			t.Fatalf("list refs: %v", err)
		}
		return rows
	}
	const assetA = "asset_aaaaaaaaaaaaaaaa"
	const assetB = "asset_bbbbbbbbbbbbbbbb"
	if got := list(assetA); len(got) != 0 {
		t.Fatalf("fresh board refs = %v, want none", got)
	}
	if _, err := store.ReplaceDocument(ctx, workspaceID, boardID, owner, 1, 1, refsTestEnvelope(t, assetA, assetB)); err != nil {
		t.Fatalf("replace with refs: %v", err)
	}
	if got := list(assetA); len(got) != 1 || got[0] != boardID {
		t.Fatalf("refs for A = %v, want [%s]", got, boardID)
	}
	if got := list(assetB); len(got) != 1 || got[0] != boardID {
		t.Fatalf("refs for B = %v, want [%s]", got, boardID)
	}
	// Removal reconciles in the accepting transaction: B drops out, A stays.
	if _, err := store.ReplaceDocument(ctx, workspaceID, boardID, owner, 2, 1, refsTestEnvelope(t, assetA)); err != nil {
		t.Fatalf("replace without B: %v", err)
	}
	if got := list(assetA); len(got) != 1 {
		t.Fatalf("refs for A after removal = %v, want one", got)
	}
	if got := list(assetB); len(got) != 0 {
		t.Fatalf("refs for B after removal = %v, want none", got)
	}
	// Non-identities never become references.
	if _, err := store.ReplaceDocument(ctx, workspaceID, boardID, owner, 3, 1, refsTestEnvelope(t, "nope", "")); err != nil {
		t.Fatalf("replace with non-identities: %v", err)
	}
	if got := list("nope"); len(got) != 0 {
		t.Fatalf("refs for malformed = %v, want none", got)
	}
	if got := list(assetA); len(got) != 0 {
		t.Fatalf("refs for A after clearing = %v, want none", got)
	}
}

func TestBoardMediaRefsInMemory(t *testing.T) {
	ctx := context.Background()
	exerciseBoardMediaRefs(t, ctx, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_refs", 1)
}

// merged from catalog_page_contract_test.go (access/preview aggregation over the in-memory store).
func exerciseCatalogPageContract(t *testing.T, store WorkspaceBoardStore, workspaceID string, setUpdatedAt func(string, time.Time)) {
	t.Helper()
	ctx := context.Background()
	const owner, reader = int64(1), int64(3)
	ids := make([]string, 7)
	for i := range ids {
		id, err := NewV1BoardPublicID()
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		if _, err := store.CreateBoard(ctx, workspaceID, owner, fmt.Sprintf("Page %d", i), id); err != nil {
			t.Fatal(err)
		}
	}
	// The newest two rows are inaccessible; the next is archived. Neither
	// may consume a position in the reader's ordered page.
	for _, i := range []int{2, 3, 4, 5, 6} {
		if _, err := store.ReplaceBoardAccess(ctx, workspaceID, ids[i], owner, 1, WorkspaceAccess(RoleViewer), []int64{reader}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ArchiveBoard(ctx, workspaceID, ids[2], owner); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	for i, id := range ids {
		setUpdatedAt(id, base.Add(-time.Duration(i)*time.Hour))
	}
	// A tie must be ordered by public id in both adapters.
	setUpdatedAt(ids[4], base.Add(-3*time.Hour))
	if ids[3] > ids[4] {
		ids[3], ids[4] = ids[4], ids[3]
	}
	want := ids[3:]
	for _, tc := range []struct {
		name    string
		request V1BoardCatalogPageRequest
		want    []string
		hasMore bool
		wantErr error
	}{
		{"first", V1BoardCatalogPageRequest{Limit: 2}, want[:2], true, nil},
		{"last-exact", V1BoardCatalogPageRequest{Offset: 2, Limit: 2}, want[2:], false, nil},
		{"last-short", V1BoardCatalogPageRequest{Offset: 3, Limit: 2}, want[3:], false, nil},
		{"empty-end", V1BoardCatalogPageRequest{Offset: 4, Limit: 2}, nil, false, nil},
		{"past-end", V1BoardCatalogPageRequest{Offset: 5, Limit: 2}, nil, false, ErrV1CatalogOffset},
		{"negative", V1BoardCatalogPageRequest{Offset: -1, Limit: 2}, nil, false, ErrV1CatalogOffset},
		{"over-cap", V1BoardCatalogPageRequest{Offset: 501, Limit: 2}, nil, false, ErrV1CatalogOffset},
		{"zero-size", V1BoardCatalogPageRequest{}, nil, false, ErrV1CatalogPage},
		{"over-size", V1BoardCatalogPageRequest{Limit: 101}, nil, false, ErrV1CatalogPage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := store.ListBoardCatalog(ctx, workspaceID, false, reader, tenancy.RoleMember, tc.request)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("page error=%v want=%v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if len(page.Entries) != len(tc.want) || page.HasMore != tc.hasMore {
				t.Fatalf("page count=%d more=%v want=%d/%v", len(page.Entries), page.HasMore, len(tc.want), tc.hasMore)
			}
			for i, id := range tc.want {
				if page.Entries[i].PublicID != id {
					t.Fatalf("page[%d]=%s want=%s", i, page.Entries[i].PublicID, id)
				}
			}
		})
	}
	archived, err := store.ListBoardCatalog(ctx, workspaceID, true, reader, tenancy.RoleMember, V1BoardCatalogPageRequest{Limit: 1})
	if err != nil || len(archived.Entries) != 1 || archived.Entries[0].PublicID != ids[2] || !archived.HasMore {
		t.Fatalf("archived page count=%d more=%v err=%v", len(archived.Entries), archived.HasMore, err)
	}
	denied, err := store.ListBoardCatalog(ctx, workspaceID, true, 7, "", V1BoardCatalogPageRequest{Limit: 1})
	if err != nil || len(denied.Entries) != 0 || denied.HasMore {
		t.Fatalf("denied page=%+v err=%v", denied, err)
	}
	_, err = store.ListBoardCatalog(ctx, workspaceID, true, 7, "", V1BoardCatalogPageRequest{Offset: 1, Limit: 1})
	if !errors.Is(err, ErrV1CatalogOffset) {
		t.Fatalf("denied offset err=%v", err)
	}
	foreign, err := store.ListBoardCatalog(ctx, "ws_missing", true, owner, tenancy.RoleOwner, V1BoardCatalogPageRequest{Limit: 1})
	if err != nil || len(foreign.Entries) != 0 || foreign.HasMore {
		t.Fatalf("foreign page=%+v err=%v", foreign, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.ListBoardCatalog(canceled, workspaceID, false, reader, tenancy.RoleMember, V1BoardCatalogPageRequest{Limit: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request err=%v", err)
	}
}

func TestBoardCatalogPageContract(t *testing.T) {
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	const workspaceID = "ws_page_contract"
	exerciseCatalogPageContract(t, store, workspaceID, func(id string, at time.Time) {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.boards[workspaceID][id].UpdatedAt = at
	})
}

func assertCatalogCap(t *testing.T, store WorkspaceBoardStore, workspaceID string, owner int64) {
	t.Helper()
	ctx := context.Background()
	var all []*V1BoardCatalogEntry
	for offset := 0; offset < maxBoardCatalogRows; offset += 100 {
		page, err := store.ListBoardCatalog(ctx, workspaceID, false, owner, tenancy.RoleOwner, V1BoardCatalogPageRequest{Offset: offset, Limit: 100})
		if err != nil || len(page.Entries) != 100 || page.HasMore != (offset+100 < maxBoardCatalogRows) {
			t.Fatalf("offset=%d count=%d more=%v err=%v", offset, len(page.Entries), page.HasMore, err)
		}
		all = append(all, page.Entries...)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].PublicID >= all[i].PublicID {
			t.Fatal("tied page order is unstable or duplicated")
		}
	}
	page, err := store.ListBoardCatalog(ctx, workspaceID, false, owner, tenancy.RoleOwner, V1BoardCatalogPageRequest{Offset: 500, Limit: 1})
	if err != nil || len(page.Entries) != 0 || page.HasMore {
		t.Fatalf("cap page=%+v err=%v", page, err)
	}
}

func TestBoardCatalogPageCap(t *testing.T) {
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	const workspaceID = "ws_page_cap"
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	for i := range 505 {
		id := fmt.Sprintf("board_%04d", i)
		if _, err := store.CreateBoard(context.Background(), workspaceID, 1, id, id); err != nil {
			t.Fatal(err)
		}
		store.boards[workspaceID][id].UpdatedAt = at
	}
	assertCatalogCap(t, store, workspaceID, 1)
}

// merged from preview_catalog_test.go (access/preview aggregation over the in-memory store).
func schema4Envelope(t *testing.T, payload string) json.RawMessage {
	t.Helper()
	raw := `{"schema_version":4,"payload":` + payload + `}`
	schemaVersion, _, err := ValidateV1DocumentStrict([]byte(raw))
	if err != nil {
		t.Fatalf("test envelope fails strict validation: %v", err)
	}
	if schemaVersion != 4 {
		t.Fatalf("test envelope schema = %d, want 4", schemaVersion)
	}
	return json.RawMessage(raw)
}

func schema4EmptyPayload() string {
	return `{"imageNodes":[],"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}`
}

func schema4ImagePayload() string {
	return `{"imageNodes":[{"id":1,"x":10,"y":20,"width":280,"sourceImageUrl":"https://cdn.example.com/a.jpg","sourceImageRatio":1,"outputImages":[]}],"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}`
}

func mustCatalogEntry(t *testing.T, store *InMemoryWorkspaceBoardStore, ws, boardID string) *V1BoardCatalogEntry {
	t.Helper()
	entries, err := store.ListBoardCatalog(context.Background(), ws, false, 1, tenancy.RoleOwner, V1BoardCatalogPageRequest{Limit: 100})
	if err != nil {
		t.Fatalf("ListBoardCatalog: %v", err)
	}
	for _, entry := range entries.Entries {
		if entry.PublicID == boardID {
			return entry
		}
	}
	t.Fatalf("board %s missing from catalog", boardID)
	return nil
}

func TestPreviewCatalogCreateAssignsVersionWithoutScene(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	boardID, _ := NewV1BoardPublicID()
	created, err := store.CreateBoard(ctx, "ws_pv", 1, "Fresh", boardID)
	if err != nil {
		t.Fatal(err)
	}
	if created.ContentVersion != previewscene.ContentVersionOf(V1InitialDocument()) {
		t.Fatalf("create content version = %q, want canonical hash", created.ContentVersion)
	}
	if len(created.PreviewScene) != 0 {
		t.Fatalf("fresh schema-1 document must carry no scene")
	}
	entry := mustCatalogEntry(t, store, "ws_pv", boardID)
	if entry.ContentVersion != created.ContentVersion || len(entry.PreviewScene) != 0 {
		t.Fatalf("catalog entry diverges from stored board: %+v", entry)
	}
}

func TestPreviewCatalogReplacePublishesScene(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	boardID, _ := NewV1BoardPublicID()
	if _, err := store.CreateBoard(ctx, "ws_pv", 1, "Scene", boardID); err != nil {
		t.Fatal(err)
	}
	envelope := schema4Envelope(t, schema4ImagePayload())
	updated, err := store.ReplaceDocument(ctx, "ws_pv", boardID, 1, 1, 4, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContentVersion != previewscene.ContentVersionOf(envelope) {
		t.Fatalf("replace content version = %q, want canonical hash", updated.ContentVersion)
	}
	scene, err := previewscene.ParseScene(updated.PreviewScene)
	if err != nil {
		t.Fatalf("stored scene does not parse: %v", err)
	}
	if scene.ContentVersion != updated.ContentVersion {
		t.Fatalf("scene content version = %q, want %q", scene.ContentVersion, updated.ContentVersion)
	}
	if len(scene.Nodes) != 1 || scene.Nodes[0].ID != "image-1" {
		t.Fatalf("stored scene nodes = %+v, want one image-1", scene.Nodes)
	}
	entry := mustCatalogEntry(t, store, "ws_pv", boardID)
	if entry.ContentVersion != updated.ContentVersion || string(entry.PreviewScene) != string(updated.PreviewScene) {
		t.Fatalf("catalog entry diverges from stored projection")
	}
	if entry.Revision != 2 || entry.SchemaVersion != 4 {
		t.Fatalf("catalog entry = revision %d schema %d, want 2/4", entry.Revision, entry.SchemaVersion)
	}
}

func TestPreviewCatalogMetadataLifecyclePreservesProjection(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	boardID, _ := NewV1BoardPublicID()
	if _, err := store.CreateBoard(ctx, "ws_pv", 1, "Meta", boardID); err != nil {
		t.Fatal(err)
	}
	envelope := schema4Envelope(t, schema4ImagePayload())
	updated, err := store.ReplaceDocument(ctx, "ws_pv", boardID, 1, 1, 4, envelope)
	if err != nil {
		t.Fatal(err)
	}
	wantVersion, wantScene := updated.ContentVersion, string(updated.PreviewScene)
	renamed, err := store.RenameBoard(ctx, "ws_pv", boardID, "Meta v2", 1)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ContentVersion != wantVersion || string(renamed.PreviewScene) != wantScene {
		t.Fatalf("rename changed the content projection")
	}
	archived, err := store.ArchiveBoard(ctx, "ws_pv", boardID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ContentVersion != wantVersion || string(archived.PreviewScene) != wantScene {
		t.Fatalf("archive changed the content projection")
	}
	restored, err := store.RestoreBoard(ctx, "ws_pv", boardID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ContentVersion != wantVersion || string(restored.PreviewScene) != wantScene {
		t.Fatalf("restore changed the content projection")
	}
}

func TestPreviewCatalogConflictPublishesNothing(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	boardID, _ := NewV1BoardPublicID()
	if _, err := store.CreateBoard(ctx, "ws_pv", 1, "Conflict", boardID); err != nil {
		t.Fatal(err)
	}
	first := schema4Envelope(t, schema4EmptyPayload())
	updated, err := store.ReplaceDocument(ctx, "ws_pv", boardID, 1, 1, 4, first)
	if err != nil {
		t.Fatal(err)
	}
	wantVersion, wantScene := updated.ContentVersion, string(updated.PreviewScene)
	stale := schema4Envelope(t, schema4ImagePayload())
	if _, err := store.ReplaceDocument(ctx, "ws_pv", boardID, 1, 1, 4, stale); err != ErrRevisionConflict {
		t.Fatalf("stale write = %v, want ErrRevisionConflict", err)
	}
	after, err := store.GetBoard(ctx, "ws_pv", boardID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != 2 || after.ContentVersion != wantVersion || string(after.PreviewScene) != wantScene {
		t.Fatalf("conflict published a projection: revision %d version %q", after.Revision, after.ContentVersion)
	}
	revisions, err := store.ListRevisions(ctx, "ws_pv", boardID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("conflict appended history: %d entries", len(revisions))
	}
}

func TestPreviewCatalogNonExtractableSchemaKeepsVersionWithoutScene(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	boardID, _ := NewV1BoardPublicID()
	if _, err := store.CreateBoard(ctx, "ws_pv", 1, "Legacy", boardID); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"schema_version":2,"payload":{"nodes":[]}}`)
	updated, err := store.ReplaceDocument(ctx, "ws_pv", boardID, 1, 1, 2, raw)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContentVersion != previewscene.ContentVersionOf(raw) {
		t.Fatalf("non-extractable document must still carry the canonical version")
	}
	if len(updated.PreviewScene) != 0 {
		t.Fatalf("non-extractable document must carry no scene")
	}
}

func TestPreviewCatalogQueryNeverReadsDocuments(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "db", "queries", "workspace_boards.sql"))
	if err != nil {
		t.Fatalf("read board queries: %v", err)
	}
	text := string(raw)
	marker := "-- name: ListWorkspaceBoardCatalog"
	start := strings.Index(text, marker)
	if start < 0 {
		t.Fatalf("ListWorkspaceBoardCatalog query missing")
	}
	rest := text[start+len(marker):]
	if next := strings.Index(rest, "-- name:"); next >= 0 {
		rest = rest[:next]
	}
	lowered := strings.ToLower(rest)
	// Strip line comments: the guard constrains the query itself, and the
	// contract rationale above names the forbidden reference explicitly.
	var query strings.Builder
	for _, line := range strings.Split(lowered, "\n") {
		if index := strings.Index(line, "--"); index >= 0 {
			line = line[:index]
		}
		query.WriteString(line)
		query.WriteByte('\n')
	}
	statement := query.String()
	if strings.Contains(statement, "snapshot_data") {
		t.Fatalf("catalog query must not read snapshot_data:\n%s", rest)
	}
	for _, want := range []string{"content_version", "preview_scene", "workspace_public_id"} {
		if !strings.Contains(statement, want) {
			t.Fatalf("catalog query must select %s:\n%s", want, rest)
		}
	}
}

func exerciseCatalogVisibility(t *testing.T, store WorkspaceBoardStore, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	var boardIDs []string
	for _, owner := range []int64{1, 3, 7, 1} {
		id, err := NewV1BoardPublicID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateBoard(ctx, workspaceID, owner, "Catalog visibility", id); err != nil {
			t.Fatal(err)
		}
		boardIDs = append(boardIDs, id)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, boardIDs[0], 1, 1, WorkspaceAccessEditor, []int64{3, 7}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ArchiveBoard(ctx, workspaceID, boardIDs[3], 1); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		user     int64
		role     string
		archived bool
		want     int
	}{
		{"workspace-owner", 1, "owner", false, 1},
		{"workspace-admin", 8, "admin", false, 0},
		{"workspace-member", 8, "member", false, 0},
		{"creator-without-membership", 1, "", false, 0},
		{"explicit-editor", 3, "member", false, 2},
		{"explicit-viewer", 7, "member", false, 2},
		{"unknown-workspace-role", 8, "unknown", false, 0},
		{"include-archived", 1, "owner", true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := store.ListBoardCatalog(ctx, workspaceID, tc.archived, tc.user, tc.role, V1BoardCatalogPageRequest{Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			if len(catalog.Entries) != tc.want {
				t.Fatalf("catalog count=%d want=%d", len(catalog.Entries), tc.want)
			}
			for _, entry := range catalog.Entries {
				if _, ok, err := store.GetBoardRole(ctx, workspaceID, entry.PublicID, tc.user, tc.role); err != nil || !ok {
					t.Fatalf("catalog leaked denied board %s: %v", entry.PublicID, err)
				}
				if !tc.archived && entry.ArchivedAt != nil {
					t.Fatal("catalog leaked archived board")
				}
			}
		})
	}
	foreign, err := store.ListBoardCatalog(ctx, "ws_nonexistent", true, 1, tenancy.RoleOwner, V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(foreign.Entries) != 0 {
		t.Fatalf("foreign workspace catalog=%d err=%v", len(foreign.Entries), err)
	}
}

func TestPreviewCatalogFiltersEffectiveAccess(t *testing.T) {
	exerciseCatalogVisibility(t, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_catalog_acl")
}
