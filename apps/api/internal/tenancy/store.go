package tenancy

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Membership roles (agreed with the DB sibling stream).
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

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
	// ErrInvalidName reports a display name outside 1..120 characters.
	ErrInvalidName = errors.New("tenancy: name must be 1..120 characters")
	// ErrWorkspaceOwnerRequired reports a workspace create without an
	// organization or an owner.
	ErrWorkspaceOwnerRequired = errors.New("tenancy: workspace requires an organization or an owner")
	// ErrPersonalWorkspaceUserRequired reports a personal workspace
	// provision without a user.
	ErrPersonalWorkspaceUserRequired = errors.New("tenancy: personal workspace requires a user")
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
// and owner references, archival timestamp.
type Workspace struct {
	InternalID     int64
	PublicID       string
	OrganizationID *string
	OwnerUserID    *int64
	Name           string
	ArchivedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsPersonal reports whether the workspace is a personal workspace
// (organization_id IS NULL).
func (w *Workspace) IsPersonal() bool {
	return w != nil && w.OrganizationID == nil
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
// Repository methods take context.Context first.
type CreateWorkspaceParams struct {
	Name           string
	OrganizationID *string
	OwnerUserID    int64
}

// Store is the AGREED tenancy persistence interface the DB sibling
// implements against sqlc. Handlers program to this interface only, so
// tests run against the in-memory fallback while production swaps in the
// postgres implementation without handler changes.
type Store interface {
	WorkspaceStore
	OrganizationStore
}

// WorkspaceStore groups workspace and workspace-membership persistence.
type WorkspaceStore interface {
	GetWorkspace(ctx context.Context, publicID string) (*Workspace, error)
	ListWorkspacesForUser(ctx context.Context, userID int64) ([]*Workspace, error)
	CreateWorkspace(ctx context.Context, params CreateWorkspaceParams) (*Workspace, error)
	UpdateWorkspace(ctx context.Context, publicID, name string) (*Workspace, error)
	ArchiveWorkspace(ctx context.Context, publicID string) (*Workspace, error)
	// EnsurePersonalWorkspace idempotently provisions the user's personal
	// workspace (INSERT ... ON CONFLICT DO NOTHING in postgres).
	// Concurrent callers converge to a single workspace.
	EnsurePersonalWorkspace(ctx context.Context, userID int64) (*Workspace, error)
	GetWorkspaceRole(ctx context.Context, workspacePublicID string, userID int64) (string, bool, error)
	ListWorkspaceMembers(ctx context.Context, workspacePublicID string) ([]*WorkspaceMember, error)
	AddWorkspaceMember(ctx context.Context, workspacePublicID string, targetUserID int64, role string) (*WorkspaceMember, error)
	UpdateWorkspaceMemberRole(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, newRole string) (*WorkspaceMember, error)
	RemoveWorkspaceMember(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64) error
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
