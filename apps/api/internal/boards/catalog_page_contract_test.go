package boards

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

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
			page, err := store.ListBoardCatalog(ctx, workspaceID, false, reader, "member", tc.request)
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
	archived, err := store.ListBoardCatalog(ctx, workspaceID, true, reader, "member", V1BoardCatalogPageRequest{Limit: 1})
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
	foreign, err := store.ListBoardCatalog(ctx, "ws_missing", true, owner, "owner", V1BoardCatalogPageRequest{Limit: 1})
	if err != nil || len(foreign.Entries) != 0 || foreign.HasMore {
		t.Fatalf("foreign page=%+v err=%v", foreign, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.ListBoardCatalog(canceled, workspaceID, false, reader, "member", V1BoardCatalogPageRequest{Limit: 1})
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
		page, err := store.ListBoardCatalog(ctx, workspaceID, false, owner, "owner", V1BoardCatalogPageRequest{Offset: offset, Limit: 100})
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
	page, err := store.ListBoardCatalog(ctx, workspaceID, false, owner, "owner", V1BoardCatalogPageRequest{Offset: 500, Limit: 1})
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
