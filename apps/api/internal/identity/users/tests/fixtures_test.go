package tests

import (
	"context"
	"testing"

	"ideaboard/api/internal/identity/users/application/administration"
	"ideaboard/api/internal/identity/users/application/lifecycle"
	"ideaboard/api/internal/identity/users/application/provisioning"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/infrastructure/postgres"
	userhttp "ideaboard/api/internal/identity/users/interfaces/http"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	User                 = identitydomain.User
	UsedUID              = identitydomain.UsedUID
	UserListRequest      = ports.UserListRequest
	UserListCounts       = ports.UserListCounts
	UserListResponse     = ports.UserListResponse
	AdminUserListItem    = ports.AdminUserListItem
	AdminBanUserRequest  = ports.AdminBanUserRequest
	LoginMetadataRequest = ports.LoginMetadataRequest

	VerifiedIdentity = ports.VerifiedIdentity
	ProvisionResult  = ports.ProvisionResult

	DeletionPhase    = ports.DeletionPhase
	DeletionProgress = ports.DeletionProgress

	IdentityAdminErrorKind    = ports.IdentityAdminErrorKind
	IdentityAdminCommandError = ports.IdentityAdminCommandError
	IdentityAdminBanInput     = ports.IdentityAdminBanInput

	AuthUserManager              = ports.AuthUserManager
	AuthUserDeleter              = ports.AuthUserDeleter
	UserUploadCleaner            = ports.UserUploadCleaner
	CreditWalletLifecycleService = ports.CreditWalletLifecycleService
	UserCreditBalanceReader      = ports.UserCreditBalanceReader
	UserEmailReader              = ports.UserEmailReader
	AdminUserImpersonator        = ports.AdminUserImpersonator
	ImpersonationAuditor         = userhttp.ImpersonationAuditor
	IdentityAdminCommands        = ports.IdentityAdminCommands
	UserLifecycle                = ports.UserLifecycle

	UserRepository  = postgres.Repository
	UserService     = administration.Service
	AdminHandler    = userhttp.AdminHandler
	InternalHandler = userhttp.InternalHandler
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

	IdentityAdminInvalid                = ports.IdentityAdminInvalid
	IdentityAdminConflict               = ports.IdentityAdminConflict
	IdentityAdminForbidden              = ports.IdentityAdminForbidden
	IdentityAdminTargetNotFound         = ports.IdentityAdminTargetNotFound
	IdentityAdminAuthIdentityNotFound   = ports.IdentityAdminAuthIdentityNotFound
	IdentityAdminAuthFailure            = ports.IdentityAdminAuthFailure
	IdentityAdminBusinessFailure        = ports.IdentityAdminBusinessFailure
	IdentityAdminReconciliationRequired = ports.IdentityAdminReconciliationRequired
	IdentityAdminUnavailable            = ports.IdentityAdminUnavailable
)

var (
	ErrUserNotFound                      = identitydomain.ErrUserNotFound
	ErrUserDeletionInProgress            = identitydomain.ErrUserDeletionInProgress
	ErrUserDeletionBlockedByPaymentState = identitydomain.ErrUserDeletionBlockedByPaymentState
	ErrUserDeletionAuthUnavailable       = identitydomain.ErrUserDeletionAuthUnavailable
	ErrUserDeletionStorageUnavailable    = identitydomain.ErrUserDeletionStorageUnavailable
	ErrInvalidStorageUID                 = identitydomain.ErrInvalidStorageUID

	NewUserRepository                = postgres.NewRepository
	NewUserServiceWithEmailReader    = administration.NewService
	NewIdentityAdminCommands         = administration.NewIdentityAdminCommands
	NewAdminHandler                  = userhttp.NewAdminHandler
	NewInternalHandler               = userhttp.NewInternalHandler
	NewStorageUID                    = identitydomain.NewStorageUID
	IsValidStorageUID                = identitydomain.IsValidStorageUID
	RespondIdentityAdminCommandError = userhttp.RespondIdentityAdminCommandError
	respondIdentityAdminCommandError = userhttp.RespondIdentityAdminCommandError
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

func newIdentityAdminCommands(users ports.UserMutationQueries, auth ports.AuthUserManager) ports.IdentityAdminCommands {
	return administration.NewIdentityAdminCommands(users, auth)
}

func identityAdminError(kind IdentityAdminErrorKind, message string, cause error) *IdentityAdminCommandError {
	return &IdentityAdminCommandError{Kind: kind, Message: message, Cause: cause}
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
