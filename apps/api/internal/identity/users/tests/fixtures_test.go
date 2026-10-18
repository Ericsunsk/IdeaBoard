package tests

import (
	"context"
	"fmt"
	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/users/application/administration"
	"ideaboard/api/internal/identity/users/application/authsync"
	"ideaboard/api/internal/identity/users/application/lifecycle"
	"ideaboard/api/internal/identity/users/application/provisioning"
	"ideaboard/api/internal/identity/users/application/reconciliation"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/testdb"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	User            = identitydomain.User
	UserListRequest = ports.UserListRequest

	VerifiedIdentity = ports.VerifiedIdentity
	ProvisionResult  = ports.ProvisionResult

	DeletionPhase = ports.DeletionPhase

	IdentityAdminErrorKind    = ports.IdentityAdminErrorKind
	IdentityAdminCommandError = ports.IdentityAdminCommandError
	IdentityAdminBanInput     = ports.IdentityAdminBanInput

	AuthUserManager              = ports.AuthUserManager
	AuthUserDeleter              = ports.AuthUserDeleter
	UserUploadCleaner            = ports.UserUploadCleaner
	CreditWalletLifecycleService = ports.CreditWalletLifecycleService
	UserCreditBalanceReader      = ports.UserCreditBalanceReader
	IdentityAdminCommands        = ports.IdentityAdminCommands
	UserLifecycle                = ports.UserLifecycle

	UserRepository = postgres.Repository
	UserService    = administration.Service
)

const (
	RoleUser  = identitydomain.RoleUser
	RoleAdmin = identitydomain.RoleAdmin

	StatusActive = identitydomain.StatusActive
	StatusBanned = identitydomain.StatusBanned

	DeletionPendingAuth      = ports.DeletionPendingAuth
	DeletionPendingStorage   = ports.DeletionPendingStorage
	DeletionPendingTombstone = ports.DeletionPendingTombstone
	DeletionCompleted        = ports.DeletionCompleted

	IdentityAdminInvalid              = ports.IdentityAdminInvalid
	IdentityAdminConflict             = ports.IdentityAdminConflict
	IdentityAdminForbidden            = ports.IdentityAdminForbidden
	IdentityAdminTargetNotFound       = ports.IdentityAdminTargetNotFound
	IdentityAdminAuthIdentityNotFound = ports.IdentityAdminAuthIdentityNotFound
	IdentityAdminAuthFailure          = ports.IdentityAdminAuthFailure
	IdentityAdminBusinessFailure      = ports.IdentityAdminBusinessFailure
	IdentityAdminUnavailable          = ports.IdentityAdminUnavailable
)

var (
	ErrUserDeletionInProgress            = identitydomain.ErrUserDeletionInProgress
	ErrUserDeletionBlockedByPaymentState = identitydomain.ErrUserDeletionBlockedByPaymentState

	NewUserRepository        = postgres.NewRepository
	NewIdentityAdminCommands = administration.NewIdentityAdminCommands
	IsValidStorageUID        = identitydomain.IsValidStorageUID
)

type LifecycleService struct {
	provisioning *provisioning.Service
	lifecycle    *lifecycle.Service
}

func NewLifecycleService(pool *pgxpool.Pool, creditService CreditWalletLifecycleService, welcomeIdentityKey []byte) *LifecycleService {
	return &LifecycleService{
		provisioning: provisioning.NewService(pool, creditService, welcomeIdentityKey),
		lifecycle:    lifecycle.NewService(pool, creditService),
	}
}

func (s *LifecycleService) VerifyWelcomeIdentityKey(ctx context.Context) error {
	return s.provisioning.VerifyWelcomeIdentityKey(ctx)
}

func (s *LifecycleService) ProvisionIdentity(ctx context.Context, identity VerifiedIdentity) (*ProvisionResult, error) {
	return s.provisioning.ProvisionIdentity(ctx, identity)
}

func (s *LifecycleService) InitializeExistingIdentities(ctx context.Context, identities []VerifiedIdentity) ([]ProvisionResult, error) {
	return s.provisioning.InitializeExistingIdentities(ctx, identities)
}

func (s *LifecycleService) SetUserStatus(ctx context.Context, userID int64, status int16) error {
	return s.lifecycle.SetUserStatus(ctx, userID, status)
}

func (s *LifecycleService) DeleteUser(ctx context.Context, userID int64, authDeleter AuthUserDeleter, uploadCleaner UserUploadCleaner) error {
	return s.lifecycle.DeleteUser(ctx, userID, authDeleter, uploadCleaner)
}

func NewUserService(
	repo *UserRepository,
	lifecycle UserLifecycle,
	creditBalanceReader UserCreditBalanceReader,
	uploadCleaner ...UserUploadCleaner,
) *UserService {
	var cleaner UserUploadCleaner
	if len(uploadCleaner) > 0 {
		cleaner = uploadCleaner[0]
	}
	return administration.NewService(repo, lifecycle, creditBalanceReader, cleaner, nil)
}

func newIdentityAdminCommands(users ports.UserMutationQueries, auth ports.AuthUserManager, sync *authsync.Store) ports.IdentityAdminCommands {
	return administration.NewIdentityAdminCommands(users, auth, sync)
}

func newUserRepoTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Open(t)
}

type userUploadCleanupCall struct {
	userID     int64
	storageUID string
}

type recordingUserUploadCleaner struct {
	calls []userUploadCleanupCall
	err   error
}

func (c *recordingUserUploadCleaner) DeleteUserUploads(_ context.Context, userID int64, storageUID string) error {
	c.calls = append(c.calls, userUploadCleanupCall{userID: userID, storageUID: storageUID})
	return c.err
}

type spyUserCreditBalanceReader struct {
	calls    [][]int64
	balances map[int64]int64
}

func (r *spyUserCreditBalanceReader) GetAvailableBalances(_ context.Context, userIDs []int64) (map[int64]int64, error) {
	r.calls = append(r.calls, append([]int64(nil), userIDs...))
	balances := make(map[int64]int64, len(userIDs))
	for _, userID := range userIDs {
		balances[userID] = r.balances[userID]
	}
	return balances, nil
}

type recordingUserLifecycle struct {
	statusCalls []int16
	deleteCalls []int64
	deleteErr   error
	deleteFn    func(context.Context, int64, AuthUserDeleter, UserUploadCleaner) error
}

func (l *recordingUserLifecycle) SetUserStatus(_ context.Context, _ int64, status int16) error {
	l.statusCalls = append(l.statusCalls, status)
	return nil
}

func (l *recordingUserLifecycle) DeleteUser(ctx context.Context, userID int64, authDeleter AuthUserDeleter, uploadCleaner UserUploadCleaner) error {
	l.deleteCalls = append(l.deleteCalls, userID)
	if l.deleteFn != nil {
		return l.deleteFn(ctx, userID, authDeleter, uploadCleaner)
	}
	return l.deleteErr
}

type recordingAuthUserDeleter struct {
	authUserIDs []string
	err         error
}

func (d *recordingAuthUserDeleter) DeleteUser(_ context.Context, authUserID string) error {
	d.authUserIDs = append(d.authUserIDs, authUserID)
	return d.err
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// reconciliation resource helpers (merged from reconciliation_helpers_test.go,
// renamed by resource).
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
