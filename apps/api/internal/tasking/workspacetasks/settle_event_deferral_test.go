package workspacetasks

// Borrowed-transaction event deferral (#917): create, complete, fail, and
// cancel on a guard-borrowed settlement transaction stage nothing until
// the guard's post-commit hooks run. Publishing earlier would announce a
// transition the guard can still roll back. No database is required;
// the fake seams serve the settlement boundary in memory.

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/db/dbutil"
)

// borrowCtx stages a borrowed settlement transaction plus a post-commit
// registry, mirroring the Control guard scope without the guard.
func borrowCtx(ctx context.Context) context.Context {
	ctx = dbutil.ContextWithAmbientTx(ctx, &fakeTx{})
	return dbutil.ContextWithAmbientPostCommit(ctx)
}

const deferralChecksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func deferralRemoteOutput() []OutputRef {
	return []OutputRef{{Kind: OutputKindRemote, URL: "https://cdn.example.com/deferred-o.png", ChecksumSHA256: deferralChecksum, ResultIndex: 0}}
}

func TestBorrowedCreateDefersPendingEventUntilPostCommit(t *testing.T) {
	env := newTxBoardTestEnv(&fakeBoardAuthorizer{access: ownerBoardAccess()})
	ctx := borrowCtx(context.Background())

	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "borrowed-create-1")
	if err != nil {
		t.Fatalf("borrowed Create: %v", err)
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events before post-commit, want 0", len(staged))
	}
	dbutil.RunAmbientPostCommit(ctx)
	staged := env.service.StagedEvents()
	if len(staged) != 1 || staged[0].Status != StatusPending || staged[0].TaskID != created.ID {
		t.Fatalf("staged after post-commit = %+v, want one pending for %q", staged, created.ID)
	}
}

func TestBorrowedTerminalSettlementsDeferEventsUntilPostCommit(t *testing.T) {
	newLiveTask := func(t *testing.T) (*txBoardTestEnv, *WorkspaceTask) {
		t.Helper()
		env := newTxBoardTestEnv(&fakeBoardAuthorizer{access: ownerBoardAccess()})
		created, err := env.service.Create(context.Background(), CreateInput{
			WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
		}, "borrowed-live-"+strings.ReplaceAll(t.Name(), "/", "-"))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if staged := env.service.StagedEvents(); len(staged) != 1 {
			t.Fatalf("owned create staged %d events, want 1", len(staged))
		}
		return env, created
	}

	t.Run("complete", func(t *testing.T) {
		env, created := newLiveTask(t)
		ctx := borrowCtx(context.Background())
		if _, err := env.service.Complete(ctx, "ws_abc", created.ID, deferralRemoteOutput()); err != nil {
			t.Fatalf("borrowed Complete: %v", err)
		}
		if staged := env.service.StagedEvents(); len(staged) != 0 {
			t.Fatalf("staged %d events before post-commit, want 0", len(staged))
		}
		dbutil.RunAmbientPostCommit(ctx)
		staged := env.service.StagedEvents()
		if len(staged) != 1 || staged[0].Status != StatusCompleted {
			t.Fatalf("staged after post-commit = %+v, want one completed", staged)
		}
	})

	t.Run("fail", func(t *testing.T) {
		env, created := newLiveTask(t)
		ctx := borrowCtx(context.Background())
		if _, err := env.service.Fail(ctx, "ws_abc", created.ID, "boom"); err != nil {
			t.Fatalf("borrowed Fail: %v", err)
		}
		if staged := env.service.StagedEvents(); len(staged) != 0 {
			t.Fatalf("staged %d events before post-commit, want 0", len(staged))
		}
		dbutil.RunAmbientPostCommit(ctx)
		staged := env.service.StagedEvents()
		if len(staged) != 1 || staged[0].Status != StatusFailed {
			t.Fatalf("staged after post-commit = %+v, want one failed", staged)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		env, created := newLiveTask(t)
		ctx := borrowCtx(context.Background())
		if _, _, err := env.service.Cancel(ctx, "ws_abc", created.ID, 7, "stop"); err != nil {
			t.Fatalf("borrowed Cancel: %v", err)
		}
		if staged := env.service.StagedEvents(); len(staged) != 0 {
			t.Fatalf("staged %d events before post-commit, want 0", len(staged))
		}
		dbutil.RunAmbientPostCommit(ctx)
		staged := env.service.StagedEvents()
		if len(staged) != 1 || staged[0].Status != StatusCancelled {
			t.Fatalf("staged after post-commit = %+v, want one cancelled", staged)
		}
	})
}
