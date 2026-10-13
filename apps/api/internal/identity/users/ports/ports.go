package ports

import (
	"context"
	"time"

	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/trust"
	"ideaboard/api/internal/identity/users/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DTOs for user queries and administration
type UserListRequest struct {
	Keyword  string `form:"keyword"`
	Role     string `form:"role"`
	Status   string `form:"status"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=10"`
}

type UserListCounts struct {
	Total       int64 `json:"total"`
	ActiveCount int64 `json:"active_count"`
	BannedCount int64 `json:"banned_count"`
	AdminCount  int64 `json:"admin_count"`
}

type AdminUserListItem struct {
	PublicID         uuid.UUID `json:"public_id"`
	Email            string    `json:"email"`
	Providers        []string  `json:"providers"`
	DisplayName      string    `json:"display_name"`
	Avatar           string    `json:"avatar,omitempty"`
	Role             string    `json:"role"`
	Status           int16     `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	AvailableCredits int64     `json:"available_credits"`
	LastLoginIP      string    `json:"last_login_ip,omitempty"`
	LastLoginCountry string    `json:"last_login_country,omitempty"`
}

type UserListResponse struct {
	UserListCounts
	List []*AdminUserListItem `json:"list"`
}

type AdminBanUserRequest struct {
	ExpiresInDays int `json:"expires_in_days"`
}

type LoginMetadataRequest struct {
	AuthUserID string `json:"auth_user_id"`
	IP         string `json:"ip"`
	Country    string `json:"country"`
}

// Provisioning types
type VerifiedIdentity = trust.ProvisioningIdentity

type ProvisionResult struct {
	User           domain.User
	Wallet         WalletBalance
	WelcomeGranted bool
}

// WalletBalance is the Identity read model needed after provisioning.
// Credits owns the full wallet projection and translates it at the adapter boundary.
type WalletBalance struct {
	UserID      int64     `json:"-"`
	PublicID    uuid.UUID `json:"public_id"`
	DisplayName string    `json:"display_name"`
	Available   int64     `json:"available_credits"`
	Reserved    int64     `json:"reserved_credits"`
}

type WalletDetails struct {
	WalletBalance
}

// Deletion lifecycle types
type DeletionPhase string

const (
	DeletionPendingAuth      DeletionPhase = "pending_auth"
	DeletionPendingStorage   DeletionPhase = "pending_storage"
	DeletionPendingTombstone DeletionPhase = "pending_tombstone"
	DeletionCompleted        DeletionPhase = "completed"
)

type DeletionProgress struct {
	UserID     int64
	AuthUserID string
	StorageUID string
	Phase      DeletionPhase
}

// Admin command error types
type IdentityAdminErrorKind string

const (
	IdentityAdminInvalid                IdentityAdminErrorKind = "invalid"
	IdentityAdminConflict               IdentityAdminErrorKind = "conflict"
	IdentityAdminForbidden              IdentityAdminErrorKind = "forbidden"
	IdentityAdminTargetNotFound         IdentityAdminErrorKind = "target_not_found"
	IdentityAdminAuthIdentityNotFound   IdentityAdminErrorKind = "auth_identity_not_found"
	IdentityAdminAuthFailure            IdentityAdminErrorKind = "auth_failure"
	IdentityAdminBusinessFailure        IdentityAdminErrorKind = "business_failure"
	IdentityAdminReconciliationRequired IdentityAdminErrorKind = "reconciliation_required"
	IdentityAdminUnavailable            IdentityAdminErrorKind = "unavailable"
)

type IdentityAdminCommandError struct {
	Kind          IdentityAdminErrorKind
	Message       string
	Cause         error
	RollbackCause error
}

func (e *IdentityAdminCommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "identity admin command failed"
}

func (e *IdentityAdminCommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type IdentityAdminBanInput struct {
	ExpiresInDays int
}

// External Integration Ports
type AuthUserManager interface {
	DeleteUser(ctx context.Context, authUserID string) error
	SetUserRole(ctx context.Context, authUserID string, role string) error
	BanUser(ctx context.Context, authUserID string, expiresInSeconds int) error
	UnbanUser(ctx context.Context, authUserID string) error
	RevokeUserSessions(ctx context.Context, authUserID string) error
}

type AuthUserDeleter interface {
	DeleteUser(context.Context, string) error
}

type UserUploadCleaner interface {
	DeleteUserUploads(ctx context.Context, userID int64, storageUID string) error
}

type WelcomeGrantCommand struct {
	UserID                int64
	Amount                int64
	IdempotencyKey        string
	ActorType             string
	ActorID               string
	Source                string
	Reason                string
	BusinessReferenceType string
	BusinessReferenceID   string
}

type WelcomeGrantResult struct {
	Wallet WalletBalance
}

type WalletLifecycle interface {
	BindWelcomeIdentityKey(context.Context, string) error
	ProvisionWallet(context.Context, int64) error
	SetWalletFrozen(context.Context, int64, bool) error
	ClaimWelcomeGrant(context.Context, string, WelcomeGrantCommand) (WelcomeGrantResult, bool, error)
}

type CreditWalletLifecycleService interface {
	LifecycleInTx(pgx.Tx) WalletLifecycle
	GetWallet(context.Context, int64) (WalletDetails, error)
}

type UserCreditBalanceReader interface {
	GetAvailableBalances(context.Context, []int64) (map[int64]int64, error)
}

type UserEmailReader interface {
	GetUserEmails(context.Context, []string) (map[string]authidentity.UserIdentity, error)
}

// AuthUserStateReader is the narrow Auth-side read seam for the Identity
// admin reconciliation job (dry-run alerting only). Implementations must be
// read-only: unknown IDs are omitted from the result, never fabricated.
type AuthUserStateReader interface {
	GetUserStates(context.Context, []string) (map[string]authidentity.AuthUserState, error)
}

type AdminUserImpersonator interface {
	ImpersonateUser(ctx context.Context, cookieHeader string, authUserID string) ([]string, error)
}

// IdentityAdminMutationOutcome distinguishes a committed business
// mutation with Auth sync still pending from a rolled-back failure
// (issue #888). Success with SyncPending=true means the business facts
// (role/status) are durably committed with a sync intent and Auth
// convergence follows by retry; any returned error means nothing was
// committed. Handlers must render SyncPending so operators never mistake
// "accepted, syncing" for "fully converged" or "failed".
type IdentityAdminMutationOutcome struct {
	SyncPending bool
	IntentID    int64
}

// IdentityAdminCommands is the complete admin identity mutation surface used by transport adapters.
type IdentityAdminCommands interface {
	SetRole(ctx context.Context, actor principal.Principal, targetPublicID string, role string) (IdentityAdminMutationOutcome, error)
	Ban(ctx context.Context, actor principal.Principal, targetPublicID string, input IdentityAdminBanInput) (IdentityAdminMutationOutcome, error)
	Unban(ctx context.Context, actor principal.Principal, targetPublicID string) (IdentityAdminMutationOutcome, error)
	RevokeSessions(ctx context.Context, actor principal.Principal, targetPublicID string) (IdentityAdminMutationOutcome, error)
	Delete(ctx context.Context, actor principal.Principal, targetPublicID string) (IdentityAdminMutationOutcome, error)
}

// UserMutationQueries defines the narrow seam required by admin mutations.
type UserMutationQueries interface {
	GetByPublicID(ctx context.Context, publicID string) (*domain.User, error)
	UpdateRole(ctx context.Context, userID int64, role string) error
	UpdateStatus(ctx context.Context, userID int64, status int16) error
	DeleteUser(ctx context.Context, userID int64, authDeleter AuthUserDeleter) error
}

// UserLifecycle governs user status changes and multi-phase account deletion.
type UserLifecycle interface {
	SetUserStatus(context.Context, int64, int16) error
	DeleteUser(context.Context, int64, AuthUserDeleter, UserUploadCleaner) error
}

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	GetByIDs(ctx context.Context, ids []int64) (map[int64]*domain.User, error)
	GetByPublicID(ctx context.Context, rawPublicID string) (*domain.User, error)
	GetByAuthUserID(ctx context.Context, authUserID string) (*domain.User, error)
	GetByStorageUID(ctx context.Context, storageUID string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	UpdateLastLoginMetadata(ctx context.Context, authUserID, ip, country string) error
	List(ctx context.Context, keyword, role string, status int16, hasStatus bool, page, pageSize int) ([]*domain.User, UserListCounts, error)
	UpdateStatus(ctx context.Context, id int64, status int16) error
	UpdateRoleByID(ctx context.Context, id int64, role string) error
	UpdateAvatarByID(ctx context.Context, id int64, avatar string) error
	EnsureStorageUIDs(ctx context.Context) error
}
