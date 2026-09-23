package boards

import (
	"context"
	"strings"
	"testing"
)

func TestV1BoardLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests()

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

	boards, err := store.ListBoards(ctx, "ws_test", false)
	if err != nil || len(boards) != 1 {
		t.Fatalf("list = %v, %v; want 1 board", boards, err)
	}

	renamed, err := store.RenameBoard(ctx, "ws_test", boardID, "Launch v2")
	if err != nil || renamed.Name != "Launch v2" || renamed.Revision != 1 {
		t.Fatalf("rename = %v, %v", renamed, err)
	}

	archived, err := store.ArchiveBoard(ctx, "ws_test", boardID)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archive = %v, %v", archived, err)
	}
	visible, err := store.ListBoards(ctx, "ws_test", false)
	if err != nil || len(visible) != 0 {
		t.Fatalf("archived boards must be excluded, got %v", visible)
	}
	if _, err := store.ReplaceDocument(ctx, "ws_test", boardID, 1, 1, 1, V1InitialDocument()); err != ErrV1Archived {
		t.Fatalf("archived write = %v, want ErrV1Archived", err)
	}
	restored, err := store.RestoreBoard(ctx, "ws_test", boardID)
	if err != nil || restored.ArchivedAt != nil {
		t.Fatalf("restore = %v, %v", restored, err)
	}
}

func TestV1DocumentConcurrency(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests()
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

func TestV1BoardACLAndLastOwner(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests()
	boardID, _ := NewV1BoardPublicID()
	if _, err := store.CreateBoard(ctx, "ws_acl", 1, "ACL", boardID); err != nil {
		t.Fatal(err)
	}
	// Creator is the implicit owner; workspace membership alone grants nothing.
	if role, ok, _ := store.GetBoardRole(ctx, "ws_acl", boardID, 1); !ok || role != RoleOwner {
		t.Fatalf("creator role = %q, %v; want owner", role, ok)
	}
	if _, ok, _ := store.GetBoardRole(ctx, "ws_acl", boardID, 2); ok {
		t.Fatal("workspace membership must not auto-grant board access")
	}
	// Only owners may grant; editor grant works.
	if _, err := store.AddBoardMember(ctx, "ws_acl", boardID, 2, 2, RoleEditor); err != ErrPermissionDenied {
		t.Fatalf("non-owner add = %v, want ErrPermissionDenied", err)
	}
	member, err := store.AddBoardMember(ctx, "ws_acl", boardID, 1, 2, RoleEditor)
	if err != nil || member.Role != RoleEditor {
		t.Fatalf("add = %v, %v", member, err)
	}
	// Re-add is idempotent (keeps existing role).
	member, err = store.AddBoardMember(ctx, "ws_acl", boardID, 1, 2, RoleViewer)
	if err != nil || member.Role != RoleEditor {
		t.Fatalf("re-add = %v, %v; want existing editor role", member, err)
	}
	// Second owner unlocks demotion of the implicit owner row path.
	if _, err := store.AddBoardMember(ctx, "ws_acl", boardID, 1, 3, RoleOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateBoardMemberRole(ctx, "ws_acl", boardID, 1, 2, RoleViewer); err != nil {
		t.Fatalf("demote with backup owner = %v", err)
	}
	// The implicit creator row cannot be demoted while it is the last owner.
	soloID, _ := NewV1BoardPublicID()
	if _, err := store.CreateBoard(ctx, "ws_acl", 9, "Solo", soloID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateBoardMemberRole(ctx, "ws_acl", soloID, 9, 9, RoleViewer); err != ErrV1LastOwner {
		t.Fatalf("demote last owner = %v, want ErrV1LastOwner", err)
	}
	// Removing an explicit owner succeeds while the creator remains owner.
	if err := store.RemoveBoardMember(ctx, "ws_acl", boardID, 1, 3); err != nil {
		t.Fatalf("remove explicit owner with creator remaining = %v", err)
	}
	// Self-leave works for non-owners.
	if err := store.RemoveBoardMember(ctx, "ws_acl", boardID, 2, 2); err != nil {
		t.Fatalf("self leave = %v", err)
	}
}

func TestV1ShareTokenJoin(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests()
	boardID, _ := NewV1BoardPublicID()
	if _, err := store.CreateBoard(ctx, "ws_share", 1, "Share", boardID); err != nil {
		t.Fatal(err)
	}
	tokenID, _ := NewV1ShareTokenID()
	if !IsV1ShareTokenID(tokenID) {
		t.Fatalf("token id = %q, want sht_ prefix", tokenID)
	}
	created, err := store.CreateShareToken(ctx, "ws_share", boardID, 1, RoleViewer, tokenID, V1ShareTokenHash(tokenID), tokenID)
	if err != nil || created.Bearer == "" {
		t.Fatalf("create share = %v, %v", created, err)
	}
	// Join is idempotent: repeat joins converge to the same role.
	board, role, err := store.JoinByToken(ctx, "ws_share", tokenID, 7)
	if err != nil || role != RoleViewer || board.PublicID != boardID {
		t.Fatalf("join = %v, %q, %v", board, role, err)
	}
	if _, role, err := store.JoinByToken(ctx, "ws_share", tokenID, 7); err != nil || role != RoleViewer {
		t.Fatalf("rejoin = %q, %v; want same viewer role", role, err)
	}
	tokens, err := store.ListShareTokens(ctx, "ws_share", boardID)
	if err != nil || len(tokens) != 1 || tokens[0].UsageCount != 2 {
		t.Fatalf("usage = %v, %v; want usage_count 2", tokens, err)
	}
	if tokens[0].Bearer != "" {
		t.Fatal("list must never return the bearer")
	}
	if len(tokens[0].AuditTrail) != 3 { // create + 2 joins
		t.Fatalf("audit trail = %d events, want 3", len(tokens[0].AuditTrail))
	}
	// Unknown bearers stay invisible.
	if _, _, err := store.JoinByToken(ctx, "ws_share", "sht_doesnotexist", 8); err != ErrShareTokenNotFound {
		t.Fatalf("unknown join = %v, want ErrShareTokenNotFound", err)
	}
	// Revocation blocks joins with an auditable outcome.
	if err := store.RevokeShareToken(ctx, "ws_share", boardID, tokenID, 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.JoinByToken(ctx, "ws_share", tokenID, 8); err != ErrShareTokenRevoked {
		t.Fatalf("revoked join = %v, want ErrShareTokenRevoked", err)
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
