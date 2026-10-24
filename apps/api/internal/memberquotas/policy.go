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
// Personal account holder facts come from the Billing Account seam;
// Workspace or financial Admin standing does not grant quota management.
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

// NormalizeRead presents absent quotas and Organization Owner quotas as
// unlimited. Role lookup failures must not expose an outdated Owner cap.
func (p Policy) NormalizeRead(ctx context.Context, scope Scope, userID int64, period string, limit *int64) (string, *int64, error) {
	role, ok, err := p.organizationRole(ctx, scope, userID)
	if err != nil {
		return "", nil, err
	}
	if ok && role == tenancy.RoleOwner || period == "" {
		return PeriodUnlimited, nil, nil
	}
	return period, limit, nil
}

// NormalizeReadBatch returns normalized quota rows in input order without
// mutating the persisted rows. Organization scopes use one role lookup;
// Personal scopes need no role lookup.
func (p Policy) NormalizeReadBatch(ctx context.Context, scope Scope, rows []QuotaRow) ([]QuotaRow, error) {
	out := make([]QuotaRow, 0, len(rows))
	var roles map[int64]string
	if len(rows) > 0 && scope.Kind == ScopeOrganization && scope.OrganizationID != "" {
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
		if roles[row.UserID] == tenancy.RoleOwner || row.Period == "" {
			row.Period, row.LimitCredits = PeriodUnlimited, nil
		}
		out = append(out, row)
	}
	return out, nil
}

// CanManage decides whether an actor may list or write quotas. Organization
// Owner/Admin membership grants management; Personal authorization is supplied
// by the Personal account holder fact.
func (p Policy) CanManage(ctx context.Context, scope Scope, actorUserID int64, personalOwner bool) (bool, error) {
	_, allowed, err := p.managerRole(ctx, scope, actorUserID, personalOwner)
	return allowed, err
}

func (p Policy) managerRole(ctx context.Context, scope Scope, actorUserID int64, personalOwner bool) (string, bool, error) {
	switch scope.Kind {
	case ScopePersonal:
		return "", personalOwner, nil
	case ScopeOrganization:
		// Organization membership is checked below.
	default:
		return "", false, nil
	}
	role, ok, err := p.organizationRole(ctx, scope, actorUserID)
	if err != nil {
		return "", false, err
	}
	return role, ok && (role == tenancy.RoleOwner || role == tenancy.RoleAdmin), nil
}

// CanSet checks manager authorization and the target role matrix together.
// Owner rows may only be written by the Owner as unlimited (including the
// stale-row cleanup path); Admin rows require an Owner actor. Member rows can
// be managed by either Organization Owner or Admin.
func (p Policy) CanSet(ctx context.Context, scope Scope, actorUserID, targetUserID int64, period string, personalOwner bool) (bool, error) {
	actorRole, allowed, err := p.managerRole(ctx, scope, actorUserID, personalOwner)
	if err != nil || !allowed {
		return false, err
	}
	if scope.Kind == ScopePersonal {
		return true, nil
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
