package workspacetasks_test

import (
	"context"
	"errors"
	"ideaboard/api/internal/boards"
	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	librarydomain "ideaboard/api/internal/library/domain"
	librarypostgres "ideaboard/api/internal/library/infrastructure/postgres"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/settlement"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Durable submit idempotency for task:create.
//
// Regression cover for the confirmed double-charge / double-image defect:
// the v1 edge's only dedup was the process-local
// platform/idempotency.Store, so a submit that committed and then died
// before the cache write (or lost its entry to the 4096-entry eviction)
// re-entered settlement with a freshly minted task id. The Credit Hold's
// ledger key was derived from that per-call id, so the ledger's
// pg_advisory_xact_lock and unique constraint could not fence it: the retry
// took a second hold, inserted a second task, and enqueued a second River
// job. Both tasks ran the provider and completed, so the user was charged
// twice and received two images for one 202.
//
// What is proven here, all against real PostgreSQL:
//   - Same key twice returns the SAME task: one task row, one hold, one
//     enqueue probe.
//   - Same key with a different body is a 409-class conflict and produces no
//     second task.
//   - The crash window itself: the durable path is exercised directly with
//     no process-local cache in the picture, and the second request still
//     replays the first task.
//   - Any failure inside the transaction rolls the durable claim back too,
//     so a failed submit never leaves an "accepted but no task" record that
//     would poison the key.
//
// Tests SKIP gracefully without API_TEST_DATABASE_DSN (house convention).

// claimCount counts durable submit claims in the fixture schema.
func (e *atomicEnv) claimCount(t *testing.T, key string) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM task_submission_idempotency c
		 INNER JOIN workspaces w ON w.internal_id = c.workspace_id
		 WHERE w.public_id = $1 AND c.operation = $2 AND c.idempotency_key = $3`,
		e.ws, workspacetasks.TaskSubmitOperation, key,
	).Scan(&count); err != nil {
		t.Fatalf("count submission claims: %v", err)
	}
	return count
}

func (e *atomicEnv) holdCount(t *testing.T, key string) int {
	t.Helper()
	return e.ledgerCount(t, `idempotency_key = $1`, settlement.TaskHoldKey(e.ws, key))
}

func (e *atomicEnv) submit(t *testing.T, key string, input workspacetasks.CreateInput) (*workspacetasks.WorkspaceTask, error) {
	t.Helper()
	input.WorkspaceID = e.ws
	input.OwnerUserID = e.userID
	if input.Service == "" {
		input.Service = "render"
	}
	if input.Resolution == "" {
		input.Resolution = "1k"
	}
	return e.service.Create(context.Background(), input, key)
}

// TestSubmitSameKeyReplaysSameTask proves the durable claim fences a retried
// submit: the second request returns the FIRST task, and the database holds
// exactly one task row, one hold, and one enqueue probe. Before the fix this
// produced a second task, a second hold (double charge), and a second River
// job (double image).
func TestSubmitSameKeyReplaysSameTask(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "replay-key-" + randomAtomicHex(t, 4)

	first, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	_ = env.service.StagedEvents()

	second, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("retried submit must succeed by replaying, got: %v", err)
	}
	_ = env.service.StagedEvents()

	if second.ID != first.ID {
		t.Fatalf("replay returned task %s, want the original %s", second.ID, first.ID)
	}
	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 1 {
		t.Fatalf("task rows = %d (err %v), want exactly 1", count, err)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("hold ledger rows = %d, want exactly 1 (retried submit must not double-charge)", got)
	}
	if got := env.probeCount(t, first.ID); got != 1 {
		t.Fatalf("enqueue probes = %d, want exactly 1 (retried submit must not enqueue a second job)", got)
	}
	if got := env.claimCount(t, key); got != 1 {
		t.Fatalf("durable claims = %d, want exactly 1", got)
	}
	// The wallet is the billing truth: reserved must reflect ONE hold.
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10): a replay must not reserve again", available, reserved)
	}
	// A replay must not re-announce the task: the winning submit already
	// emitted task.created for it.
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("replay staged %d events, want 0 (task.created must not re-emit)", len(staged))
	}
}

// TestSubmitSameKeyDifferentBodyConflicts proves a key reused with a
// different request body is a conflict, not a replay of the wrong task and
// not a second settlement. This is the case the process-local store also
// guarded, but only while the process lived.
func TestSubmitSameKeyDifferentBodyConflicts(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "conflict-key-" + randomAtomicHex(t, 4)

	first, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	_ = env.service.StagedEvents()

	// Same key, different body (quantity 2 bills differently).
	_, err = env.submit(t, key, workspacetasks.CreateInput{Quantity: 2})
	if !errors.Is(err, workspacetasks.ErrIdempotencyConflict) {
		t.Fatalf("same key + different body err = %v, want ErrIdempotencyConflict", err)
	}

	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 1 {
		t.Fatalf("task rows = %d (err %v), want 1: a conflict must not create a second task", count, err)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("hold ledger rows = %d, want 1: a conflict must not charge again", got)
	}
	if _, err := env.store.GetTask(context.Background(), env.ws, first.ID); err != nil {
		t.Fatalf("original task must survive the conflict: %v", err)
	}
}

// TestSubmitReplaySurvivesLostProcessCache reproduces the crash window that
// made the original defect reachable: the first submit commits, then the
// caller "dies" before any process-local cache is written. The retry must
// still be fenced by the DURABLE claim.
//
// The property under test is precisely that the answer comes from the
// database, not from memory: this file never consults
// platform/idempotency.Store, and the fixture service is rebuilt from the
// same pool so nothing process-local can bridge the two calls.
func TestSubmitReplaySurvivesLostProcessCache(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "crash-window-" + randomAtomicHex(t, 4)

	first, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	_ = env.service.StagedEvents()

	// Simulate the process losing every in-memory replay cache entry: build a
	// brand-new Service over the same database. A cache-backed fix would
	// miss here; the durable claim must not.
	restarted := workspacetasks.NewService(workspacetasks.ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.store, TaskBilling: env.store,
		Quoter: env.serviceDeps.Quoter, Holder: env.holder, Queue: env.queue,
		Resolver: allowTestPoolResolver(), SnapshotSaver: env.serviceDeps.SnapshotSaver, Acceptance: env.serviceDeps.Acceptance,
	})

	second, err := restarted.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, key)
	if err != nil {
		t.Fatalf("submit after cache loss must replay from the durable claim, got: %v", err)
	}
	_ = restarted.StagedEvents()

	if second.ID != first.ID {
		t.Fatalf("post-crash replay returned %s, want the original %s", second.ID, first.ID)
	}
	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 1 {
		t.Fatalf("task rows = %d (err %v), want 1", count, err)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("hold ledger rows = %d, want 1: the crash window must not double-charge", got)
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10)", available, reserved)
	}
}

// TestSubmitFailureRollsBackDurableClaim proves the claim is transactional,
// not a pre-write reservation. If settlement fails after the claim is taken,
// the claim must roll back with it — otherwise the key would be permanently
// poisoned with "accepted" but no task, and every retry would replay a task
// that does not exist.
func TestSubmitFailureRollsBackDurableClaim(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "rollback-key-" + randomAtomicHex(t, 4)

	// Fail the enqueue leg: the transaction is open and the claim is already
	// written when this trips.
	env.queue.failOnce(errors.New("river unavailable"))

	if _, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1}); err == nil {
		t.Fatal("submit with failing enqueue must fail")
	}

	if got := env.claimCount(t, key); got != 0 {
		t.Fatalf("durable claims = %d, want 0: the claim must roll back with the failed settlement", got)
	}
	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 0 {
		t.Fatalf("task rows = %d (err %v), want 0", count, err)
	}
	if got := env.holdCount(t, key); got != 0 {
		t.Fatalf("hold ledger rows = %d, want 0", got)
	}

	// The key must still be usable: a rolled-back claim must not poison it.
	created, err := env.submit(t, key, workspacetasks.CreateInput{Quantity: 1})
	if err != nil {
		t.Fatalf("retry after rolled-back claim must succeed, got: %v", err)
	}
	_ = env.service.StagedEvents()
	if got := env.claimCount(t, key); got != 1 {
		t.Fatalf("durable claims after retry = %d, want 1", got)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("hold ledger rows after retry = %d, want 1", got)
	}
	if created.ID == "" {
		t.Fatal("retry after rollback produced no task")
	}
}

// TestSubmitEmptyKeyFailsClosed proves the submit path cannot be unfenced.
// An empty key would mean "no fence at all", which is the defect class this
// seam removes, so it is rejected before any ledger, task, or queue write.
func TestSubmitEmptyKeyFailsClosed(t *testing.T) {
	env := openAtomicEnv(t, 10)

	if _, err := env.submit(t, "   ", workspacetasks.CreateInput{Quantity: 1}); err == nil {
		t.Fatal("empty idempotency key must be rejected, never settled unfenced")
	}
	if count, err := env.store.CountTasks(context.Background(), env.ws); err != nil || count != 0 {
		t.Fatalf("task rows = %d (err %v), want 0", count, err)
	}
	var claims int
	if err := env.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM task_submission_idempotency`).Scan(&claims); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("durable claims = %d, want 0", claims)
	}
}

// TestSubmitHoldKeyIsDurableNotTaskScoped pins the derivation rule that the
// original bug turned on: the Hold's ledger key must be a function of the
// client key, stable across attempts, and must NOT vary with the per-call
// task id (which is exactly what made a retry un-fenceable).
func TestSubmitHoldKeyIsDurableNotTaskScoped(t *testing.T) {
	key := TaskSubmitKeyFixture()
	first := settlement.TaskHoldKey("ws_alpha", key)
	second := settlement.TaskHoldKey("ws_alpha", key)

	if first != second {
		t.Fatalf("hold key is not stable for one client key: %q vs %q", first, second)
	}
	if other := settlement.TaskHoldKey("ws_beta", key); other == first {
		t.Fatal("hold key must be workspace-scoped: two workspaces shared a ledger key")
	}
	if other := settlement.TaskHoldKey("ws_alpha", key+"-other"); other == first {
		t.Fatal("hold key must vary with the client key")
	}
	// Must stay inside the ledger's 160-char bound.
	if len(first) > 160 {
		t.Fatalf("hold key length = %d, want <= 160 (ledger bound)", len(first))
	}
	if len(first) == len("task-hold:")+len("task_") {
		t.Fatal("hold key looks task-id derived")
	}
}

// TaskSubmitKeyFixture returns a stable opaque key for derivation assertions.
func TaskSubmitKeyFixture() string { return "client-key-fixture" }

// merged from acceptance_postgres_test.go (submission acceptance and board-bound submit aggregation over Service.Create).
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
	if _, err := tenancypg.NewStore(env.pool, nil).RemoveWorkspaceMember(dbutil.ContextWithAmbientTx(ctx, tx), env.ws, env.userID, 1, env.boards); err != nil {
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

// merged from board_binding_postgres_test.go (submission acceptance and board-bound submit aggregation over Service.Create).
// Board-bound submission and completion against real PostgreSQL (#937).
//
// What is proven here (skips without API_TEST_DATABASE_DSN):
//   - A Board-bound submit settles atomically: task row (with the durable
//     Board binding + accepted inputs in the workflow_input envelope),
//     durable idempotency claim, Credit Hold, and River enqueue probe
//     commit together.
//   - An invisible Board fails the submit BEFORE any Quote/Hold: no task
//     row, no hold, no probe.
//   - Duplicate completion converges: the second Complete is a state
//     conflict, outputs are stored once, and settlement stays atomic.
//   - Concurrent submits under one key settle exactly one task; a
//     concurrent submit with a different Board conflicts instead of
//     forking a second task.

type pgBoardAuthorizer struct {
	access *workspacetasks.BoardAccess
	err    error
}

func (f *pgBoardAuthorizer) ResolveBoardAccess(_ context.Context, _, _ string, _ int64, _ string) (*workspacetasks.BoardAccess, error) {
	return f.access, f.err
}

func pgBoardAccess(version int64) *workspacetasks.BoardAccess {
	return &workspacetasks.BoardAccess{
		BoardID: "board_postgres_1", IsOwner: true,
		CanRead: true, CanGenerate: true, CanWrite: true,
		Archived: false, AccessVersion: version,
	}
}

func boardBoundService(env *atomicEnv, authorizer workspacetasks.BoardAuthorizer) *workspacetasks.Service {
	deps := env.serviceDeps
	deps.Acceptance = &workspacemedia.TaskSubmissionAuthorizer{Boards: boards.NewPostgresWorkspaceBoardStore(env.pool, workspacemedia.ValidateDocumentAssets), TaskBoards: authorizer}
	return workspacetasks.NewService(deps)
}

func boardEnvelope(t *testing.T, env *atomicEnv, taskID string) (string, int64) {
	t.Helper()
	var boardID string
	var version int64
	if err := env.pool.QueryRow(context.Background(),
		`SELECT COALESCE(t.workflow_input ->> 'board_id', ''), COALESCE((t.workflow_input ->> 'board_access_version')::bigint, 0)
		 FROM tasks t INNER JOIN workspaces w ON w.internal_id = t.workspace_id
		 WHERE w.public_id = $1 AND t.public_id = $2`,
		env.ws, taskID,
	).Scan(&boardID, &version); err != nil {
		t.Fatalf("read task envelope: %v", err)
	}
	return boardID, version
}

func taskRowCount(t *testing.T, env *atomicEnv) int {
	t.Helper()
	var count int
	if err := env.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM tasks t INNER JOIN workspaces w ON w.internal_id = t.workspace_id WHERE w.public_id = $1`,
		env.ws,
	).Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	return count
}

func TestBoardBoundSubmitPersistsBindingAtomically(t *testing.T) {
	env := openAtomicEnv(t, 10)
	service := boardBoundService(env, &pgBoardAuthorizer{access: pgBoardAccess(7)})
	key := "board-pg-key-" + randomAtomicHex(t, 4)
	created, err := service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
		MediaAssetIDs: []string{}, BoardID: "board_postgres_1", WorkspaceRole: tenancy.RoleMember,
	}, key)
	if err != nil {
		t.Fatalf("bound Create: %v", err)
	}
	_ = service.StagedEvents()
	if created.BoardID != "board_postgres_1" || created.BoardAccessVersion != 7 {
		t.Fatalf("binding = (%q, %d), want (board_postgres_1, 7)", created.BoardID, created.BoardAccessVersion)
	}
	// The envelope carries the binding in the same committed row.
	if boardID, version := boardEnvelope(t, env, created.ID); boardID != "board_postgres_1" || version != 7 {
		t.Fatalf("envelope binding = (%q, %d), want (board_postgres_1, 7)", boardID, version)
	}
	// Atomicity: claim + hold + enqueue probe all committed with the row.
	if got := env.claimCount(t, key); got != 1 {
		t.Fatalf("claims = %d, want 1", got)
	}
	if got := env.holdCount(t, key); got != 1 {
		t.Fatalf("holds = %d, want 1", got)
	}
	if got := env.probeCount(t, created.ID); got != 1 {
		t.Fatalf("enqueue probes = %d, want 1", got)
	}
}

func TestBoardBoundSubmitInvisibleFailsBeforeHold(t *testing.T) {
	env := openAtomicEnv(t, 10)
	service := boardBoundService(env, &pgBoardAuthorizer{access: nil, err: workspacetasks.ErrBoardNotFound})
	key := "board-pg-hidden-" + randomAtomicHex(t, 4)
	_, err := service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
		BoardID: "board_hidden_1", WorkspaceRole: tenancy.RoleMember,
	}, key)
	if !errors.Is(err, workspacetasks.ErrBoardNotFound) {
		t.Fatalf("hidden Board err = %v, want ErrBoardNotFound", err)
	}
	_ = service.StagedEvents()
	if got := taskRowCount(t, env); got != 0 {
		t.Fatalf("tasks = %d, want 0 (rejected before insert)", got)
	}
	if got := env.holdCount(t, key); got != 0 {
		t.Fatalf("holds = %d, want 0 (rejected before Hold)", got)
	}
	if got := env.claimCount(t, key); got != 0 {
		t.Fatalf("claims = %d, want 0 (rejected before claim)", got)
	}
}

func TestDuplicateCompletionConvergesOnce(t *testing.T) {
	env := openAtomicEnv(t, 10)
	service := boardBoundService(env, &pgBoardAuthorizer{access: pgBoardAccess(3)})
	created := func() *workspacetasks.WorkspaceTask {
		key := "board-pg-dup-" + randomAtomicHex(t, 4)
		task, err := service.Create(context.Background(), workspacetasks.CreateInput{
			WorkspaceID: env.ws, OwnerUserID: env.userID,
			Service: "render", Quantity: 1, Resolution: "1k",
			BoardID: "board_postgres_1", WorkspaceRole: tenancy.RoleMember,
		}, key)
		if err != nil {
			t.Fatalf("bound Create: %v", err)
		}
		_ = service.StagedEvents()
		return task
	}()
	completed, err := service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	_ = service.StagedEvents()
	if completed.BoardID != "board_postgres_1" {
		t.Fatalf("completed binding = %q, want the bound Board", completed.BoardID)
	}
	// A redelivered completion converges as a conflict, never a second
	// settlement: outputs stay stored once.
	if _, err := service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); !errors.Is(err, workspacetasks.ErrStateConflict) {
		t.Fatalf("second Complete err = %v, want ErrStateConflict", err)
	}
	var outputCount int
	if err := env.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM task_outputs o INNER JOIN tasks t ON t.internal_id = o.task_id WHERE t.public_id = $1`,
		created.ID,
	).Scan(&outputCount); err != nil {
		t.Fatalf("count outputs: %v", err)
	}
	if outputCount != len(atomicRemoteOutput()) {
		t.Fatalf("outputs = %d, want %d (exactly-once completion)", outputCount, len(atomicRemoteOutput()))
	}
}

func TestConcurrentBoardSubmitsSettleOnce(t *testing.T) {
	env := openAtomicEnv(t, 10)
	service := boardBoundService(env, &pgBoardAuthorizer{access: pgBoardAccess(5)})
	key := "board-pg-race-" + randomAtomicHex(t, 4)
	const racers = 8
	ids := make([]string, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task, err := service.Create(context.Background(), workspacetasks.CreateInput{
				WorkspaceID: env.ws, OwnerUserID: env.userID,
				Service: "render", Quantity: 1, Resolution: "1k",
				BoardID: "board_postgres_1", WorkspaceRole: tenancy.RoleMember,
			}, key)
			if err == nil {
				ids[i] = task.ID
			} else {
				errs[i] = err
			}
		}(i)
	}
	wg.Wait()
	winner := ""
	for i := 0; i < racers; i++ {
		if errs[i] != nil {
			t.Fatalf("racer %d err = %v, want replay of the winner", i, errs[i])
		}
		if winner == "" {
			winner = ids[i]
		} else if ids[i] != winner {
			t.Fatalf("racer %d got %s, want the single winner %s", i, ids[i], winner)
		}
	}
	if got := taskRowCount(t, env); got != 1 {
		t.Fatalf("tasks = %d, want exactly 1", got)
	}
	if boardID, version := boardEnvelope(t, env, winner); boardID != "board_postgres_1" || version != 5 {
		t.Fatalf("winner envelope = (%q, %d), want (board_postgres_1, 5)", boardID, version)
	}
	// Same key with a different Board conflicts instead of forking.
	_, err := service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
		BoardID: "board_other_1", WorkspaceRole: tenancy.RoleMember,
	}, key)
	if !errors.Is(err, workspacetasks.ErrIdempotencyConflict) {
		t.Fatalf("same key different board err = %v, want ErrIdempotencyConflict", err)
	}
	if got := taskRowCount(t, env); got != 1 {
		t.Fatalf("tasks after conflict = %d, want still 1", got)
	}
	if !contract.IsTaskID(winner) {
		t.Fatalf("winner id = %q, want task_...", winner)
	}
}
