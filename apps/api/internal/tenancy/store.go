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

const MaxWorkspaceMembers = 5

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
	// ErrDefaultWorkspaceProtected reports an archive/rename (delete)
	// attempt on a Default workspace. Handlers render it as 403
	// (workspace_default_protected): the Default workspace is never
	// deleted, archived, or renamed.
	ErrDefaultWorkspaceProtected = errors.New("tenancy: default workspace cannot be archived, deleted, or renamed")
	// ErrDefaultWorkspaceConflict reports a second Default workspace create
	// for an owner that already holds a live Default. Handlers render it
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
// and owner references, the Default marker, archival timestamp.
type Workspace struct {
	InternalID     int64
	PublicID       string
	OrganizationID *string
	OwnerUserID    *int64
	Name           string
	// IsDefault marks the user's Default workspace: exactly one live row
	// per owner, never deleted, archived, or renamed, auto-provisioned
	// when missing. Organization workspaces never carry it.
	IsDefault  bool
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
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

// IsArchived reports whether the workspace is archived.
func (w *Workspace) IsArchived() bool {
	return w != nil && w.ArchivedAt != nil
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
// it, and an owner holding a live Default rejects a second one with
// ErrDefaultWorkspaceConflict. Plain POST creation always passes false
// (non-Default personal workspaces are unlimited).
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
type WorkspaceStore interface {
	GetWorkspace(ctx context.Context, publicID string) (*Workspace, error)
	ListWorkspacesForUser(ctx context.Context, userID int64) ([]*Workspace, error)
	CreateWorkspace(ctx context.Context, params CreateWorkspaceParams) (*Workspace, error)
	UpdateWorkspace(ctx context.Context, publicID, name string) (*Workspace, error)
	ArchiveWorkspace(ctx context.Context, publicID string) (*Workspace, error)
	// EnsureDefaultWorkspace idempotently provisions the user's Default
	// workspace (INSERT ... ON CONFLICT DO NOTHING in postgres).
	// Concurrent callers converge to a single Default workspace. When the
	// user holds no live Default (fresh profile, or only non-Default
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
