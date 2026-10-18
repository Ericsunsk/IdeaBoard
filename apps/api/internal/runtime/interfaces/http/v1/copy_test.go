package v1

// Board copy acceptance over real PostgreSQL + HTTP (issue #943,
// criteria 1-2).
//
// Exercises the supported copy chain (create name-only + document accept)
// as the copier would drive it from Gallery: the copy mints a new board
// identity with workspace_access=none, access_version=1 and no direct
// grants; every accepted media identity is gated on the copier's
// independent readability (#943 media gate in replaceBoardDocument); and
// the copy keeps working on its own authorization after the source board
// revokes or archives (per-board reference rows, never inherited ACL).
// Skips without API_TEST_DATABASE_DSN like every other Postgres integration
// test.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/platform/contract"
)

// copyDocPayload builds a strict-valid payload carrying one image node
// that references assetID through both the source and output media refs.
func copyDocPayload(assetID string) string {
	payload, err := json.Marshal(map[string]any{
		"schemaVersion": 4,
		"imageNodes": []any{
			map[string]any{
				"id":             1,
				"x":              0,
				"y":              0,
				"width":          280,
				"sourceImageUrl": "https://example.com/y.png",
				"sourceMediaRef": map[string]any{"assetId": assetID},
				"outputImages": []any{
					map[string]any{
						"url":      "https://example.com/o.png",
						"mediaRef": map[string]any{"assetId": assetID},
					},
				},
			},
		},
		"promptNodes": []any{},
		"fusionNodes": []any{},
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}

func copyPutDoc(t *testing.T, f *v1MediaAccessPGFixture, ws, board, payload, ifMatch, key string, userID int64) (int, map[string]any) {
	t.Helper()
	call := v1Call{method: http.MethodPut,
		path:   "/api/v1/workspaces/" + ws + "/boards/" + board + "/document",
		userID: userID,
		body:   fmt.Sprintf(`{"data":{"schema_version":1,"payload":%s}}`, payload),
		headers: map[string]string{
			contract.IdempotencyKeyHeader: key,
			contract.IfMatchHeader:        ifMatch,
		}}
	status, _, resp := doV1(t, f.engine, call)
	return status, resp
}

func TestV1BoardCopyAcceptancePG(t *testing.T) {
	f := newV1MediaAccessPGFixture(t)
	ctx := context.Background()
	ws := f.ws.PublicID
	owner, member := f.owner, f.member

	// Owner uploads two assets: A will back the shared source board, B a
	// second source the member only sees transiently (laundering probe).
	assetA := pgAccessUpload(t, f, owner, "", "copy-acc-a")
	assetB := pgAccessUpload(t, f, owner, "", "copy-acc-b")

	mustPGCopyBoard := func(name string, creator int64) string {
		t.Helper()
		boardID, err := boards.NewV1BoardPublicID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.boards.CreateBoard(ctx, ws, creator, name, boardID); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return boardID
	}
	source := mustPGCopyBoard("Source", owner)
	source2 := mustPGCopyBoard("Source2", owner)

	// Owner accepts both assets on their boards (owner source authorizes).
	if status, resp := copyPutDoc(t, f, ws, source, copyDocPayload(assetA), "1", "copy-src-a", owner); status != 200 {
		t.Fatalf("owner accept A = %d %v, want 200", status, resp)
	}
	if status, resp := copyPutDoc(t, f, ws, source2, copyDocPayload(assetB), "1", "copy-src-b", owner); status != 200 {
		t.Fatalf("owner accept B = %d %v, want 200", status, resp)
	}

	// Owner shares both sources with the member as viewer (version 1->2).
	if _, err := f.boards.ReplaceBoardAccess(ctx, ws, source, owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{member}); err != nil {
		t.Fatalf("grant source: %v", err)
	}
	if _, err := f.boards.ReplaceBoardAccess(ctx, ws, source2, owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{member}); err != nil {
		t.Fatalf("grant source2: %v", err)
	}

	// Copy precondition: the source document reads for the member (this is
	// the server-side proof the copy flow requires before minting).
	status, _, srcDoc := doV1(t, f.engine, v1Call{method: http.MethodGet,
		path: "/api/v1/workspaces/" + ws + "/boards/" + source + "/document", userID: member})
	if status != 200 {
		t.Fatalf("member source read = %d %v, want 200", status, srcDoc)
	}
	srcData := v1Data(t, srcDoc).(map[string]any)

	// The member mints the copy name-only: new identity, baseline none,
	// version 1, no direct grants, no inherited ACL facts.
	status, _, created := doV1(t, f.engine, v1Call{method: http.MethodPost,
		path: "/api/v1/workspaces/" + ws + "/boards", userID: member,
		body:    `{"data":{"name":"Source Copy"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "copy-mint-1"}})
	if status != http.StatusCreated {
		t.Fatalf("copy create = %d %v, want 201", status, created)
	}
	copyData := v1Data(t, created).(map[string]any)
	copyID, _ := copyData["id"].(string)
	if copyID == "" || copyID == source {
		t.Fatalf("copy id = %q, want fresh identity", copyID)
	}
	if copyData["workspace_access"] != "none" {
		t.Fatalf("copy workspace_access = %v, want none", copyData["workspace_access"])
	}
	if copyData["access_version"] != float64(1) {
		t.Fatalf("copy access_version = %v, want 1", copyData["access_version"])
	}
	if copyData["access_state"] != "private" {
		t.Fatalf("copy access_state = %v, want private", copyData["access_state"])
	}
	if rel, _ := copyData["relationship"].(map[string]any); rel["has_direct_grant"] == true {
		t.Fatalf("copy relationship = %v, want no direct grant", rel)
	}
	members, err := f.boards.ListBoardMembers(ctx, ws, copyID)
	if err != nil {
		t.Fatalf("list copy members: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("copy members = %d, want 0 (no inherited grants)", len(members))
	}

	// The member accepts the source bytes onto the copy (asset A readable
	// through the still-shared source board): independent reference rows.
	// The GET envelope carries {schema_version, payload}; the accept must
	// carry the INNER payload, otherwise the snapshot would nest envelopes
	// and reconcile no references.
	inner, ok := srcData["document"].(map[string]any)["payload"]
	if !ok {
		t.Fatalf("source document has no inner payload: %v", srcData["document"])
	}
	srcPayload, err := json.Marshal(inner)
	if err != nil || len(srcPayload) == 0 {
		t.Fatalf("re-encode source payload: %v", err)
	}
	if status, resp := copyPutDoc(t, f, ws, copyID, string(srcPayload), "1", "copy-accept-1", member); status != 200 {
		t.Fatalf("copy accept = %d %v, want 200", status, resp)
	}
	refs, err := f.boards.ListBoardsReferencingAsset(ctx, ws, assetA)
	if err != nil {
		t.Fatalf("list refs A: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs A = %v, want source+copy", refs)
	}
	want := map[string]bool{source: true, copyID: true}
	for _, id := range refs {
		if !want[id] {
			t.Fatalf("refs A = %v, want source+copy", refs)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("refs A = %v, want source+copy", refs)
	}

	// Idempotent replay of the same accept replays revision 2, never a new
	// revision or duplicate reference rows.
	if status, resp := copyPutDoc(t, f, ws, copyID, string(srcPayload), "1", "copy-accept-1", member); status != 200 {
		t.Fatalf("copy accept replay = %d %v, want 200", status, resp)
	} else if rev := v1Data(t, resp).(map[string]any)["revision"]; rev != float64(2) {
		t.Fatalf("copy accept replay revision = %v, want 2", rev)
	}

	// Owner revokes the member from the source (direct-grant removal).
	if err := func() error {
		_, err := f.boards.ReplaceBoardAccess(ctx, ws, source, owner, 2, boards.WorkspaceAccessNone, nil)
		return err
	}(); err != nil {
		t.Fatalf("revoke source: %v", err)
	}

	// The source is gone for the member, but the copy keeps working on its
	// own authorization: document, asset metadata, and asset bytes metadata.
	status, _, forbidden := doV1(t, f.engine, v1Call{method: http.MethodGet,
		path: "/api/v1/workspaces/" + ws + "/boards/" + source + "/document", userID: member})
	if status != 404 {
		t.Fatalf("revoked source read = %d %v, want 404", status, forbidden)
	}
	status, _, copyDoc := doV1(t, f.engine, v1Call{method: http.MethodGet,
		path: "/api/v1/workspaces/" + ws + "/boards/" + copyID + "/document", userID: member})
	if status != 200 {
		t.Fatalf("copy read after revoke = %d %v, want 200", status, copyDoc)
	}
	status, _, meta := doV1(t, f.engine, v1Call{method: http.MethodGet,
		path: "/api/v1/workspaces/" + ws + "/media-assets/" + assetA, userID: member})
	if status != 200 {
		t.Fatalf("copy-asset metadata after revoke = %d %v, want 200", status, meta)
	}

	// Laundering probe: the member remembers asset B (seen via source2,
	// revoked below) and tries to link it into a board of their own. The
	// media gate fails closed with 404 and writes no reference.
	if err := func() error {
		_, err := f.boards.ReplaceBoardAccess(ctx, ws, source2, owner, 2, boards.WorkspaceAccessNone, nil)
		return err
	}(); err != nil {
		t.Fatalf("revoke source2: %v", err)
	}
	launder := mustPGCopyBoard("Launder", member)
	if status, resp := copyPutDoc(t, f, ws, launder, copyDocPayload(assetB), "1", "copy-launder-1", member); status != 404 {
		t.Fatalf("laundered accept = %d %v, want 404 media_not_found", status, resp)
	} else if code := v1ErrorCode(t, resp); code != string(contract.CodeMediaNotFound) {
		t.Fatalf("laundered accept code = %s, want media_not_found", code)
	}
	if refs, err := f.boards.ListBoardsReferencingAsset(ctx, ws, assetB); err != nil {
		t.Fatalf("list refs B: %v", err)
	} else if len(refs) != 1 || refs[0] != source2 {
		t.Fatalf("refs B = %v, want only source2 (no laundered row)", refs)
	}

	// Archiving the source never touches the copy's authorization either.
	if _, err := f.boards.ArchiveBoard(ctx, ws, source, owner); err != nil {
		t.Fatalf("archive source: %v", err)
	}
	status, _, copyDoc = doV1(t, f.engine, v1Call{method: http.MethodGet,
		path: "/api/v1/workspaces/" + ws + "/boards/" + copyID + "/document", userID: member})
	if status != 200 {
		t.Fatalf("copy read after source archive = %d %v, want 200", status, copyDoc)
	}
	status, _, meta = doV1(t, f.engine, v1Call{method: http.MethodGet,
		path: "/api/v1/workspaces/" + ws + "/media-assets/" + assetA, userID: member})
	if status != 200 {
		t.Fatalf("copy-asset metadata after source archive = %d %v, want 200", status, meta)
	}
}
