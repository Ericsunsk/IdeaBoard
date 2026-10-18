package workspacemedia

import (
	"context"
	"encoding/json"
	"errors"
	"ideaboard/api/internal/boards"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Media asset authorization matrix (#936, in-memory twin).
//
// Pins the single-home decision on the memory store: owner always reads;
// non-members and foreign workspaces never read; members read only through
// an independent source (library seed, task seed, upload-target board, or
// a readable referencing board); losing one board's access never revokes
// an independent source; list filters before paginating.

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

	if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, owner, tenancy.RoleMember, access); err != nil {
		t.Fatalf("owner read = %v, want allow", err)
	}
	if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("member without source = %v, want ErrNotFound", err)
	}
	if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, "outsider", access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-member read = %v, want ErrNotFound", err)
	}
	if _, err := store.AuthorizeAsset(ctx, "ws_other", asset.ID, owner, tenancy.RoleMember, access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign workspace read = %v, want ErrNotFound", err)
	}
	if _, err := store.AuthorizeAsset(ctx, ws, "asset_missing00000001", owner, tenancy.RoleMember, access); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown asset read = %v, want ErrNotFound", err)
	}
	if _, err := store.AuthorizeAsset(ctx, ws, "nope", owner, tenancy.RoleMember, access); !errors.Is(err, ErrNotFound) {
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
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); !errors.Is(err, ErrNotFound) {
			t.Fatalf("pre-library member = %v, want deny", err)
		}
		store.SeedLibraryLinkForTests(ws, asset.ID)
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); err != nil {
			t.Fatalf("library member = %v, want allow", err)
		}
	})

	t.Run("task", func(t *testing.T) {
		store := NewInMemoryWorkspaceMediaStoreForTests()
		asset := accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "").ID)
		const taskID, boardID = "task_abc00000000001", "board_task000000001"
		access := &stubBoardAccess{}
		store.SeedTaskAssetForTests(ws, taskID, owner, "", &TaskAsset{Asset: asset})
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); !errors.Is(err, ErrNotFound) {
			t.Fatalf("other submitter's Board-less output = %v, want deny", err)
		}
		store.SeedTaskAssetForTests(ws, taskID, member, "", &TaskAsset{Asset: asset})
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); err != nil {
			t.Fatalf("submitter = %v, want allow", err)
		}
		store.SeedTaskAssetForTests(ws, taskID, owner, boardID, &TaskAsset{Asset: asset})
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); !errors.Is(err, ErrNotFound) {
			t.Fatalf("private task output = %v, want deny", err)
		}
		access.roles = map[string]boards.AccessRole{boardID: boards.RoleViewer}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); err != nil {
			t.Fatalf("task Board reader = %v, want allow", err)
		}
		delete(access.roles, boardID)
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); !errors.Is(err, ErrNotFound) {
			t.Fatalf("revoked task Board reader = %v, want deny", err)
		}
	})

	t.Run("upload target", func(t *testing.T) {
		store := NewInMemoryWorkspaceMediaStoreForTests()
		asset := accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "board_target0000001").ID)
		// Target board readable: allow. Target board lost: deny (no other source).
		access := &stubBoardAccess{roles: map[string]boards.AccessRole{"board_target0000001": boards.RoleViewer}}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); err != nil {
			t.Fatalf("target-board member = %v, want allow", err)
		}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, &stubBoardAccess{}); !errors.Is(err, ErrNotFound) {
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
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, access); err != nil {
			t.Fatalf("document member = %v, want allow", err)
		}
		// Reference listed but board unreadable: deny.
		denied := &stubBoardAccess{refs: map[string][]string{asset.ID: {"board_doc00000000001"}}}
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, denied); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unreadable-board member = %v, want deny", err)
		}
	})

	t.Run("multi-source independence", func(t *testing.T) {
		store := NewInMemoryWorkspaceMediaStoreForTests()
		asset := accessTestComplete(t, ctx, store, ws, accessTestSession(t, ctx, store, ws, owner, "board_target0000001").ID)
		store.SeedLibraryLinkForTests(ws, asset.ID)
		// Losing the target board must not revoke the library source.
		if _, err := store.AuthorizeAsset(ctx, ws, asset.ID, member, tenancy.RoleMember, &stubBoardAccess{}); err != nil {
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
	page, err := store.ListAuthorizedAssets(ctx, ws, member, tenancy.RoleMember, access, 2, 0)
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
	second, err := store.ListAuthorizedAssets(ctx, ws, member, tenancy.RoleMember, access, 2, 2)
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

// merged from access_postgres_test.go (access.go aggregation: postgres asset-authorization matrix over real SQL facts).
// PostgreSQL asset authorization (#936): the same decision matrix against
// the real SQL facts — owner, upload-target window, document references
// via the REAL boards store through the narrow port, library rows, task
// outputs, multi-source independence, list filtering, and reference GC.
// Skips without API_TEST_DATABASE_DSN.

type accessPGEnv struct {
	media  *Service
	boards *boards.PostgresWorkspaceBoardStore
	ws     string
	owner  int64
	member int64
}

func openAccessPGEnv(t *testing.T) (*accessPGEnv, context.Context, *pgxpool.Pool) {
	t.Helper()
	db := testdb.Open(t)
	ctx := context.Background()
	tenancyStore := tenancypg.NewStore(db)
	ws, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "media-access-pg",
		OwnerUserID: 1,
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := tenancyStore.AddWorkspaceMember(ctx, ws.PublicID, 10, tenancy.RoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	return &accessPGEnv{
		media:  NewService(db),
		boards: boards.NewPostgresWorkspaceBoardStore(db, ValidateDocumentAssets),
		ws:     ws.PublicID,
		owner:  1,
		member: 10,
	}, ctx, db
}

func pgUploadComplete(t *testing.T, ctx context.Context, svc *Service, ws string, owner int64, targetBoard string) *Asset {
	t.Helper()
	session, err := svc.CreateSession(ctx, CreateSessionParams{
		WorkspaceID:         ws,
		OwnerUserID:         owner,
		Filename:            "photo.png",
		MIMEType:            "image/png",
		SizeBytes:           64,
		ChecksumSHA256:      strings.Repeat("cd", 32),
		MediaKind:           MediaKindImage,
		TargetBoardPublicID: targetBoard,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_, asset, err := svc.CompleteSession(ctx, ws, session.ID, CompleteFacts{
		ChecksumSHA256: strings.Repeat("cd", 32),
		SizeBytes:      64,
		MediaKind:      MediaKindImage,
		Width:          100,
		Height:         80,
	}, CreateAssetParams{})
	if err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}
	return asset
}

func pgDocEnvelope(t *testing.T, assetIDs ...string) json.RawMessage {
	t.Helper()
	nodes := make([]map[string]any, 0, len(assetIDs))
	for i, id := range assetIDs {
		nodes = append(nodes, map[string]any{
			"id": float64(i + 1), "x": 0, "y": 0, "width": 280,
			"sourceImageUrl": "https://example.com/y.png",
			"sourceMediaRef": map[string]any{"assetId": id},
			"outputImages":   []any{},
		})
	}
	payload, err := json.Marshal(map[string]any{
		"schemaVersion": 4, "imageNodes": nodes, "promptNodes": []any{}, "fusionNodes": []any{},
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

func TestAccessPostgresOwnerAndDeny(t *testing.T) {
	env, ctx, _ := openAccessPGEnv(t)
	asset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.owner, tenancy.RoleOwner, env.boards); err != nil {
		t.Fatalf("owner = %v, want allow", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("member without source = %v, want ErrNotFound", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, 8, "", env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-member = %v, want ErrNotFound", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, "ws_foreign0000000001", asset.ID, env.owner, tenancy.RoleOwner, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign workspace = %v, want ErrNotFound", err)
	}
}

func TestAccessPostgresUploadTargetWindow(t *testing.T) {
	env, ctx, _ := openAccessPGEnv(t)
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.boards.CreateBoard(ctx, env.ws, env.owner, "target", boardID); err != nil {
		t.Fatalf("create board: %v", err)
	}
	// Bound BEFORE any document exists: a board reader already reads the
	// bytes (no workspace-readable window for others, no doc needed).
	asset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, boardID)
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-reader member = %v, want deny", err)
	}
	if _, err := env.boards.ReplaceBoardAccess(ctx, env.ws, boardID, env.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{env.member}); err != nil {
		t.Fatalf("grant viewer: %v", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); err != nil {
		t.Fatalf("board reader = %v, want allow via upload target", err)
	}
	// Losing the board grant revokes the target source live (no row touched).
	if err := func() error {
		_, err := env.boards.ReplaceBoardAccess(ctx, env.ws, boardID, env.owner, 2, boards.WorkspaceAccessNone, nil)
		return err
	}(); err != nil {
		t.Fatalf("remove grant: %v", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after grant loss = %v, want deny", err)
	}
	// Forged bindings fail closed at session create.
	if _, err := env.media.CreateSession(ctx, CreateSessionParams{
		WorkspaceID: env.ws, OwnerUserID: env.owner, Filename: "x.png",
		MIMEType: "image/png", SizeBytes: 64, ChecksumSHA256: strings.Repeat("cd", 32),
		MediaKind:           MediaKindImage,
		TargetBoardPublicID: "board_forged000000001",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forged target create = %v, want ErrNotFound", err)
	}
}

func TestAccessPostgresDocumentLifecycle(t *testing.T) {
	env, ctx, _ := openAccessPGEnv(t)
	asset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.boards.CreateBoard(ctx, env.ws, env.owner, "doc", boardID); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if _, err := env.boards.ReplaceBoardAccess(ctx, env.ws, boardID, env.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{env.member}); err != nil {
		t.Fatalf("grant viewer: %v", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pre-accept member = %v, want deny", err)
	}
	if _, err := env.boards.ReplaceDocument(ctx, env.ws, boardID, env.owner, 1, 1, pgDocEnvelope(t, asset.ID)); err != nil {
		t.Fatalf("accept with ref: %v", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); err != nil {
		t.Fatalf("post-accept member = %v, want allow", err)
	}
	// GC: removing the reference in a later accept revokes the source in
	// the same transaction; the owner source survives.
	if _, err := env.boards.ReplaceDocument(ctx, env.ws, boardID, env.owner, 2, 1, pgDocEnvelope(t)); err != nil {
		t.Fatalf("accept without ref: %v", err)
	}
	refs, err := env.boards.ListBoardsReferencingAsset(ctx, env.ws, asset.ID)
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("refs after removal = %v, want none", refs)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("post-removal member = %v, want deny", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.owner, tenancy.RoleOwner, env.boards); err != nil {
		t.Fatalf("owner after removal = %v, want allow", err)
	}
}

func TestAccessPostgresLibraryAndTaskSources(t *testing.T) {
	env, ctx, pool := openAccessPGEnv(t)
	asset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pre-source member = %v, want deny", err)
	}
	// Published library row: folder + media-linked asset. The 'Quick Saves'
	// literal mirrors library/domain.QuickSavesFolderName (durable seam in
	// db/queries/workspace_library.sql); this fixture keeps raw SQL so the
	// seed stays independent of the library package.
	var folderInternal int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO library_folders (public_id, workspace_id, name, is_system, is_default)
		SELECT $1, w.internal_id, 'Quick Saves', TRUE, TRUE FROM workspaces AS w WHERE w.public_id=$2
		RETURNING internal_id`, "lfolder_pg00000000001", env.ws,
	).Scan(&folderInternal); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO library_assets (public_id, workspace_id, folder_id, media_asset_id)
		SELECT $1, w.internal_id, $2, m.internal_id FROM workspaces AS w, media_assets AS m
		WHERE w.public_id=$3 AND m.public_id=$4`,
		"lasset_pg000000000001", folderInternal, env.ws, asset.ID,
	); err != nil {
		t.Fatalf("seed library asset: %v", err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); err != nil {
		t.Fatalf("library member = %v, want allow", err)
	}

	// Task output visibility follows its submitter and live Board access.
	asset2 := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	if _, err := pool.Exec(ctx,
		`INSERT INTO billing_accounts (public_id, workspace_id)
		SELECT $1, w.internal_id FROM workspaces AS w WHERE w.public_id=$2`,
		"ba_pg0000000000000001", env.ws,
	); err != nil {
		t.Fatalf("seed billing account: %v", err)
	}
	pgAcceptOutputOrigin(t, pool, env, "task_pg00000000000001", env.owner, "", asset2)
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset2.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other submitter's Board-less output = %v, want deny", err)
	}
	asset2 = pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	pgAcceptOutputOrigin(t, pool, env, "task_pg00000000000002", env.member, "", asset2)
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset2.ID, env.member, tenancy.RoleMember, env.boards); err != nil {
		t.Fatalf("submitter = %v, want allow", err)
	}
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.boards.CreateBoard(ctx, env.ws, env.owner, "task", boardID); err != nil {
		t.Fatal(err)
	}
	asset2 = pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	pgAcceptOutputOrigin(t, pool, env, "task_pg00000000000003", env.owner, boardID, asset2)
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset2.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private Board output = %v, want deny", err)
	}
	if _, err := env.boards.ReplaceBoardAccess(ctx, env.ws, boardID, env.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{env.member}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset2.ID, env.member, tenancy.RoleMember, env.boards); err != nil {
		t.Fatalf("task Board reader = %v, want allow", err)
	}
	if _, err := env.media.ArchiveAsset(ctx, env.ws, asset2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset2.ID, env.member, tenancy.RoleMember, env.boards); err != nil {
		t.Fatalf("archived task output metadata = %v, want allow", err)
	}
	if err := func() error {
		_, err := env.boards.ReplaceBoardAccess(ctx, env.ws, boardID, env.owner, 2, boards.WorkspaceAccessNone, nil)
		return err
	}(); err != nil {
		t.Fatal(err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset2.ID, env.member, tenancy.RoleMember, env.boards); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked task Board reader = %v, want deny", err)
	}
}

func pgAcceptOutputOrigin(t *testing.T, pool *pgxpool.Pool, env *accessPGEnv, taskID string, submitter int64, boardID string, asset *Asset) {
	t.Helper()
	ctx := t.Context()
	if _, err := pool.Exec(ctx,
		`INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input)
  SELECT $1, w.internal_id, $2, b.internal_id, 'generation', jsonb_build_object('board_id', $5::text)
  FROM workspaces AS w INNER JOIN billing_accounts AS b ON b.workspace_id=w.internal_id
  WHERE w.public_id=$3 AND b.public_id=$4`,
		taskID, submitter, env.ws, "ba_pg0000000000000001", boardID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := dbsqlc.New(pool).InsertWorkspaceTaskOutput(ctx, dbsqlc.InsertWorkspaceTaskOutputParams{
		PublicID: taskID, WorkspacePublicID: env.ws, OutputKind: "media_asset",
		MediaAssetID: pgtype.Int8{Int64: asset.InternalID, Valid: true}, Metadata: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAccessPostgresListFilters(t *testing.T) {
	env, ctx, _ := openAccessPGEnv(t)
	mine := pgUploadComplete(t, ctx, env.media, env.ws, env.member, "")
	pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	page, err := env.media.ListAuthorizedAssets(ctx, env.ws, env.member, tenancy.RoleMember, env.boards, 10, 0)
	if err != nil {
		t.Fatalf("list = %v", err)
	}
	if len(page) != 1 || page[0].ID != mine.ID {
		t.Fatalf("member list = %v, want only own asset", page)
	}
	all, err := env.media.ListAuthorizedAssets(ctx, env.ws, env.owner, tenancy.RoleOwner, env.boards, 10, 0)
	if err != nil {
		t.Fatalf("owner list = %v", err)
	}
	// Owner reads own asset; the member's asset stays hidden.
	if len(all) != 1 {
		t.Fatalf("owner list size = %d, want 1", len(all))
	}
	for _, a := range all {
		if a.ID == mine.ID {
			t.Fatalf("owner list leaked member asset %s", a.ID)
		}
	}
}

func TestAccessPostgresExistingDocumentReferencesBackfill(t *testing.T) {
	env, ctx, pool := openAccessPGEnv(t)
	assets := make([]*Asset, 6)
	for i := range assets {
		assets[i] = pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	}
	ref := func(i int) map[string]any { return map[string]any{"assetId": assets[i].ID} }
	payload, err := json.Marshal(map[string]any{
		"schemaVersion": 4,
		"imageNodes":    []any{map[string]any{"sourceMediaRef": ref(0), "outputImages": []any{map[string]any{"mediaRef": ref(1)}}}},
		"promptNodes":   []any{map[string]any{"inputImages": []any{map[string]any{"mediaRef": ref(2)}}, "prompt": assets[5].ID}},
		"fusionNodes":   []any{map[string]any{"inputImages": []any{map[string]any{"mediaRef": ref(3)}}, "outputImages": []any{map[string]any{"mediaRef": ref(4)}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.boards.CreateBoard(ctx, env.ws, env.owner, "existing", boardID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.boards.ReplaceBoardAccess(ctx, env.ws, boardID, env.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{env.member}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.boards.ReplaceDocument(ctx, env.ws, boardID, env.owner, 1, 1, pgDocEnvelope(t)); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "20261017120000_board_media_access.sql"))
	if err != nil {
		t.Fatal(err)
	}
	_, backfill, ok := strings.Cut(string(migration), "WITH documents AS (")
	if !ok {
		t.Fatal("reference backfill statement missing")
	}
	envelope, err := json.Marshal(map[string]any{"schema_version": 1, "payload": json.RawMessage(payload)})
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range [][]byte{envelope, payload} {
		if _, err := pool.Exec(ctx, `UPDATE board_documents SET snapshot_data=$1::jsonb`, snapshot); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM board_document_media_refs`); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if _, err := pool.Exec(ctx, "WITH documents AS ("+backfill); err != nil {
				t.Fatal(err)
			}
		}
		for i, asset := range assets {
			_, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards)
			if i < 5 && err != nil {
				t.Fatalf("typed reference %d = %v, want allow", i, err)
			}
			if i == 5 && !errors.Is(err, ErrNotFound) {
				t.Fatalf("free-text reference = %v, want deny", err)
			}
		}
	}
}
