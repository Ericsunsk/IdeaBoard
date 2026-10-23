package tenancy

// Domain guards: sentinel errors and role-transition rules shared by the
// in-memory double and the postgres backend. Pure policy, no storage.

import (
	"errors"
	"strings"
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
	// deleted or renamed. Personal Default invites (#952) reuse the same
	// sentinel: the Default workspace is private and never gains
	// collaborators, so AddWorkspaceMember on a Default workspace fails
	// with this error (403 workspace_default_protected) instead of a new
	// wire code.
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
	// ErrNotUserPublicID reports an identifier without the usr_ prefix.
	ErrNotUserPublicID = errors.New("tenancy: not a user public id")
	// ErrMalformedUserPublicID reports a usr_ identifier with bad hex.
	ErrMalformedUserPublicID = errors.New("tenancy: malformed user public id")
	// ErrOrganizationLimitReached reports a create/join blocked by the
	// at-most-one-Organization cardinality (ADR-0038 #951, fail closed).
	// Handlers render it as 409 organization_limit_reached.
	ErrOrganizationLimitReached = errors.New("tenancy: user already in an organization")
	// ErrOrganizationOwnerImmutable reports a direct Owner role mutation
	// outside the explicit handover path (#948 is the only Owner change
	// path; direct promote/demote/remove of an Owner fails closed).
	// Handlers render it as 403 membership_transition_forbidden.
	ErrOrganizationOwnerImmutable = errors.New("tenancy: organization owner change requires explicit handover")
	// ErrOrganizationClosed reports a mutation attempted on a closed
	// Organization (#955). Closure is permanent; every
	// member/workspace/billing mutation fails closed while closed.
	// Handlers render it as 409 organization_closed.
	ErrOrganizationClosed = errors.New("tenancy: organization is closed")
	// ErrOrganizationClosureBlocked reports a closure refused by unsettled
	// in-flight obligations (#955): pending/running Tasks, unsettled
	// payment orders, pending refunds, open disputes, or a pending
	// handover invite on the shared pool. Handlers render it as 409
	// organization_closure_blocked.
	ErrOrganizationClosureBlocked = errors.New("tenancy: organization has unsettled obligations")
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

// CheckWorkspaceWritable fails closed when the owning Organization is
// closed (#955). Personal scopes (nil org) always pass; callers keep
// their own not-found handling for missing rows. It is the single
// domain gate behind the tasking, media, boards, and vision closed-org
// checks: every settlement/accept path converges here instead of
// re-reading ClosedAt inline.
func CheckWorkspaceWritable(org *Organization) error {
	if org != nil && org.IsClosed() {
		return ErrOrganizationClosed
	}
	return nil
}

// checkMemberAddParams tunes the shared invite gate across the three
// membership families (workspace, organization, billing). Role literals
// share the same wire values ("owner"/"admin"); families differ only in
// which grants flow through the invite path and which error they render.
type checkMemberAddParams struct {
	ActorRole, NewRole   string
	OwnerRole, AdminRole string
	// OwnerImmutable rejects owner grants on this path (organization:
	// handover only). Workspace/billing owners may grant owner here.
	OwnerImmutable bool
	// AdminCanGrantAdmin rejects admin->admin grants (organization only).
	AdminCanGrantAdmin         bool
	ForbiddenErr, ImmutableErr error
	IsValid                    func(string) bool
}

func checkMemberAdd(p checkMemberAddParams) error {
	actor := strings.TrimSpace(p.ActorRole)
	newRole := strings.TrimSpace(p.NewRole)
	owner := p.OwnerRole
	if owner == "" {
		owner = RoleOwner
	}
	admin := p.AdminRole
	if admin == "" {
		admin = RoleAdmin
	}
	isValid := p.IsValid
	if isValid == nil {
		isValid = IsValidRole
	}
	if !isValid(newRole) {
		return p.ForbiddenErr
	}
	if p.OwnerImmutable && newRole == owner {
		return p.ImmutableErr
	}
	switch actor {
	case owner:
		return nil
	case admin:
		if newRole == owner {
			return p.ForbiddenErr
		}
		if !p.AdminCanGrantAdmin && newRole == admin {
			return p.ForbiddenErr
		}
		return nil
	default:
		return p.ForbiddenErr
	}
}

// checkMemberMutationParams tunes the shared role-transition gate.
type checkMemberMutationParams struct {
	ActorRole                  string
	ActorUserID, TargetUserID  int64
	TargetCurrentRole, NewRole string
	OwnerCount                 int
	IsRemove                   bool
	OwnerRole, AdminRole       string
	// OwnerImmutable rejects any transition to/from owner (organization:
	// handover only).
	OwnerImmutable bool
	// AdminCanMutateAdmin allows admin->admin touches (workspace/billing
	// true; organization false).
	AdminCanMutateAdmin bool
	// PrivilegedTouch reports whether an update touches a privileged
	// grant and therefore requires the owner: workspace elevates on
	// new owner/admin, organization on either side admin, billing on
	// new owner.
	PrivilegedTouch                          func(targetCurrent, newRole string) bool
	ForbiddenErr, ImmutableErr, LastOwnerErr error
	IsValid                                  func(string) bool
}

func checkMemberMutation(p checkMemberMutationParams) error {
	actor := strings.TrimSpace(p.ActorRole)
	targetCurrent := strings.TrimSpace(p.TargetCurrentRole)
	newRole := strings.TrimSpace(p.NewRole)
	owner := p.OwnerRole
	if owner == "" {
		owner = RoleOwner
	}
	admin := p.AdminRole
	if admin == "" {
		admin = RoleAdmin
	}
	isValid := p.IsValid
	if isValid == nil {
		isValid = IsValidRole
	}
	isSelf := p.ActorUserID == p.TargetUserID
	if p.OwnerImmutable && (targetCurrent == owner || (!p.IsRemove && newRole == owner)) {
		return p.ImmutableErr
	}
	if p.IsRemove {
		if targetCurrent == owner && p.OwnerCount <= 1 {
			return p.LastOwnerErr
		}
		if isSelf {
			return nil
		}
		switch actor {
		case owner:
			return nil
		case admin:
			if targetCurrent == owner {
				return p.ForbiddenErr
			}
			if !p.AdminCanMutateAdmin && targetCurrent == admin {
				return p.ForbiddenErr
			}
			return nil
		default:
			return p.ForbiddenErr
		}
	}
	if !isValid(newRole) {
		return p.ForbiddenErr
	}
	if newRole == targetCurrent {
		return nil
	}
	if targetCurrent == owner && p.OwnerCount <= 1 {
		return p.LastOwnerErr
	}
	if p.PrivilegedTouch != nil && p.PrivilegedTouch(targetCurrent, newRole) && actor != owner {
		return p.ForbiddenErr
	}
	switch actor {
	case owner:
		return nil
	case admin:
		if targetCurrent == owner || newRole == owner {
			return p.ForbiddenErr
		}
		return nil
	default:
		return p.ForbiddenErr
	}
}

// CheckOrganizationMemberAdd enforces invite rules: actor must be
// owner|admin; only owners may grant admin; owner grants never flow
// through this path (handover only).
func CheckOrganizationMemberAdd(actorRole, newRole string) error {
	return checkMemberAdd(checkMemberAddParams{
		ActorRole: actorRole, NewRole: newRole,
		OwnerRole: RoleOwner, AdminRole: RoleAdmin,
		OwnerImmutable: true,
		ForbiddenErr:   ErrForbidden,
		ImmutableErr:   ErrOrganizationOwnerImmutable,
		IsValid:        IsValidRole,
	})
}

// CheckOrganizationMemberMutation enforces org role-transition rules:
//
//   - actor must be owner|admin, except a member may remove themselves
//     (leave; owner leave is still blocked without handover).
//   - only Owner may change Admin roles (promote to admin or demote an
//     admin); Admin touching an Admin fails closed.
//   - Owner rows are immutable here: any transition to or from owner
//     fails closed with ErrOrganizationOwnerImmutable (handover only).
//   - the last owner cannot be removed or demoted (defense in depth;
//     normally unreachable since Owner rows are immutable).
func CheckOrganizationMemberMutation(actorRole string, actorUserID, targetUserID int64, targetCurrentRole, newRole string, ownerCount int, isRemove bool) error {
	return checkMemberMutation(checkMemberMutationParams{
		ActorRole: actorRole, ActorUserID: actorUserID, TargetUserID: targetUserID,
		TargetCurrentRole: targetCurrentRole, NewRole: newRole,
		OwnerCount: ownerCount, IsRemove: isRemove,
		OwnerRole: RoleOwner, AdminRole: RoleAdmin,
		OwnerImmutable: true,
		PrivilegedTouch: func(targetCurrent, newRole string) bool {
			return targetCurrent == RoleAdmin || newRole == RoleAdmin
		},
		ForbiddenErr: ErrForbidden,
		ImmutableErr: ErrOrganizationOwnerImmutable,
		LastOwnerErr: ErrLastOwner,
		IsValid:      IsValidRole,
	})
}

// EffectiveWorkspaceRole derives the caller's workspace role for
// organization scopes (ADR-0038 #951): org Owner/Admin automatically hold
// their org role on every org workspace without an explicit row; org
// Members hold only their explicit workspace row; non-org-members hold
// nothing (even with a stale direct row, fail closed to join-first).
// Personal scopes use the direct row only.
func EffectiveWorkspaceRole(workspace *Workspace, orgRole string, orgMember bool, directRole string, directMember bool) (string, bool) {
	if workspace == nil {
		return "", false
	}
	if workspace.IsPersonal() {
		if !directMember {
			return "", false
		}
		return directRole, true
	}
	if !orgMember {
		return "", false
	}
	switch strings.TrimSpace(orgRole) {
	case RoleOwner, RoleAdmin:
		return strings.TrimSpace(orgRole), true
	default:
		if !directMember {
			return "", false
		}
		return directRole, true
	}
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
	return checkMemberAdd(checkMemberAddParams{
		ActorRole: actorRole, NewRole: newRole,
		OwnerRole: RoleOwner, AdminRole: RoleAdmin,
		AdminCanGrantAdmin: true,
		ForbiddenErr:       ErrForbidden,
		IsValid:            IsValidRole,
	})
}

// CheckMemberMutation enforces server-side role-transition rules for
// PATCH (newRole) and DELETE (isRemove) on workspace members:
//
//   - actor must be owner|admin, except a member may remove themselves.
//   - only owners may promote to owner or admin (owner-only promotion).
//   - the last owner cannot be removed or demoted (ownerCount counts
//     current owners including the target when the target is an owner).
func CheckMemberMutation(actorRole string, actorUserID, targetUserID int64, targetCurrentRole, newRole string, ownerCount int, isRemove bool) error {
	return checkMemberMutation(checkMemberMutationParams{
		ActorRole: actorRole, ActorUserID: actorUserID, TargetUserID: targetUserID,
		TargetCurrentRole: targetCurrentRole, NewRole: newRole,
		OwnerCount: ownerCount, IsRemove: isRemove,
		OwnerRole: RoleOwner, AdminRole: RoleAdmin,
		AdminCanMutateAdmin: true,
		PrivilegedTouch: func(_, newRole string) bool {
			return newRole == RoleOwner || newRole == RoleAdmin
		},
		ForbiddenErr: ErrForbidden,
		LastOwnerErr: ErrLastOwner,
		IsValid:      IsValidRole,
	})
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
// bills, and member quotas (Owner/Admin both).
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
	return checkMemberAdd(checkMemberAddParams{
		ActorRole: actorBillingRole, NewRole: newRole,
		OwnerRole: BillingRoleOwner, AdminRole: BillingRoleAdmin,
		AdminCanGrantAdmin: true,
		ForbiddenErr:       ErrBillingForbidden,
		IsValid:            IsValidBillingRole,
	})
}

// CheckBillingMemberMutation enforces billing role transitions and the
// handover invariant: the last owner cannot be removed or demoted, only
// owners transfer ownership, and management is never lifetime-bound to the
// provisioning identity (any owner may hand over to another manager).
func CheckBillingMemberMutation(actorRole string, actorUserID, targetUserID int64, targetCurrentRole, newRole string, ownerCount int, isRemove bool) error {
	return checkMemberMutation(checkMemberMutationParams{
		ActorRole: actorRole, ActorUserID: actorUserID, TargetUserID: targetUserID,
		TargetCurrentRole: targetCurrentRole, NewRole: newRole,
		OwnerCount: ownerCount, IsRemove: isRemove,
		OwnerRole: BillingRoleOwner, AdminRole: BillingRoleAdmin,
		AdminCanMutateAdmin: true,
		PrivilegedTouch: func(_, newRole string) bool {
			return newRole == BillingRoleOwner
		},
		ForbiddenErr: ErrBillingForbidden,
		LastOwnerErr: ErrBillingLastOwner,
		IsValid:      IsValidBillingRole,
	})
}
