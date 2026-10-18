package workspacemedia

// Media asset authorization matrix (#936, in-memory twin).
//
// Pins the single-home decision on the memory store: owner always reads;
// non-members and foreign workspaces never read; members read only through
// an independent source (library seed, task seed, upload-target board, or
// a readable referencing board); losing one board's access never revokes
// an independent source; list filters before paginating.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/boards"
)

type stubBoardAccess struct {
	roles map[string]boards.AccessRole
	refs  map[string][]string
}

func (s *stubBoardAccess) GetBoardRole(_ context.Context, _, boardPublicID string, _ int64, _ string) (boards.AccessRole, bool, error) {
	role, ok := s.roles[strings.TrimSpace(boardPublicID)]
	if !ok {
		return "", false, nil
	}
	return role, true, nil
}

func (s *stubBoardAccess) ListBoardsReferencingAsset(_ context.Context, _, assetPublicID string) ([]string, error) {
	return append([]string(nil), s.refs[strings.TrimSpace(assetPublicID)]...), nil
}

func accessTestSession(t *testing.T, ctx context.Context, store *InMemoryWorkspaceMediaStore, ws string, owner int64, targetBoard string) *Session {
	t.Helper()
	session, err := store.CreateSession(ctx, CreateSessionParams{
		WorkspaceID:         ws,
		OwnerUserID:         owner,
		Filename:            "photo.png",
		MIMEType:            "image/png",
		SizeBytes:           64,
		ChecksumSHA256:      strings.Repeat("ab", 32),
		MediaKind:           MediaKindImage,
		TargetBoardPublicID: targetBoard,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return session
}

func accessTestComplete(t *testing.T, ctx context.Context, store *InMemoryWorkspaceMediaStore, ws, uploadID string) *Asset {
	t.Helper()
	_, asset, err := store.CompleteSession(ctx, ws, uploadID, CompleteFacts{
		ChecksumSHA256: strings.Repeat("ab", 32),
		SizeBytes:      64,
		MediaKind:      MediaKindImage,
	}, CreateAssetParams{})
	if err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}
	return asset
}

func TestAuthorizeAssetOwnerAndDenials(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceMediaStoreForTests()
	const ws, owner, member, stranger = "ws_acc1", 101, 102, 103
	_ = stranger
	session := accessTestSession(t, ctx, store, ws, owner, "")
	asset := accessTestComplete(t, ctx, store, ws, session.ID)
	access := &stubBoardAccess{}

	if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, owner, "member", access); err != nil {
		t.Fatalf("owner read = %v, want allow", err)
	}
	if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("member without source = %v, want ErrNotFound", err)
	}
	if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "outsider", access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-member read = %v, want ErrNotFound", err)
	}
	if _, err := store.AuthorizeAsset(ctx, "ws_other", asset.ID, owner, "member", access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign workspace read = %v, want ErrNotFound", err)
	}
	if _, err := store.AuthorizeAsset(ctx, ws, "asset_missing00000001", owner, "member", access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown asset read = %v, want ErrNotFound", err)
	}
	if _, err := store.AuthorizeAsset(ctx, ws, "nope", owner, "member", access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed asset read = %v, want ErrNotFound", err)
	}
}

func TestAuthorizeAssetIndependentSources(t *testing.T) {
	ctx := context.Background()
	const ws, owner, member = "ws_acc2", 101, 102

	t.Run("library", func(t *testing.T) {
		store := NewInMemoryWorkspaceMediaStoreForTests()
		asset := accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "").ID)
		access := &stubBoardAccess{}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); !errors.Is(err, ErrNotFound) {
			t.Fatalf("pre-library member = %v, want deny", err)
		}
		store.SeedLibraryLinkForTests(ws, asset.ID)
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); err != nil {
			t.Fatalf("library member = %v, want allow", err)
		}
	})

	t.Run("task", func(t *testing.T) {
		store := NewInMemoryWorkspaceMediaStoreForTests()
		asset := accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "").ID)
		const taskID, boardID = "task_abc00000000001", "board_task000000001"
		access := &stubBoardAccess{}
		store.SeedTaskAssetForTests(ws, taskID, owner, "", &TaskAsset{Asset: asset})
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); !errors.Is(err, ErrNotFound) {
			t.Fatalf("other submitter's Board-less output = %v, want deny", err)
		}
		store.SeedTaskAssetForTests(ws, taskID, member, "", &TaskAsset{Asset: asset})
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); err != nil {
			t.Fatalf("submitter = %v, want allow", err)
		}
		store.SeedTaskAssetForTests(ws, taskID, owner, boardID, &TaskAsset{Asset: asset})
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); !errors.Is(err, ErrNotFound) {
			t.Fatalf("private task output = %v, want deny", err)
		}
		access.roles = map[string]boards.AccessRole{boardID: boards.RoleViewer}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); err != nil {
			t.Fatalf("task Board reader = %v, want allow", err)
		}
		delete(access.roles, boardID)
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); !errors.Is(err, ErrNotFound) {
			t.Fatalf("revoked task Board reader = %v, want deny", err)
		}
	})

	t.Run("upload target", func(t *testing.T) {
		store := NewInMemoryWorkspaceMediaStoreForTests()
		asset := accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "board_target0000001").ID)
		// Target board readable: allow. Target board lost: deny (no other source).
		access := &stubBoardAccess{roles: map[string]boards.AccessRole{"board_target0000001": boards.RoleViewer}}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); err != nil {
			t.Fatalf("target-board member = %v, want allow", err)
		}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", &stubBoardAccess{}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("lost-target member = %v, want deny", err)
		}
	})

	t.Run("document reference", func(t *testing.T) {
		store := NewInMemoryWorkspaceMediaStoreForTests()
		asset := accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "").ID)
		access := &stubBoardAccess{
			refs:  map[string][]string{asset.ID: {"board_doc00000000001"}},
			roles: map[string]boards.AccessRole{"board_doc00000000001": boards.RoleViewer},
		}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", access); err != nil {
			t.Fatalf("document member = %v, want allow", err)
		}
		// Reference listed but board unreadable: deny.
		denied := &stubBoardAccess{refs: map[string][]string{asset.ID: {"board_doc00000000001"}}}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", denied); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unreadable-board member = %v, want deny", err)
		}
	})

	t.Run("multi-source independence", func(t *testing.T) {
		store := NewInMemoryWorkspaceMediaStoreForTests()
		asset := accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "board_target0000001").ID)
		store.SeedLibraryLinkForTests(ws, asset.ID)
		// Losing the target board must not revoke the library source.
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "member", &stubBoardAccess{}); err != nil {
			t.Fatalf("library after lost target = %v, want allow", err)
		}
	})
}

func TestListAuthorizedAssetsFiltersBeforePaginating(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryWorkspaceMediaStoreForTests()
	const ws, owner, member = "ws_acc3", 101, 102
	// Interleave owner-unauthorized and member-owned assets: the member's
	// page must contain only readable rows with stable newest-first order.
	var memberIDs []string
	for i := 0; i < 3; i++ {
		accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "").ID)
		memberIDs = append(memberIDs, accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, member, "").ID).ID)
	}
	access := &stubBoardAccess{}
	page, err := store.ListAuthorizedAssets(ctx, ws, member, "member", access, 2, 0)
	if err != nil {
		t.Fatalf("list = %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("page size = %d, want 2", len(page))
	}
	for _, asset := range page {
		if asset.OwnerUserID != member {
			t.Fatalf("page leaked owner asset %s", asset.ID)
		}
	}
	second, err := store.ListAuthorizedAssets(ctx, ws, member, "member", access, 2, 2)
	if err != nil {
		t.Fatalf("second page = %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second page size = %d, want 1", len(second))
	}
	seen := map[string]bool{page[0].ID: true, page[1].ID: true, second[0].ID: true}
	for _, id := range memberIDs {
		if !seen[id] {
			t.Fatalf("member asset %s missing from authorized pages", id)
		}
	}
	if _, err := store.ListAuthorizedAssets(ctx, ws, member, "outsider", access, 10, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-member list = %v, want ErrNotFound", err)
	}
}
