package boards

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
)

// Y.Doc collaboration seam (ADR-0020): strip/sanitize boundary plus the
// in-memory update-log + compaction lifecycle. PostgreSQL twins run below
// and skip without API_TEST_DATABASE_DSN like every other PG integration
// test (testdb.Open handles the skip).

func TestStripYNodeKeepsWhitelistDropsTransient(t *testing.T) {
	stripped, ok := StripYNode("n1", map[string]any{
		"id":       "n1",
		"type":     "image",
		"position": map[string]any{"x": 10.0, "y": 20.0},
		"data":     map[string]any{"prompt": "cat"},
		"style":    map[string]any{"color": "red"},
		"selected": true, "dragging": true, "resizing": true,
		"bogus": "dropped",
	})
	if !ok {
		t.Fatal("valid node dropped")
	}
	for _, key := range []string{"id", "type", "position", "data", "style"} {
		if _, present := stripped[key]; !present {
			t.Fatalf("whitelisted key %q missing: %v", key, stripped)
		}
	}
	for _, key := range []string{"selected", "dragging", "resizing", "bogus"} {
		if _, present := stripped[key]; present {
			t.Fatalf("transient/illegal key %q survived stripping", key)
		}
	}
}

func TestStripYNodeDropsMismatchedEntries(t *testing.T) {
	for name, entry := range map[string]any{
		"non-object":   "n1",
		"id mismatch":  map[string]any{"id": "n2", "type": "image"},
		"missing type": map[string]any{"id": "n1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := StripYNode("n1", entry); ok {
				t.Fatalf("StripYNode(%v) = ok, want drop", entry)
			}
		})
	}
	// Malformed position drops the key but keeps the entry.
	stripped, ok := StripYNode("n1", map[string]any{
		"id": "n1", "type": "image",
		"position": map[string]any{"x": "far", "y": 1.0},
	})
	if !ok {
		t.Fatal("entry with bad position must be kept")
	}
	if _, present := stripped["position"]; present {
		t.Fatal("malformed position must be dropped")
	}
}

func TestStripYEdgeKeepsEndpointsDropsTransient(t *testing.T) {
	stripped, ok := StripYEdge("e1", map[string]any{
		"id": "e1", "type": "link", "source": "n1", "target": "n2",
		"data":     map[string]any{"label": "x"},
		"selected": true, "bogus": 1,
	})
	if !ok {
		t.Fatal("valid edge dropped")
	}
	for _, key := range []string{"id", "type", "source", "target", "data"} {
		if _, present := stripped[key]; !present {
			t.Fatalf("edge key %q missing: %v", key, stripped)
		}
	}
	if _, present := stripped["selected"]; present {
		t.Fatal("edge transient selected must be dropped")
	}
	if _, ok := StripYEdge("e1", map[string]any{"id": "e1", "source": "n1"}); ok {
		t.Fatal("edge without target must be dropped")
	}
}

func ySnapshotForTests(t *testing.T, payload string) json.RawMessage {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		t.Fatal(err)
	}
	value["y_state"] = "AAA="
	envelope, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"payload":        value,
	})
	if err != nil {
		t.Fatal(err)
	}
	return json.RawMessage(envelope)
}

func TestSanitizeYCompactSnapshot(t *testing.T) {
	raw := ySnapshotForTests(t, `{"nodes":{"n1":{"id":"n1","type":"image","position":{"x":1,"y":2},"data":{"id":1,"type":"image-node"},"selected":true,"bogus":1}},"edges":{},"legacy":"dropped"}`)
	version, sanitized, err := SanitizeYCompactSnapshot(raw)
	if err != nil || version != 4 {
		t.Fatalf("sanitize = %d, %v; want 4, nil", version, err)
	}
	var envelope struct {
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(sanitized, &envelope); err != nil {
		t.Fatal(err)
	}
	if _, present := envelope.Payload["legacy"]; present {
		t.Fatal("illegal top-level field must be dropped")
	}
	images, ok := envelope.Payload["imageNodes"].([]any)
	if !ok || len(images) != 1 {
		t.Fatal("canonical image projection missing")
	}
	if envelope.Payload["y_state"] != "AAA=" {
		t.Fatal("opaque Y state missing")
	}
	if _, present := envelope.Payload["nodes"]; present {
		t.Fatal("duplicate map projection persisted")
	}
	if _, _, err := ValidateV1DocumentStrict(sanitized); err != nil {
		t.Fatalf("sanitized snapshot must pass strict validation: %v", err)
	}
}

func TestSanitizeYCompactSnapshotRejects(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown envelope field": `{"schema_version":1,"payload":{},"bogus":1}`,
		"zero schema":            `{"schema_version":0,"payload":{}}`,
		"empty":                  ``,
		"missing CRDT state":     `{"schema_version":1,"payload":{"nodes":{},"edges":{}}}`,
		"invalid CRDT state":     `{"schema_version":1,"payload":{"nodes":{},"edges":{},"y_state":"!!!"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := SanitizeYCompactSnapshot([]byte(raw)); err == nil {
				t.Fatalf("SanitizeYCompactSnapshot(%q) = nil, want error", raw)
			}
		})
	}
}

func newYDocFixture(t *testing.T) (context.Context, *InMemoryWorkspaceBoardStore, string) {
	t.Helper()
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBoard(ctx, "ws_ydoc", 1, "Y Board", boardID); err != nil {
		t.Fatal(err)
	}
	// Collaborator 2 joins as editor: the Y writer path under test.
	if _, err := store.ReplaceBoardAccess(ctx, "ws_ydoc", boardID, 1, 1, WorkspaceAccessEditor, []int64{2}); err != nil {
		t.Fatal(err)
	}
	return ctx, store, boardID
}

func TestYUpdateAppendListInMemory(t *testing.T) {
	ctx, store, boardID := newYDocFixture(t)
	for want := int64(1); want <= 2; want++ {
		pending, err := store.AppendYUpdate(ctx, "ws_ydoc", boardID, 2, []byte{0x1, byte(want)})
		if err != nil || pending != want {
			t.Fatalf("append %d = pending %d, %v; want %d, nil", want, pending, err, want)
		}
	}
	records, err := store.ListYUpdates(ctx, "ws_ydoc", boardID, 100)
	if err != nil || len(records) != 2 {
		t.Fatalf("list = %d, %v; want 2, nil", len(records), err)
	}
	if records[0].AuthorUserID != 2 || string(records[0].Update) != "\x01\x01" {
		t.Fatalf("first record = %+v, want author 2", records[0])
	}
	// Entries are copies.
	records[0].Update[0] = 0xFF
	again, _ := store.ListYUpdates(ctx, "ws_ydoc", boardID, 100)
	if again[0].Update[0] == 0xFF {
		t.Fatal("update entries must be immutable copies")
	}
}

func TestYUpdateAppendRejects(t *testing.T) {
	ctx, store, boardID := newYDocFixture(t)
	if _, err := store.AppendYUpdate(ctx, "ws_ydoc", boardID, 2, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty update = %v, want ErrInvalidInput", err)
	}
	// User 3 is a workspace member without a board grant: fail-closed.
	if _, err := store.AppendYUpdate(ctx, "ws_ydoc", boardID, 3, []byte{0x1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ungranted writer = %v, want ErrNotFound", err)
	}
	if _, err := store.ArchiveBoard(ctx, "ws_ydoc", boardID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendYUpdate(ctx, "ws_ydoc", boardID, 2, []byte{0x1}); !errors.Is(err, ErrV1Archived) {
		t.Fatalf("archived append = %v, want ErrV1Archived", err)
	}
	if _, err := store.CompactYDocument(ctx, "ws_ydoc", boardID, 2, ySnapshotForTests(t, `{"nodes":{},"edges":{}}`)); !errors.Is(err, ErrV1Archived) {
		t.Fatalf("archived compact = %v, want ErrV1Archived", err)
	}
}

func TestSnapshotWritePreservesYTailUntilCompaction(t *testing.T) {
	ctx, store, boardID := newYDocFixture(t)
	if _, err := store.AppendYUpdate(ctx, "ws_ydoc", boardID, 2, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceDocument(ctx, "ws_ydoc", boardID, 1, 99, 1, V1InitialDocument()); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale snapshot write = %v, want ErrRevisionConflict", err)
	}
	written, err := store.ReplaceDocument(ctx, "ws_ydoc", boardID, 1, 1, 1, V1InitialDocument())
	if err != nil || written.Revision != 2 {
		t.Fatalf("snapshot write = %v, %v; want revision 2", written, err)
	}
	pending, err := store.ListYUpdates(ctx, "ws_ydoc", boardID, 100)
	if err != nil || len(pending) != 1 {
		t.Fatalf("snapshot write changed Y tail: %d, %v", len(pending), err)
	}
	compacted, err := store.CompactYDocument(ctx, "ws_ydoc", boardID, 2, ySnapshotForTests(t, `{"nodes":{},"edges":{}}`))
	if err != nil || compacted.Revision != 3 {
		t.Fatalf("compaction = %v, %v; want revision 3", compacted, err)
	}
	pending, err = store.ListYUpdates(ctx, "ws_ydoc", boardID, 100)
	if err != nil || len(pending) != 0 {
		t.Fatalf("compaction did not trim Y tail: %d, %v", len(pending), err)
	}
	revisions, err := store.ListRevisions(ctx, "ws_ydoc", boardID)
	if err != nil || len(revisions) != 3 || revisions[2].SourceRevision != 2 {
		t.Fatalf("revision chain after both write paths = %+v, %v", revisions, err)
	}
}

func TestYCompactLifecycleInMemory(t *testing.T) {
	ctx, store, boardID := newYDocFixture(t)
	if _, err := store.AppendYUpdate(ctx, "ws_ydoc", boardID, 2, []byte{0x1}); err != nil {
		t.Fatal(err)
	}
	snapshot := ySnapshotForTests(t, `{"nodes":{"n1":{"id":"n1","type":"image","selected":true}},"edges":{"e1":{"id":"e1","source":"n1","target":"n1"}}}`)
	compacted, err := store.CompactYDocument(ctx, "ws_ydoc", boardID, 2, snapshot)
	if err != nil || compacted.Revision != 2 {
		t.Fatalf("compact = rev %v, %v; want 2, nil", compacted, err)
	}
	// The sanitized snapshot (not the raw one) is persisted.
	if strings.Contains(string(compacted.Document), "selected") {
		t.Fatalf("compacted document carries transient state: %s", compacted.Document)
	}
	// Every compaction appends one immutable revision; the log is trimmed.
	revisions, err := store.ListRevisions(ctx, "ws_ydoc", boardID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions = %d, %v; want 2", len(revisions), err)
	}
	if revisions[1].Revision != 2 || revisions[1].SourceRevision != 1 || revisions[1].CreatedBy != 2 {
		t.Fatalf("revision linkage = %+v", revisions[1])
	}
	pending, err := store.ListYUpdates(ctx, "ws_ydoc", boardID, 100)
	if err != nil || len(pending) != 0 {
		t.Fatalf("post-compact log = %d, %v; want trimmed", len(pending), err)
	}
	// Second compaction converges without any 409-style conflict.
	second, err := store.CompactYDocument(ctx, "ws_ydoc", boardID, 2, ySnapshotForTests(t, `{"nodes":{},"edges":{}}`))
	if err != nil || second.Revision != 3 {
		t.Fatalf("second compact = rev %v, %v; want 3, nil", second, err)
	}
	// Invalid snapshots never touch the document.
	if _, err := store.CompactYDocument(ctx, "ws_ydoc", boardID, 2, []byte(`{"schema_version":1}`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid compact = %v, want ErrInvalidInput", err)
	}
	current, _ := store.GetBoard(ctx, "ws_ydoc", boardID)
	if current.Revision != 3 {
		t.Fatalf("failed compact moved revision to %d", current.Revision)
	}
}

// TestYUpdatePostgresLifecycle proves the update log + compaction point
// against real PostgreSQL: appends aggregate per board, compaction writes
// board_documents, appends one board_revisions row, and trims the log.
// Skips without API_TEST_DATABASE_DSN (testdb.Open handles the skip).
func TestYUpdatePostgresLifecycle(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	tenancyStore := tenancypg.NewStore(db)
	const owner, collaborator = int64(1), int64(3)
	ws, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "ydoc-pg", OwnerUserID: owner})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	store := NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator)
	seedBoardTestMembers(t, store, ws.InternalID, collaborator)
	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBoard(ctx, ws.PublicID, owner, "Y Board", boardID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, owner, 1, WorkspaceAccessEditor, []int64{collaborator}); err != nil {
		t.Fatal(err)
	}

	pending, err := store.AppendYUpdate(ctx, ws.PublicID, boardID, collaborator, []byte{0x7, 0x9})
	if err != nil || pending != 1 {
		t.Fatalf("append = pending %d, %v; want 4, nil", pending, err)
	}
	records, err := store.ListYUpdates(ctx, ws.PublicID, boardID, 100)
	if err != nil || len(records) != 1 || records[0].AuthorUserID != collaborator {
		t.Fatalf("list = %+v, %v; want 1 record by %d", records, err, collaborator)
	}

	compacted, err := store.CompactYDocument(ctx, ws.PublicID, boardID, collaborator,
		ySnapshotForTests(t, `{"nodes":{"n1":{"id":"n1","type":"image","dragging":true}},"edges":{}}`))
	if err != nil || compacted.Revision != 2 {
		t.Fatalf("compact = rev %v, %v; want 2, nil", compacted, err)
	}
	if strings.Contains(string(compacted.Document), "dragging") {
		t.Fatalf("compacted document carries transient state: %s", compacted.Document)
	}
	revisions, err := store.ListRevisions(ctx, ws.PublicID, boardID)
	if err != nil || len(revisions) != 2 || revisions[1].Revision != 2 {
		t.Fatalf("revisions = %d, %v; want 2 ending at rev 2", len(revisions), err)
	}
	trimmed, err := store.ListYUpdates(ctx, ws.PublicID, boardID, 100)
	if err != nil || len(trimmed) != 0 {
		t.Fatalf("post-compact log = %d, %v; want trimmed", len(trimmed), err)
	}
}
