package v1

import (
	"context"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/platform/contract"
	"net/http"
	"strings"
	"testing"
)

func TestV1BoardRenameInvalidatesConditionalReads(t *testing.T) {
	for _, suffix := range []string{"", "/bundle"} {
		t.Run(suffix, func(t *testing.T) {
			f := newV1BoardsFixture(t)
			status, headers, _ := doV1RoundTrip(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(suffix), userID: f.owner}, "audit32-")
			if status != http.StatusOK {
				t.Fatal(status)
			}
			etag := headers.Get(contract.ETagHeader)
			renamed, err := f.boards.RenameBoard(context.Background(), f.workspace.PublicID, f.boardID, "Renamed", f.owner)
			if err != nil {
				t.Fatal(err)
			}
			status, _, body := doV1RoundTrip(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(suffix), userID: f.owner, headers: map[string]string{contract.IfNoneMatchHeader: etag}}, "audit32-")
			if status != http.StatusOK {
				t.Fatalf("status=%d body=%s", status, body)
			}
			if renamed.Revision != 1 {
				t.Fatalf("rename changed document revision: %d", renamed.Revision)
			}
			if !strings.Contains(body, `"name":"Renamed"`) {
				t.Fatalf("missing renamed metadata: %s", body)
			}
		})
	}
}

func TestV1BoardMetadataValidatorsCoverAccessAndArchive(t *testing.T) {
	for _, mutation := range []string{"access", "archive"} {
		t.Run(mutation, func(t *testing.T) {
			f := newV1BoardsFixture(t)
			ctx := context.Background()
			status, headers, _ := doV1RoundTrip(t, f.engine, v1Call{
				method: http.MethodGet, path: f.boardPath(""), userID: f.editor,
			}, "conditional-")
			if status != http.StatusOK {
				t.Fatal(status)
			}
			previousETag := headers.Get(contract.ETagHeader)
			board, err := f.boards.GetBoard(ctx, f.workspace.PublicID, f.boardID)
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "access" {
				_, err = f.boards.ReplaceBoardAccess(ctx, f.workspace.PublicID, f.boardID, f.owner, board.AccessVersion, boards.WorkspaceAccessViewer, []int64{f.editor})
			} else {
				_, err = f.boards.ArchiveBoard(ctx, f.workspace.PublicID, f.boardID, f.owner)
			}
			if err != nil {
				t.Fatal(err)
			}
			status, headers, body := doV1RoundTrip(t, f.engine, v1Call{
				method: http.MethodGet, path: f.boardPath(""), userID: f.editor,
				headers: map[string]string{contract.IfNoneMatchHeader: previousETag},
			}, "conditional-")
			if status != http.StatusOK || headers.Get(contract.ETagHeader) == previousETag || !strings.Contains(body, `"write":false`) {
				t.Fatalf("changed capabilities must invalidate metadata: status=%d headers=%v body=%s", status, headers, body)
			}
			// Metadata changes never advance the document's CAS version.
			status, _, _ = doV1RoundTrip(t, f.engine, v1Call{
				method: http.MethodGet, path: f.boardPath("/document"), userID: f.owner,
				headers: map[string]string{contract.IfNoneMatchHeader: "1"},
			}, "conditional-")
			if status != http.StatusNotModified {
				t.Fatalf("document revision changed: status=%d", status)
			}
		})
	}
}

func TestV1BoardConditionalReadsAuthorizeBeforeValidation(t *testing.T) {
	f := newV1BoardsFixture(t)
	for _, suffix := range []string{"", "/bundle", "/document"} {
		status, _, _ := doV1RoundTrip(t, f.engine, v1Call{
			method: http.MethodGet, path: f.boardPath(suffix), userID: f.outsider,
			headers: map[string]string{contract.IfNoneMatchHeader: "*"},
		}, "conditional-")
		if status != http.StatusNotFound {
			t.Fatalf("unauthorized %s = %d, want 404", suffix, status)
		}
	}
	// A different authorized caller has a different relationship/capability representation.
	_, headers, _ := doV1RoundTrip(t, f.engine, v1Call{
		method: http.MethodGet, path: f.boardPath(""), userID: f.owner,
	}, "conditional-")
	status, _, body := doV1RoundTrip(t, f.engine, v1Call{
		method: http.MethodGet, path: f.boardPath(""), userID: f.editor,
		headers: map[string]string{contract.IfNoneMatchHeader: headers.Get(contract.ETagHeader)},
	}, "conditional-")
	if status != http.StatusOK || !strings.Contains(body, `"is_creator":false`) {
		t.Fatalf("caller-specific read = %d %s", status, body)
	}
}
