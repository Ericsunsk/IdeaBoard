// Package postgres implements tenancy billing-account resolution (#915).
//
// A Billing Account holds the unified balance; many Workspaces may share one
// account. Resolution is explicit and fail-closed: explicit
// workspace_billing_links first, else the Organization account for org
// workspaces, else the per-workspace account. Missing, ambiguous, or
// conflicting ownership fails closed instead of silently picking another
// wallet. Joining a Workspace never merges personal funds.
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

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Billing owner types for the API projection.
const (
	BillingOwnerWorkspace    = "workspace"
	BillingOwnerOrganization = "organization"
)

// BillingAccount is the stable funds-identity projection. Internal numeric
// keys never cross the HTTP boundary except InternalID for joins; the API
// addresses the account via its ba_ PublicID plus the resolving Workspace.
type BillingAccount struct {
	InternalID int64
	PublicID   string
	OwnerType  string
	// WorkspaceInternalID is the resolving workspace (consumption location).
	WorkspaceInternalID int64
	WorkspacePublicID   string
	Shared              bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

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
	return repositoryQueryer{ctx: ctx, db: s.db, q: s.q}
}

// EnsureBillingAccountForWorkspace provisions the stable billing account on
// first read (Get-or-Create) via the #915 resolution order. The identity is
// stable across Workspace Owner changes: it never derives from the current
// Workspace Owner's personal wallet.
//
// Concurrency: the partial unique indexes collapse concurrent first reads
// to one row; the loser reads back the winner on duplicate error.
func (s *BillingAccountStore) EnsureBillingAccountForWorkspace(ctx context.Context, workspaceInternalID int64, workspacePublicID string) (BillingAccount, error) {
	if s == nil || s.q == nil {
		return BillingAccount{}, tenancy.ErrBillingStoreNotConfigured
	}
	row, shared, err := resolveBillingAccountRow(ctx, s.dbWithContext(ctx).q, workspaceInternalID)
	if err != nil {
		return BillingAccount{}, err
	}
	return mapBillingAccount(row, workspaceInternalID, workspacePublicID, shared), nil
}

// EnsureBillingAccountForWorkspaceWithQueries applies the same resolution to
// an existing query set, including a caller-owned transaction. Task
// settlement uses this form so resolution stays in the task insert
// transaction without duplicating logic.
func EnsureBillingAccountForWorkspaceWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.BillingAccount, error) {
	row, _, err := resolveBillingAccountRow(ctx, queries, workspaceInternalID)
	return row, err
}

// resolveBillingAccountRow implements the stable order: explicit link, else
// org account, else per-workspace account. It provisions the missing account
// Get-or-Create style and fails closed on ambiguity or conflict.
func resolveBillingAccountRow(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.BillingAccount, bool, error) {
	if queries == nil || workspaceInternalID <= 0 {
		return dbsqlc.BillingAccount{}, false, tenancy.ErrBillingWorkspaceInvalid
	}
	// 1. Explicit link (studio-shared or pinned org link).
	if link, err := queries.GetWorkspaceBillingLink(ctx, workspaceInternalID); err == nil {
		row, err := queries.GetBillingAccountByInternalID(ctx, link.BillingAccountID)
		if err != nil {
			if database.IsNotFound(err) {
				return dbsqlc.BillingAccount{}, false, tenancy.ErrBillingNotFound
			}
			return dbsqlc.BillingAccount{}, false, err
		}
		if err := checkLinkOwnership(ctx, queries, workspaceInternalID, row); err != nil {
			return dbsqlc.BillingAccount{}, false, err
		}
		return row, true, nil
	} else if !database.IsNotFound(err) {
		return dbsqlc.BillingAccount{}, false, err
	}
	// 2/3. Need the workspace row for org scope.
	ws, err := queries.GetWorkspaceByInternalID(ctx, workspaceInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return dbsqlc.BillingAccount{}, false, tenancy.ErrBillingWorkspaceInvalid
		}
		return dbsqlc.BillingAccount{}, false, err
	}
	if ws.OrganizationID.Valid {
		return resolveOrganizationAccount(ctx, queries, ws.OrganizationID.Int64)
	}
	return resolveWorkspaceAccount(ctx, queries, workspaceInternalID)
}

// resolveOrganizationAccount returns the shared org pool, provisioning once.
func resolveOrganizationAccount(ctx context.Context, queries *dbsqlc.Queries, organizationInternalID int64) (dbsqlc.BillingAccount, bool, error) {
	if organizationInternalID <= 0 {
		return dbsqlc.BillingAccount{}, false, tenancy.ErrBillingWorkspaceInvalid
	}
	if row, err := queries.GetBillingAccountByOrganizationID(ctx, pgtype.Int8{Int64: organizationInternalID, Valid: true}); err == nil {
		return row, true, nil
	} else if !database.IsNotFound(err) {
		return dbsqlc.BillingAccount{}, false, err
	}
	publicID, err := tenancy.NewBillingAccountPublicID()
	if err != nil {
		return dbsqlc.BillingAccount{}, false, err
	}
	created, err := queries.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		PublicID: publicID, WorkspaceID: pgtype.Int8{},
		OrganizationID: pgtype.Int8{Int64: organizationInternalID, Valid: true},
	})
	if err != nil {
		if database.IsDuplicateConstraintError(err) {
			if row, readErr := queries.GetBillingAccountByOrganizationID(ctx, pgtype.Int8{Int64: organizationInternalID, Valid: true}); readErr == nil {
				return row, true, nil
			}
		}
		return dbsqlc.BillingAccount{}, false, err
	}
	ensureBillingOwnerForOrg(ctx, queries, created.InternalID, organizationInternalID)
	return created, true, nil
}

// resolveWorkspaceAccount returns the per-workspace account, provisioning once.
func resolveWorkspaceAccount(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.BillingAccount, bool, error) {
	key := pgtype.Int8{Int64: workspaceInternalID, Valid: true}
	if row, err := queries.GetBillingAccountByWorkspaceID(ctx, key); err == nil {
		return row, false, nil
	} else if !database.IsNotFound(err) {
		return dbsqlc.BillingAccount{}, false, err
	}
	publicID, err := tenancy.NewBillingAccountPublicID()
	if err != nil {
		return dbsqlc.BillingAccount{}, false, err
	}
	created, err := queries.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		PublicID: publicID, WorkspaceID: key, OrganizationID: pgtype.Int8{},
	})
	if err != nil {
		if database.IsDuplicateConstraintError(err) {
			if row, readErr := queries.GetBillingAccountByWorkspaceID(ctx, key); readErr == nil {
				return row, false, nil
			}
		}
		return dbsqlc.BillingAccount{}, false, err
	}
	ensureBillingOwnerForWorkspace(ctx, queries, created.InternalID, workspaceInternalID)
	return created, false, nil
}

// checkLinkOwnership fails closed when an explicit link points at a
// conflicting owner: org workspaces may only link to their own org pool,
// personal workspaces only to non-org accounts. Joining a Workspace never
// merges funds across ownership boundaries.
func checkLinkOwnership(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64, account dbsqlc.BillingAccount) error {
	ws, err := queries.GetWorkspaceByInternalID(ctx, workspaceInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return tenancy.ErrBillingWorkspaceInvalid
		}
		return err
	}
	if ws.OrganizationID.Valid {
		if !account.OrganizationID.Valid || account.OrganizationID.Int64 != ws.OrganizationID.Int64 {
			return tenancy.ErrBillingConflict
		}
		return nil
	}
	if account.OrganizationID.Valid {
		return tenancy.ErrBillingConflict
	}
	return nil
}

// ensureBillingOwnerForWorkspace seeds the workspace owner as billing owner
// (idempotent). Management stays transferable afterwards.
func ensureBillingOwnerForWorkspace(ctx context.Context, queries *dbsqlc.Queries, billingAccountID, workspaceInternalID int64) {
	ws, err := queries.GetWorkspaceByInternalID(ctx, workspaceInternalID)
	if err != nil || !ws.OwnerUserID.Valid || ws.OwnerUserID.Int64 <= 0 {
		return
	}
	_ = ensureBillingOwner(ctx, queries, billingAccountID, ws.OwnerUserID.Int64)
}

// ensureBillingOwnerForOrg seeds the org owner as billing owner when
// discoverable. Empty pools stay manageable once a member is granted.
func ensureBillingOwnerForOrg(ctx context.Context, queries *dbsqlc.Queries, billingAccountID, organizationInternalID int64) {
	owners, err := queries.ListOrganizationOwners(ctx, organizationInternalID)
	if err != nil || len(owners) == 0 {
		return
	}
	_ = ensureBillingOwner(ctx, queries, billingAccountID, owners[0].UserID)
}

func ensureBillingOwner(ctx context.Context, queries *dbsqlc.Queries, billingAccountID, userID int64) error {
	if billingAccountID <= 0 || userID <= 0 {
		return tenancy.ErrBillingWorkspaceInvalid
	}
	if _, err := queries.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
		BillingAccountID: billingAccountID, UserID: userID,
	}); err == nil {
		return nil
	} else if !database.IsNotFound(err) {
		return err
	}
	_, err := queries.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
		BillingAccountID: billingAccountID, UserID: userID, Role: tenancy.BillingRoleOwner,
	})
	return err
}

// AdminBillingAccount is the platform-Admin locate/audit projection of one
// stable funds identity (#923). Unlike the workspace-resolving BillingAccount
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
		out = append(out, AdminBillingAccount{
			InternalID:           row.InternalID,
			PublicID:             row.PublicID,
			OwnerType:            ownerTypeOf(dbsqlc.BillingAccount{InternalID: row.InternalID, PublicID: row.PublicID, WorkspaceID: row.WorkspaceID, OrganizationID: row.OrganizationID}),
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
func (s *BillingAccountStore) GetBillingAccountByInternalID(ctx context.Context, billingAccountInternalID int64) (BillingAccount, error) {
	if s == nil || s.q == nil || billingAccountInternalID <= 0 {
		return BillingAccount{}, tenancy.ErrBillingNotFound
	}
	row, err := s.dbWithContext(ctx).q.GetBillingAccountByInternalID(ctx, billingAccountInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return BillingAccount{}, tenancy.ErrBillingNotFound
		}
		return BillingAccount{}, err
	}
	return BillingAccount{
		InternalID: row.InternalID, PublicID: row.PublicID, OwnerType: ownerTypeOf(row),
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
		InternalID: row.InternalID, PublicID: row.PublicID, OwnerType: ownerTypeOf(row),
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
func (s *BillingAccountStore) GetBillingAccountByPublicID(ctx context.Context, publicID string) (BillingAccount, error) {
	if s == nil || s.q == nil {
		return BillingAccount{}, tenancy.ErrBillingStoreNotConfigured
	}
	trimmed := strings.TrimSpace(publicID)
	if !isBillingAccountID(trimmed) {
		return BillingAccount{}, tenancy.ErrBillingNotFound
	}
	row, err := s.dbWithContext(ctx).q.GetBillingAccountByPublicID(ctx, trimmed)
	if err != nil {
		if database.IsNotFound(err) {
			return BillingAccount{}, tenancy.ErrBillingNotFound
		}
		return BillingAccount{}, err
	}
	return BillingAccount{
		InternalID: row.InternalID, PublicID: row.PublicID, OwnerType: ownerTypeOf(row),
		CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}, nil
}

func isBillingAccountID(id string) bool {
	return strings.HasPrefix(id, "ba_") && len(id) > 4
}

// LinkWorkspaceToBillingAccount creates the explicit stable association for
// no-org studio sharing (and pinned org links). It fails closed on missing,
// ambiguous, or conflicting ownership, and refuses silent payer switches:
// an existing link to a different account reports ErrBillingConflict
// (Workspace更换付款账户不在本批范围).
//
// The actor must hold Workspace Owner/Admin on the workspace AND Billing
// Owner/Admin on the target account; space-only admins cannot rewire funds.
func (s *BillingAccountStore) LinkWorkspaceToBillingAccount(ctx context.Context, workspaceInternalID, billingAccountInternalID, actorUserID int64) error {
	if s == nil || s.db == nil {
		return tenancy.ErrBillingStoreNotConfigured
	}
	return dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		return linkWorkspaceToBillingAccount(ctx, q, s.db, workspaceInternalID, billingAccountInternalID, actorUserID)
	})
}

func linkWorkspaceToBillingAccount(ctx context.Context, q *dbsqlc.Queries, pool *pgxpool.Pool, workspaceInternalID, billingAccountInternalID, actorUserID int64) error {
	if workspaceInternalID <= 0 || billingAccountInternalID <= 0 || actorUserID <= 0 {
		return tenancy.ErrBillingWorkspaceInvalid
	}
	// Workspace side: must exist and actor must be owner/admin.
	ws, err := q.GetWorkspaceByInternalID(ctx, workspaceInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return tenancy.ErrBillingWorkspaceInvalid
		}
		return err
	}
	_ = ws
	wsRole, err := workspaceRoleForLink(ctx, q, workspaceInternalID, actorUserID)
	if err != nil {
		return err
	}
	if wsRole != tenancy.RoleOwner && wsRole != tenancy.RoleAdmin {
		return tenancy.ErrForbidden
	}
	// Funds side: target must exist and actor must be billing owner/admin.
	account, err := q.GetBillingAccountByInternalID(ctx, billingAccountInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return tenancy.ErrBillingNotFound
		}
		return err
	}
	billingRole, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
		BillingAccountID: billingAccountInternalID, UserID: actorUserID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return tenancy.ErrBillingForbidden
		}
		return err
	}
	if !tenancy.IsBillingManager(billingRole.Role) {
		return tenancy.ErrBillingForbidden
	}
	// Ownership boundary: no cross-owner silent merges.
	if err := checkLinkOwnership(ctx, q, workspaceInternalID, account); err != nil {
		return err
	}
	// No silent payer switch.
	if existing, err := q.GetWorkspaceBillingLink(ctx, workspaceInternalID); err == nil {
		if existing.BillingAccountID == billingAccountInternalID {
			return nil
		}
		return tenancy.ErrBillingConflict
	} else if !database.IsNotFound(err) {
		return err
	}
	_, err = q.UpsertWorkspaceBillingLink(ctx, dbsqlc.UpsertWorkspaceBillingLinkParams{
		WorkspaceID: workspaceInternalID, BillingAccountID: billingAccountInternalID,
	})
	return err
}

func workspaceRoleForLink(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID, userID int64) (string, error) {
	row, err := q.GetWorkspaceMembershipByIDs(ctx, dbsqlc.GetWorkspaceMembershipByIDsParams{
		WorkspaceID: workspaceInternalID, UserID: userID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return "", tenancy.ErrForbidden
		}
		return "", err
	}
	return row.Role, nil
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
		if _, err := q.GetBillingAccountByInternalID(ctx, billingAccountInternalID); err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingNotFound
			}
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
		actor, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: actorUserID,
		})
		actorRole := ""
		if err == nil {
			actorRole = actor.Role
		} else if !database.IsNotFound(err) {
			return err
		}
		target, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: targetUserID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingNotFound
			}
			return err
		}
		owners, err := q.CountBillingAccountOwners(ctx, billingAccountInternalID)
		if err != nil {
			return err
		}
		if err := tenancy.CheckBillingMemberMutation(actorRole, actorUserID, targetUserID, target.Role, newRole, int(owners), false); err != nil {
			if errors.Is(err, tenancy.ErrBillingLastOwner) {
				return err
			}
			return tenancy.ErrBillingForbidden
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

// TransferBillingAccountOwnership hands management to another manager: the
// new owner becomes owner and the previous owner steps down to admin.
// Identity, balance, and history stay stable; only management changes.
func (s *BillingAccountStore) TransferBillingAccountOwnership(ctx context.Context, billingAccountInternalID, currentOwnerID, newOwnerID int64) error {
	if s == nil || s.db == nil {
		return tenancy.ErrBillingStoreNotConfigured
	}
	if currentOwnerID <= 0 || newOwnerID <= 0 || currentOwnerID == newOwnerID {
		return tenancy.ErrBillingWorkspaceInvalid
	}
	return dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		current, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: currentOwnerID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingForbidden
			}
			return err
		}
		if !tenancy.IsBillingOwner(current.Role) {
			return tenancy.ErrBillingForbidden
		}
		target, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: newOwnerID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingNotFound
			}
			return err
		}
		_ = target
		if _, err := q.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: newOwnerID, Role: tenancy.BillingRoleOwner,
		}); err != nil {
			return err
		}
		_, err = q.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: currentOwnerID, Role: tenancy.BillingRoleAdmin,
		})
		return err
	})
}

func ownerTypeOf(row dbsqlc.BillingAccount) string {
	if row.OrganizationID.Valid {
		return BillingOwnerOrganization
	}
	return BillingOwnerWorkspace
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

func mapBillingAccount(row dbsqlc.BillingAccount, workspaceInternalID int64, workspacePublicID string, shared bool) BillingAccount {
	return BillingAccount{
		InternalID:          row.InternalID,
		PublicID:            row.PublicID,
		OwnerType:           ownerTypeOf(row),
		WorkspaceInternalID: workspaceInternalID,
		WorkspacePublicID:   workspacePublicID,
		Shared:              shared,
		CreatedAt:           dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:           dbutil.TimeValue(row.UpdatedAt),
	}
}
