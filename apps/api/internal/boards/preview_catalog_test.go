package boards

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ideaboard/api/internal/boards/previewscene"
)

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
	entries, err := store.ListBoardCatalog(context.Background(), ws, false, 1, "owner", V1BoardCatalogPageRequest{Limit: 100})
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
	foreign, err := store.ListBoardCatalog(ctx, "ws_nonexistent", true, 1, "owner", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(foreign.Entries) != 0 {
		t.Fatalf("foreign workspace catalog=%d err=%v", len(foreign.Entries), err)
	}
}

func TestPreviewCatalogFiltersEffectiveAccess(t *testing.T) {
	exerciseCatalogVisibility(t, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_catalog_acl")
}
