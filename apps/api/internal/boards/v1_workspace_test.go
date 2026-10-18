package boards

import (
	"context"
	"strings"
	"testing"
)

func TestV1BoardLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})

	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(boardID, "board_") {
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

	boards, err := store.ListBoardCatalog(ctx, "ws_test", false, 1, "owner", V1BoardCatalogPageRequest{Limit: 100})
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
	visible, err := store.ListBoardCatalog(ctx, "ws_test", false, 1, "owner", V1BoardCatalogPageRequest{Limit: 100})
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
