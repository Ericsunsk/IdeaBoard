package workspacetasks_test

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

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/tasking/workspacetasks"
)

type pgBoardAuthorizer struct {
	access *workspacetasks.BoardAccess
	err    error
}

func (f *pgBoardAuthorizer) ResolveBoardAccess(_ context.Context, _, _ string, _ int64, _ string) (*workspacetasks.BoardAccess, error) {
	return f.access, f.err
}

func pgBoardAccess(version int64) *workspacetasks.BoardAccess {
	return &workspacetasks.BoardAccess{
		BoardID: "board_pg_1", IsOwner: true,
		CanRead: true, CanGenerate: true, CanWrite: true,
		Archived: false, AccessVersion: version,
	}
}

func boardBoundService(env *atomicEnv, authorizer workspacetasks.BoardAuthorizer) *workspacetasks.Service {
	deps := env.serviceDeps
	deps.Boards = authorizer
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
		MediaAssetIDs: []string{}, BoardID: "board_pg_1", WorkspaceRole: "member",
	}, key)
	if err != nil {
		t.Fatalf("bound Create: %v", err)
	}
	_ = service.StagedEvents()
	if created.BoardID != "board_pg_1" || created.BoardAccessVersion != 7 {
		t.Fatalf("binding = (%q, %d), want (board_pg_1, 7)", created.BoardID, created.BoardAccessVersion)
	}
	// The envelope carries the binding in the same committed row.
	if boardID, version := boardEnvelope(t, env, created.ID); boardID != "board_pg_1" || version != 7 {
		t.Fatalf("envelope binding = (%q, %d), want (board_pg_1, 7)", boardID, version)
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
		BoardID: "board_hidden_1", WorkspaceRole: "member",
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
			BoardID: "board_pg_1", WorkspaceRole: "member",
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
	if completed.BoardID != "board_pg_1" {
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
				BoardID: "board_pg_1", WorkspaceRole: "member",
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
	if boardID, version := boardEnvelope(t, env, winner); boardID != "board_pg_1" || version != 5 {
		t.Fatalf("winner envelope = (%q, %d), want (board_pg_1, 5)", boardID, version)
	}
	// Same key with a different Board conflicts instead of forking.
	_, err := service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
		BoardID: "board_other_1", WorkspaceRole: "member",
	}, key)
	if !errors.Is(err, workspacetasks.ErrIdempotencyConflict) {
		t.Fatalf("same key different board err = %v, want ErrIdempotencyConflict", err)
	}
	if got := taskRowCount(t, env); got != 1 {
		t.Fatalf("tasks after conflict = %d, want still 1", got)
	}
	if !strings.HasPrefix(winner, "task_") {
		t.Fatalf("winner id = %q, want task_...", winner)
	}
}
