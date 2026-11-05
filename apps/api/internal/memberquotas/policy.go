package memberquotas

import (
	"context"
	"errors"

	"ideaboard/api/internal/tenancy"
)

// ScopeKind identifies the account scope whose quota is being managed.
// Organization and Personal scopes intentionally share the quota surface,
// while their manager facts come from different stores.
type ScopeKind uint8

const (
	ScopePersonal ScopeKind = iota
	ScopeOrganization
)

// Scope is the small set of workspace facts needed by the quota policy.
// OrganizationID is required only for Organization scopes.
type Scope struct {
	Kind           ScopeKind
	OrganizationID string
}

// OrganizationRoleReader is the tenancy seam used by the quota policy.
// Batch normalization requires one role lookup for the whole page.
type OrganizationRoleReader interface {
	GetOrganizationRole(ctx context.Context, organizationPublicID string, userID int64) (role string, member bool, err error)
	GetOrganizationRoles(ctx context.Context, organizationPublicID string, userIDs []int64) (map[int64]string, error)
}

// Policy owns the Organization role matrix for quota reads and writes.
// MemberQuotas is Org-only (#975): Personal scopes carry no quota
// concept, so every entry point (CanManage/CanSet/Self/Page/NormalizeRead)
// on a Personal scope fails closed with an error (never a silent denial
// or success); callers return the unlimited constant directly without
// entering Policy.
type Policy struct {
	OrganizationRoles OrganizationRoleReader
}

func (p Policy) organizationRole(ctx context.Context, scope Scope, userID int64) (string, bool, error) {
	if scope.Kind != ScopeOrganization || scope.OrganizationID == "" {
		return "", false, nil
	}
	if p.OrganizationRoles == nil {
		return "", false, errors.New("memberquotas: organization role reader is not configured")
	}
	return p.OrganizationRoles.GetOrganizationRole(ctx, scope.OrganizationID, userID)
}

// requireOrganizationScope fails closed on Personal scopes: Personal
// carries no quota concept, so management reads/writes never silently
// allow or deny there.
func requireOrganizationScope(scope Scope) error {
	if scope.Kind != ScopeOrganization {
		return ErrPersonalQuotaUnsupported
	}
	return nil
}

// normalizeOne presents Owner and empty caps as unlimited; every other cap
// passes through.
func normalizeOne(period string, limit *int64, owner bool) (string, *int64) {
	if owner || period == "" {
		return PeriodUnlimited, nil
	}
	return period, limit
}

// isManageable reports Organization manager standing (Owner/Admin
// membership); space-only roles never grant quota management.
func isManageable(role string, ok bool) bool {
	return ok && (role == tenancy.RoleOwner || role == tenancy.RoleAdmin)
}

// NormalizeRead presents absent quotas and Organization Owner quotas as
// unlimited. Org-only (#975): Personal scopes never reach here (callers
// return the unlimited constant directly); a Personal scope fails closed.
// Role lookup failures must not expose an outdated Owner cap.
func (p Policy) NormalizeRead(ctx context.Context, scope Scope, userID int64, period string, limit *int64) (string, *int64, error) {
	if err := requireOrganizationScope(scope); err != nil {
		return "", nil, err
	}
	role, ok, err := p.organizationRole(ctx, scope, userID)
	if err != nil {
		return "", nil, err
	}
	period, limit = normalizeOne(period, limit, ok && role == tenancy.RoleOwner)
	return period, limit, nil
}

// NormalizeReadBatch returns normalized quota rows in input order without
// mutating the persisted rows. Org-only (#975): Personal scopes never
// reach here (Page fails closed before this point); a Personal scope fails
// closed.
func (p Policy) NormalizeReadBatch(ctx context.Context, scope Scope, rows []QuotaRow) ([]QuotaRow, error) {
	if err := requireOrganizationScope(scope); err != nil {
		return nil, err
	}
	out := make([]QuotaRow, 0, len(rows))
	var roles map[int64]string
	if len(rows) > 0 && scope.OrganizationID != "" {
		if p.OrganizationRoles == nil {
			return nil, errors.New("memberquotas: organization role reader is not configured")
		}
		userIDs := make([]int64, 0, len(rows))
		for _, row := range rows {
			userIDs = append(userIDs, row.UserID)
		}
		var err error
		roles, err = p.OrganizationRoles.GetOrganizationRoles(ctx, scope.OrganizationID, userIDs)
		if err != nil {
			return nil, err
		}
	}
	for _, row := range rows {
		row.Period, row.LimitCredits = normalizeOne(row.Period, row.LimitCredits, roles[row.UserID] == tenancy.RoleOwner)
		out = append(out, row)
	}
	return out, nil
}

// CanManage decides whether an actor may list or write quotas. Only
// Organization scopes are manageable (Owner/Admin membership); Personal
// scopes fail closed with an error because Personal quotas do not exist.
func (p Policy) CanManage(ctx context.Context, scope Scope, actorUserID int64) (bool, error) {
	if err := requireOrganizationScope(scope); err != nil {
		return false, err
	}
	role, ok, err := p.organizationRole(ctx, scope, actorUserID)
	if err != nil {
		return false, err
	}
	return isManageable(role, ok), nil
}

// CanSet checks manager authorization and the target role matrix together.
// Owner rows may only be written by the Owner as unlimited (including the
// stale-row cleanup path); Admin rows require an Owner actor. Member rows can
// be managed by either Organization Owner or Admin. Personal scopes fail
// closed with an error because Personal quotas do not exist.
func (p Policy) CanSet(ctx context.Context, scope Scope, actorUserID, targetUserID int64, period string) (bool, error) {
	if err := requireOrganizationScope(scope); err != nil {
		return false, err
	}
	actorRole, ok, err := p.organizationRole(ctx, scope, actorUserID)
	if err != nil {
		return false, err
	}
	if !isManageable(actorRole, ok) {
		return false, nil
	}
	targetRole, targetOK, err := p.organizationRole(ctx, scope, targetUserID)
	if err != nil {
		return false, err
	}
	if !targetOK {
		return true, nil
	}
	if targetRole == tenancy.RoleOwner {
		return actorRole == tenancy.RoleOwner && period == PeriodUnlimited, nil
	}
	if targetRole == tenancy.RoleAdmin && actorRole != tenancy.RoleOwner {
		return false, nil
	}
	return true, nil
}
