package tenancy

// Workspace and organization lifecycle types and persistence seams.

import (
	"context"
	"time"
)

// Membership roles (agreed with the DB sibling stream).
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

const MaxWorkspaceMembers = 5

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

// Organization mirrors the agreed organizations row. Closure (#955) is a
// state flag, not a delete: ClosedAt/ClosedBy suspend all member access
// and new spending while membership, Workspace, permission, and billing
// history rows are retained for audit; closure is permanent.
type Organization struct {
	InternalID int64
	PublicID   string
	Name       string
	// ClosedAt marks an Owner-initiated closure; nil means active.
	ClosedAt *time.Time
	// ClosedBy is the Owner user id that initiated the closure.
	ClosedBy  *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsClosed reports whether the Organization is currently closed.
func (o *Organization) IsClosed() bool {
	return o != nil && o.ClosedAt != nil
}

// Status reports the lifecycle status wire value: closed or active.
func (o *Organization) Status() string {
	if o.IsClosed() {
		return OrganizationStatusClosed
	}
	return OrganizationStatusActive
}

// Organization lifecycle statuses (#955). Wire-stable.
const (
	OrganizationStatusActive = "active"
	OrganizationStatusClosed = "closed"
)

// OrganizationClosureExpired records that the remaining credits are permanently
// unavailable. Legal and purchase commitments remain applicable.
const OrganizationClosureExpired = "expired"

// OrganizationClosureMemberSnapshot pins one membership row at closure.
// OrganizationClosure is the immutable audit of permanent closure. Financial
// history remains available for reconciliation and legally required remedies.
type OrganizationClosureMemberSnapshot struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

// OrganizationClosure is the immutable audit of permanent closure. Financial
// history remains available for reconciliation and legally required remedies.
type OrganizationClosure struct {
	InternalID             int64
	OrganizationInternalID int64
	OrganizationPublicID   string
	BillingAccountID       int64
	ClosedAt               time.Time
	ClosedBy               *int64
	Reason                 string
	MembershipSnapshot     []OrganizationClosureMemberSnapshot
	WorkspacePublicIDs     []string
	AvailableBalance       int64
	ReservedBalance        int64
	CreditedOrderNumbers   []string
	Disposition            string
	DispositionDetail      string
}

// CreateWorkspaceParams carries validated workspace creation input.
// IsDefault requests the Default marker: only personal scope may carry
// it, and an owner holding a Default rejects a second one with
// ErrDefaultWorkspaceConflict. Plain POST creation always passes false
// (non-Default personal workspaces are unlimited).
// Billing scope (ADR-0038) is the creation scope and nothing else:
//   - Organization workspaces always use the organization pool.
//   - Personal workspaces always use the owner's deterministic Personal
//     account (the Default-workspace-anchored account); no per-workspace
//     account is minted and no link row is written. Settlement bills the
//     actor's own Personal account (payer = actor, #973/#974), never the
//     workspace owner as payer for another member.
//
// The scope is fixed at creation: there is no payer switch, no
// billing_account_id / new_billing_account option, and no link indirection.
// IsDefault workspaces own the deterministic personal account row.
// Creating an Organization never moves existing Personal workspaces.
//
// Repository methods take context.Context first.
type CreateWorkspaceParams struct {
	Name           string
	OrganizationID *string
	OwnerUserID    int64
	IsDefault      bool
}

// Store is the AGREED tenancy persistence interface the DB sibling
// implements against sqlc. Production swaps in the postgres
// implementation without handler changes.
type Store interface {
	WorkspaceStore
	OrganizationStore
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
	WorkspaceInviteStore
}

// OrganizationStore groups organization and org-scoped workspace reads.
type OrganizationStore interface {
	CreateOrganization(ctx context.Context, ownerUserID int64, name string) (*Organization, error)
	GetOrganization(ctx context.Context, publicID string) (*Organization, error)
	ListOrganizationsForUser(ctx context.Context, userID int64) ([]*Organization, error)
	UpdateOrganization(ctx context.Context, publicID, name string) (*Organization, error)
	// CloseOrganization permanently closes the Organization and its remaining
	// credits. Only Owner may close; unsettled obligations block closure.
	// Financial history is retained; no automatic refund or Personal transfer occurs.
	CloseOrganization(ctx context.Context, organizationPublicID string, actorUserID int64, reason string) (*Organization, error)
	GetOrganizationRole(ctx context.Context, organizationPublicID string, userID int64) (string, bool, error)
	// GetOrganizationRoles resolves one role per user inside an organization
	// in a single lookup, replacing per-row GetOrganizationRole round trips
	// (member-quota list normalization, organization member expansion).
	// Users holding no membership are omitted (callers read them as
	// non-members, matching GetOrganizationRole ok=false).
	GetOrganizationRoles(ctx context.Context, organizationPublicID string, userIDs []int64) (map[int64]string, error)
	// ListOrganizationRolesForUser resolves one user's role per organization
	// in a single lookup, replacing per-organization GetOrganizationRole
	// round trips (workspace listing fan-out, organization list
	// expand=role). Organizations holding no membership are omitted.
	ListOrganizationRolesForUser(ctx context.Context, organizationPublicIDs []string, userID int64) (map[string]string, error)
	// ListOrganizationWorkspaceCounts reports visible workspace counts per
	// organization in a single lookup, replacing per-organization
	// ListWorkspacesByOrganizationID round trips (organization list
	// expand=workspace_counts). Closed organizations report zero: their
	// workspaces stay hidden from listing, matching ListWorkspacesForUser.
	// Unknown organizations are omitted (callers read them as zero).
	ListOrganizationWorkspaceCounts(ctx context.Context, organizationPublicIDs []string) (map[string]int64, error)
	ListOrganizationWorkspaces(ctx context.Context, organizationPublicID string) ([]*Workspace, error)
	CreateOrganizationWorkspace(ctx context.Context, organizationPublicID string, ownerUserID int64, name string) (*Workspace, error)
	ListOrganizationMembers(ctx context.Context, organizationPublicID string) ([]*OrganizationMember, error)
	AddOrganizationMember(ctx context.Context, organizationPublicID string, targetUserID int64, role string) (*OrganizationMember, error)
	UpdateOrganizationMemberRole(ctx context.Context, organizationPublicID string, actorUserID, targetUserID int64, newRole string) (*OrganizationMember, error)
	RemoveOrganizationMember(ctx context.Context, organizationPublicID string, actorUserID, targetUserID int64) error
	OrganizationInviteStore
}
