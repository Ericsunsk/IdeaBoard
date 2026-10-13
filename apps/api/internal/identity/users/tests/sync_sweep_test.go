package tests

// Sweep wiring test (issue #888): the reconciler-level sweep entrypoint
// converges a committed intent through the Auth manager seam.

import (
	"context"
	"testing"

	"ideaboard/api/internal/identity/users/application/reconciliation"
)

type recordingRoleManager struct {
	roles   []string
	bans    int
	unbans  int
	roleErr error
	banErr  error
}

func (f *recordingRoleManager) DeleteUser(_ context.Context, _ string) error { return nil }

func (f *recordingRoleManager) SetUserRole(_ context.Context, _ string, role string) error {
	f.roles = append(f.roles, role)
	return f.roleErr
}

func (f *recordingRoleManager) BanUser(_ context.Context, _ string, _ int) error {
	f.bans++
	return f.banErr
}

func (f *recordingRoleManager) UnbanUser(_ context.Context, _ string) error {
	f.unbans++
	return f.banErr
}

func (f *recordingRoleManager) RevokeUserSessions(_ context.Context, _ string) error { return nil }

func TestReconcilerSweepAuthSyncConvergesCommittedIntent(t *testing.T) {
	pool := newUserRepoTestDB(t)
	ctx := context.Background()

	userID, _ := seedReconUser(t, pool, reconSeedUser{
		displayName: "sweep-sync", authUserID: reconAuthID("auth-sweep-sync"),
		storageUID: "SWSYNC", role: "user", status: StatusActive,
	})
	var intentID int64
	// Seed an already-due intent; TIMESTAMPTZ(3) can round now() past the sweep clock.
	if err := pool.QueryRow(ctx, `
INSERT INTO identity_auth_sync_intents (user_id, auth_user_id, op, desired_role, desired_banned, base_version, next_attempt_at)
VALUES ($1, 'auth-sweep-sync', 'role', 'user', false, 0, CURRENT_TIMESTAMP - INTERVAL '1 second')
RETURNING id`, userID).Scan(&intentID); err != nil {
		t.Fatalf("seed sync intent: %v", err)
	}

	manager := &recordingRoleManager{}
	reconciler := reconciliation.NewReconciler(reconciliation.ReconcilerDeps{
		DB:        pool,
		Users:     NewUserRepository(pool),
		Manager:   manager,
		Lifecycle: &recordingUserLifecycle{},
	})
	converged, err := reconciler.SweepAuthSync(ctx)
	if err != nil {
		t.Fatalf("sweep auth sync: %v", err)
	}
	if converged != 1 {
		t.Fatalf("converged = %d, want 1", converged)
	}
	if len(manager.roles) != 1 || manager.roles[0] != "user" {
		t.Fatalf("pushed roles = %v, want one user push", manager.roles)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM identity_auth_sync_intents WHERE id = $1`, intentID).Scan(&status); err != nil || status != "done" {
		t.Fatalf("intent status = %q, err = %v, want done", status, err)
	}
}
