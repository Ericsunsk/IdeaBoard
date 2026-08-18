package users

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/jackc/pgx/v5"
)

func lifecycleHold(userID int64, key string) credits.HoldCommand {
	return credits.HoldCommand{
		UserID: userID,
		Amount: credits.CreditAmount(1),
		Metadata: credits.CommandMetadata{
			IdempotencyKey:    key,
			Actor:             credits.Actor{Type: credits.ActorUser, ID: "lifecycle-test"},
			Source:            "lifecycle_test",
			Reason:            "verify wallet lifecycle",
			BusinessReference: credits.BusinessReference{Type: "test", ID: key},
		},
	}
}

func TestProvisionIdentityCreatesUserWalletAndWelcomeGrantAtomically(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	identity := VerifiedIdentity{
		AuthUserID: "auth-welcome-1", DisplayName: "welcome-user",
		Avatar:        "https://lh3.googleusercontent.com/avatar",
		ClaimIdentity: " Welcome.User@example.com ", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	}

	first, err := service.ProvisionIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("ProvisionIdentity() error = %v", err)
	}
	if first.User.ID <= 0 || first.User.Status != 1 || !first.WelcomeGranted {
		t.Fatalf("ProvisionIdentity() result = %+v", first)
	}
	if first.User.Avatar != identity.Avatar {
		t.Fatalf("Avatar = %q, want %q", first.User.Avatar, identity.Avatar)
	}
	if first.Wallet.Available != 100 || first.Wallet.Reserved != 0 {
		t.Fatalf("wallet = %+v, want available=100 reserved=0", first.Wallet)
	}

	replay, err := service.ProvisionIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("replayed ProvisionIdentity() error = %v", err)
	}
	if replay.User.ID != first.User.ID || replay.WelcomeGranted {
		t.Fatalf("replay = %+v, first = %+v", replay, first)
	}
	if replay.Wallet.Available != 100 {
		t.Fatalf("replay wallet = %+v, want unchanged 100 credits", replay.Wallet)
	}

	var claimCount, transactionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_welcome_claims`).Scan(&claimCount); err != nil {
		t.Fatalf("count welcome claims: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_transactions WHERE wallet_user_id = $1`, first.User.ID).Scan(&transactionCount); err != nil {
		t.Fatalf("count credit transactions: %v", err)
	}
	if claimCount != 1 || transactionCount != 1 {
		t.Fatalf("claim count=%d transaction count=%d, want 1/1", claimCount, transactionCount)
	}
}

func TestProvisionIdentityConcurrentRetriesGrantWelcomeCreditsOnce(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	identity := VerifiedIdentity{
		AuthUserID: "auth-concurrent-1", DisplayName: "concurrent-user",
		ClaimIdentity: "concurrent-user@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	}

	const attempts = 8
	results := make(chan *ProvisionResult, attempts)
	errors := make(chan error, attempts)
	var waitGroup sync.WaitGroup
	for range attempts {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			result, err := service.ProvisionIdentity(context.Background(), identity)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatalf("concurrent ProvisionIdentity() error = %v", err)
	}

	var userID int64
	granted := 0
	for result := range results {
		if userID == 0 {
			userID = result.User.ID
		}
		if result.User.ID != userID || result.Wallet.Available != 100 {
			t.Fatalf("concurrent result = %+v", result)
		}
		if result.WelcomeGranted {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("WelcomeGranted count = %d, want 1", granted)
	}
}

func TestProvisionIdentityDoesNotGrantAgainAfterTrustedClaimIdentityChanges(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	identity := VerifiedIdentity{
		AuthUserID: "auth-email-change-1", DisplayName: "email-change", ClaimIdentity: "first@example.com", ClaimIdentityKind: "canonical_email", ClaimIdentityVerified: true, Role: "user",
	}

	first, err := service.ProvisionIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("ProvisionIdentity(first email) error = %v", err)
	}
	identity.ClaimIdentity = "second@example.com"
	second, err := service.ProvisionIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("ProvisionIdentity(changed email) error = %v", err)
	}
	if !first.WelcomeGranted || second.WelcomeGranted || second.Wallet.Available != 100 {
		t.Fatalf("first=%+v second=%+v, want exactly one 100-Credit Welcome Grant", first, second)
	}

	var claimCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_welcome_claims WHERE claimed_user_id = $1`, first.User.ID).Scan(&claimCount); err != nil {
		t.Fatalf("count user Welcome claims: %v", err)
	}
	if claimCount != 1 {
		t.Fatalf("user Welcome claim count = %d, want 1", claimCount)
	}
}

func TestProvisionIdentityFailsClosedWithoutTrustedClaimIdentityOrClaimKey(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	var initialUserCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&initialUserCount); err != nil {
		t.Fatalf("count initial users: %v", err)
	}

	for _, testCase := range []struct {
		name     string
		service  *LifecycleService
		identity VerifiedIdentity
	}{
		{
			name:     "missing claim key",
			service:  NewLifecycleService(pool, creditService, nil),
			identity: VerifiedIdentity{AuthUserID: "auth-no-key", DisplayName: "no-key", ClaimIdentity: "no-key@example.com", ClaimIdentityKind: "canonical_email", ClaimIdentityVerified: true, Role: "user"},
		},
		{
			name:     "claim identity not trusted",
			service:  NewLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef")),
			identity: VerifiedIdentity{AuthUserID: "auth-unverified", DisplayName: "unverified", ClaimIdentity: "unverified@example.com", ClaimIdentityKind: "canonical_email", Role: "user", ClaimIdentityVerified: false},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := testCase.service.ProvisionIdentity(context.Background(), testCase.identity); err == nil {
				t.Fatal("ProvisionIdentity() error = nil, want fail closed")
			}
		})
	}

	var userCount, claimCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM credit_welcome_claims`).Scan(&claimCount); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if userCount != initialUserCount || claimCount != 0 {
		t.Fatalf("partial state users=%d (initial %d) claims=%d", userCount, initialUserCount, claimCount)
	}
}

func TestSetUserStatusFreezesAndRestoresWalletAtomically(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := NewLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	result, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-status-1", DisplayName: "status-user", ClaimIdentity: "status-user@example.com", ClaimIdentityKind: "canonical_email", ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("ProvisionIdentity() error = %v", err)
	}
	restriction, err := creditService.RestrictWallet(ctx, credits.RestrictWalletCommand{
		UserID:        result.User.ID,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: credits.BusinessReference{Type: "finding", ID: "identity-status-finding"},
		Reason:        "verify Identity lifecycle isolation",
		Actor:         credits.Actor{Type: credits.ActorSystem, ID: "reconciliation"},
	})
	if err != nil {
		t.Fatalf("RestrictWallet() error = %v", err)
	}

	if err := service.SetUserStatus(ctx, result.User.ID, 0); err != nil {
		t.Fatalf("SetUserStatus(banned) error = %v", err)
	}
	if _, err := creditService.Hold(ctx, lifecycleHold(result.User.ID, "status:frozen")); !errors.Is(err, credits.ErrWalletFrozen) {
		t.Fatalf("Hold(frozen wallet) error = %v, want ErrWalletFrozen", err)
	}
	var userStatus int16
	var walletStatus string
	if err := pool.QueryRow(ctx, `
SELECT users.status, credit_wallets.status
FROM users JOIN credit_wallets ON credit_wallets.user_id = users.id
WHERE users.id = $1`, result.User.ID).Scan(&userStatus, &walletStatus); err != nil {
		t.Fatalf("read statuses: %v", err)
	}
	if userStatus != 0 || walletStatus != "frozen" {
		t.Fatalf("user status=%d wallet status=%s, want 0/frozen", userStatus, walletStatus)
	}

	if err := service.SetUserStatus(ctx, result.User.ID, 1); err != nil {
		t.Fatalf("SetUserStatus(active) error = %v", err)
	}
	wallet, err := creditService.GetWallet(ctx, result.User.ID)
	if err != nil {
		t.Fatalf("GetWallet(restored lifecycle) error = %v", err)
	}
	if wallet.Status != credits.WalletStatusActive || len(wallet.ActiveRestrictions) != 1 || wallet.ActiveRestrictions[0].RestrictionID != restriction.RestrictionID {
		t.Fatalf("restored lifecycle cleared independent restriction: %+v", wallet)
	}
	if _, err := creditService.Hold(ctx, lifecycleHold(result.User.ID, "status:active-restricted")); !errors.Is(err, credits.ErrWalletRestricted) {
		t.Fatalf("Hold(restored but restricted Wallet) error = %v, want ErrWalletRestricted", err)
	}
	if _, err := creditService.ReleaseWalletRestriction(ctx, credits.ReleaseWalletRestrictionCommand{
		RestrictionID: restriction.RestrictionID,
		Reason:        "identity lifecycle test complete",
		Actor:         credits.Actor{Type: credits.ActorSystem, ID: "reconciliation"},
	}); err != nil {
		t.Fatalf("ReleaseWalletRestriction() error = %v", err)
	}
	if _, err := creditService.Hold(ctx, lifecycleHold(result.User.ID, "status:active")); err != nil {
		t.Fatalf("Hold(after explicit restriction release) error = %v", err)
	}
}

func TestSetUserStatusAndTombstoneMissingUserReturnNotFound(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))

	if err := service.SetUserStatus(context.Background(), 999999, 0); !database.IsNotFound(err) {
		t.Fatalf("SetUserStatus(missing) error = %v, want not found", err)
	}
	if err := service.DeleteUser(context.Background(), 999999, &recordingAuthUserDeleter{}, &recordingUserUploadCleaner{}); !database.IsNotFound(err) {
		t.Fatalf("DeleteUser(missing) error = %v, want not found", err)
	}
}

func TestTombstoneUserPreservesAccountingAndPreventsWelcomeReclaim(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := NewLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	first, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-delete-1", DisplayName: "delete-user", ClaimIdentity: "same@example.com", ClaimIdentityKind: "canonical_email", ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("ProvisionIdentity(first) error = %v", err)
	}
	restriction, err := creditService.RestrictWallet(ctx, credits.RestrictWalletCommand{
		UserID:        first.User.ID,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: credits.BusinessReference{Type: "finding", ID: "deletion-finding"},
		Reason:        "verify deletion preserves restrictions",
		Actor:         credits.Actor{Type: credits.ActorSystem, ID: "reconciliation"},
	})
	if err != nil {
		t.Fatalf("RestrictWallet(deletion) error = %v", err)
	}
	if err := service.DeleteUser(ctx, first.User.ID, &recordingAuthUserDeleter{}, &recordingUserUploadCleaner{}); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	var displayName, avatar, walletStatus string
	var authUserID *string
	var status int16
	if err := pool.QueryRow(ctx, `
SELECT users.display_name, users.auth_user_id, users.avatar, users.status, credit_wallets.status
FROM users JOIN credit_wallets ON credit_wallets.user_id = users.id WHERE users.id = $1`, first.User.ID).Scan(
		&displayName, &authUserID, &avatar, &status, &walletStatus,
	); err != nil {
		t.Fatalf("read tombstone: %v", err)
	}
	if displayName != "已注销用户" || authUserID != nil || avatar != "" || status != 0 || walletStatus != "frozen" {
		t.Fatalf("tombstone display_name=%q auth=%v avatar=%q status=%d wallet=%q", displayName, authUserID, avatar, status, walletStatus)
	}
	tombstoneWallet, err := creditService.GetWallet(ctx, first.User.ID)
	if err != nil {
		t.Fatalf("GetWallet(tombstone) error = %v", err)
	}
	if len(tombstoneWallet.ActiveRestrictions) != 1 || tombstoneWallet.ActiveRestrictions[0].RestrictionID != restriction.RestrictionID {
		t.Fatalf("tombstone cleared independent restriction: %+v", tombstoneWallet.ActiveRestrictions)
	}

	second, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-delete-2", DisplayName: "replacement-user", ClaimIdentity: "SAME@example.com", ClaimIdentityKind: "canonical_email", ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("ProvisionIdentity(replacement) error = %v", err)
	}
	if second.User.ID == first.User.ID || second.WelcomeGranted || second.Wallet.Available != 0 {
		t.Fatalf("replacement result = %+v, first user id = %d", second, first.User.ID)
	}
	var claimCount, firstLedgerCount, secondLedgerCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_welcome_claims`).Scan(&claimCount); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_transactions WHERE wallet_user_id = $1`, first.User.ID).Scan(&firstLedgerCount); err != nil {
		t.Fatalf("count first ledger: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_transactions WHERE wallet_user_id = $1`, second.User.ID).Scan(&secondLedgerCount); err != nil {
		t.Fatalf("count second ledger: %v", err)
	}
	if claimCount != 1 || firstLedgerCount != 1 || secondLedgerCount != 0 {
		t.Fatalf("claims=%d first ledger=%d second ledger=%d, want 1/1/0", claimCount, firstLedgerCount, secondLedgerCount)
	}
}

func TestInitializeExistingIdentityProvisionsWalletAndWelcomeGrant(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	authUserID := "auth-existing-1"
	var userID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO users (display_name, auth_user_id, storage_uid, role, status)
VALUES ('existing-user', $1, 'EXST01', 'user', 1)
RETURNING id`, authUserID).Scan(&userID); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}
	service := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	results, err := service.InitializeExistingIdentities(ctx, []VerifiedIdentity{{
		AuthUserID: authUserID, DisplayName: "existing-user", ClaimIdentity: "existing-user@example.com", ClaimIdentityKind: "canonical_email", ClaimIdentityVerified: true, Role: "user",
	}})
	if err != nil {
		t.Fatalf("InitializeExistingIdentities() error = %v", err)
	}
	if len(results) != 1 || results[0].User.ID != userID || !results[0].WelcomeGranted || results[0].Wallet.Available != 100 {
		t.Fatalf("initializer results = %+v", results)
	}
}

func TestDirectUserRepositoryInsertProvisionsCreditWalletProjection(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	repo := NewUserRepository(db)
	user := &User{DisplayName: "direct-user-insert", Role: "user", Status: 1}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	balances, err := credits.NewService(pool).GetAvailableBalances(ctx, []int64{user.ID})
	if err != nil {
		t.Fatalf("read provisioned wallet balance: %v", err)
	}
	if got := balances[user.ID]; got != 0 {
		t.Fatalf("available balance = %d, want 0", got)
	}
}

func TestInitializeExistingBannedIdentityGrantsWelcomeCreditsIntoFrozenWallet(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	authUserID := "auth-existing-banned-1"
	var userID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO users (display_name, auth_user_id, storage_uid, role, status)
VALUES ('existing-banned-user', $1, 'EXBN01', 'user', 0)
RETURNING id`, authUserID).Scan(&userID); err != nil {
		t.Fatalf("seed banned user: %v", err)
	}
	creditService := credits.NewService(pool)
	service := NewLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	results, err := service.InitializeExistingIdentities(ctx, []VerifiedIdentity{{
		AuthUserID: authUserID, DisplayName: "existing-banned-user", ClaimIdentity: "existing-banned-user@example.com", ClaimIdentityKind: "canonical_email", ClaimIdentityVerified: true, Role: "user",
	}})
	if err != nil {
		t.Fatalf("InitializeExistingIdentities() error = %v", err)
	}
	if len(results) != 1 || !results[0].WelcomeGranted || results[0].Wallet.Available != 100 {
		t.Fatalf("initializer results = %+v", results)
	}
	if _, err := creditService.Hold(ctx, lifecycleHold(userID, "initializer:banned")); !errors.Is(err, credits.ErrWalletFrozen) {
		t.Fatalf("Hold(initialized banned wallet) error = %v, want ErrWalletFrozen", err)
	}
}

func TestWelcomeIdentityKeyRotationFailsClosedWithoutPartialState(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	first := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	if err := first.VerifyWelcomeIdentityKey(ctx); err != nil {
		t.Fatalf("bind first Welcome key: %v", err)
	}
	rotated := NewLifecycleService(pool, credits.NewService(pool), []byte("abcdef0123456789abcdef0123456789"))
	if err := rotated.VerifyWelcomeIdentityKey(ctx); !errors.Is(err, credits.ErrWelcomeIdentityKeyMismatch) {
		t.Fatalf("rotated key error = %v, want ErrWelcomeIdentityKeyMismatch", err)
	}
	if _, err := rotated.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-rotated-key", DisplayName: "rotated-key",
		ClaimIdentity: "rotated-key@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	}); !errors.Is(err, credits.ErrWelcomeIdentityKeyMismatch) {
		t.Fatalf("ProvisionIdentity(rotated key) error = %v, want ErrWelcomeIdentityKeyMismatch", err)
	}
	var userCount, claimCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE auth_user_id = 'auth-rotated-key'`).Scan(&userCount); err != nil {
		t.Fatalf("count rotated-key users: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_welcome_claims`).Scan(&claimCount); err != nil {
		t.Fatalf("count Welcome claims: %v", err)
	}
	if userCount != 0 || claimCount != 0 {
		t.Fatalf("partial state users=%d claims=%d, want 0/0", userCount, claimCount)
	}
}

func TestAdminProvisionCreatesAndRepairsLifecycleAtomically(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	service := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	identity := VerifiedIdentity{
		AuthUserID: "auth-admin-lifecycle", DisplayName: "admin",
		ClaimIdentity: "admin@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "admin", EnsureActive: true,
	}
	created, err := service.ProvisionIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("provision new admin: %v", err)
	}
	if created.User.Role != "admin" || created.User.Status != 1 || created.Wallet.Available != 100 || !created.WelcomeGranted {
		t.Fatalf("created admin = %+v", created)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET role = 'user', status = 0 WHERE id = $1`, created.User.ID); err != nil {
		t.Fatalf("drift local admin: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT set_credit_wallet_status($1, 'frozen')`, created.User.ID); err != nil {
		t.Fatalf("freeze drifted admin wallet: %v", err)
	}
	repaired, err := service.ProvisionIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("repair existing admin: %v", err)
	}
	if repaired.User.ID != created.User.ID || repaired.User.Role != "admin" || repaired.User.Status != 1 || repaired.WelcomeGranted || repaired.Wallet.Available != 100 {
		t.Fatalf("repaired admin = %+v, created = %+v", repaired, created)
	}
	var walletStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM credit_wallets WHERE user_id = $1`, created.User.ID).Scan(&walletStatus); err != nil {
		t.Fatalf("read repaired wallet status: %v", err)
	}
	if walletStatus != "active" {
		t.Fatalf("repaired wallet status = %s, want active", walletStatus)
	}
}

func TestUserDeletionFailuresRemainFrozenAndRetryConverges(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	lifecycle := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	provisioned, err := lifecycle.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-delete-converge", DisplayName: "delete-converge",
		ClaimIdentity: "delete-converge@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("provision deletion user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO tasks (task_id, user_id, prompt, user_prompt, params, status, error_msg, result)
VALUES ('delete-convergence-task', $1, '', '', '', 'pending', '', '')`, provisioned.User.ID); err != nil {
		t.Fatalf("seed retained task: %v", err)
	}

	authDeleter := &recordingAuthUserDeleter{err: errors.New("auth unavailable")}
	cleaner := &recordingUserUploadCleaner{err: errors.New("storage unavailable")}
	service := NewUserService(NewUserRepository(db), lifecycle, nil, cleaner)
	if err := service.DeleteUser(ctx, provisioned.User.ID, authDeleter); err == nil {
		t.Fatal("Auth failure should stop deletion")
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingAuth, 0, "frozen")
	if _, err := lifecycle.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: *provisioned.User.AuthUserID, DisplayName: "delete-converge",
		ClaimIdentity: "delete-converge@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "admin", EnsureActive: true,
	}); !errors.Is(err, ErrUserDeletionInProgress) {
		t.Fatalf("EnsureActive pending deletion error = %v, want ErrUserDeletionInProgress", err)
	}
	if err := lifecycle.SetUserStatus(ctx, provisioned.User.ID, 1); !errors.Is(err, ErrUserDeletionInProgress) {
		t.Fatalf("reactivate pending deletion error = %v, want ErrUserDeletionInProgress", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingAuth, 0, "frozen")

	authDeleter.err = nil
	if err := service.DeleteUser(ctx, provisioned.User.ID, authDeleter); err == nil {
		t.Fatal("storage failure should stop deletion")
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingStorage, 0, "frozen")

	cleaner.err = nil
	if _, err := pool.Exec(ctx, `
CREATE FUNCTION fail_user_tombstone_once() RETURNS trigger AS $$
BEGIN
  IF NEW.auth_user_id IS NULL THEN RAISE EXCEPTION 'injected tombstone failure'; END IF;
  RETURN NEW;
END $$ LANGUAGE plpgsql`); err != nil {
		t.Fatalf("install tombstone failure function: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER fail_user_tombstone_once BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION fail_user_tombstone_once()`); err != nil {
		t.Fatalf("install tombstone failure trigger: %v", err)
	}
	if err := service.DeleteUser(ctx, provisioned.User.ID, nil); err == nil {
		t.Fatal("injected tombstone failure should stop deletion")
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingTombstone, 0, "frozen")
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_user_tombstone_once ON users`); err != nil {
		t.Fatalf("remove tombstone failure trigger: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP FUNCTION fail_user_tombstone_once()`); err != nil {
		t.Fatalf("remove tombstone failure function: %v", err)
	}

	if err := service.DeleteUser(ctx, provisioned.User.ID, nil); err != nil {
		t.Fatalf("retry deletion to convergence: %v", err)
	}
	if err := service.DeleteUser(ctx, provisioned.User.ID, nil); err != nil {
		t.Fatalf("completed deletion idempotency retry: %v", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionCompleted, 0, "frozen")
	if len(authDeleter.authUserIDs) != 2 {
		t.Fatalf("Auth delete calls = %v, want initial failure plus one retry", authDeleter.authUserIDs)
	}
	if len(cleaner.calls) != 2 {
		t.Fatalf("storage cleanup calls = %v, want failure plus one successful retry", cleaner.calls)
	}
	var authUserID *string
	var retainedAuthUserID, retainedStorageUID *string
	var displayName, tombstoneStorageUID string
	var taskCount, claimCount int
	if err := pool.QueryRow(ctx, `SELECT auth_user_id, display_name, storage_uid FROM users WHERE id = $1`, provisioned.User.ID).Scan(&authUserID, &displayName, &tombstoneStorageUID); err != nil {
		t.Fatalf("read converged tombstone: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT auth_user_id, storage_uid FROM user_deletion_requests WHERE user_id = $1`, provisioned.User.ID).Scan(&retainedAuthUserID, &retainedStorageUID); err != nil {
		t.Fatalf("read completed deletion request PII: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE user_id = $1`, provisioned.User.ID).Scan(&taskCount); err != nil {
		t.Fatalf("count retained tasks: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_welcome_claims WHERE claimed_user_id = $1`, provisioned.User.ID).Scan(&claimCount); err != nil {
		t.Fatalf("count retained Welcome claim: %v", err)
	}
	if authUserID != nil || retainedAuthUserID != nil || retainedStorageUID != nil || displayName != "已注销用户" || !IsValidStorageUID(tombstoneStorageUID) || tombstoneStorageUID == provisioned.User.StorageUID || taskCount != 1 || claimCount != 1 {
		t.Fatalf("tombstone auth=%v request_auth=%v request_storage=%v display_name=%q storage_uid=%q tasks=%d claims=%d", authUserID, retainedAuthUserID, retainedStorageUID, displayName, tombstoneStorageUID, taskCount, claimCount)
	}
}

func TestUserDeletionIsBlockedByUnsettledPaymentState(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	lifecycle := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	provisioned, err := lifecycle.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-delete-payment-guard", DisplayName: "delete-payment-guard",
		ClaimIdentity: "delete-payment-guard@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("provision deletion user: %v", err)
	}
	secretstest.EnableEncryption(t)
	apiKeyCiphertext, err := secrets.EncryptString("sk_test_delete_guard")
	if err != nil {
		t.Fatalf("encrypt payment API key fixture: %v", err)
	}
	webhookSecretCiphertext, err := secrets.EncryptString("whsec_test_delete_guard")
	if err != nil {
		t.Fatalf("encrypt payment webhook secret fixture: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO payment_stripe_credentials (
  id, account_id, mode, charges_enabled, stripe_product_id, stripe_webhook_endpoint_id,
  api_key_ciphertext, webhook_secret_ciphertext, secret_material_kind, created_by
)
VALUES (
  '0198cd29-f457-7e4b-b95e-6cfde41b9777', 'acct_delete_guard', 'test', true,
	  'prod_delete_guard', 'we_delete_guard', $1, $2, 'encrypted', 42
)`, apiKeyCiphertext, webhookSecretCiphertext); err != nil {
		t.Fatalf("seed payment credential generation: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO payment_orders (
  id, public_order_number, public_token, user_id, client_idempotency_key, request_hash,
  status, stripe_credential_id, stripe_account_id, stripe_product_id, idea_quantity, base_amount_minor, terms_version,
  refund_policy_version, return_base_url, expires_at
)
VALUES (
  '0198cd29-f457-7e4b-b95e-6cfde41b9555', 'IBP-20260805-GUARD1234567',
  '0198cd29-f457-7e4b-b95e-6cfde41b9666', $1, 'delete-payment-guard',
  '841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f',
  'creating', '0198cd29-f457-7e4b-b95e-6cfde41b9777', 'acct_delete_guard', 'prod_delete_guard', 501, 501, 'v1', 'v1',
  'https://app.example.com', now() + interval '30 minutes'
)`, provisioned.User.ID); err != nil {
		t.Fatalf("seed unsettled payment order: %v", err)
	}

	if err := lifecycle.DeleteUser(ctx, provisioned.User.ID, &recordingAuthUserDeleter{}, &recordingUserUploadCleaner{}); !errors.Is(err, ErrUserDeletionBlockedByPaymentState) {
		t.Fatalf("DeleteUser() error = %v, want ErrUserDeletionBlockedByPaymentState", err)
	}
	var deletionRequests int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_deletion_requests WHERE user_id = $1`, provisioned.User.ID).Scan(&deletionRequests); err != nil {
		t.Fatalf("read deletion request count: %v", err)
	}
	if deletionRequests != 0 {
		t.Fatalf("deletion request count = %d, want 0", deletionRequests)
	}
	var userStatus int16
	var walletStatus string
	if err := pool.QueryRow(ctx, `
SELECT users.status, credit_wallets.status
FROM users JOIN credit_wallets ON credit_wallets.user_id = users.id
WHERE users.id = $1`, provisioned.User.ID).Scan(&userStatus, &walletStatus); err != nil {
		t.Fatalf("read payment-guard state: %v", err)
	}
	if userStatus != 1 || walletStatus != "active" {
		t.Fatalf("payment-guard state user=%d wallet=%s, want active/active", userStatus, walletStatus)
	}
}

func TestIdentityAdminCommandsResumeDeletionFromPendingStorage(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	lifecycle := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
	provisioned, err := lifecycle.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-command-delete-resume", DisplayName: "command-delete-resume",
		ClaimIdentity: "command-delete-resume@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("provision deletion user: %v", err)
	}

	cleaner := &recordingUserUploadCleaner{err: errors.New("storage unavailable")}
	userService := NewUserService(NewUserRepository(db), lifecycle, nil, cleaner)
	events := &[]string{}
	authManager := &fakeIdentityAdminAuth{events: events}
	commands := newIdentityAdminCommands(userService, authManager)
	actor := identityAdminActor(t)

	err = commands.Delete(ctx, actor, provisioned.User.PublicID.String())
	assertIdentityAdminErrorKind(t, err, IdentityAdminBusinessFailure)
	assertDeletionState(t, pool, provisioned.User.ID, DeletionPendingStorage, 0, "frozen")

	cleaner.err = nil
	commands.auth = nil
	if err := commands.Delete(ctx, actor, provisioned.User.PublicID.String()); err != nil {
		t.Fatalf("resume deletion from pending storage: %v", err)
	}
	assertDeletionState(t, pool, provisioned.User.ID, DeletionCompleted, 0, "frozen")

	// A lost success response can be retried after tombstoning has cleared auth_user_id.
	if err := commands.Delete(ctx, actor, provisioned.User.PublicID.String()); err != nil {
		t.Fatalf("retry completed deletion: %v", err)
	}
	assertIdentityAdminEvents(t, events, "auth-delete")
}

func TestProvisionConcurrentWithBanOrDeleteNeverLeavesInactiveUserWithActiveWallet(t *testing.T) {
	for _, operation := range []string{"ban", "delete"} {
		t.Run(operation, func(t *testing.T) {
			_, pool := testdb.OpenWithPool(t)
			ctx := context.Background()
			lifecycle := NewLifecycleService(pool, credits.NewService(pool), []byte("0123456789abcdef0123456789abcdef"))
			identity := VerifiedIdentity{
				AuthUserID: "auth-concurrent-" + operation, DisplayName: "concurrent-" + operation,
				ClaimIdentity: "concurrent-" + operation + "@example.com", ClaimIdentityKind: "canonical_email",
				ClaimIdentityVerified: true, Role: "user",
			}
			created, err := lifecycle.ProvisionIdentity(ctx, identity)
			if err != nil {
				t.Fatalf("seed identity: %v", err)
			}

			start := make(chan struct{})
			errs := make(chan error, 2)
			go func() {
				<-start
				_, err := lifecycle.ProvisionIdentity(ctx, identity)
				errs <- err
			}()
			go func() {
				<-start
				if operation == "ban" {
					errs <- lifecycle.SetUserStatus(ctx, created.User.ID, 0)
					return
				}
				errs <- lifecycle.DeleteUser(ctx, created.User.ID, &recordingAuthUserDeleter{}, &recordingUserUploadCleaner{})
			}()
			close(start)
			for range 2 {
				err := <-errs
				if err != nil && !errors.Is(err, ErrUserDeletionInProgress) {
					t.Fatalf("concurrent %s error: %v", operation, err)
				}
			}

			var userStatus int16
			var walletStatus string
			if err := pool.QueryRow(ctx, `
SELECT users.status, credit_wallets.status
FROM users JOIN credit_wallets ON credit_wallets.user_id = users.id
WHERE users.id = $1`, created.User.ID).Scan(&userStatus, &walletStatus); err != nil {
				t.Fatalf("read final status: %v", err)
			}
			if userStatus != 0 || walletStatus != "frozen" {
				t.Fatalf("final state user=%d wallet=%s, want inactive/frozen", userStatus, walletStatus)
			}
		})
	}
}

func assertDeletionState(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID int64, wantPhase DeletionPhase, wantUserStatus int16, wantWalletStatus string) {
	t.Helper()
	var phase DeletionPhase
	var userStatus int16
	var walletStatus string
	if err := pool.QueryRow(t.Context(), `
SELECT request.phase, users.status, wallet.status
FROM user_deletion_requests request
JOIN users ON users.id = request.user_id
JOIN credit_wallets wallet ON wallet.user_id = request.user_id
WHERE request.user_id = $1`, userID).Scan(&phase, &userStatus, &walletStatus); err != nil {
		t.Fatalf("read deletion state: %v", err)
	}
	if phase != wantPhase || userStatus != wantUserStatus || walletStatus != wantWalletStatus {
		t.Fatalf("deletion state phase=%s user=%d wallet=%s, want %s/%d/%s", phase, userStatus, walletStatus, wantPhase, wantUserStatus, wantWalletStatus)
	}
}
