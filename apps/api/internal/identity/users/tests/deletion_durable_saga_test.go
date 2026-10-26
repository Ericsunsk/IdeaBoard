package tests

// Durable account-deletion saga (#887): the initialization (business
// deactivate + Wallet freeze + durable deletion request) commits before
// any external Auth/Media call, and every Auth -> Media -> tombstone
// phase advances in its own durable transaction. No saga step joins the
// Control guard ambient transaction, so a guard audit/commit failure
// after an external success keeps its progress and retries converge
// instead of restoring active.

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/testdb"
)

type scriptedAuthDeleter struct {
	calls []string
	errs  []error
}

func (d *scriptedAuthDeleter) DeleteUser(_ context.Context, authUserID string) error {
	d.calls = append(d.calls, authUserID)
	if len(d.errs) > 0 {
		err := d.errs[0]
		d.errs = d.errs[1:]
		return err
	}
	return nil
}

type scriptedUploadCleaner struct {
	calls []userUploadCleanupCall
	errs  []error
}

func (c *scriptedUploadCleaner) DeleteUserUploads(_ context.Context, userID int64, storageUID string) error {
	c.calls = append(c.calls, userUploadCleanupCall{userID: userID, storageUID: storageUID})
	if len(c.errs) > 0 {
		err := c.errs[0]
		c.errs = c.errs[1:]
		return err
	}
	return nil
}

func provisionDurableSagaUser(t *testing.T, lifecycle *LifecycleService, authUserID, email string) *ProvisionResult {
	t.Helper()
	provisioned, err := lifecycle.ProvisionIdentity(context.Background(), VerifiedIdentity{
		AuthUserID: authUserID, DisplayName: "durable-saga-user",
		ClaimIdentity: email, ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("provision deletion user: %v", err)
	}
	return provisioned
}

// TestDeletionInitSurvivesGuardRollback pins the initialization durability:
// even when the Control guard transaction rolls back (audit/commit
// failure), the business deactivate + Wallet freeze + durable request
// committed by the saga stay frozen and never return to active.
func TestDeletionInitSurvivesGuardRollback(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	lifecycle := newLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	provisioned := provisionDurableSagaUser(t, lifecycle, "auth-durable-init-1", "durable-init-1@example.com")

	carrier, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin guard carrier tx: %v", err)
	}
	guardCtx := dbutil.ContextWithAmbientTx(ctx, carrier)

	authDeleter := &recordingAuthUserDeleter{err: errors.New("auth unavailable")}
	cleaner := &recordingUserUploadCleaner{}
	if err := lifecycle.DeleteUser(guardCtx, provisioned.User.ID, authDeleter, cleaner); err == nil {
		_ = carrier.Rollback(ctx)
		t.Fatal("Auth failure should stop deletion")
	}
	// Initialization must already be durable on the pool, outside the
	// still-open carrier: frozen user/wallet plus the pending request.
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingAuth, 0, "frozen")

	// Guard audit/commit failure: the whole carrier rolls back. The
	// saga initialization must survive it and never restore active.
	if err := carrier.Rollback(ctx); err != nil {
		t.Fatalf("rollback guard carrier: %v", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingAuth, 0, "frozen")
	if err := lifecycle.SetUserStatus(ctx, provisioned.User.ID, 1); !errors.Is(err, ErrUserDeletionInProgress) {
		t.Fatalf("reactivate after guard rollback error = %v, want ErrUserDeletionInProgress", err)
	}

	// Retry without the carrier converges through the remaining phases.
	authDeleter.err = nil
	if err := lifecycle.DeleteUser(ctx, provisioned.User.ID, authDeleter, cleaner); err != nil {
		t.Fatalf("retry deletion after guard rollback: %v", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionCompleted, 0, "frozen")
}

// TestDeletionFullSagaSurvivesGuardRollback runs the entire saga inside a
// guard carrier (Production Control path shape) with fake Auth/Media,
// then rolls the carrier back to simulate an external-success-then-
// audit/commit failure. The completed tombstone must survive.
func TestDeletionFullSagaSurvivesGuardRollback(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	creditService := credits.NewService(pool)
	lifecycle := newLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	provisioned := provisionDurableSagaUser(t, lifecycle, "auth-durable-full-1", "durable-full-1@example.com")
	originalStorageUID := provisioned.User.StorageUID

	carrier, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin guard carrier tx: %v", err)
	}
	guardCtx := dbutil.ContextWithAmbientTx(ctx, carrier)

	authDeleter := &recordingAuthUserDeleter{}
	cleaner := &recordingUserUploadCleaner{}
	if err := lifecycle.DeleteUser(guardCtx, provisioned.User.ID, authDeleter, cleaner); err != nil {
		_ = carrier.Rollback(ctx)
		t.Fatalf("DeleteUser under guard carrier: %v", err)
	}
	if err := carrier.Rollback(ctx); err != nil {
		t.Fatalf("rollback guard carrier after saga success: %v", err)
	}

	assertDeletionState(t, pool, provisioned.User.ID, DeletionCompleted, 0, "frozen")
	var displayName, tombstoneStorageUID string
	var authUserID *string
	var status int16
	if err := pool.QueryRow(ctx, `SELECT display_name, auth_user_id, storage_uid, status FROM users WHERE id = $1`, provisioned.User.ID).Scan(&displayName, &authUserID, &tombstoneStorageUID, &status); err != nil {
		t.Fatalf("read tombstone after guard rollback: %v", err)
	}
	if displayName != "已注销用户" || authUserID != nil || status != 0 || tombstoneStorageUID == originalStorageUID || !IsValidStorageUID(tombstoneStorageUID) {
		t.Fatalf("tombstone display=%q auth=%v status=%d storage=%q", displayName, authUserID, status, tombstoneStorageUID)
	}
	// Historical Wallet stays readable but frozen: no new spending.
	tombstoneAccountID, err := creditService.ResolvePersonalAccount(ctx, provisioned.User.ID)
	if err != nil {
		t.Fatalf("resolve tombstone account: %v", err)
	}
	wallet, err := creditService.GetWallet(ctx, tombstoneAccountID)
	if err != nil {
		t.Fatalf("GetWallet(tombstone) error = %v", err)
	}
	if wallet.Status != domain.WalletStatusFrozen {
		t.Fatalf("tombstone wallet status = %q, want frozen", wallet.Status)
	}
	if _, err := creditService.Settlement().Hold(ctx, lifecycleHold(t, creditService, provisioned.User.ID, "durable-full:frozen")); !errors.Is(err, domain.ErrWalletFrozen) {
		t.Fatalf("Hold(tombstone wallet) error = %v, want ErrWalletFrozen", err)
	}
	// Idempotent retry after the guard failure does not touch externals again.
	authCalls := len(authDeleter.authUserIDs)
	storageCalls := len(cleaner.calls)
	if err := lifecycle.DeleteUser(ctx, provisioned.User.ID, authDeleter, cleaner); err != nil {
		t.Fatalf("idempotent retry after guard rollback: %v", err)
	}
	if len(authDeleter.authUserIDs) != authCalls || len(cleaner.calls) != storageCalls {
		t.Fatalf("idempotent rerun touched externals: auth %d->%d storage %d->%d",
			authCalls, len(authDeleter.authUserIDs), storageCalls, len(cleaner.calls))
	}
}

// TestControlDeletePhaseInterruptionRetryAndTombstone covers the
// Production Control path (IdentityAdminCommands.Delete with fake
// Auth/Media): interruption at each phase, duplicate execution, and the
// final PII tombstone. No failure restores active.
func TestControlDeletePhaseInterruptionRetryAndTombstone(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	lifecycle := newLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	provisioned := provisionDurableSagaUser(t, lifecycle, "auth-durable-control-1", "durable-control-1@example.com")

	cleaner := &scriptedUploadCleaner{errs: []error{errors.New("storage unavailable")}}
	userService := NewUserService(NewUserRepository(pool), lifecycle, nil, cleaner)
	events := &[]string{}
	authManager := &fakeIdentityAdminAuth{events: events, deleteErrors: []error{errors.New("auth unavailable"), errors.New("auth unavailable")}}
	commands := newIdentityAdminCommands(userService, authManager, nil)
	actor := identityAdminActor(t)
	target := provisioned.User.PublicID.String()

	if _, err := commands.Delete(ctx, actor, target); err == nil {
		t.Fatal("Auth failure should stop control deletion")
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingAuth, 0, "frozen")

	// Duplicate execution while pending stays frozen and reports failure.
	if _, err := commands.Delete(ctx, actor, target); err == nil {
		t.Fatal("duplicate control deletion during pending_auth should not succeed")
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingAuth, 0, "frozen")

	// Auth recovers; storage interruption parks at pending_storage.
	authManager.deleteErrors = nil
	if _, err := commands.Delete(ctx, actor, target); err == nil {
		t.Fatal("storage failure should stop control deletion")
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingStorage, 0, "frozen")

	// Storage recovers; the saga converges to the PII tombstone.
	cleaner.errs = nil
	// The resumed control call needs an Auth manager again (loadTarget
	// requires Auth wiring only for role/ban paths; Delete resolves the
	// target from the business row, so nil Auth is fine for resume).
	commands = newIdentityAdminCommands(userService, nil, nil)
	if _, err := commands.Delete(ctx, actor, target); err != nil {
		t.Fatalf("resume control deletion to tombstone: %v", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionCompleted, 0, "frozen")

	// PII tombstone contract: business PII cleared, request PII cleared,
	// historical Wallet readable but frozen, tasks/ledger retained.
	var displayName, avatar, tombstoneStorageUID string
	var remainingAuth *string
	var status int16
	if err := pool.QueryRow(ctx, `SELECT display_name, avatar, storage_uid, auth_user_id, status FROM users WHERE id = $1`, provisioned.User.ID).Scan(
		&displayName, &avatar, &tombstoneStorageUID, &remainingAuth, &status,
	); err != nil {
		t.Fatalf("read control tombstone: %v", err)
	}
	if displayName != "已注销用户" || avatar != "" || remainingAuth != nil || status != 0 || !IsValidStorageUID(tombstoneStorageUID) {
		t.Fatalf("control tombstone display=%q avatar=%q auth=%v status=%d storage=%q",
			displayName, avatar, remainingAuth, status, tombstoneStorageUID)
	}
	var reqAuth, reqStorage *string
	var completed bool
	if err := pool.QueryRow(ctx, `SELECT auth_user_id, storage_uid, completed_at IS NOT NULL FROM user_deletion_requests WHERE user_id = $1`, provisioned.User.ID).Scan(&reqAuth, &reqStorage, &completed); err != nil {
		t.Fatalf("read completed control deletion request: %v", err)
	}
	if reqAuth != nil || reqStorage != nil || !completed {
		t.Fatalf("completed request PII auth=%v storage=%v completed=%v", reqAuth, reqStorage, completed)
	}

	// Final duplicate execution after completion stays successful and
	// never restores active.
	if _, err := commands.Delete(ctx, actor, target); err != nil {
		t.Fatalf("duplicate control deletion after completion: %v", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionCompleted, 0, "frozen")
	if err := lifecycle.SetUserStatus(ctx, provisioned.User.ID, 1); !errors.Is(err, ErrUserDeletionInProgress) {
		t.Fatalf("reactivate completed deletion error = %v, want ErrUserDeletionInProgress", err)
	}
}

// TestControlDeleteConvergesAfterLostStorageAdvance pins Media
// idempotent convergence: Storage cleanup succeeds externally but the
// phase advance is lost; the retry re-runs the already-cleaned prefix
// (idempotent success) and still reaches the tombstone.
func TestControlDeleteConvergesAfterLostStorageAdvance(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	lifecycle := newLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	provisioned := provisionDurableSagaUser(t, lifecycle, "auth-durable-storage-1", "durable-storage-1@example.com")
	originalStorageUID := provisioned.User.StorageUID

	if _, err := pool.Exec(ctx, `
CREATE FUNCTION fail_storage_phase_advance_once() RETURNS trigger AS $$
BEGIN
  IF NEW.phase = 'pending_tombstone' THEN RAISE EXCEPTION 'injected storage phase advance failure'; END IF;
  RETURN NEW;
END $$ LANGUAGE plpgsql`); err != nil {
		t.Fatalf("install storage phase advance failure function: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER fail_storage_phase_advance_once BEFORE UPDATE ON user_deletion_requests
FOR EACH ROW EXECUTE FUNCTION fail_storage_phase_advance_once()`); err != nil {
		t.Fatalf("install storage phase advance failure trigger: %v", err)
	}

	cleaner := &scriptedUploadCleaner{}
	userService := NewUserService(NewUserRepository(pool), lifecycle, nil, cleaner)
	authManager := &fakeIdentityAdminAuth{events: &[]string{}}
	commands := newIdentityAdminCommands(userService, authManager, nil)
	actor := identityAdminActor(t)
	target := provisioned.User.PublicID.String()

	if _, err := commands.Delete(ctx, actor, target); err == nil {
		t.Fatal("lost storage phase advance should surface an error")
	}
	if len(cleaner.calls) != 1 {
		t.Fatalf("storage cleanup calls = %v, want one external success before lost advance", cleaner.calls)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingStorage, 0, "frozen")

	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_storage_phase_advance_once ON user_deletion_requests`); err != nil {
		t.Fatalf("remove storage phase advance failure trigger: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP FUNCTION fail_storage_phase_advance_once()`); err != nil {
		t.Fatalf("remove storage phase advance failure function: %v", err)
	}

	// Retry re-runs the already-cleaned prefix: the fake cleaner
	// succeeds again (mirroring idempotent prefix deletion), then the
	// tombstone completes.
	commands = newIdentityAdminCommands(userService, nil, nil)
	if _, err := commands.Delete(ctx, actor, target); err != nil {
		t.Fatalf("retry after lost storage phase advance: %v", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionCompleted, 0, "frozen")
	if len(cleaner.calls) != 2 {
		t.Fatalf("storage cleanup calls = %v, want success plus idempotent retry", cleaner.calls)
	}
	if cleaner.calls[1].storageUID != originalStorageUID {
		t.Fatalf("storage retry storageUID = %q, want original %q", cleaner.calls[1].storageUID, originalStorageUID)
	}
}

// TestDeletionAuthGoneIsConvergence pins Auth idempotent convergence on
// the durable path: after a lost Auth phase advance, the retry sees
// Auth answer not_found and still converges.
func TestDeletionAuthGoneIsConvergence(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	lifecycle := newLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	provisioned := provisionDurableSagaUser(t, lifecycle, "auth-durable-gone-1", "durable-gone-1@example.com")

	if _, err := pool.Exec(ctx, `
CREATE FUNCTION fail_auth_phase_advance_durable() RETURNS trigger AS $$
BEGIN
  IF NEW.phase = 'pending_storage' THEN RAISE EXCEPTION 'injected auth phase advance failure'; END IF;
  RETURN NEW;
END $$ LANGUAGE plpgsql`); err != nil {
		t.Fatalf("install auth phase advance failure function: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER fail_auth_phase_advance_durable BEFORE UPDATE ON user_deletion_requests
FOR EACH ROW EXECUTE FUNCTION fail_auth_phase_advance_durable()`); err != nil {
		t.Fatalf("install auth phase advance failure trigger: %v", err)
	}

	authDeleter := &scriptedAuthDeleter{}
	cleaner := &scriptedUploadCleaner{}
	userService := NewUserService(NewUserRepository(pool), lifecycle, nil, cleaner)
	_ = userService
	if err := lifecycle.DeleteUser(ctx, provisioned.User.ID, authDeleter, cleaner); err == nil {
		t.Fatal("lost Auth phase advance should surface an error")
	}
	if len(authDeleter.calls) != 1 || len(cleaner.calls) != 0 {
		t.Fatalf("calls auth=%v storage=%v, want one Auth success and no storage yet", authDeleter.calls, cleaner.calls)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingAuth, 0, "frozen")

	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_auth_phase_advance_durable ON user_deletion_requests`); err != nil {
		t.Fatalf("remove auth phase advance failure trigger: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP FUNCTION fail_auth_phase_advance_durable()`); err != nil {
		t.Fatalf("remove auth phase advance failure function: %v", err)
	}

	// Retry: Auth no longer holds the identity (prior attempt deleted
	// it), so it answers not_found, which must converge.
	authDeleter.errs = []error{autherrors.ErrAuthUserNotFound}
	if err := lifecycle.DeleteUser(ctx, provisioned.User.ID, authDeleter, cleaner); err != nil {
		t.Fatalf("retry after lost Auth phase advance: %v", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionCompleted, 0, "frozen")
	if len(authDeleter.calls) != 2 || len(cleaner.calls) != 1 {
		t.Fatalf("retry calls auth=%v storage=%v, want 2 Auth and 1 storage", authDeleter.calls, cleaner.calls)
	}
}
