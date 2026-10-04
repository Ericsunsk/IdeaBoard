package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/users/application/reconciliation"
	"ideaboard/api/internal/identity/users/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeAuthStateReader struct {
	states map[string]authidentity.AuthUserState
	err    error
	calls  [][]string
}

func (f *fakeAuthStateReader) GetUserStates(_ context.Context, ids []string) (map[string]authidentity.AuthUserState, error) {
	f.calls = append(f.calls, append([]string(nil), ids...))
	if f.err != nil {
		return nil, f.err
	}
	states := make(map[string]authidentity.AuthUserState, len(ids))
	for _, id := range ids {
		if state, ok := f.states[id]; ok {
			states[id] = state
		}
	}
	return states, nil
}

type fakeRunDispatcher struct {
	runIDs []int64
	err    error
}

func (f *fakeRunDispatcher) DispatchReconciliationRun(_ context.Context, _ pgx.Tx, runID int64) error {
	f.runIDs = append(f.runIDs, runID)
	return f.err
}

var _ ports.AuthUserStateReader = (*fakeAuthStateReader)(nil)

type reconSeedUser struct {
	displayName string
	authUserID  *string
	storageUID  string
	role        string
	status      int16
}

func reconAuthID(value string) *string { return &value }

func seedReconUser(t *testing.T, pool *pgxpool.Pool, user reconSeedUser) (id int64, publicID string) {
	t.Helper()
	var authUserID any
	if user.authUserID != nil {
		authUserID = *user.authUserID
	}
	err := pool.QueryRow(context.Background(), `
INSERT INTO users (display_name, auth_user_id, storage_uid, role, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, public_id::text`, user.displayName, authUserID, user.storageUID, user.role, user.status,
	).Scan(&id, &publicID)
	if err != nil {
		t.Fatalf("seed reconciliation user %q: %v", user.displayName, err)
	}
	return id, publicID
}

func reconAuthState(authUserID, role string, banned bool) authidentity.AuthUserState {
	return authidentity.AuthUserState{AuthUserID: authUserID, Role: role, Banned: banned}
}

func newTestReconciler(pool *pgxpool.Pool, auth ports.AuthUserStateReader, autoRemediate bool) *reconciliation.Reconciler {
	return reconciliation.NewReconciler(reconciliation.ReconcilerDeps{
		DB:            pool,
		Users:         NewUserRepository(pool),
		Auth:          auth,
		AutoRemediate: autoRemediate,
	})
}

func reconRunForSlot(t *testing.T, reconciler *reconciliation.Reconciler, dispatcher *fakeRunDispatcher, slot time.Time) int64 {
	t.Helper()
	run, err := reconciler.ScheduleReconciliation(context.Background(), dispatcher, slot, false)
	if err != nil {
		t.Fatalf("schedule reconciliation run: %v", err)
	}
	if run == nil || run.ID <= 0 {
		t.Fatalf("scheduled run = %+v, want a run ID", run)
	}
	if len(dispatcher.runIDs) != 1 || dispatcher.runIDs[0] != run.ID {
		t.Fatalf("dispatched runs = %v, want exactly the scheduled run", dispatcher.runIDs)
	}
	return run.ID
}

func mustReconUserRow(t *testing.T, pool *pgxpool.Pool, id int64) (role string, status int16) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `SELECT role, status FROM users WHERE id = $1`, id).Scan(&role, &status); err != nil {
		t.Fatalf("read user row %d: %v", id, err)
	}
	return role, status
}

// linkFixtureUsers gives every pre-existing (fixture) user a matching Auth
// state so only intentionally seeded drift produces findings. Each test
// schema is isolated, so touching fixture rows cannot leak across tests.
func linkFixtureUsers(t *testing.T, pool *pgxpool.Pool, auth *fakeAuthStateReader) {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT id, role, status FROM users WHERE auth_user_id IS NULL`)
	if err != nil {
		t.Fatalf("list unlinked users: %v", err)
	}
	type fixtureUser struct {
		id     int64
		role   string
		status int16
	}
	var fixtures []fixtureUser
	for rows.Next() {
		var user fixtureUser
		if err := rows.Scan(&user.id, &user.role, &user.status); err != nil {
			rows.Close()
			t.Fatalf("scan unlinked user: %v", err)
		}
		fixtures = append(fixtures, user)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("read unlinked users: %v", err)
	}
	for _, user := range fixtures {
		link := fmt.Sprintf("auth-fixture-user-%d", user.id)
		if _, err := pool.Exec(ctx, `UPDATE users SET auth_user_id = $1 WHERE id = $2`, link, user.id); err != nil {
			t.Fatalf("link fixture user %d: %v", user.id, err)
		}
		auth.states[link] = authidentity.AuthUserState{
			AuthUserID: link,
			Role:       user.role,
			Banned:     user.status == StatusBanned,
		}
	}
}
