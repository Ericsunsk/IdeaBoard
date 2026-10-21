package tenancy

import (
	"context"
	"errors"
	"strings"
	"time"

	eventoutbox "ideaboard/api/internal/events/outbox"
)

// Membership roles (agreed with the DB sibling stream).
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Billing Account financial roles (#915). Financial management (recharge,
// bills, member quotas, workspace budgets) is separate from Workspace
// content membership. Owner and Admin both manage recharge/bills/quotas;
// only Owner transfers ownership. Financial management never grants Board
// content access.
const (
	BillingRoleOwner = "owner"
	BillingRoleAdmin = "admin"
)

const MaxWorkspaceMembers = 5

// MaxBillingAccountMembers bounds explicit financial managers per account.
// One User may manage many independent accounts; this cap is per account.
const MaxBillingAccountMembers = 10

var (
	// ErrNotFound reports an invisible or missing resource. Handlers render
	// it as 404 (workspace_not_found / organization_not_found /
	// workspace_member_not_found) per Baseline 4 anti-enumeration.
	ErrNotFound = errors.New("tenancy: not found")
	// ErrForbidden reports a visible-but-forbidden action (403).
	ErrForbidden = errors.New("tenancy: forbidden")
	// ErrLastOwner blocks removing or demoting the last owner (403 with
	// membership_transition_forbidden).
	ErrLastOwner = errors.New("tenancy: last owner cannot be removed or demoted")
	// ErrConflict reports a public_id collision during minting; callers
	// retry with a fresh identifier.
	ErrConflict = errors.New("tenancy: public id conflict")
	// ErrWorkspaceMemberLimitReached includes every role, including owners.
	ErrWorkspaceMemberLimitReached = errors.New("tenancy: workspace member limit reached")
	// ErrInvalidName reports a display name outside 1..120 characters.
	ErrInvalidName = errors.New("tenancy: name must be 1..120 characters")
	// ErrWorkspaceOwnerRequired reports a workspace create without an
	// organization or an owner.
	ErrWorkspaceOwnerRequired = errors.New("tenancy: workspace requires an organization or an owner")
	// ErrDefaultWorkspaceProtected reports a delete/rename
	// attempt on a Default workspace. Handlers render it as 403
	// (workspace_default_protected): the Default workspace is never
	// deleted or renamed.
	ErrDefaultWorkspaceProtected = errors.New("tenancy: default workspace cannot be deleted or renamed")
	// ErrWorkspaceActiveTasks blocks deletion until pending/running Tasks settle.
	ErrWorkspaceActiveTasks = errors.New("tenancy: workspace has active tasks")
	// ErrDefaultWorkspaceConflict reports a second Default workspace create
	// for an owner that already holds a Default. Handlers render it
	// as 409 (workspace_default_conflict).
	ErrDefaultWorkspaceConflict = errors.New("tenancy: default workspace already exists")
	// ErrPersonalWorkspaceUserRequired reports a default workspace
	// provision without a user.
	ErrPersonalWorkspaceUserRequired = errors.New("tenancy: default workspace requires a user")
	// ErrBillingStoreNotConfigured reports a billing account store
	// without queries.
	ErrBillingStoreNotConfigured = errors.New("tenancy: billing account store not configured")
	// ErrBillingWorkspaceInvalid reports billing account provisioning
	// with an invalid workspace.
	ErrBillingWorkspaceInvalid = errors.New("tenancy: billing account workspace is invalid")
	// ErrBillingNotFound reports a missing billing account. Handlers render
	// it as 404 billing_account_not_found after the workspace visibility
	// gate (hidden workspaces still converge to workspace_not_found first,
	// per Baseline 4 anti-enumeration).
	ErrBillingNotFound = errors.New("tenancy: billing account not found")
	// ErrBillingForbidden reports a visible-but-forbidden financial action
	// (403 billing_account_forbidden): the caller holds Workspace standing
	// but no Billing Account Owner/Admin grant.
	ErrBillingForbidden = errors.New("tenancy: billing account forbidden")
	// ErrBillingConflict reports an ambiguous resolution or a conflicting
	// ownership association (409 billing_account_conflict). Callers fail
	// closed instead of silently picking another wallet.
	ErrBillingConflict = errors.New("tenancy: billing account conflict")
	// ErrBillingAmbiguous reports multiple candidate accounts with no
	// explicit link (409 billing_account_conflict). Same wire mapping as
	// ErrBillingConflict; kept distinct for fail-closed diagnosis.
	ErrBillingAmbiguous = errors.New("tenancy: billing account ambiguous")
	// ErrBillingLastOwner blocks removing or demoting the last billing
	// owner (403 with billing_account_forbidden).
	ErrBillingLastOwner = errors.New("tenancy: last billing owner cannot be removed or demoted")
	// ErrDefaultQuotaNotConfigured reports an invite blocked by a missing
	// account default new-member quota policy (#947, fail closed). The
	// membership write rolls back; handlers render
	// default_member_quota_not_configured (409) and guide the caller to
	// have a Billing Owner/Admin set the default first. Absent default
	// is never an implicit unlimited.
	ErrDefaultQuotaNotConfigured = errors.New("tenancy: default member quota not configured")
	// ErrNotUserPublicID reports an identifier without the usr_ prefix.
	ErrNotUserPublicID = errors.New("tenancy: not a user public id")
	// ErrMalformedUserPublicID reports a usr_ identifier with bad hex.
	ErrMalformedUserPublicID = errors.New("tenancy: malformed user public id")
)

// IsValidRole reports whether role is a known membership role.
func IsValidRole(role string) bool {
	switch strings.TrimSpace(role) {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	default:
		return false
	}
}

// Workspace mirrors the agreed workspaces row: internal numeric key for
// joins, opaque ws_ public_id for API exposure, NULL-able organization
// and owner references and the Default marker. Deletion is an immediate
// irreversible hard delete: there is no archived state.
type Workspace struct {
	InternalID     int64
	PublicID       string
	OrganizationID *string
	OwnerUserID    *int64
	Name           string
	// IsDefault marks the user's Default workspace: exactly one row
	// per owner, never deleted or renamed, auto-provisioned
	// when missing. Organization workspaces never carry it.
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsPersonal reports whether the workspace is a personal workspace
// (organization_id IS NULL).
func (w *Workspace) IsPersonal() bool {
	return w != nil && w.OrganizationID == nil
}

// IsDefaultWorkspace reports whether the workspace is the owner's Default
// workspace (personal scope plus the is_default marker).
func (w *Workspace) IsDefaultWorkspace() bool {
	return w != nil && w.IsPersonal() && w.IsDefault
}

// Organization mirrors the agreed organizations row.
type Organization struct {
	InternalID int64
	PublicID   string
	Name       string
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// WorkspaceMember is one workspace_memberships row projected for the API.
type WorkspaceMember struct {
	WorkspacePublicID string
	UserID            int64
	UserExternalID    string
	Role              string
	CreatedAt         time.Time
}

// CreateWorkspaceParams carries validated workspace creation input.
// IsDefault requests the Default marker: only personal scope may carry
// it, and an owner holding a Default rejects a second one with
// ErrDefaultWorkspaceConflict. Plain POST creation always passes false
// (non-Default personal workspaces are unlimited).
// Billing linkage (#944, creation-time explicit account):
//   - Organization workspaces always use the organization pool; passing
//     either billing field with an OrganizationID fails closed with
//     ErrBillingConflict (never silently pick the creator personal pool).
//   - Personal workspaces default to legacy auto mode (no link, per-workspace
//     account resolved lazily) when both fields are empty, preserving the
//     #915 resolution contract for direct Store callers. The Public API
//     passes an explicit BillingAccountPublicID (shared, caller must be
//     Billing Owner/Admin) or NewBillingAccount=true (fresh isolated
//     per-workspace account, creator becomes billing owner) so creation
//     never auto-creates a wallet implicitly and never leaves a half-linked
//     workspace on authorization failure (single transaction, fail closed).
//   - BillingAccountPublicID and NewBillingAccount are mutually exclusive
//     (both set reports ErrBillingConflict). IsDefault workspaces reject
//     both (the Default row owns the deterministic personal account).
//
// Repository methods take context.Context first.
type CreateWorkspaceParams struct {
	Name           string
	OrganizationID *string
	OwnerUserID    int64
	IsDefault      bool
	// BillingAccountPublicID names an existing ba_ account to share
	// (explicit workspace_billing_links row, same transaction).
	BillingAccountPublicID *string
	// NewBillingAccount mints a fresh isolated per-workspace account
	// anchored to the new workspace (creator becomes billing owner).
	NewBillingAccount bool
}

// Store is the AGREED tenancy persistence interface the DB sibling
// implements against sqlc. Production swaps in the postgres
// implementation without handler changes.
type Store interface {
	WorkspaceStore
	OrganizationStore
}

// MemberGrantCleanup describes a Board access change caused by membership departure.
type MemberGrantCleanup struct {
	BoardPublicID string
	AccessVersion int64
	AccessSignals []eventoutbox.BoardAccessOutboxRecord
}

// WorkspaceMemberGrantStore joins Board access cleanup to a membership departure.
type WorkspaceMemberGrantStore interface {
	RemoveWorkspaceMemberGrants(ctx context.Context, workspacePublicID string, userID int64) ([]MemberGrantCleanup, error)
}

// WorkspaceStore groups workspace and workspace-membership persistence.
// DeleteWorkspace is an immediate irreversible hard delete: it removes
// the workspace row with its boards and files in one operation. Billing
// records, the credit ledger, user rows and control audit records are
// retained. The Default workspace is never deleted
// (ErrDefaultWorkspaceProtected); pending/running Tasks block deletion
// (ErrWorkspaceActiveTasks) until canonical settlement completes.
// Unknown ids read as ErrNotFound.
type WorkspaceStore interface {
	GetWorkspace(ctx context.Context, publicID string) (*Workspace, error)
	ListWorkspacesForUser(ctx context.Context, userID int64) ([]*Workspace, error)
	CreateWorkspace(ctx context.Context, params CreateWorkspaceParams) (*Workspace, error)
	UpdateWorkspace(ctx context.Context, publicID, name string) (*Workspace, error)
	DeleteWorkspace(ctx context.Context, publicID string) error
	// EnsureDefaultWorkspace idempotently provisions the user's Default
	// workspace (INSERT ... ON CONFLICT DO NOTHING in postgres).
	// Concurrent callers converge to a single Default workspace. When the
	// user holds no Default (fresh profile, or only non-Default
	// personal rows), a new Default row is minted; existing non-Default
	// rows are left untouched.
	EnsureDefaultWorkspace(ctx context.Context, userID int64) (*Workspace, error)
	GetWorkspaceRole(ctx context.Context, workspacePublicID string, userID int64) (string, bool, error)
	ListWorkspaceMembers(ctx context.Context, workspacePublicID string) ([]*WorkspaceMember, error)
	AddWorkspaceMember(ctx context.Context, workspacePublicID string, targetUserID int64, role string) (*WorkspaceMember, error)
	UpdateWorkspaceMemberRole(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, newRole string) (*WorkspaceMember, error)
	RemoveWorkspaceMember(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, grants WorkspaceMemberGrantStore) ([]MemberGrantCleanup, error)
}

// OrganizationStore groups organization and org-scoped workspace reads.
type OrganizationStore interface {
	CreateOrganization(ctx context.Context, ownerUserID int64, name string) (*Organization, error)
	GetOrganization(ctx context.Context, publicID string) (*Organization, error)
	ListOrganizationsForUser(ctx context.Context, userID int64) ([]*Organization, error)
	UpdateOrganization(ctx context.Context, publicID, name string) (*Organization, error)
	GetOrganizationRole(ctx context.Context, organizationPublicID string, userID int64) (string, bool, error)
	ListOrganizationWorkspaces(ctx context.Context, organizationPublicID string) ([]*Workspace, error)
	CreateOrganizationWorkspace(ctx context.Context, organizationPublicID string, ownerUserID int64, name string) (*Workspace, error)
}

// ValidateName validates workspace/organization display names (1..120
// after trim, matching the OpenAPI bound).
func ValidateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || len([]rune(trimmed)) > 120 {
		return ErrInvalidName
	}
	return nil
}

// CheckMemberAdd enforces server-side rules for adding a member:
// actor must be owner|admin, and only owners may add owners.
func CheckMemberAdd(actorRole, newRole string) error {
	switch strings.TrimSpace(actorRole) {
	case RoleOwner:
		return nil
	case RoleAdmin:
		if strings.TrimSpace(newRole) == RoleOwner {
			return ErrForbidden
		}
		return nil
	default:
		return ErrForbidden
	}
}

// CheckMemberMutation enforces server-side role-transition rules for
// PATCH (newRole) and DELETE (isRemove) on workspace members:
//
//   - actor must be owner|admin, except a member may remove themselves.
//   - only owners may promote to owner or admin (owner-only promotion).
//   - the last owner cannot be removed or demoted (ownerCount counts
//     current owners including the target when the target is an owner).
func CheckMemberMutation(actorRole string, actorUserID, targetUserID int64, targetCurrentRole, newRole string, ownerCount int, isRemove bool) error {
	actorRole = strings.TrimSpace(actorRole)
	targetCurrentRole = strings.TrimSpace(targetCurrentRole)
	newRole = strings.TrimSpace(newRole)
	isSelf := actorUserID == targetUserID

	if isRemove {
		if targetCurrentRole == RoleOwner && ownerCount <= 1 {
			return ErrLastOwner
		}
		if isSelf {
			return nil
		}
		switch actorRole {
		case RoleOwner:
			return nil
		case RoleAdmin:
			if targetCurrentRole == RoleOwner {
				return ErrForbidden
			}
			return nil
		default:
			return ErrForbidden
		}
	}

	if !IsValidRole(newRole) {
		return ErrForbidden
	}
	if newRole == targetCurrentRole {
		return nil
	}
	if targetCurrentRole == RoleOwner && ownerCount <= 1 {
		return ErrLastOwner
	}
	// Owner-only promotion: raising privilege to admin/owner needs owner.
	elevated := newRole == RoleOwner || newRole == RoleAdmin
	if elevated && actorRole != RoleOwner {
		return ErrForbidden
	}
	switch actorRole {
	case RoleOwner:
		return nil
	case RoleAdmin:
		// Admins may demote admins/members to member but never touch owners.
		if targetCurrentRole == RoleOwner || newRole == RoleOwner {
			return ErrForbidden
		}
		return nil
	default:
		return ErrForbidden
	}
}

// BillingAccountMember is one billing_account_members row.
type BillingAccountMember struct {
	BillingAccountID int64
	UserID           int64
	Role             string
	CreatedAt        time.Time
}

// IsValidBillingRole reports whether role is a known billing manager role.
func IsValidBillingRole(role string) bool {
	switch strings.TrimSpace(role) {
	case BillingRoleOwner, BillingRoleAdmin:
		return true
	default:
		return false
	}
}

// IsBillingManager reports whether a billing role may manage recharge,
// bills, member quotas, and workspace budgets (Owner/Admin both).
func IsBillingManager(role string) bool {
	return IsValidBillingRole(strings.TrimSpace(role))
}

// IsBillingOwner reports whether a billing role may transfer ownership.
func IsBillingOwner(role string) bool {
	return strings.TrimSpace(role) == BillingRoleOwner
}

// CheckBillingMemberAdd enforces server-side rules for granting a billing
// manager: actor must be owner|admin, and only owners may grant owner.
// Workspace Owner/Admin standing alone never grants this (fail closed).
func CheckBillingMemberAdd(actorBillingRole, newRole string) error {
	switch strings.TrimSpace(actorBillingRole) {
	case BillingRoleOwner:
		return nil
	case BillingRoleAdmin:
		if strings.TrimSpace(newRole) == BillingRoleOwner {
			return ErrBillingForbidden
		}
		return nil
	default:
		return ErrBillingForbidden
	}
}

// CheckBillingMemberMutation enforces billing role transitions and the
// handover invariant: the last owner cannot be removed or demoted, only
// owners transfer ownership, and management is never lifetime-bound to the
// provisioning identity (any owner may hand over to another manager).
func CheckBillingMemberMutation(actorRole string, actorUserID, targetUserID int64, targetCurrentRole, newRole string, ownerCount int, isRemove bool) error {
	actorRole = strings.TrimSpace(actorRole)
	targetCurrentRole = strings.TrimSpace(targetCurrentRole)
	newRole = strings.TrimSpace(newRole)
	isSelf := actorUserID == targetUserID

	if isRemove {
		if targetCurrentRole == BillingRoleOwner && ownerCount <= 1 {
			return ErrBillingLastOwner
		}
		if isSelf {
			return nil
		}
		switch actorRole {
		case BillingRoleOwner:
			return nil
		case BillingRoleAdmin:
			if targetCurrentRole == BillingRoleOwner {
				return ErrBillingForbidden
			}
			return nil
		default:
			return ErrBillingForbidden
		}
	}

	if !IsValidBillingRole(newRole) {
		return ErrBillingForbidden
	}
	if newRole == targetCurrentRole {
		return nil
	}
	if targetCurrentRole == BillingRoleOwner && ownerCount <= 1 {
		return ErrBillingLastOwner
	}
	if newRole == BillingRoleOwner && actorRole != BillingRoleOwner {
		return ErrBillingForbidden
	}
	switch actorRole {
	case BillingRoleOwner:
		return nil
	case BillingRoleAdmin:
		if targetCurrentRole == BillingRoleOwner || newRole == BillingRoleOwner {
			return ErrBillingForbidden
		}
		return nil
	default:
		return ErrBillingForbidden
	}
}

// Billing transfer invite lifecycle (#948, invite-style studio handover).
//
// Only a Billing Owner may initiate; the recipient accepts or rejects, and
// the initiator may cancel while pending. Acceptance hands management to
// the recipient (new Owner, previous Owner steps down to Admin) and grants
// the recipient Workspace Owner on every non-Default business Workspace
// associated with the account; the Default Workspace never transfers.
// Identity, balance, Ledger, and history stay stable; no funds move to the
// recipient's other accounts, quotas never reset, and no welcome grant is
// minted. Row-pinned settlement keeps existing Task/Search/payment/refund
// attribution on the original account.
const (
	BillingTransferPending   = "pending"
	BillingTransferAccepted  = "accepted"
	BillingTransferRejected  = "rejected"
	BillingTransferCancelled = "cancelled"
)

// BillingTransferInvite is one billing_account_transfer_invites row
// projected for the API. BillingAccountPublicID is the stable ba_ funds
// identity snapshot; ScopeWorkspacePublicIDs is the non-Default business
// Workspace scope granted at acceptance (empty until accepted).
type BillingTransferInvite struct {
	InternalID              int64
	PublicID                string
	BillingAccountID        int64
	BillingAccountPublicID  string
	FromUserID              int64
	ToUserID                int64
	Status                  string
	ScopeWorkspacePublicIDs []string
	CreatedAt               time.Time
	DecidedAt               *time.Time
	DecidedBy               *int64
}

// IsValidBillingTransferStatus reports whether status is a known invite state.
func IsValidBillingTransferStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case BillingTransferPending, BillingTransferAccepted, BillingTransferRejected, BillingTransferCancelled:
		return true
	default:
		return false
	}
}
