package httpapi

import (
	"context"
	"net/http"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/platform/contract"
)

const previewCatalogSchema4Body = `{"data":{"schema_version":4,"payload":{"imageNodes":[{"id":1,"x":10,"y":20,"width":280,"sourceImageRatio":1,"outputImages":[]}],"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}}}`

func putBoardDocument(t *testing.T, f *v1BoardsFixture, key, ifMatch, body string) (int, map[string]any) {
	t.Helper()
	call := v1Call{method: http.MethodPut, path: f.boardPath("/document"), userID: f.editor, body: body,
		headers: map[string]string{contract.IdempotencyKeyHeader: key, contract.IfMatchHeader: ifMatch}}
	status, _, payload := doV1(t, f.engine, call)
	return status, payload
}

func catalogCard(t *testing.T, f *v1BoardsFixture, boardID string) map[string]any {
	t.Helper()
	status, _, listed := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(""), userID: f.owner})
	if status != 200 {
		t.Fatalf("list = %d %v, want 200", status, listed)
	}
	for _, item := range listed["data"].([]any) {
		card := item.(map[string]any)
		if card["id"] == boardID {
			return card
		}
	}
	t.Fatalf("board %s missing from catalog", boardID)
	return nil
}

func TestV1BoardsCatalogCarriesScene(t *testing.T) {
	f := newV1BoardsFixture(t)

	// Before any document accept the card carries an identity but no scene.
	card := catalogCard(t, f, f.boardID)
	if _, ok := card["content_version"].(string); !ok {
		t.Fatalf("fresh card must carry a content version: %v", card)
	}
	if _, present := card["preview_scene"]; present {
		t.Fatalf("fresh schema-1 card must carry no scene: %v", card)
	}
	if _, present := card["document"]; present {
		t.Fatalf("catalog cards must never embed the document: %v", card)
	}

	status, putPayload := putBoardDocument(t, f, "pv-doc-1", "1", previewCatalogSchema4Body)
	if status != 200 {
		t.Fatalf("replace document = %d %v, want 200", status, putPayload)
	}
	docData := putPayload["data"].(map[string]any)
	docVersion, _ := docData["content_version"].(string)
	if docVersion == "" {
		t.Fatalf("document response must carry the content version: %v", docData)
	}
	docScene, ok := docData["preview_scene"].(map[string]any)
	if !ok || docScene["version"] != float64(1) {
		t.Fatalf("document response must carry the scene: %v", docData)
	}

	card = catalogCard(t, f, f.boardID)
	if card["content_version"] != docVersion {
		t.Fatalf("catalog version = %v, want %v", card["content_version"], docVersion)
	}
	cardScene, ok := card["preview_scene"].(map[string]any)
	if !ok {
		t.Fatalf("catalog card must carry the scene: %v", card)
	}
	nodes, ok := cardScene["nodes"].([]any)
	if !ok || len(nodes) != 1 || nodes[0].(map[string]any)["id"] != "image-1" {
		t.Fatalf("catalog scene nodes = %v, want one image-1", cardScene["nodes"])
	}
	if card["revision"] != float64(2) || card["schema_version"] != float64(4) {
		t.Fatalf("catalog card revision/schema = %v/%v, want 2/4", card["revision"], card["schema_version"])
	}

	// Rename preserves the projection; stale writes publish nothing.
	status, _ = boardIDEMKey(t, f.engine, http.MethodPatch, f.boardPath(""), f.owner, "", `{"data":{"name":"Renamed"}}`)
	if status != 200 {
		t.Fatalf("rename = %d, want 200", status)
	}
	if renamed := catalogCard(t, f, f.boardID); renamed["content_version"] != docVersion {
		t.Fatalf("rename changed the content version: %v", renamed["content_version"])
	}
	status, stalePayload := putBoardDocument(t, f, "pv-doc-stale", "1", previewCatalogSchema4Body)
	if status != 409 {
		t.Fatalf("stale write = %d %v, want 409", status, stalePayload)
	}
	if after := catalogCard(t, f, f.boardID); after["content_version"] != docVersion {
		t.Fatalf("conflict changed the content version: %v", after["content_version"])
	}
}

type catalogRoleReadCounter struct {
	boards.WorkspaceBoardStore
	roleReads int
}

func (s *catalogRoleReadCounter) GetBoardRole(ctx context.Context, workspaceID, boardID string, userID int64, workspaceRole string) (boards.AccessRole, bool, error) {
	s.roleReads++
	return s.WorkspaceBoardStore.GetBoardRole(ctx, workspaceID, boardID, userID, workspaceRole)
}

func TestV1BoardsCatalogDoesNotReadPerCardRoles(t *testing.T) {
	f := newV1BoardsFixture(t)
	store := &catalogRoleReadCounter{WorkspaceBoardStore: f.boards}
	f.engine = newV1BoardsTestEngine(f.store, store, f.dir)
	catalogCard(t, f, f.boardID)
	if store.roleReads != 0 {
		t.Fatalf("catalog read roles %d times; authorization must be part of the catalog read", store.roleReads)
	}
}

func TestV1BoardsCatalogPaginationValidation(t *testing.T) {
	f := newV1BoardsFixture(t)
	for _, tc := range []struct {
		name, query string
		status      int
		code        contract.Code
	}{
		{"default", "", http.StatusOK, ""},
		{"maximum", "?page_size=100", http.StatusOK, ""},
		{"zero", "?page_size=0", http.StatusBadRequest, contract.CodeBadRequest},
		{"too-large", "?page_size=101", http.StatusBadRequest, contract.CodeBadRequest},
		{"non-numeric", "?page_size=abc", http.StatusBadRequest, contract.CodeBadRequest},
		{"malformed-cursor", "?cursor=garbage", http.StatusBadRequest, contract.CodePaginationCursorInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(tc.query), userID: f.owner})
			if status != tc.status {
				t.Fatalf("status=%d want=%d payload=%v", status, tc.status, payload)
			}
			if tc.code != "" && v1ErrorCode(t, payload) != string(tc.code) {
				t.Fatalf("error=%v want=%s", payload, tc.code)
			}
		})
	}
	for _, offset := range []int{-1, 1, 2, 501} {
		cursor, err := contract.EncodeCursor(v1ListCursor{Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath("?cursor=" + cursor), userID: f.owner})
		if offset == 1 {
			if status != http.StatusOK || len(payload["data"].([]any)) != 0 || payload["meta"].(map[string]any)["has_more"] != false {
				t.Fatalf("empty final page=%d %v", status, payload)
			}
		} else if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodePaginationCursorInvalid) {
			t.Fatalf("offset=%d status=%d payload=%v", offset, status, payload)
		}
	}
}
