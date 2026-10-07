package tests

// Durable Auth sync tests (issue #888): real PostgreSQL transactions +
// fake Auth manager. Each test proves one acceptance item: business-first
// atomic commit with a durable intent, no request-path Auth I/O, retry
// convergence after Auth/database failures, crash recovery, same-user
// serialization/version fencing, and fail-closed demotion/ban.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/users/application/authsync"
	identitycreditsadapter "ideaboard/api/internal/identity/users/infrastructure/credits"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type durableSyncFixture struct {
	pool     *pgxpool.Pool
	commands IdentityAdminCommands
	sync     *authsync.Service
	auth     *fakeIdentityAdminAuth
}

func newDurableSyncFixture(t *testing.T) *durableSyncFixture {
	t.Helper()
	pool := newUserRepoTestDB(t)
	auth := &fakeIdentityAdminAuth{events: &[]string{}}
	repo := NewUserRepository(pool)
	creditService := credits.NewService(pool)
	lifecycle := NewLifecycleService(pool, identitycreditsadapter.New(creditService), []byte("0123456789abcdef0123456789abcdef"))
	userService := NewUserService(repo, lifecycle, &spyUserCreditBalanceReader{balances: map[int64]int64{}})
	store := authsync.NewStore(pool)
	return &durableSyncFixture{
		pool:     pool,
		commands: newIdentityAdminCommands(userService, auth, store),
		sync:     authsync.NewService(authsync.ServiceDeps{Store: store, Auth: auth, Lifecycle: lifecycle}),
		auth:     auth,
	}
}

func (f *durableSyncFixture) seedUser(t *testing.T, display, authID, storageUID, role string, status int16) (int64, string) {
	t.Helper()
	ctx := context.Background()
	repo := NewUserRepository(f.pool)
	user := &User{DisplayName: display, Role: role, Status: status, StorageUID: storageUID}
	if authID != "" {
		user.AuthUserID = &authID
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("seed user %q: %v", display, err)
	}
	creditService := credits.NewService(f.pool)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin wallet transaction: %v", err)
	}
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(ctx, user.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("provision wallet: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit wallet transaction: %v", err)
	}
	return user.ID, user.PublicID.String()
}

func durableUserRow(t *testing.T, pool *pgxpool.Pool, userID int64) (role string, status int16, version int64) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `SELECT role, status, auth_sync_version FROM users WHERE id = $1`, userID).Scan(&role, &status, &version); err != nil {
		t.Fatalf("read user row %d: %v", userID, err)
	}
	return role, status, version
}

func durableIntentCount(t *testing.T, pool *pgxpool.Pool, userID int64, status string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM identity_auth_sync_intents WHERE user_id = $1 AND status = $2`, userID, status).Scan(&count); err != nil {
		t.Fatalf("count intents: %v", err)
	}
	return count
}

func durableWalletStatus(t *testing.T, pool *pgxpool.Pool, userID int64) creditsdomain.WalletStatus {
	t.Helper()
	wallet, err := credits.NewService(pool).GetWallet(context.Background(), userID)
	if err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	return wallet.Status
}

func TestDurablePromoteConvergesAfterCommit(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-promote", "auth-durable-promote", "DPRM01", "user", StatusActive)

	// Request path: business commits, no Auth I/O happens here.
	outcome, err := f.commands.SetRole(ctx, identityAdminActor(t), publicID, "admin")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !outcome.SyncPending || outcome.IntentID <= 0 {
		t.Fatalf("outcome = %+v, want SyncPending with an intent", outcome)
	}
	if role, _, version := durableUserRow(t, f.pool, userID); role != "admin" || version != 1 {
		t.Fatalf("business role = %q version = %d, want admin/1 committed", role, version)
	}
	if len(*f.auth.events) != 0 {
		t.Fatalf("auth events = %v, want no request-path Auth I/O", *f.auth.events)
	}

	// Sweep converges the committed intent.
	converged, err := f.sync.ConvergeDue(ctx)
	if err != nil {
		t.Fatalf("converge: %v", err)
	}
	if converged != 1 {
		t.Fatalf("converged = %d, want 1", converged)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-role:admin")
	if got := durableIntentCount(t, f.pool, userID, "done"); got != 1 {
		t.Fatalf("done intents = %d, want 1", got)
	}

	// A second sweep is a no-op: converged state stays silent.
	converged, err = f.sync.ConvergeDue(ctx)
	if err != nil {
		t.Fatalf("second converge: %v", err)
	}
	if converged != 0 {
		t.Fatalf("second converged = %d, want 0", converged)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-role:admin")
}

func TestDurableDemoteIsFailClosedOnAuthOutage(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-demote", "auth-durable-demote", "DDRM01", "admin", StatusActive)

	// The business demotion commits even though Auth is down: access is
	// denied first (fail-closed), convergence follows by retry.
	f.auth.roleErrors = []error{errors.New("auth service unavailable")}
	outcome, err := f.commands.SetRole(ctx, identityAdminActor(t), publicID, "user")
	if err != nil {
		t.Fatalf("demote: %v", err)
	}
	if !outcome.SyncPending {
		t.Fatal("demote outcome is not sync-pending")
	}
	if role, _, _ := durableUserRow(t, f.pool, userID); role != "user" {
		t.Fatalf("business role = %q, want demoted even during Auth outage", role)
	}

	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("converge during outage: %v", err)
	}
	if got := durableIntentCount(t, f.pool, userID, "pending"); got != 1 {
		t.Fatalf("pending intents = %d, want 1 durable retry", got)
	}

	// Auth recovers: the next sweep converges without any new request.
	f.auth.roleErrors = nil
	f.sync.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
	converged, err := f.sync.ConvergeDue(ctx)
	if err != nil {
		t.Fatalf("converge after recovery: %v", err)
	}
	if converged != 1 {
		t.Fatalf("converged = %d, want 1", converged)
	}
	if got := durableIntentCount(t, f.pool, userID, "done"); got != 1 {
		t.Fatalf("done intents = %d, want 1", got)
	}
}

func TestDurableBanFreezesWalletAndConverges(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-ban", "auth-durable-ban", "DBAN01", "user", StatusActive)

	outcome, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{Reason: "abuse", ExpiresInDays: 7})
	if err != nil {
		t.Fatalf("ban: %v", err)
	}
	if !outcome.SyncPending || outcome.IntentID <= 0 {
		t.Fatalf("outcome = %+v, want SyncPending with an intent", outcome)
	}
	if _, status, _ := durableUserRow(t, f.pool, userID); status != StatusBanned {
		t.Fatalf("business status = %d, want banned", status)
	}
	if status := durableWalletStatus(t, f.pool, userID); status != creditsdomain.WalletStatusFrozen {
		t.Fatalf("wallet status = %q, want frozen atomically with the ban", status)
	}

	converged, err := f.sync.ConvergeDue(ctx)
	if err != nil {
		t.Fatalf("converge: %v", err)
	}
	if converged != 1 {
		t.Fatalf("converged = %d, want 1", converged)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-ban")
	if got := durableIntentCount(t, f.pool, userID, "done"); got != 1 {
		t.Fatalf("done intents = %d, want 1", got)
	}
}

func TestDurableUnbanUnfreezesWalletAndConverges(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-unban", "auth-durable-unban", "DUBN01", "user", StatusBanned)
	if _, err := f.pool.Exec(ctx, `UPDATE users SET status = 0 WHERE id = $1`, userID); err != nil {
		t.Fatalf("seed banned state: %v", err)
	}

	outcome, err := f.commands.Unban(ctx, identityAdminActor(t), publicID)
	if err != nil {
		t.Fatalf("unban: %v", err)
	}
	if !outcome.SyncPending {
		t.Fatal("unban outcome is not sync-pending")
	}
	if _, status, _ := durableUserRow(t, f.pool, userID); status != StatusActive {
		t.Fatalf("business status = %d, want active", status)
	}
	if status := durableWalletStatus(t, f.pool, userID); status != creditsdomain.WalletStatusActive {
		t.Fatalf("wallet status = %q, want active atomically with the unban", status)
	}

	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("converge: %v", err)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-unban")
}

func TestDurableBusinessFailureLeavesNoIntent(t *testing.T) {
	pool := newUserRepoTestDB(t)
	ctx := context.Background()
	// Fake business seam fails the status write inside a real store
	// transaction: nothing may commit, in particular no orphan intent.
	authUserID := "auth-biz-fail"
	var seededID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (display_name, auth_user_id, storage_uid, role, status) VALUES ('biz fail', $1, 'BIZF01', 'user', 1) RETURNING id`, authUserID).Scan(&seededID); err != nil {
		t.Fatalf("seed biz fail user: %v", err)
	}
	users := &fakeIdentityAdminUsers{
		target:       &User{ID: seededID, PublicID: uuid.MustParse(identityAdminTargetPublicID), DisplayName: "biz fail", AuthUserID: &authUserID, Role: "user", Status: 1},
		statusErrors: []error{errors.New("wallet freeze failed")},
		events:       &[]string{},
	}
	commands := newIdentityAdminCommands(users, &fakeIdentityAdminAuth{events: &[]string{}}, authsync.NewStore(pool))

	_, err := commands.Ban(ctx, identityAdminActor(t), identityAdminTargetPublicID, IdentityAdminBanInput{})
	assertIdentityAdminErrorKind(t, err, IdentityAdminBusinessFailure)
	if got := durableIntentCount(t, pool, seededID, "pending"); got != 0 {
		t.Fatalf("pending intents = %d, want 0 after business rollback", got)
	}
	if _, status, version := durableUserRow(t, pool, seededID); status != StatusActive || version != 0 {
		t.Fatalf("business row = status %d version %d, want active/0 (rolled back)", status, version)
	}
}

func TestDurableCarrierRollbackIsAtomic(t *testing.T) {
	// Guard parity: the mutation joins the ambient carrier transaction,
	// so a later audit/commit failure discards the business write and
	// the intent together.
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-carrier", "auth-durable-carrier", "DCAR01", "user", StatusActive)

	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier: %v", err)
	}
	carrierCtx := dbutil.ContextWithAmbientTx(ctx, tx)
	outcome, err := f.commands.Ban(carrierCtx, identityAdminActor(t), publicID, IdentityAdminBanInput{Reason: "abuse"})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("ban in carrier: %v", err)
	}
	if !outcome.SyncPending {
		_ = tx.Rollback(ctx)
		t.Fatal("carrier ban outcome is not sync-pending")
	}
	// The business write is visible inside the carrier...
	var status int16
	if err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status); err != nil || status != StatusBanned {
		_ = tx.Rollback(ctx)
		t.Fatalf("carrier status = %d, err = %v, want banned inside the unit", status, err)
	}
	// ...and discarded with it when the guard fails (audit/commit error).
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier: %v", err)
	}
	if _, status, version := durableUserRow(t, f.pool, userID); status != StatusActive || version != 0 {
		t.Fatalf("business row = status %d version %d, want active/0 after carrier rollback", status, version)
	}
	if got := durableIntentCount(t, f.pool, userID, "pending"); got != 0 {
		t.Fatalf("pending intents = %d, want 0 after carrier rollback", got)
	}
}

func TestDurableCrashRecoveryConverges(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-crash", "auth-durable-crash", "DCRS01", "user", StatusActive)

	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{Reason: "abuse"}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	// Simulate a crash: drop every in-memory handle, rebuild from the
	// same database, and converge. Durability lives in the rows.
	crashedStore := authsync.NewStore(f.pool)
	crashedSync := authsync.NewService(authsync.ServiceDeps{Store: crashedStore, Auth: f.auth})
	converged, err := crashedSync.ConvergeDue(ctx)
	if err != nil {
		t.Fatalf("converge after restart: %v", err)
	}
	if converged != 1 {
		t.Fatalf("converged = %d, want 1", converged)
	}
	if got := durableIntentCount(t, f.pool, userID, "done"); got != 1 {
		t.Fatalf("done intents = %d, want 1", got)
	}

	// A worker that died between claim and finalize leaves an in_flight
	// lease: the next sweep reaps it instead of stranding it.
	if _, err := f.commands.Unban(ctx, identityAdminActor(t), publicID); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE identity_auth_sync_intents SET status = 'in_flight', claimed_at = now() - interval '10 minutes' WHERE user_id = $1 AND status = 'pending'`, userID); err != nil {
		t.Fatalf("craft orphaned claim: %v", err)
	}
	converged, err = crashedSync.ConvergeDue(ctx)
	if err != nil {
		t.Fatalf("converge orphaned claim: %v", err)
	}
	if converged != 1 {
		t.Fatalf("converged = %d, want 1 reaped claim", converged)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-ban", "auth-unban")
}

func TestDurableSameUserConcurrencySerializes(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-race", "auth-durable-race", "DRAC01", "user", StatusActive)

	// Five concurrent same-state promotions: serialization holds, every
	// mutation commits, and exactly one intent covers the converged
	// value (same-state retries dedupe against the pending intent
	// instead of fabricating duplicate pushes).
	const writers = 5
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := f.commands.SetRole(ctx, identityAdminActor(t), publicID, "admin")
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent promote %d: %v", i, err)
		}
	}
	if role, _, version := durableUserRow(t, f.pool, userID); role != "admin" || version != 1 {
		t.Fatalf("business row = role %q version %d, want admin/1 (one state change)", role, version)
	}
	if got := durableIntentCount(t, f.pool, userID, "pending"); got != 1 {
		t.Fatalf("pending intents = %d, want 1 (same-state dedupe)", got)
	}

	// Sequential alternating mutations bump the version once per real
	// state change; convergence supersedes the stale intents and pushes
	// only the final fact.
	for _, role := range []string{"user", "admin", "user"} {
		if _, err := f.commands.SetRole(ctx, identityAdminActor(t), publicID, role); err != nil {
			t.Fatalf("alternate to %q: %v", role, err)
		}
	}
	if role, _, version := durableUserRow(t, f.pool, userID); role != "user" || version != 4 {
		t.Fatalf("business row = role %q version %d, want user/4", role, version)
	}

	// Three concurrent sweeps: each intent is dispositioned exactly once,
	// and only the final fact reaches Auth.
	var converged atomic.Int64
	sweepErrs := make([]error, 3)
	var sweepWG sync.WaitGroup
	for i := 0; i < 3; i++ {
		sweepWG.Add(1)
		go func(i int) {
			defer sweepWG.Done()
			n, err := f.sync.ConvergeDue(ctx)
			converged.Add(int64(n))
			sweepErrs[i] = err
		}(i)
	}
	sweepWG.Wait()
	for i, err := range sweepErrs {
		if err != nil {
			t.Fatalf("concurrent sweep %d: %v", i, err)
		}
	}
	if converged.Load() != 4 {
		t.Fatalf("converged total = %d, want 4 (one per intent, no double handling)", converged.Load())
	}
	pushes := 0
	for _, event := range *f.auth.events {
		if event == "auth-role:user" {
			pushes++
		}
	}
	if pushes != 1 {
		t.Fatalf("auth demote pushes = %d, want exactly 1 (only the final fact)", pushes)
	}
	if got := durableIntentCount(t, f.pool, userID, "done"); got != 1 {
		t.Fatalf("done intents = %d, want 1", got)
	}
	var superseded int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM identity_auth_sync_intents WHERE user_id = $1 AND status = 'superseded'`, userID).Scan(&superseded); err != nil || superseded != 3 {
		t.Fatalf("superseded intents = %d, err = %v, want 3", superseded, err)
	}
}

func TestDurableStaleIntentNeverOverwritesNewState(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-stale", "auth-durable-stale", "DSTL01", "user", StatusActive)

	// Ban (v1 intent) then unban (v2 intent) before any sweep: the ban
	// push must never run against the now-active business state.
	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{Reason: "abuse"}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if _, err := f.commands.Unban(ctx, identityAdminActor(t), publicID); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if _, status, version := durableUserRow(t, f.pool, userID); status != StatusActive || version != 2 {
		t.Fatalf("business row = status %d version %d, want active/2", status, version)
	}

	converged, err := f.sync.ConvergeDue(ctx)
	if err != nil {
		t.Fatalf("converge: %v", err)
	}
	if converged != 2 {
		t.Fatalf("converged = %d, want 2 (one superseded, one done)", converged)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-unban")
	var superseded int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM identity_auth_sync_intents WHERE user_id = $1 AND status = 'superseded' AND op = 'ban'`, userID).Scan(&superseded); err != nil || superseded != 1 {
		t.Fatalf("superseded ban intents = %d, err = %v, want 1", superseded, err)
	}
	if got := durableIntentCount(t, f.pool, userID, "done"); got != 1 {
		t.Fatalf("done intents = %d, want 1 (the unban)", got)
	}
}

func TestDurableAuthIdentityGoneFailsClosed(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-gone", "auth-durable-gone", "DGON01", "user", StatusActive)

	// Auth already deleted the identity: the push fails permanently, the
	// business ban stays (fail-closed), and no retry spins forever.
	f.auth.banErrors = []error{autherrors.ErrAuthUserNotFound}
	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	converged, err := f.sync.ConvergeDue(ctx)
	if err != nil {
		t.Fatalf("converge: %v", err)
	}
	if converged != 1 {
		t.Fatalf("converged = %d, want 1 terminal failure", converged)
	}
	if got := durableIntentCount(t, f.pool, userID, "failed"); got != 1 {
		t.Fatalf("failed intents = %d, want 1", got)
	}
	if _, status, _ := durableUserRow(t, f.pool, userID); status != StatusBanned {
		t.Fatalf("business status = %d, want banned (fail-closed)", status)
	}

	pushes := len(*f.auth.events)
	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("second converge: %v", err)
	}
	if len(*f.auth.events) != pushes {
		t.Fatalf("auth events grew to %v, want no retry of a terminal failure", *f.auth.events)
	}
}

func TestDurableDeletionBlocksUnban(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "durable-del", "auth-durable-del", "DDEL01", "user", StatusBanned)
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_deletion_requests (user_id, auth_user_id, storage_uid, phase) VALUES ($1, 'auth-durable-del', 'DDEL01', 'pending_storage')`, userID); err != nil {
		t.Fatalf("seed deletion request: %v", err)
	}

	_, err := f.commands.Unban(ctx, identityAdminActor(t), publicID)
	assertIdentityAdminErrorKind(t, err, IdentityAdminBusinessFailure)
	if got := durableIntentCount(t, f.pool, userID, "pending"); got != 0 {
		t.Fatalf("pending intents = %d, want 0 (nothing committed)", got)
	}
	if _, status, _ := durableUserRow(t, f.pool, userID); status != StatusBanned {
		t.Fatalf("business status = %d, want banned (deletion owns the lifecycle)", status)
	}
}

func TestDurableSameStateRoleConfirmsDriftHeal(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	_, publicID := f.seedUser(t, "durable-heal", "auth-durable-heal", "DHEAL1", "user", StatusActive)

	if _, err := f.commands.SetRole(ctx, identityAdminActor(t), publicID, "admin"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("converge: %v", err)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-role:admin")

	// Auth drifted afterwards (or the ack was lost): a same-state retry
	// persists a confirming intent and the sweep re-pushes the fact.
	outcome, err := f.commands.SetRole(ctx, identityAdminActor(t), publicID, "admin")
	if err != nil {
		t.Fatalf("same-state promote: %v", err)
	}
	if !outcome.SyncPending || outcome.IntentID <= 0 {
		t.Fatalf("outcome = %+v, want a confirming intent", outcome)
	}
	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("re-converge: %v", err)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-role:admin", "auth-role:admin")
}

func TestDurableSameStateBanReportsPendingWithoutNewPush(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	_, publicID := f.seedUser(t, "durable-noop", "auth-durable-noop", "DNOOP1", "user", StatusActive)

	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{Reason: "abuse"}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	// A repeated ban while the first intent is still pending reports
	// pending honestly without fabricating a replacement push (which
	// would clobber the stored reason/expiry).
	second, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{Reason: "different"})
	if err != nil {
		t.Fatalf("repeated ban: %v", err)
	}
	if !second.SyncPending || second.IntentID != 0 {
		t.Fatalf("repeated ban outcome = %+v, want pending with no new intent", second)
	}
	var intents int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM identity_auth_sync_intents WHERE op = 'ban'`).Scan(&intents); err != nil {
		t.Fatalf("count intents: %v", err)
	}
	if intents != 1 {
		t.Fatalf("ban intents = %d, want exactly 1 (no replacement push)", intents)
	}
}
