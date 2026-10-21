// Package postgres implements tenancy.Store against PostgreSQL/sqlc.
//
// This is the #801 DB-stream backend for the agreed tenancy interface owned
// by internal/tenancy. User identity translation reuses
// tenancy.FormatUserPublicID/ParseUserPublicID (usr_<uuid_hex> over the
// stored users.public_id UUID); the users table is never altered here.
// Repository methods take context.Context first and use dbWithContext(ctx).
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Public list bounds. Whole-account commands enumerate their complete scope.
const maxTenancyRows = int32(500)

// mintAttempts bounds public_id regeneration after a random collision.
const mintAttempts = 5

// maxRevisionPasses bounds the leaf-first board revision sweep; a healthy
// chain converges in depth-many passes.
const maxRevisionPasses = 1000

// objectDeleteAttempts bounds the best-effort post-commit storage cleanup
// per key.
const objectDeleteAttempts = 3

// ObjectDeleter is the narrow storage seam hard delete needs. It is
// satisfied by media storage.Service; the store depends on the shape,
// never on the media implementation.
type ObjectDeleter interface {
	DeleteObjectByKey(ctx context.Context, objectKey string) error
}

// Store is the postgres tenancy.Store. It must never be replaced by the
// test-only InMemoryStore in production wiring.
type Store struct {
	db      *pgxpool.Pool
	q       *dbsqlc.Queries
	objects ObjectDeleter
}

// Compile-time convergence with the agreed interface.
var _ tenancy.Store = (*Store)(nil)

type repositoryQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

// NewStore builds a postgres tenancy Store over pool. objects may be nil
// in tests that only assert row effects; a nil deleter skips post-commit
// storage cleanup.
func NewStore(db *pgxpool.Pool, objects ObjectDeleter) *Store {
	return &Store{db: db, q: dbsqlc.New(db), objects: objects}
}

func (s *Store) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: s.db, q: s.q}
}

func (db repositoryQueryer) withTx(run func(*dbsqlc.Queries) error) error {
	// Ambient guard transaction (control audit atomicity, step 3): when
	// the guard staged its mutation transaction in the context, the
	// tenancy writes join it as participants. The transaction is
	// BORROWED (the guard began it and the guard commits or rolls it
	// back), so this path never begins, commits, or rolls back. With no
	// carrier the legacy owned-transaction path below runs unchanged.
	if ambient, ok := dbutil.AmbientTxFromContext(db.ctx); ok {
		return run(dbsqlc.New(ambient))
	}
	return dbutil.WithTx(db.ctx, db.db, run)
}

func notFound(err error) error {
	if database.IsNotFound(err) {
		return tenancy.ErrNotFound
	}
	return err
}

func isPublicIDConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "uq_workspaces_public_id") ||
		strings.Contains(lowered, "uq_organizations_public_id")
}

// isDefaultConflict reports a duplicate violation of the single-Default
// partial unique index (a second Default create for the same owner).
func isDefaultConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "ux_workspaces_default_owner")
}

func nullInt64(value int64) pgtype.Int8 {
	if value <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: value, Valid: true}
}

func mapOrganization(row dbsqlc.Organization) *tenancy.Organization {
	return &tenancy.Organization{
		InternalID: row.InternalID,
		PublicID:   row.PublicID,
		Name:       row.Name,
		ArchivedAt: dbutil.TimePtr(row.ArchivedAt),
		CreatedAt:  dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:  dbutil.TimeValue(row.UpdatedAt),
	}
}

func mapWorkspace(row dbsqlc.Workspace, organizationPublicID *string) *tenancy.Workspace {
	return &tenancy.Workspace{
		InternalID:     row.InternalID,
		PublicID:       row.PublicID,
		OrganizationID: organizationPublicID,
		OwnerUserID:    dbutil.Int64Ptr(row.OwnerUserID),
		Name:           row.Name,
		IsDefault:      row.IsDefault,
		CreatedAt:      dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:      dbutil.TimeValue(row.UpdatedAt),
	}
}

// resolveOrgPublicID maps a nullable organization internal id to its public
// id. Callers pass the already-known public id when they have it.
func (s *Store) resolveOrgPublicID(ctx context.Context, q *dbsqlc.Queries, organizationID pgtype.Int8) (*string, error) {
	if !organizationID.Valid {
		return nil, nil
	}
	row, err := q.GetOrganizationByInternalID(ctx, organizationID.Int64)
	if err != nil {
		return nil, notFound(err)
	}
	publicID := row.PublicID
	return &publicID, nil
}

func (s *Store) mapWorkspaceRow(ctx context.Context, row dbsqlc.Workspace) (*tenancy.Workspace, error) {
	orgPublicID, err := s.resolveOrgPublicID(ctx, s.q, row.OrganizationID)
	if err != nil {
		return nil, err
	}
	return mapWorkspace(row, orgPublicID), nil
}

// userExternalIDs batch-resolves users.id to usr_<hex> external ids.
func userExternalIDs(ctx context.Context, q *dbsqlc.Queries, userIDs []int64) (map[int64]string, error) {
	external := make(map[int64]string, len(userIDs))
	if len(userIDs) == 0 {
		return external, nil
	}
	rows, err := q.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		external[row.ID] = tenancy.FormatUserPublicID(uuid.UUID(row.PublicID.Bytes))
	}
	return external, nil
}

func mapWorkspaceMember(publicID string, row dbsqlc.WorkspaceMembership, externalIDs map[int64]string) *tenancy.WorkspaceMember {
	return &tenancy.WorkspaceMember{
		WorkspacePublicID: publicID,
		UserID:            row.UserID,
		UserExternalID:    externalIDs[row.UserID],
		Role:              row.Role,
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
	}
}

// GetWorkspace resolves a workspace by opaque public id. Deletion is an
// immediate hard delete, so missing rows read as 404.
func (s *Store) GetWorkspace(ctx context.Context, publicID string) (*tenancy.Workspace, error) {
	row, err := s.dbWithContext(ctx).q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(publicID))
	if err != nil {
		return nil, notFound(err)
	}
	return s.mapWorkspaceRow(ctx, row)
}

// ListWorkspacesForUser lists membership-visible workspaces. Owners always
// hold an owner membership row (granted at create/ensure time), so a
// membership join matches the in-memory Store exactly.
func (s *Store) ListWorkspacesForUser(ctx context.Context, userID int64) ([]*tenancy.Workspace, error) {
	rows, err := s.dbWithContext(ctx).q.ListWorkspacesByUserID(ctx, dbsqlc.ListWorkspacesByUserIDParams{
		UserID: userID, RowLimit: maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	return s.mapWorkspaceRows(ctx, rows)
}

func (s *Store) mapWorkspaceRows(ctx context.Context, rows []dbsqlc.Workspace) ([]*tenancy.Workspace, error) {
	orgIDs := make(map[int64]struct{})
	for _, row := range rows {
		if row.OrganizationID.Valid {
			orgIDs[row.OrganizationID.Int64] = struct{}{}
		}
	}
	publicIDs := make(map[int64]string, len(orgIDs))
	for internalID := range orgIDs {
		row, err := s.q.GetOrganizationByInternalID(ctx, internalID)
		if err != nil {
			return nil, notFound(err)
		}
		publicIDs[internalID] = row.PublicID
	}
	out := make([]*tenancy.Workspace, 0, len(rows))
	for _, row := range rows {
		var orgPublicID *string
		if row.OrganizationID.Valid {
			publicID := publicIDs[row.OrganizationID.Int64]
			orgPublicID = &publicID
		}
		out = append(out, mapWorkspace(row, orgPublicID))
	}
	return out, nil
}

// CreateWorkspace mints a ws_ workspace under an organization (by public id)
// or a personal owner. The owner invariant (organization OR owner) is
// enforced by the database CHECK; the owner gains an owner membership row.
// Personal creates without an organization are always non-Default
// (IsDefault must stay false on this path); only EnsureDefaultWorkspace
// mints the Default row. A second Default create for the same owner fails
// with tenancy.ErrDefaultWorkspaceConflict.
//
// Billing linkage (#944) is atomic with the workspace row in the same
// transaction, so authorization failures never leave a half-linked
// workspace: org workspaces resolve to the shared org pool (billing opts
// rejected with ErrBillingConflict, never the creator personal pool);
// personal workspaces link to an explicit ba_ account (caller must be
// Billing Owner/Admin, ErrBillingForbidden) or mint a fresh isolated
// per-workspace account (NewBillingAccount, creator becomes billing
// owner, no balance merge). Empty billing opts keep the legacy lazy
// mode (no link row, per-workspace account resolved on first billing
// read) for direct Store callers; the Public API always passes an
// explicit choice so new spaces never auto-create a wallet implicitly.
func (s *Store) CreateWorkspace(ctx context.Context, params tenancy.CreateWorkspaceParams) (*tenancy.Workspace, error) {
	if err := tenancy.ValidateName(params.Name); err != nil {
		return nil, err
	}
	if params.IsDefault && params.OrganizationID != nil {
		return nil, tenancy.ErrDefaultWorkspaceConflict
	}
	// Billing opts are mutually exclusive and Default/org-scoped guarded.
	if params.BillingAccountPublicID != nil && params.NewBillingAccount {
		return nil, tenancy.ErrBillingConflict
	}
	if params.IsDefault && (params.BillingAccountPublicID != nil || params.NewBillingAccount) {
		return nil, tenancy.ErrBillingConflict
	}
	var billingTarget string
	if params.BillingAccountPublicID != nil {
		billingTarget = strings.TrimSpace(*params.BillingAccountPublicID)
		if !strings.HasPrefix(billingTarget, "ba_") || len(billingTarget) <= 3 {
			return nil, tenancy.ErrBillingNotFound
		}
		if params.OrganizationID != nil {
			return nil, tenancy.ErrBillingConflict
		}
	}
	if params.NewBillingAccount && params.OrganizationID != nil {
		return nil, tenancy.ErrBillingConflict
	}
	var orgInternal pgtype.Int8
	var orgPublicID *string
	if params.OrganizationID != nil {
		org, err := s.GetOrganization(ctx, *params.OrganizationID)
		if err != nil {
			return nil, err
		}
		orgInternal = pgtype.Int8{Int64: org.InternalID, Valid: true}
		orgPublicID = &org.PublicID
	}
	if !orgInternal.Valid && params.OwnerUserID <= 0 {
		return nil, tenancy.ErrWorkspaceOwnerRequired
	}
	var created dbsqlc.Workspace
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewWorkspacePublicID()
		if err != nil {
			return nil, err
		}
		err = s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
			// Organization workspaces carry no owner_user_id per
			// chk_workspaces_owner_xor (org owns the workspace); the
			// creator still gains an owner membership row below.
			ownerCol := nullInt64(params.OwnerUserID)
			if orgInternal.Valid {
				ownerCol = pgtype.Int8{}
			}
			row, insertErr := q.CreateWorkspace(ctx, dbsqlc.CreateWorkspaceParams{
				PublicID:       publicID,
				OrganizationID: orgInternal,
				OwnerUserID:    ownerCol,
				Name:           strings.TrimSpace(params.Name),
				IsDefault:      params.IsDefault && !orgInternal.Valid,
			})
			if insertErr != nil {
				return insertErr
			}
			created = row
			if params.OwnerUserID > 0 {
				if err := q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{
					WorkspaceID: row.InternalID,
					UserID:      params.OwnerUserID,
					Role:        tenancy.RoleOwner,
				}); err != nil {
					return err
				}
			}
			return s.createBillingInTx(ctx, q, row, orgInternal, params, billingTarget)
		})
		if isPublicIDConflict(err) {
			continue
		}
		if isDefaultConflict(err) {
			return nil, tenancy.ErrDefaultWorkspaceConflict
		}
		if err != nil {
			return nil, err
		}
		return mapWorkspace(created, orgPublicID), nil
	}
	return nil, tenancy.ErrConflict
}

// createBillingInTx provisions the creation-time billing linkage inside the
// same workspace transaction (#944). Org workspaces ensure the shared org
// pool (no per-workspace account, no link row); Default rows mint their own
// deterministic personal account; personal NewBillingAccount mints a fresh
// isolated per-workspace account; explicit BillingAccountPublicID links to
// an authorized non-org account; empty opts keep the legacy lazy mode.
func (s *Store) createBillingInTx(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.Workspace, orgInternal pgtype.Int8, params tenancy.CreateWorkspaceParams, billingTarget string) error {
	if orgInternal.Valid {
		// Org pool Get-or-Create (concurrent first workspaces collapse on
		// the org unique index; the loser reads back the winner).
		if account, err := q.GetBillingAccountByOrganizationID(ctx, orgInternal); err == nil {
			_, err = lockBillingAccountInTx(ctx, q, account.InternalID)
			return err
		} else if !database.IsNotFound(err) {
			return err
		}
		publicID, err := tenancy.NewBillingAccountPublicID()
		if err != nil {
			return err
		}
		created, err := q.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
			PublicID: publicID, WorkspaceID: pgtype.Int8{},
			OrganizationID: orgInternal,
		})
		if err != nil {
			if database.IsDuplicateConstraintError(err) {
				return nil
			}
			return err
		}
		ensureBillingOwnerForOrg(ctx, q, created.InternalID, orgInternal.Int64)
		return nil
	}
	// Personal scope.
	if params.IsDefault || params.NewBillingAccount {
		return s.ensureWorkspaceAccountInTx(ctx, q, row.InternalID, params.OwnerUserID)
	}
	if billingTarget == "" {
		return nil
	}
	target, err := q.GetBillingAccountByPublicID(ctx, billingTarget)
	if err != nil {
		if database.IsNotFound(err) {
			return tenancy.ErrBillingNotFound
		}
		return err
	}
	if _, err := lockBillingAccountInTx(ctx, q, target.InternalID); err != nil {
		return err
	}
	if target.OrganizationID.Valid {
		return tenancy.ErrBillingConflict
	}
	member, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
		BillingAccountID: target.InternalID, UserID: params.OwnerUserID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return tenancy.ErrBillingForbidden
		}
		return err
	}
	if !tenancy.IsBillingManager(member.Role) {
		return tenancy.ErrBillingForbidden
	}
	_, err = q.UpsertWorkspaceBillingLink(ctx, dbsqlc.UpsertWorkspaceBillingLinkParams{
		WorkspaceID: row.InternalID, BillingAccountID: target.InternalID,
	})
	return err
}

// ensureWorkspaceAccountInTx Get-or-Creates the per-workspace account for a
// freshly minted workspace plus the creator billing-owner grant. The wallet
// row follows via the account-insert trigger; no link row is written (the
// per-workspace account is the resolution fallback).
func (s *Store) ensureWorkspaceAccountInTx(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID, ownerUserID int64) error {
	if workspaceInternalID <= 0 {
		return tenancy.ErrBillingWorkspaceInvalid
	}
	if _, err := q.GetBillingAccountByWorkspaceID(ctx, pgtype.Int8{Int64: workspaceInternalID, Valid: true}); err == nil {
		return nil
	} else if !database.IsNotFound(err) {
		return err
	}
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewBillingAccountPublicID()
		if err != nil {
			return err
		}
		account, err := q.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
			PublicID:       publicID,
			WorkspaceID:    pgtype.Int8{Int64: workspaceInternalID, Valid: true},
			OrganizationID: pgtype.Int8{},
		})
		if err != nil {
			if database.IsDuplicateConstraintError(err) {
				if _, readErr := q.GetBillingAccountByWorkspaceID(ctx, pgtype.Int8{Int64: workspaceInternalID, Valid: true}); readErr == nil {
					return nil
				}
				continue
			}
			return err
		}
		if ownerUserID <= 0 {
			return tenancy.ErrWorkspaceOwnerRequired
		}
		_, err = q.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: account.InternalID, UserID: ownerUserID, Role: tenancy.BillingRoleOwner,
		})
		return err
	}
	return tenancy.ErrConflict
}

// ensureOrganizationAccountInTx Get-or-Creates the shared org pool plus the
// creator billing-owner grant inside the caller's transaction (#944). The
// wallet row follows via the account-insert trigger. Concurrent first
// workspaces collapse on the org unique index; the loser reads back the
// winner and leaves existing grants untouched.
func (s *Store) ensureOrganizationAccountInTx(ctx context.Context, q *dbsqlc.Queries, organizationInternalID, ownerUserID int64) error {
	if organizationInternalID <= 0 {
		return tenancy.ErrBillingWorkspaceInvalid
	}
	orgKey := pgtype.Int8{Int64: organizationInternalID, Valid: true}
	if _, err := q.GetBillingAccountByOrganizationID(ctx, orgKey); err == nil {
		return nil
	} else if !database.IsNotFound(err) {
		return err
	}
	publicID, err := tenancy.NewBillingAccountPublicID()
	if err != nil {
		return err
	}
	created, err := q.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		PublicID: publicID, WorkspaceID: pgtype.Int8{},
		OrganizationID: orgKey,
	})
	if err != nil {
		if database.IsDuplicateConstraintError(err) {
			return nil
		}
		return err
	}
	if ownerUserID > 0 {
		_, _ = q.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: created.InternalID, UserID: ownerUserID, Role: tenancy.BillingRoleOwner,
		})
	} else {
		ensureBillingOwnerForOrg(ctx, q, created.InternalID, organizationInternalID)
	}
	return nil
}

// UpdateWorkspace renames an existing workspace; missing rows read
// as not found. The Default workspace is never renamed: attempts fail with
// tenancy.ErrDefaultWorkspaceProtected. The is_default marker is immutable
// after creation, so the pre-check below cannot race a flag flip.
func (s *Store) UpdateWorkspace(ctx context.Context, publicID, name string) (*tenancy.Workspace, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	workspace, err := s.GetWorkspace(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if workspace.IsDefaultWorkspace() {
		return nil, tenancy.ErrDefaultWorkspaceProtected
	}
	rows, err := s.dbWithContext(ctx).q.UpdateWorkspaceName(ctx, dbsqlc.UpdateWorkspaceNameParams{
		Name: strings.TrimSpace(name), PublicID: strings.TrimSpace(publicID),
	})
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, tenancy.ErrNotFound
	}
	return s.GetWorkspace(ctx, publicID)
}

// DeleteWorkspace is an immediate irreversible hard delete: it removes
// the workspace row with its boards and files in one operation. Billing
// records, the credit ledger, user rows and control audit records are
// retained (never named in the delete order below). The Default workspace
// is never deleted (ErrDefaultWorkspaceProtected); unknown ids read as
// ErrNotFound. Pending/running Tasks block deletion until their canonical
// lifecycle settles any Credit Hold.
//
// Delete order inside one workspace transaction follows the RESTRICT edges
// so no step blocks on a later one:
//
//	execution string-keyed rows -> tasks (cascades task_outputs)
//	library refs -> library_assets (cascades materializations)
//	tasks + library assets -> media_assets -> workspace
//	board_revisions (leaf-first, parent_revision_id is RESTRICT) -> boards
//
// Cascade edges (memberships, billing accounts, folders, upload sessions,
// events, deliveries, documents, media refs, origins, idempotency) are left
// to their owning FK. Explicit deletes below release RESTRICT blockers or
// remove rows without foreign keys.
//
// Object bytes leave the database first: storage keys are collected inside
// the transaction, the rows commit, and only then are the objects deleted
// best-effort with retries outside the transaction. A storage failure never
// rolls back the committed row delete; it is logged.
func (s *Store) DeleteWorkspace(ctx context.Context, publicID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("tenancy: workspace delete: store not configured")
	}
	publicID = strings.TrimSpace(publicID)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tenancy: workspace delete: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Match handover/link ordering: account first, then Workspace. This also
	// waits for an in-progress handover before evaluating its deletion guard.
	q := dbsqlc.New(tx)
	lockedAccountIDs, err := q.LockWorkspaceBillingAccounts(ctx, publicID)
	if err != nil {
		return err
	}
	var (
		workspaceID int64
		isDefault   bool
	)
	// The Default marker is immutable but the exemption is re-checked
	// under row lock so a flag flip cannot race the delete.
	if err := tx.QueryRow(ctx, `
SELECT internal_id, is_default
FROM workspaces WHERE public_id = $1 FOR UPDATE`, publicID).Scan(&workspaceID, &isDefault); err != nil {
		if database.IsNotFound(err) {
			return tenancy.ErrNotFound
		}
		return err
	}
	if isDefault {
		return tenancy.ErrDefaultWorkspaceProtected
	}
	// Task inserts hold the Workspace FK KEY SHARE lock until submission
	// commits (including Hold and enqueue). Our FOR UPDATE lock therefore
	// waits for earlier submissions and prevents later inserts from racing
	// this check. Never erase the facts canonical settlement still needs.
	var activeTasks bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM tasks WHERE workspace_id = $1 AND status IN ('pending', 'running')
)`, workspaceID).Scan(&activeTasks); err != nil {
		return fmt.Errorf("tenancy: workspace delete: check active tasks: %w", err)
	}
	if activeTasks {
		return tenancy.ErrWorkspaceActiveTasks
	}
	// Vision Search in-flight operations converge on the submit-time
	// account (#918): pending/reconciliation_pending must settle before
	// the scope disappears. Terminal rows stay readable under their
	// workspace_id snapshot (no FK), so history attribution survives.
	var activeSearch bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM ai_search_operations WHERE workspace_id = $1 AND status IN ('pending', 'reconciliation_pending')
)`, workspaceID).Scan(&activeSearch); err != nil {
		return fmt.Errorf("tenancy: workspace delete: check active search: %w", err)
	}
	if activeSearch {
		return tenancy.ErrWorkspaceActiveTasks
	}
	// Funds guards (#948): resolve the existing payer without provisioning
	// (deletion must never mint a wallet as a side effect). Unsettled
	// payment orders and pending handover invites converge on the stored
	// payer first; a shared anchor still referenced by other linked
	// workspaces or other workspaces' tasks never deletes its funds row
	// while in use (fail closed with billing_account_conflict).
	if err := guardWorkspaceDeleteFunds(ctx, tx, workspaceID, lockedAccountIDs); err != nil {
		return err
	}
	objects, err := collectWorkspaceStorageKeys(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	if err := deleteWorkspaceRows(ctx, tx, workspaceID, publicID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("tenancy: workspace delete: commit: %w", err)
	}
	slog.Info("workspace deleted", "workspace_id", publicID)
	s.cleanupWorkspaceObjects(ctx, objects)
	return nil
}

// guardWorkspaceDeleteFunds enforces the #948 funds guards inside the
// workspace delete transaction (row lock already held, so concurrent
// submissions/invites serialize here). It resolves the existing payer
// without provisioning and fails closed with ErrBillingConflict when
// unsettled payment orders, pending handover invites, or shared-anchor
// references still need the funds row. Wallets, Ledger, snapshots, and
// operation rows are never named here (red lines, retained by construction).
func guardWorkspaceDeleteFunds(ctx context.Context, tx pgx.Tx, workspaceID int64, lockedAccountIDs []int64) error {
	var payerID *int64
	// Explicit link first (studio sharing), then org pool, then per-workspace.
	var linkedID int64
	if err := tx.QueryRow(ctx, `SELECT billing_account_id FROM workspace_billing_links WHERE workspace_id = $1`, workspaceID).Scan(&linkedID); err == nil {
		payerID = &linkedID
	} else if !database.IsNotFound(err) {
		return fmt.Errorf("tenancy: workspace delete: resolve link: %w", err)
	}
	if payerID == nil {
		var orgID pgtype.Int8
		if err := tx.QueryRow(ctx, `SELECT organization_id FROM workspaces WHERE internal_id = $1`, workspaceID).Scan(&orgID); err != nil {
			return fmt.Errorf("tenancy: workspace delete: resolve org scope: %w", err)
		}
		if orgID.Valid && orgID.Int64 > 0 {
			var orgAccount int64
			if err := tx.QueryRow(ctx, `SELECT internal_id FROM billing_accounts WHERE organization_id = $1 LIMIT 1`, orgID.Int64).Scan(&orgAccount); err == nil {
				payerID = &orgAccount
			} else if !database.IsNotFound(err) {
				return fmt.Errorf("tenancy: workspace delete: resolve org account: %w", err)
			}
		} else {
			var wsAccount int64
			if err := tx.QueryRow(ctx, `SELECT internal_id FROM billing_accounts WHERE workspace_id = $1 LIMIT 1`, workspaceID).Scan(&wsAccount); err == nil {
				payerID = &wsAccount
			} else if !database.IsNotFound(err) {
				return fmt.Errorf("tenancy: workspace delete: resolve workspace account: %w", err)
			}
		}
	}
	if payerID == nil {
		return nil
	}
	// A first explicit link may have committed while deletion waited for
	// the Workspace lock. Refuse that changed scope instead of acquiring a
	// new account lock after the Workspace and inverting handover ordering.
	if !slices.Contains(lockedAccountIDs, *payerID) {
		return tenancy.ErrBillingConflict
	}
	var unsettled bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM payment_orders WHERE billing_account_id = ANY($1::bigint[]) AND status IN ('creating', 'checkout_open', 'async_payment_pending', 'paid_pending_credit')
)`, lockedAccountIDs).Scan(&unsettled); err != nil {
		return fmt.Errorf("tenancy: workspace delete: check unsettled payments: %w", err)
	}
	if unsettled {
		return tenancy.ErrBillingConflict
	}
	var pendingInvite bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM billing_account_transfer_invites WHERE billing_account_id = ANY($1::bigint[]) AND status = 'pending'
)`, lockedAccountIDs).Scan(&pendingInvite); err != nil {
		return fmt.Errorf("tenancy: workspace delete: check pending handover: %w", err)
	}
	if pendingInvite {
		return tenancy.ErrBillingConflict
	}
	// Shared-anchor retention: the per-workspace anchor row cascades from
	// the workspace delete. When other linked workspaces or other
	// workspaces' tasks still reference that anchor, keep the funds row
	// (fail closed) instead of deleting shared money.
	var anchorID int64
	if err := tx.QueryRow(ctx, `SELECT internal_id FROM billing_accounts WHERE workspace_id = $1 LIMIT 1`, workspaceID).Scan(&anchorID); err != nil {
		if database.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("tenancy: workspace delete: resolve anchor: %w", err)
	}
	var linkedOthers bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM workspace_billing_links WHERE billing_account_id = $1 AND workspace_id <> $2
)`, anchorID, workspaceID).Scan(&linkedOthers); err != nil {
		return fmt.Errorf("tenancy: workspace delete: check shared links: %w", err)
	}
	if linkedOthers {
		return tenancy.ErrBillingConflict
	}
	var otherTasks bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM tasks WHERE billing_account_id = $1 AND workspace_id <> $2
)`, anchorID, workspaceID).Scan(&otherTasks); err != nil {
		return fmt.Errorf("tenancy: workspace delete: check shared tasks: %w", err)
	}
	if otherTasks {
		return tenancy.ErrBillingConflict
	}
	return nil
}

// collectWorkspaceStorageKeys snapshots media asset and upload-session
// object keys. Pending upload sessions carry NULL keys and are skipped.
func collectWorkspaceStorageKeys(ctx context.Context, tx pgx.Tx, workspaceID int64) ([]string, error) {
	rows, err := tx.Query(ctx, `
SELECT DISTINCT btrim(storage_key)
FROM (
  SELECT storage_key FROM media_assets WHERE workspace_id = $1
  UNION ALL
  SELECT storage_key FROM upload_sessions WHERE workspace_id = $1
) objects
WHERE storage_key IS NOT NULL AND btrim(storage_key) <> ''`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("tenancy: workspace delete: collect storage keys: %w", err)
	}
	defer rows.Close()
	objects, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("tenancy: workspace delete: scan storage keys: %w", err)
	}
	return objects, nil
}

// deleteWorkspaceRows removes every workspace-owned row in RESTRICT-safe
// order. Red-line tables (credit_*, task_billing_snapshots/settlements,
// users, payments, rewards, credit_wallets, statistics aggregates,
// control_audit_records) share no FK into these deletes and are never
// named here.
func deleteWorkspaceRows(ctx context.Context, tx pgx.Tx, workspaceID int64, publicID string) error {
	exec := func(query string, args ...any) error {
		_, err := tx.Exec(ctx, query, args...)
		return err
	}
	// String-keyed execution rows reference tasks by public_id (no FK);
	// delete them before the task rows disappear.
	if err := exec(`DELETE FROM task_execution_attempts WHERE task_id IN (
  SELECT public_id FROM tasks WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete execution attempts: %w", err)
	}
	if err := exec(`DELETE FROM task_execution_metrics WHERE task_id IN (
  SELECT public_id FROM tasks WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete execution metrics: %w", err)
	}
	if err := exec(`DELETE FROM task_cancellations WHERE task_id IN (
  SELECT public_id FROM tasks WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete cancellations: %w", err)
	}
	// library_asset_media_assets carries no FK (orphan risk): clear by
	// either side before parents disappear.
	if err := exec(`DELETE FROM library_asset_media_assets WHERE library_asset_id IN (
  SELECT internal_id FROM library_assets WHERE workspace_id = $1)
  OR media_asset_id IN (
  SELECT internal_id FROM media_assets WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete library asset links: %w", err)
	}
	// library_assets -> media_assets and -> folders are RESTRICT: clear
	// assets before both parents.
	if err := exec(`DELETE FROM library_assets WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete library assets: %w", err)
	}
	// Task deletion cascades outputs before media_assets and releases the
	// billing_accounts RESTRICT edge before the workspace cascades accounts.
	if err := exec(`DELETE FROM tasks WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete tasks: %w", err)
	}
	// board_revisions.parent_revision_id is RESTRICT on a self edge:
	// sweep leaves first until the chain drains.
	if err := deleteWorkspaceBoardRevisionsLeafFirst(ctx, tx, workspaceID); err != nil {
		return err
	}
	// Boards cascade documents, memberships, and media refs.
	if err := exec(`DELETE FROM boards WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete boards: %w", err)
	}
	// media_assets -> upload_sessions is RESTRICT: media first.
	if err := exec(`DELETE FROM media_assets WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete media assets: %w", err)
	}
	// TEXT outbox rows carry no FK; they leave with the workspace.
	if err := exec(`DELETE FROM task_status_event_outbox WHERE workspace_id = $1`, publicID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete task outbox: %w", err)
	}
	if err := exec(`DELETE FROM board_access_event_outbox WHERE workspace_id = $1`, publicID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete board access outbox: %w", err)
	}
	if err := exec(`DELETE FROM workspaces WHERE internal_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: delete workspace: %w", err)
	}
	return nil
}

// deleteWorkspaceBoardRevisionsLeafFirst drains board revisions without
// tripping the parent_revision_id RESTRICT edge: each pass deletes only
// revisions no surviving row names as parent.
func deleteWorkspaceBoardRevisionsLeafFirst(ctx context.Context, tx pgx.Tx, workspaceID int64) error {
	for pass := 0; pass < maxRevisionPasses; pass++ {
		tag, err := tx.Exec(ctx, `DELETE FROM board_revisions WHERE board_id IN (
  SELECT internal_id FROM boards WHERE workspace_id = $1)
  AND internal_id NOT IN (
  SELECT parent_revision_id FROM board_revisions
  WHERE parent_revision_id IS NOT NULL AND board_id IN (
  SELECT internal_id FROM boards WHERE workspace_id = $2))`, workspaceID, workspaceID)
		if err != nil {
			return fmt.Errorf("tenancy: workspace delete: delete board revisions: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
	}
	return fmt.Errorf("tenancy: workspace delete: board revision chain did not drain")
}

// cleanupWorkspaceObjects deletes stored bytes after the rows committed.
// Best-effort with retries: failures are logged, never returned, so
// storage trouble cannot roll back (or fail) the delete.
func (s *Store) cleanupWorkspaceObjects(ctx context.Context, keys []string) {
	if s == nil || s.objects == nil {
		return
	}
	for _, key := range keys {
		var err error
		for attempt := 0; attempt < objectDeleteAttempts; attempt++ {
			if err = s.objects.DeleteObjectByKey(ctx, key); err == nil {
				break
			}
		}
		if err != nil {
			slog.Warn("workspace delete storage delete failed", "storage_key", key, "err", err)
		}
	}
}

// EnsureDefaultWorkspace idempotently provisions the user's Default
// workspace plus its deterministic personal Billing Account (#944). Concurrent
// callers collapse on ux_workspaces_default_owner
// (INSERT ... ON CONFLICT DO NOTHING) and all read back the same row.
// Users holding only non-Default personal rows get a fresh Default row;
// existing rows are left untouched. The personal account (per-workspace row
// on the Default workspace, creator as billing owner, wallet via trigger)
// is ensured in the same transaction, so new-user provision never leaves a
// Default workspace without its funds identity and concurrent logins never
// fork duplicate accounts (per-workspace unique index + read-back).
func (s *Store) EnsureDefaultWorkspace(ctx context.Context, userID int64) (*tenancy.Workspace, error) {
	if userID <= 0 {
		return nil, tenancy.ErrPersonalWorkspaceUserRequired
	}
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewWorkspacePublicID()
		if err != nil {
			return nil, err
		}
		var row dbsqlc.Workspace
		err = s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
			if _, insertErr := q.InsertDefaultWorkspaceIgnore(ctx, dbsqlc.InsertDefaultWorkspaceIgnoreParams{
				PublicID:    publicID,
				OwnerUserID: pgtype.Int8{Int64: userID, Valid: true},
				Name:        "My Workspace",
			}); insertErr != nil {
				return insertErr
			}
			selected, selectErr := q.GetDefaultWorkspaceByOwner(ctx, pgtype.Int8{Int64: userID, Valid: true})
			if selectErr != nil {
				return selectErr
			}
			row = selected
			if err := q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{
				WorkspaceID: row.InternalID,
				UserID:      userID,
				Role:        tenancy.RoleOwner,
			}); err != nil {
				return err
			}
			return s.ensureWorkspaceAccountInTx(ctx, q, row.InternalID, userID)
		})
		if isPublicIDConflict(err) {
			continue
		}
		if err != nil {
			return nil, notFound(err)
		}
		return mapWorkspace(row, nil), nil
	}
	return nil, tenancy.ErrConflict
}

// GetWorkspaceRole reports the caller's direct workspace membership role.
// It returns ok=false when the workspace exists but the user is not a
// member, and ErrNotFound when the workspace does not exist.
func (s *Store) GetWorkspaceRole(ctx context.Context, workspacePublicID string, userID int64) (string, bool, error) {
	workspace, err := s.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return "", false, err
	}
	row, err := s.dbWithContext(ctx).q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: userID,
	})
	if database.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.Role, true, nil
}

// ListWorkspaceMembers lists direct workspace members ordered by user id
// (parity with the in-memory Store ordering).
func (s *Store) ListWorkspaceMembers(ctx context.Context, workspacePublicID string) ([]*tenancy.WorkspaceMember, error) {
	workspace, err := s.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspaceMembers(ctx, dbsqlc.ListWorkspaceMembersParams{
		WorkspaceID: workspace.InternalID, RowLimit: maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	return s.mapWorkspaceMembers(ctx, workspace.PublicID, rows)
}

func (s *Store) mapWorkspaceMembers(ctx context.Context, publicID string, rows []dbsqlc.WorkspaceMembership) ([]*tenancy.WorkspaceMember, error) {
	userIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	externalIDs, err := userExternalIDs(ctx, s.q, userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]*tenancy.WorkspaceMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapWorkspaceMember(publicID, row, externalIDs))
	}
	sortMembersByUserID(out)
	return out, nil
}

func sortMembersByUserID(members []*tenancy.WorkspaceMember) {
	sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
}

// AddWorkspaceMember grants a membership idempotently: re-adding keeps the
// existing role (parity with the in-memory Store).
//
// First join of the account consumption scope (#947) atomically seeds the
// member quota row from the account default in the same transaction:
// the membership insert, stable payer resolution, default fail-closed
// check, and quota seed commit or roll back together. Re-adding keeps
// the existing role and never copies quota; multi-space joins preserve
// the first row (ON CONFLICT DO NOTHING); rejoining keeps the cap and
// accrued usage. Absent default fails closed with
// ErrDefaultQuotaNotConfigured (never implicit unlimited); the inviter
// cannot raise the default (no quota params on this path).
func (s *Store) AddWorkspaceMember(ctx context.Context, workspacePublicID string, targetUserID int64, role string) (*tenancy.WorkspaceMember, error) {
	role = strings.TrimSpace(role)
	if !tenancy.IsValidRole(role) {
		return nil, tenancy.ErrForbidden
	}
	var added *tenancy.WorkspaceMember
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		workspaceID, err := q.LockWorkspaceMemberships(ctx, strings.TrimSpace(workspacePublicID))
		if err != nil {
			return notFound(err)
		}
		lookup := dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspaceID, UserID: targetUserID}
		row, err := q.GetWorkspaceMembership(ctx, lookup)
		if database.IsNotFound(err) {
			count, countErr := q.CountWorkspaceMembers(ctx, workspaceID)
			if countErr != nil {
				return countErr
			}
			if count >= tenancy.MaxWorkspaceMembers {
				return tenancy.ErrWorkspaceMemberLimitReached
			}
			if _, err := q.InsertWorkspaceMembershipIgnore(ctx, dbsqlc.InsertWorkspaceMembershipIgnoreParams{
				WorkspaceID: workspaceID, UserID: targetUserID, Role: role,
			}); err != nil {
				return err
			}
			row, err = q.GetWorkspaceMembership(ctx, lookup)
			if err != nil {
				return notFound(err)
			}
			if err := seedMemberQuotaFromDefault(ctx, q, workspaceID, targetUserID); err != nil {
				return err
			}
		}
		if err != nil {
			return notFound(err)
		}
		externalIDs, err := userExternalIDs(ctx, q, []int64{targetUserID})
		if err != nil {
			return err
		}
		added = mapWorkspaceMember(workspacePublicID, row, externalIDs)
		return nil
	})
	return added, err
}

// seedMemberQuotaFromDefault seeds one member quota row from the account
// default inside the caller's membership transaction (#947). It resolves
// the stable payer without changing resolution order (explicit link, org
// pool, per-workspace account, provisioning Get-or-Create like reads),
// fails closed with ErrDefaultQuotaNotConfigured when the account has no
// default (never implicit unlimited), and preserves existing rows
// (multi-space joins and rejoins keep the first cap and usage).
func seedMemberQuotaFromDefault(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID, targetUserID int64) error {
	account, _, err := resolveBillingAccountRow(ctx, q, workspaceInternalID)
	if err != nil {
		return err
	}
	if _, err := q.GetDefaultMemberQuota(ctx, account.InternalID); err != nil {
		if database.IsNotFound(err) {
			return tenancy.ErrDefaultQuotaNotConfigured
		}
		return err
	}
	_, err = q.InsertMemberQuotaFromDefault(ctx, dbsqlc.InsertMemberQuotaFromDefaultParams{
		BillingAccountID: account.InternalID, UserID: targetUserID,
		UpdatedByUserID: pgtype.Int8{},
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

// UpdateWorkspaceMemberRole applies the agreed role-transition rules before
// persisting the new role.
func (s *Store) UpdateWorkspaceMemberRole(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, newRole string) (*tenancy.WorkspaceMember, error) {
	var updated *tenancy.WorkspaceMember
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		workspaceID, err := q.LockWorkspaceMemberships(ctx, strings.TrimSpace(workspacePublicID))
		if err != nil {
			return notFound(err)
		}
		target, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
			WorkspaceID: workspaceID, UserID: targetUserID,
		})
		if err != nil {
			return notFound(err)
		}
		actorRole, owners, err := s.mutationContext(ctx, q, workspaceID, actorUserID)
		if err != nil {
			return err
		}
		if err := tenancy.CheckMemberMutation(actorRole, actorUserID, targetUserID, target.Role, newRole, owners, false); err != nil {
			return err
		}
		if _, err := q.UpdateWorkspaceMemberRole(ctx, dbsqlc.UpdateWorkspaceMemberRoleParams{
			WorkspaceID: workspaceID, UserID: targetUserID, Role: strings.TrimSpace(newRole),
		}); err != nil {
			return err
		}
		row, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
			WorkspaceID: workspaceID, UserID: targetUserID,
		})
		if err != nil {
			return notFound(err)
		}
		externalIDs, err := userExternalIDs(ctx, q, []int64{targetUserID})
		if err != nil {
			return err
		}
		updated = mapWorkspaceMember(workspacePublicID, row, externalIDs)
		return nil
	})
	return updated, err
}

// RemoveWorkspaceMember applies the agreed removal rules (last-owner guard,
// self-removal) before deleting the membership row.
func (s *Store) RemoveWorkspaceMember(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, grants tenancy.WorkspaceMemberGrantStore) ([]tenancy.MemberGrantCleanup, error) {
	var cleaned []tenancy.MemberGrantCleanup
	err := dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		workspaceID, err := q.LockWorkspaceMemberships(ctx, strings.TrimSpace(workspacePublicID))
		if err != nil {
			return notFound(err)
		}
		target, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
			WorkspaceID: workspaceID, UserID: targetUserID,
		})
		if err != nil {
			return notFound(err)
		}
		actorRole, owners, err := s.mutationContext(ctx, q, workspaceID, actorUserID)
		if err != nil {
			return err
		}
		if err := tenancy.CheckMemberMutation(actorRole, actorUserID, targetUserID, target.Role, "", owners, true); err != nil {
			return err
		}
		if grants == nil {
			return tenancy.ErrConflict
		}
		cleaned, err = grants.RemoveWorkspaceMemberGrants(dbutil.ContextWithAmbientTx(ctx, tx), workspacePublicID, targetUserID)
		if err != nil {
			return err
		}

		rows, err := q.DeleteWorkspaceMembership(ctx, dbsqlc.DeleteWorkspaceMembershipParams{
			WorkspaceID: workspaceID, UserID: targetUserID,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return tenancy.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cleaned, nil
}

func (s *Store) mutationContext(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID, actorUserID int64) (string, int, error) {
	actor, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspaceInternalID, UserID: actorUserID,
	})
	actorRole := ""
	if err == nil {
		actorRole = actor.Role
	} else if !database.IsNotFound(err) {
		return "", 0, err
	}
	owners, err := q.CountWorkspaceOwners(ctx, workspaceInternalID)
	if err != nil {
		return "", 0, err
	}
	return actorRole, int(owners), nil
}

// CreateOrganization mints an org_ organization and grants the creator the
// owner membership in the same transaction. The stable organization Billing
// Account (#944) is provisioned in the same transaction with the creator as
// billing owner (wallet via trigger), so org creation/first config never
// leaves an org without its funds identity and org workspaces never fall
// back to the creator personal pool.
func (s *Store) CreateOrganization(ctx context.Context, ownerUserID int64, name string) (*tenancy.Organization, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	var created dbsqlc.Organization
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewOrganizationPublicID()
		if err != nil {
			return nil, err
		}
		err = s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
			row, insertErr := q.CreateOrganization(ctx, dbsqlc.CreateOrganizationParams{
				PublicID: publicID, Name: strings.TrimSpace(name),
			})
			if insertErr != nil {
				return insertErr
			}
			created = row
			if err := q.UpsertOrganizationMembership(ctx, dbsqlc.UpsertOrganizationMembershipParams{
				OrganizationID: row.InternalID,
				UserID:         ownerUserID,
				Role:           tenancy.RoleOwner,
			}); err != nil {
				return err
			}
			return s.ensureOrganizationAccountInTx(ctx, q, row.InternalID, ownerUserID)
		})
		if isPublicIDConflict(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return mapOrganization(created), nil
	}
	return nil, tenancy.ErrConflict
}

// GetOrganization resolves an organization by opaque public id.
func (s *Store) GetOrganization(ctx context.Context, publicID string) (*tenancy.Organization, error) {
	row, err := s.dbWithContext(ctx).q.GetOrganizationByPublicID(ctx, strings.TrimSpace(publicID))
	if err != nil {
		return nil, notFound(err)
	}
	return mapOrganization(row), nil
}

// ListOrganizationsForUser lists membership-visible organizations.
func (s *Store) ListOrganizationsForUser(ctx context.Context, userID int64) ([]*tenancy.Organization, error) {
	rows, err := s.dbWithContext(ctx).q.ListOrganizationsByUserID(ctx, dbsqlc.ListOrganizationsByUserIDParams{
		UserID: userID, RowLimit: maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*tenancy.Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapOrganization(row))
	}
	return out, nil
}

// UpdateOrganization renames a live organization; archived or missing rows
// read as not found.
func (s *Store) UpdateOrganization(ctx context.Context, publicID, name string) (*tenancy.Organization, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.UpdateOrganizationName(ctx, dbsqlc.UpdateOrganizationNameParams{
		Name: strings.TrimSpace(name), PublicID: strings.TrimSpace(publicID),
	})
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, tenancy.ErrNotFound
	}
	return s.GetOrganization(ctx, publicID)
}

// GetOrganizationRole reports the caller's direct organization membership
// role, ok=false when the organization exists but the user is not a member.
func (s *Store) GetOrganizationRole(ctx context.Context, organizationPublicID string, userID int64) (string, bool, error) {
	organization, err := s.GetOrganization(ctx, organizationPublicID)
	if err != nil {
		return "", false, err
	}
	row, err := s.dbWithContext(ctx).q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
		OrganizationID: organization.InternalID, UserID: userID,
	})
	if database.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.Role, true, nil
}

// ListOrganizationWorkspaces lists workspaces owned by an organization.
func (s *Store) ListOrganizationWorkspaces(ctx context.Context, organizationPublicID string) ([]*tenancy.Workspace, error) {
	organization, err := s.GetOrganization(ctx, organizationPublicID)
	if err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspacesByOrganizationID(ctx, dbsqlc.ListWorkspacesByOrganizationIDParams{
		OrganizationID: pgtype.Int8{Int64: organization.InternalID, Valid: true},
		RowLimit:       maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*tenancy.Workspace, 0, len(rows))
	for _, row := range rows {
		orgPublicID := organization.PublicID
		out = append(out, mapWorkspace(row, &orgPublicID))
	}
	return out, nil
}

// CreateOrganizationWorkspace mints a ws_ workspace under an organization.
// The creator gains an owner membership row.
func (s *Store) CreateOrganizationWorkspace(ctx context.Context, organizationPublicID string, ownerUserID int64, name string) (*tenancy.Workspace, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	organization, err := s.GetOrganization(ctx, organizationPublicID)
	if err != nil {
		return nil, err
	}
	params := tenancy.CreateWorkspaceParams{
		Name:           name,
		OrganizationID: &organization.PublicID,
		OwnerUserID:    ownerUserID,
	}
	return s.CreateWorkspace(ctx, params)
}
