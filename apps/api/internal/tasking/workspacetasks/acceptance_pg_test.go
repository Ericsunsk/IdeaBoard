package workspacetasks_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	librarydomain "ideaboard/api/internal/library/domain"
	librarypostgres "ideaboard/api/internal/library/infrastructure/postgres"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/tasking/workspacetasks"
	tenancypg "ideaboard/api/internal/tenancy/postgres"

	"github.com/jackc/pgx/v5"
)

type acceptanceEnv struct {
	*atomicEnv
	boards       *boards.PostgresWorkspaceBoardStore
	media        *workspacemedia.Service
	library      *librarypostgres.Service
	source       string
	destination  string
	privateAsset *workspacemedia.Asset
}

func newAcceptanceEnv(t *testing.T) *acceptanceEnv {
	t.Helper()
	env := openAtomicEnv(t, 10)
	ctx := t.Context()
	boardStore := boards.NewPostgresWorkspaceBoardStore(env.pool, workspacemedia.ValidateDocumentAssets)
	media := workspacemedia.NewService(env.pool)
	// The caller owns the Workspace, but not the source Board or asset.
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspace_memberships(workspace_id,user_id,role) SELECT internal_id,1,'member' FROM workspaces WHERE public_id=$1`, env.ws); err != nil {
		t.Fatal(err)
	}
	createBoard := func(owner int64) string {
		id, err := boards.NewV1BoardPublicID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := boardStore.CreateBoard(ctx, env.ws, owner, "Acceptance", id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	source := createBoard(1)
	destination := createBoard(env.userID)
	if _, err := boardStore.ReplaceBoardAccess(ctx, env.ws, source, 1, 1, boards.WorkspaceAccessEditor, []int64{env.userID}); err != nil {
		t.Fatal(err)
	}
	asset := acceptanceUpload(t, media, env.ws, 1, source)
	authorizer := &boardtaskaccess.Authorizer{Store: boardStore}
	library := librarypostgres.NewService(env.pool, &workspacemedia.LibraryPublicationAuthorizer{Boards: boardStore, TaskBoards: authorizer})
	env.serviceDeps.Acceptance = &workspacemedia.TaskSubmissionAuthorizer{Boards: boardStore, TaskBoards: authorizer}
	env.service = workspacetasks.NewService(env.serviceDeps)
	return &acceptanceEnv{atomicEnv: env, boards: boardStore, media: media, library: library, source: source, destination: destination, privateAsset: asset}
}

func acceptanceUpload(t *testing.T, media *workspacemedia.Service, workspaceID string, owner int64, target string) *workspacemedia.Asset {
	t.Helper()
	session, err := media.CreateSession(t.Context(), workspacemedia.CreateSessionParams{
		WorkspaceID: workspaceID, OwnerUserID: owner, TargetBoardPublicID: target,
		Filename: "acceptance.png", MIMEType: "image/png", MediaKind: workspacemedia.MediaKindImage,
		SizeBytes: 64, ChecksumSHA256: strings.Repeat("cd", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, asset, err := media.CompleteSession(t.Context(), workspaceID, session.ID, workspacemedia.CompleteFacts{
		SizeBytes: 64, ChecksumSHA256: strings.Repeat("cd", 32), MediaKind: workspacemedia.MediaKindImage, Width: 100, Height: 80,
	}, workspacemedia.CreateAssetParams{})
	if err != nil {
		t.Fatal(err)
	}
	return asset
}

func assertNoAcceptedTask(t *testing.T, env *acceptanceEnv, key string) {
	t.Helper()
	if count := taskRowCount(t, env.atomicEnv); count != 0 {
		t.Fatalf("denied submission wrote %d tasks", count)
	}
	if count := env.claimCount(t, key); count != 0 {
		t.Fatalf("denied submission retained %d claims", count)
	}
	if count := env.holdCount(t, key); count != 0 {
		t.Fatalf("denied submission created %d holds", count)
	}
	var jobs int
	if err := env.pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM settle_enqueue_probe`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("denied submission queued %d jobs", jobs)
	}
}

func waitForAcceptanceLock(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		var waiting bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pg_backend_pid()=ANY(pg_blocking_pids(pid)))`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("acceptance did not block on Workspace lock")
		}
	}
}

func TestTaskAcceptanceRejectsRetainedPrivateInputsAndMask(t *testing.T) {
	for _, bound := range []bool{false, true} {
		for _, mask := range []bool{false, true} {
			name := "boardless"
			if bound {
				name = "own-board"
			}
			if mask {
				name += "-mask"
			} else {
				name += "-input"
			}
			t.Run(name, func(t *testing.T) {
				env := newAcceptanceEnv(t)
				if _, err := env.boards.ReplaceBoardAccess(t.Context(), env.ws, env.source, 1, 2, boards.WorkspaceAccessNone, nil); err != nil {
					t.Fatal(err)
				}
				input := workspacetasks.CreateInput{WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render", Quantity: 1}
				if bound {
					input.BoardID = env.destination
				}
				if mask {
					input.Params = map[string]any{"mask_asset_id": env.privateAsset.ID}
				} else {
					input.MediaAssetIDs = []string{env.privateAsset.ID}
				}
				key := "retained-" + name
				if _, err := env.service.Create(t.Context(), input, key); !errors.Is(err, workspacetasks.ErrMediaNotFound) {
					t.Fatalf("retained private media accepted: %v", err)
				}
				assertNoAcceptedTask(t, env, key)
			})
		}
	}
}

func TestTaskAcceptanceRevocationWinsBeforeSettlement(t *testing.T) {
	env := newAcceptanceEnv(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, env.privateAsset.ID, env.userID, "owner", env.boards); err != nil {
		t.Fatalf("preflight must allow: %v", err)
	}
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := dbsqlc.New(tx).LockWorkspaceMemberships(ctx, env.ws); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	const key = "revoke-before-settlement"
	go func() {
		_, err := env.service.Create(ctx, workspacetasks.CreateInput{WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render", Quantity: 1, BoardID: env.destination, MediaAssetIDs: []string{env.privateAsset.ID}}, key)
		result <- err
	}()
	waitForAcceptanceLock(t, ctx, tx)
	if _, err := env.boards.ReplaceBoardAccess(dbutil.ContextWithAmbientTx(ctx, tx), env.ws, env.source, 1, 2, boards.WorkspaceAccessNone, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, workspacetasks.ErrMediaNotFound) {
		t.Fatalf("revoked input accepted: %v", err)
	}
	assertNoAcceptedTask(t, env, key)
}

func TestManualLibraryPublicationRejectsUnreadableOrdinaryUpload(t *testing.T) {
	env := newAcceptanceEnv(t)
	if _, err := env.boards.ReplaceBoardAccess(t.Context(), env.ws, env.source, 1, 2, boards.WorkspaceAccessNone, nil); err != nil {
		t.Fatal(err)
	}
	publicID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.library.SaveManagedMediaAsset(t.Context(), env.ws, librarydomain.V1SaveManagedAssetInput{MediaAssetID: env.privateAsset.ID, OperatorUserID: env.userID}, publicID); !errors.Is(err, librarydomain.ErrV1AssetNotFound) {
		t.Fatalf("unreadable ordinary upload published: %v", err)
	}
	assets, err := env.library.ListAssets(t.Context(), env.ws)
	if err != nil || len(assets) != 0 {
		t.Fatalf("denied publication created authority: %d,%v", len(assets), err)
	}
}

func TestManualLibraryPublicationMembershipRevocationWinsBeforeInsert(t *testing.T) {
	env := newAcceptanceEnv(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	// The asset owner is a current member and has valid read and publication rights.
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, env.privateAsset.ID, 1, "member", env.boards); err != nil {
		t.Fatal(err)
	}
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := dbsqlc.New(tx).LockWorkspaceMemberships(ctx, env.ws); err != nil {
		t.Fatal(err)
	}
	publicID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, _, err := env.library.SaveManagedMediaAsset(ctx, env.ws, librarydomain.V1SaveManagedAssetInput{MediaAssetID: env.privateAsset.ID, OperatorUserID: 1}, publicID)
		result <- err
	}()
	waitForAcceptanceLock(t, ctx, tx)
	if _, err := tenancypg.NewStore(env.pool).RemoveWorkspaceMember(dbutil.ContextWithAmbientTx(ctx, tx), env.ws, env.userID, 1, env.boards); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, librarydomain.ErrV1AssetNotFound) {
		t.Fatalf("revoked publication accepted: %v", err)
	}
	assets, err := env.library.ListAssets(ctx, env.ws)
	if err != nil || len(assets) != 0 {
		t.Fatalf("revoked publication created authority: %d,%v", len(assets), err)
	}
}

func TestTaskAcceptanceRejectsRevokedGenerateCapability(t *testing.T) {
	env := newAcceptanceEnv(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := dbsqlc.New(tx).LockWorkspaceMemberships(ctx, env.ws); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	const key = "revoke-bound-board"
	go func() {
		_, err := env.service.Create(ctx, workspacetasks.CreateInput{WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render", Quantity: 1, BoardID: env.source}, key)
		result <- err
	}()
	waitForAcceptanceLock(t, ctx, tx)
	if _, err := env.boards.ReplaceBoardAccess(dbutil.ContextWithAmbientTx(ctx, tx), env.ws, env.source, 1, 2, boards.WorkspaceAccessNone, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, workspacetasks.ErrBoardNotFound) {
		t.Fatalf("revoked Board accepted: %v", err)
	}
	assertNoAcceptedTask(t, env, key)
}

func TestBoardOutputPublicationRequiresProducingBoardCreator(t *testing.T) {
	env := newAcceptanceEnv(t)
	ctx := t.Context()
	output := acceptanceUpload(t, env.media, env.ws, env.userID, env.source)
	task, err := env.service.Create(ctx, workspacetasks.CreateInput{WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render", Quantity: 1, BoardID: env.source}, "editor-produced")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.Complete(ctx, env.ws, task.ID, []workspacetasks.OutputRef{{Kind: workspacetasks.OutputKindAsset, AssetID: output.ID, Role: "result", ResultIndex: 0}}); err != nil {
		t.Fatal(err)
	}
	publicID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.library.SaveManagedMediaAsset(ctx, env.ws, librarydomain.V1SaveManagedAssetInput{MediaAssetID: output.ID, OperatorUserID: env.userID}, publicID); !errors.Is(err, workspacetasks.ErrForbidden) {
		t.Fatalf("selected Editor published Board output via submitter/asset ownership: %v", err)
	}
	if _, _, err := env.library.SaveManagedMediaAsset(ctx, env.ws, librarydomain.V1SaveManagedAssetInput{MediaAssetID: output.ID, OperatorUserID: 1}, publicID); err != nil {
		t.Fatalf("producing Board Creator could not explicitly publish: %v", err)
	}
}

func TestOutputPublicationOriginSurvivesTaskDeletion(t *testing.T) {
	for _, bound := range []bool{true, false} {
		name := "boardless"
		if bound {
			name = "board-bound"
		}
		t.Run(name, func(t *testing.T) {
			env := newAcceptanceEnv(t)
			ctx := t.Context()
			input := workspacetasks.CreateInput{WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render", Quantity: 1}
			if bound {
				input.BoardID = env.source
			}
			output := acceptanceUpload(t, env.media, env.ws, env.userID, "")
			task, err := env.service.Create(ctx, input, "retained-output-origin")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := env.service.Complete(ctx, env.ws, task.ID, []workspacetasks.OutputRef{{Kind: workspacetasks.OutputKindAsset, AssetID: output.ID, Role: "result"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := env.pool.Exec(ctx, `DELETE FROM tasks WHERE public_id=$1`, task.ID); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_outputs`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("ephemeral outputs = %d,%v, want deleted", count, err)
			}
			publicID, err := librarydomain.NewV1AssetPublicID()
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = env.library.SaveManagedMediaAsset(ctx, env.ws, librarydomain.V1SaveManagedAssetInput{MediaAssetID: output.ID, OperatorUserID: env.userID}, publicID)
			if !bound {
				if err != nil {
					t.Fatalf("Boardless submitter lost publication after Task deletion: %v", err)
				}
				return
			}
			if !errors.Is(err, workspacetasks.ErrForbidden) {
				t.Fatalf("Editor published retained Board output: %v", err)
			}
			if _, _, err := env.library.SaveManagedMediaAsset(ctx, env.ws, librarydomain.V1SaveManagedAssetInput{MediaAssetID: output.ID, OperatorUserID: 1}, publicID); err != nil {
				t.Fatalf("Board Creator lost retained output publication: %v", err)
			}
		})
	}
}

func TestBoardDeletionDoesNotBroadenOutputPublication(t *testing.T) {
	env := newAcceptanceEnv(t)
	ctx := t.Context()
	output := acceptanceUpload(t, env.media, env.ws, env.userID, "")
	task, err := env.service.Create(ctx, workspacetasks.CreateInput{WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render", Quantity: 1, BoardID: env.source}, "deleted-board-output")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.Complete(ctx, env.ws, task.ID, []workspacetasks.OutputRef{{Kind: workspacetasks.OutputKindAsset, AssetID: output.ID, Role: "result"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `DELETE FROM tasks WHERE public_id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `DELETE FROM boards WHERE public_id=$1`, env.source); err != nil {
		t.Fatal(err)
	}
	publicID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.library.SaveManagedMediaAsset(ctx, env.ws, librarydomain.V1SaveManagedAssetInput{MediaAssetID: output.ID, OperatorUserID: env.userID}, publicID); !errors.Is(err, workspacetasks.ErrForbidden) {
		t.Fatalf("deleted Board output became an ordinary owned upload: %v", err)
	}
}
