// Package postgres implements tenancy billing-account resolution (ADR-0038).
//
// A Billing Account holds the unified balance; many Workspaces share one
// account. Exactly one Personal account exists per user (the
// Default-workspace-anchored account) and exactly one shared account per
// Organization. Resolution is fixed at Workspace creation scope: org
// workspaces resolve to the Organization pool, personal workspaces to
// their owner's deterministic Personal account (settlement bills the
// actor's own account, payer = actor). Missing or ownerless scopes fail
// closed instead of silently picking another wallet. Joining a Workspace
// never merges personal funds; creating an Organization never moves
// existing Personal workspaces.
//
// All repository methods take context.Context first and use dbWithContext.
// Resolution joins the caller-owned transaction via the *dbsqlc.Queries
// variants so Task settlement keeps Quote/Hold/Task/River atomic.
package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockBillingAccountInTx is the common serialization point for financial
// authority and Workspace scope changes. Acquire it before invitation or
// Workspace locks, and read permissions only after the lock is held.
func lockBillingAccountInTx(ctx context.Context, q *dbsqlc.Queries, accountID int64) (dbsqlc.BillingAccount, error) {
	row, err := q.LockBillingAccount(ctx, accountID)
	if database.IsNotFound(err) {
		return row, tenancy.ErrBillingNotFound
	}
	return row, err
}

// checkBillingMemberMutationInTx keeps the account lock, current membership facts,
// transition policy, and public error classification together. Callers supply
// their current transaction and perform the authorized write before it ends.
func checkBillingMemberMutationInTx(ctx context.Context, q *dbsqlc.Queries, accountID, actorUserID, targetUserID int64, newRole string, isRemove bool) error {
	if _, err := lockBillingAccountInTx(ctx, q, accountID); err != nil {
		return err
	}
	actor, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
		BillingAccountID: accountID, UserID: actorUserID,
	})
	actorRole := ""
	if err == nil {
		actorRole = actor.Role
	} else if !database.IsNotFound(err) {
		return err
	}
	target, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
		BillingAccountID: accountID, UserID: targetUserID,
	})
	if database.IsNotFound(err) {
		return tenancy.ErrBillingNotFound
	}
	if err != nil {
		return err
	}
	owners, err := q.CountBillingAccountOwners(ctx, accountID)
	if err != nil {
		return err
	}
	if err := tenancy.CheckBillingMemberMutation(actorRole, actorUserID, targetUserID, target.Role, newRole, int(owners), isRemove); err != nil {
		if errors.Is(err, tenancy.ErrBillingLastOwner) {
			return err
		}
		return tenancy.ErrBillingForbidden
	}
	return nil
}

// Billing owner types for the API projection (aliases of the funds kernel
// OwnerType values; single source is tenancy/funds).
const (
	BillingOwnerWorkspace    = funds.OwnerTypeWorkspace
	BillingOwnerOrganization = funds.OwnerTypeOrganization
)

// BillingAccountStore provisions and resolves stable billing accounts.
// Repository methods take context.Context first and use dbWithContext(ctx).
type BillingAccountStore struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// NewBillingAccountStore builds a provisioner over pool. Pass nil pool in
// tests that stub the interface instead.
func NewBillingAccountStore(db *pgxpool.Pool) *BillingAccountStore {
	if db == nil {
		return &BillingAccountStore{}
	}
	return &BillingAccountStore{db: db, q: dbsqlc.New(db)}
}

func (s *BillingAccountStore) dbWithContext(ctx context.Context) repositoryQueryer {
	return newRepositoryQueryer(ctx, s.db, s.q)
}

// EnsureBillingAccountForWorkspace provisions the stable billing account on
// first read (Get-or-Create) via the fixed creation-scope order (ADR-0038).
// The identity is stable across Workspace Owner changes: it never derives
// from the current Workspace Owner's personal wallet.
//
// Concurrency: the partial unique indexes collapse concurrent first reads
// to one row; the loser reads back the winner on duplicate error.
//
// Deep-module delegation: the fixed order lives in tenancy/funds (the funds
// identity kernel); this stays a thin adapter so there is exactly one order
// implementation. Resolution joins the caller transaction via dbWithContext
// (ambient-carrier borrow), the same borrow-or-begin discipline funds.Store
// uses.
func (s *BillingAccountStore) EnsureBillingAccountForWorkspace(ctx context.Context, workspaceInternalID int64, workspacePublicID string) (tenancy.BillingAccount, error) {
	if s == nil || s.q == nil {
		return tenancy.BillingAccount{}, tenancy.ErrBillingStoreNotConfigured
	}
	row, err := funds.ResolveAccountRowByInternalIDWithQueries(ctx, s.dbWithContext(ctx).q, workspaceInternalID)
	if err != nil {
		return tenancy.BillingAccount{}, err
	}
	return mapBillingAccount(row, workspaceInternalID, workspacePublicID), nil
}

// ResolveFunds resolves workspace (ws_) to the stable ba_ funds identity in
// one borrow-or-begin unit (deep funds kernel, owner-based order). v1 write
// and manager paths use this seam so they address the owner/shared pool;
// read-only billing views (overview, balance, usage, records) use
// ResolveFundsForActor so Personal scopes bill the viewer.
func (s *BillingAccountStore) ResolveFunds(ctx context.Context, workspacePublicID string) (funds.Identity, error) {
	if s == nil || s.db == nil {
		// Still allow ambient-carrier borrow when the pool is nil (settlement
		// joins); without a carrier dbutil fails closed as before.
		return funds.NewStore(nil).ResolveFunds(ctx, workspacePublicID)
	}
	return funds.NewStore(s.db).ResolveFunds(ctx, workspacePublicID)
}

// ResolveFundsForActor resolves workspace (ws_) to ba_ for one viewer
// (payer = actor, #973/#978): the Organization pool for org workspaces,
// else the viewer's own Personal account. Read-only billing views use
// this seam so Personal members each see their own (BillingAccount, User)
// column with zero cross pollution; Org reads keep the shared pool.
func (s *BillingAccountStore) ResolveFundsForActor(ctx context.Context, workspacePublicID string, actorUserID int64) (funds.Identity, error) {
	if s == nil || s.db == nil {
		return funds.NewStore(nil).ResolveFundsForActor(ctx, workspacePublicID, actorUserID)
	}
	return funds.NewStore(s.db).ResolveFundsForActor(ctx, workspacePublicID, actorUserID)
}

// ensureOrganizationBillingAccountInTx delegates to the funds kernel so
// creation and resolution share one Get-or-Create seam.
func ensureOrganizationBillingAccountInTx(ctx context.Context, queries *dbsqlc.Queries, organizationInternalID, ownerUserID int64) (dbsqlc.BillingAccount, error) {
	return funds.EnsureOrganizationBillingAccountInTx(ctx, queries, organizationInternalID, ownerUserID)
}

// AdminBillingAccount is the platform-Admin locate/audit projection of one
// stable funds identity (#923). Unlike the workspace-resolving tenancy.BillingAccount
// above, it carries both linkage public ids so Admin can audit personal,
// studio-shared, and organization accounts without a resolving Workspace.
type AdminBillingAccount struct {
	InternalID           int64
	PublicID             string
	OwnerType            string
	WorkspacePublicID    string
	OrganizationPublicID string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ListBillingAccountsForAdmin pages stable Billing Accounts newest-first for
// platform-Admin locate and audit (#923). ownerScope is one of all,
// organization, or workspace (unknown scopes fail closed as invalid).
// beforeID pages with the reconciliation-style numeric cursor (0 = first
// page); limit clamps to 1..200 (default 50). The returned page is the raw
// slice; callers derive next_before_id from its last InternalID.
func (s *BillingAccountStore) ListBillingAccountsForAdmin(ctx context.Context, beforeID int64, ownerScope string, limit int) ([]AdminBillingAccount, error) {
	if s == nil || s.q == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	switch strings.TrimSpace(ownerScope) {
	case "", "all":
		ownerScope = "all"
	case "organization", "workspace":
		ownerScope = strings.TrimSpace(ownerScope)
	default:
		return nil, tenancy.ErrBillingWorkspaceInvalid
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if beforeID < 0 {
		return nil, tenancy.ErrBillingWorkspaceInvalid
	}
	rows, err := s.dbWithContext(ctx).q.ListBillingAccountsForAdmin(ctx, dbsqlc.ListBillingAccountsForAdminParams{
		BeforeID: beforeID, OwnerScope: ownerScope, RowLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]AdminBillingAccount, 0, len(rows))
	for _, row := range rows {
		ownerType := funds.OwnerTypeWorkspace
		if row.OrganizationID.Valid {
			ownerType = funds.OwnerTypeOrganization
		}
		out = append(out, AdminBillingAccount{
			InternalID:           row.InternalID,
			PublicID:             row.PublicID,
			OwnerType:            ownerType,
			WorkspacePublicID:    textOrEmpty(row.WorkspacePublicID),
			OrganizationPublicID: textOrEmpty(row.OrganizationPublicID),
			CreatedAt:            dbutil.TimeValue(row.CreatedAt),
			UpdatedAt:            dbutil.TimeValue(row.UpdatedAt),
		})
	}
	return out, nil
}

// ListBillingAccountMembersForAdmin lists financial managers of one account
// for platform-Admin audit (#923). Unlike ListBillingAccountMembers there is
// no caller membership gate: the Control Plane guard already established the
// platform-Admin operator, and customer-side account Owner/Admin standing is
// never equivalent to that operator (they cannot call this seam).
func (s *BillingAccountStore) ListBillingAccountMembersForAdmin(ctx context.Context, billingAccountInternalID int64) ([]tenancy.BillingAccountMember, error) {
	if s == nil || s.q == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	if billingAccountInternalID <= 0 {
		return nil, tenancy.ErrBillingNotFound
	}
	if _, err := s.dbWithContext(ctx).q.GetBillingAccountByInternalID(ctx, billingAccountInternalID); err != nil {
		if database.IsNotFound(err) {
			return nil, tenancy.ErrBillingNotFound
		}
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListBillingAccountMembers(ctx, dbsqlc.ListBillingAccountMembersParams{
		BillingAccountID: billingAccountInternalID, RowLimit: 500,
	})
	if err != nil {
		return nil, err
	}
	out := make([]tenancy.BillingAccountMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancy.BillingAccountMember{
			BillingAccountID: r.BillingAccountID, UserID: r.UserID, Role: r.Role,
			CreatedAt: dbutil.TimeValue(r.CreatedAt),
		})
	}
	return out, nil
}

// ListExplicitlyLinkedWorkspaces lists workspaces pinned to one account via
// workspace_billing_links for Admin audit (#923). Per-workspace and org-pool
// ownership (the billing_accounts row itself) is reported through the
// account linkage fields, not here.
func (s *BillingAccountStore) ListExplicitlyLinkedWorkspaces(ctx context.Context, billingAccountInternalID int64) ([]tenancy.Workspace, error) {
	if s == nil || s.q == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	if billingAccountInternalID <= 0 {
		return nil, tenancy.ErrBillingNotFound
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspacesByBillingAccountID(ctx, dbsqlc.ListWorkspacesByBillingAccountIDParams{
		BillingAccountID: billingAccountInternalID, RowLimit: 500,
	})
	if err != nil {
		return nil, err
	}
	out := make([]tenancy.Workspace, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancy.Workspace{
			InternalID: r.InternalID, PublicID: r.PublicID, Name: r.Name, IsDefault: r.IsDefault,
			CreatedAt: dbutil.TimeValue(r.CreatedAt), UpdatedAt: dbutil.TimeValue(r.UpdatedAt),
		})
	}
	return out, nil
}

// fund-operation attribution (#916). Internal callers only; the ba_ public id
// crosses the HTTP boundary, never the numeric key.
// GetBillingAccountByInternalID resolves a stable account for display and
// fund-operation attribution (#916). Internal callers only; the ba_ public id
// crosses the HTTP boundary, never the numeric key.
func (s *BillingAccountStore) GetBillingAccountByInternalID(ctx context.Context, billingAccountInternalID int64) (tenancy.BillingAccount, error) {
	if s == nil || s.q == nil || billingAccountInternalID <= 0 {
		return tenancy.BillingAccount{}, tenancy.ErrBillingNotFound
	}
	row, err := s.dbWithContext(ctx).q.GetBillingAccountByInternalID(ctx, billingAccountInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return tenancy.BillingAccount{}, tenancy.ErrBillingNotFound
		}
		return tenancy.BillingAccount{}, err
	}
	ownerType := funds.OwnerTypeWorkspace
	if row.OrganizationID.Valid {
		ownerType = funds.OwnerTypeOrganization
	}
	return tenancy.BillingAccount{
		InternalID: row.InternalID, PublicID: row.PublicID, OwnerType: ownerType,
		CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}, nil
}

// GetAdminBillingAccountByPublicID resolves one stable account with its
// ownership linkage public ids for platform-Admin audit (#923). Hidden ba_
// ids converge to ErrBillingNotFound, per anti-enumeration.
func (s *BillingAccountStore) GetAdminBillingAccountByPublicID(ctx context.Context, publicID string) (AdminBillingAccount, error) {
	if s == nil || s.q == nil {
		return AdminBillingAccount{}, tenancy.ErrBillingStoreNotConfigured
	}
	trimmed := strings.TrimSpace(publicID)
	if !isBillingAccountID(trimmed) {
		return AdminBillingAccount{}, tenancy.ErrBillingNotFound
	}
	row, err := s.dbWithContext(ctx).q.GetBillingAccountByPublicID(ctx, trimmed)
	if err != nil {
		if database.IsNotFound(err) {
			return AdminBillingAccount{}, tenancy.ErrBillingNotFound
		}
		return AdminBillingAccount{}, err
	}
	detail := AdminBillingAccount{
		InternalID: row.InternalID, PublicID: row.PublicID, OwnerType: func() string {
			if row.OrganizationID.Valid {
				return funds.OwnerTypeOrganization
			}
			return funds.OwnerTypeWorkspace
		}(),
		CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}
	if row.WorkspaceID.Valid {
		if workspace, err := s.dbWithContext(ctx).q.GetWorkspaceByInternalID(ctx, row.WorkspaceID.Int64); err == nil {
			detail.WorkspacePublicID = workspace.PublicID
		} else if !database.IsNotFound(err) {
			return AdminBillingAccount{}, err
		}
	}
	if row.OrganizationID.Valid {
		if organization, err := s.dbWithContext(ctx).q.GetOrganizationByInternalID(ctx, row.OrganizationID.Int64); err == nil {
			detail.OrganizationPublicID = organization.PublicID
		} else if !database.IsNotFound(err) {
			return AdminBillingAccount{}, err
		}
	}
	return detail, nil
}

// GetBillingAccountByPublicID resolves a stable ba_ account for explicit
// linking. Hidden ba_ ids converge to ErrBillingNotFound (never workspace
// existence), per anti-enumeration.
func (s *BillingAccountStore) GetBillingAccountByPublicID(ctx context.Context, publicID string) (tenancy.BillingAccount, error) {
	if s == nil || s.q == nil {
		return tenancy.BillingAccount{}, tenancy.ErrBillingStoreNotConfigured
	}
	trimmed := strings.TrimSpace(publicID)
	if !isBillingAccountID(trimmed) {
		return tenancy.BillingAccount{}, tenancy.ErrBillingNotFound
	}
	row, err := s.dbWithContext(ctx).q.GetBillingAccountByPublicID(ctx, trimmed)
	if err != nil {
		if database.IsNotFound(err) {
			return tenancy.BillingAccount{}, tenancy.ErrBillingNotFound
		}
		return tenancy.BillingAccount{}, err
	}
	return tenancy.BillingAccount{
		InternalID: row.InternalID, PublicID: row.PublicID, OwnerType: func() string {
			if row.OrganizationID.Valid {
				return funds.OwnerTypeOrganization
			}
			return funds.OwnerTypeWorkspace
		}(),
		CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}, nil
}

func isBillingAccountID(id string) bool {
	return strings.HasPrefix(id, "ba_") && len(id) > 4
}

// GetBillingAccountRole reports the caller's financial role on one account.
func (s *BillingAccountStore) GetBillingAccountRole(ctx context.Context, billingAccountInternalID, userID int64) (string, bool, error) {
	if s == nil || s.q == nil {
		return "", false, tenancy.ErrBillingStoreNotConfigured
	}
	row, err := s.dbWithContext(ctx).q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
		BillingAccountID: billingAccountInternalID, UserID: userID,
	})
	if database.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.Role, true, nil
}

// ListBillingAccountMembers lists financial managers (owner/admin only).
func (s *BillingAccountStore) ListBillingAccountMembers(ctx context.Context, billingAccountInternalID, actorUserID int64) ([]tenancy.BillingAccountMember, error) {
	if s == nil || s.q == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	role, ok, err := s.GetBillingAccountRole(ctx, billingAccountInternalID, actorUserID)
	if err != nil {
		return nil, err
	}
	if !ok || !tenancy.IsBillingManager(role) {
		return nil, tenancy.ErrBillingForbidden
	}
	rows, err := s.dbWithContext(ctx).q.ListBillingAccountMembers(ctx, dbsqlc.ListBillingAccountMembersParams{
		BillingAccountID: billingAccountInternalID, RowLimit: 500,
	})
	if err != nil {
		return nil, err
	}
	out := make([]tenancy.BillingAccountMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancy.BillingAccountMember{
			BillingAccountID: r.BillingAccountID, UserID: r.UserID, Role: r.Role,
			CreatedAt: dbutil.TimeValue(r.CreatedAt),
		})
	}
	return out, nil
}

// AddBillingAccountMember grants a financial manager (owner/admin checks +
// per-account cap). Joining an account grants no Workspace/Board access.
func (s *BillingAccountStore) AddBillingAccountMember(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64, role string) (*tenancy.BillingAccountMember, error) {
	if s == nil || s.db == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	role = strings.TrimSpace(role)
	if !tenancy.IsValidBillingRole(role) {
		return nil, tenancy.ErrBillingForbidden
	}
	var added *tenancy.BillingAccountMember
	err := dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		if _, err := lockBillingAccountInTx(ctx, q, billingAccountInternalID); err != nil {
			return err
		}
		actor, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: actorUserID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingForbidden
			}
			return err
		}
		if err := tenancy.CheckBillingMemberAdd(actor.Role, role); err != nil {
			return err
		}
		members, err := q.ListBillingAccountMembers(ctx, dbsqlc.ListBillingAccountMembersParams{
			BillingAccountID: billingAccountInternalID, RowLimit: 500,
		})
		if err != nil {
			return err
		}
		for _, m := range members {
			if m.UserID == targetUserID {
				added = &tenancy.BillingAccountMember{
					BillingAccountID: m.BillingAccountID, UserID: m.UserID, Role: m.Role,
					CreatedAt: dbutil.TimeValue(m.CreatedAt),
				}
				return nil
			}
		}
		if len(members) >= tenancy.MaxBillingAccountMembers {
			return tenancy.ErrBillingConflict
		}
		row, err := q.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: targetUserID, Role: role,
		})
		if err != nil {
			return err
		}
		added = &tenancy.BillingAccountMember{
			BillingAccountID: row.BillingAccountID, UserID: row.UserID, Role: row.Role,
			CreatedAt: dbutil.TimeValue(row.CreatedAt),
		}
		return nil
	})
	return added, err
}

// UpdateBillingAccountMemberRole applies handover-safe transitions. Only
// owners transfer ownership; the last owner cannot be demoted or removed.
func (s *BillingAccountStore) UpdateBillingAccountMemberRole(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64, newRole string) (*tenancy.BillingAccountMember, error) {
	if s == nil || s.db == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	var updated *tenancy.BillingAccountMember
	err := dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		if err := checkBillingMemberMutationInTx(ctx, q, billingAccountInternalID, actorUserID, targetUserID, newRole, false); err != nil {
			return err
		}
		row, err := q.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: targetUserID, Role: strings.TrimSpace(newRole),
		})
		if err != nil {
			return err
		}
		updated = &tenancy.BillingAccountMember{
			BillingAccountID: row.BillingAccountID, UserID: row.UserID, Role: row.Role,
			CreatedAt: dbutil.TimeValue(row.CreatedAt),
		}
		return nil
	})
	return updated, err
}

// textOrEmpty projects a nullable public id for Admin display. Invalid
// values render empty; the ba_/ws_/org_ wire shape is set only from valid
// database facts, never synthesized here.
func textOrEmpty(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func mapBillingAccount(row dbsqlc.BillingAccount, workspaceInternalID int64, workspacePublicID string) tenancy.BillingAccount {
	ownerType := funds.OwnerTypeWorkspace
	if row.OrganizationID.Valid {
		ownerType = funds.OwnerTypeOrganization
	}
	return tenancy.BillingAccount{
		InternalID:          row.InternalID,
		PublicID:            row.PublicID,
		OwnerType:           ownerType,
		WorkspaceInternalID: workspaceInternalID,
		WorkspacePublicID:   workspacePublicID,
		CreatedAt:           dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:           dbutil.TimeValue(row.UpdatedAt),
	}
}
