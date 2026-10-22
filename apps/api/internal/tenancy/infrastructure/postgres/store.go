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
	*OrganizationStore
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
	return &Store{OrganizationStore: NewOrganizationStore(db), db: db, q: dbsqlc.New(db), objects: objects}
}

func (s *Store) dbWithContext(ctx context.Context) repositoryQueryer {
	return newRepositoryQueryer(ctx, s.db, s.q)
}

// Bind reads as well as writes to the caller-owned transaction. Lifecycle
// adapters share this constructor so composed repositories see staged state.
func newRepositoryQueryer(ctx context.Context, pool *pgxpool.Pool, q *dbsqlc.Queries) repositoryQueryer {
	if ambient, ok := dbutil.AmbientTxFromContext(ctx); ok {
		q = dbsqlc.New(ambient)
	}
	return repositoryQueryer{ctx: ctx, db: pool, q: q}
}

func (db repositoryQueryer) withTx(run func(*dbsqlc.Queries) error) error {
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
	orgPublicID, err := s.resolveOrgPublicID(ctx, s.dbWithContext(ctx).q, row.OrganizationID)
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

// ListWorkspacesForUser lists effective-visible workspaces (ADR-0038
// #951): direct membership rows plus automatic org Owner/Admin access to
// every workspace of their organization. Personal scopes use direct rows
// only; org Members see only explicitly granted workspaces.
func (s *Store) ListWorkspacesForUser(ctx context.Context, userID int64) ([]*tenancy.Workspace, error) {
	rows, err := s.dbWithContext(ctx).q.ListWorkspacesByUserID(ctx, dbsqlc.ListWorkspacesByUserIDParams{
		UserID: userID, RowLimit: maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	mapped, err := s.mapWorkspaceRows(ctx, rows)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(mapped))
	for _, w := range mapped {
		seen[w.PublicID] = struct{}{}
	}
	orgs, err := s.dbWithContext(ctx).q.ListOrganizationsByUserID(ctx, dbsqlc.ListOrganizationsByUserIDParams{
		UserID: userID, RowLimit: maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	for _, org := range orgs {
		roleRow, err := s.dbWithContext(ctx).q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
			OrganizationID: org.InternalID, UserID: userID,
		})
		if err != nil {
			continue
		}
		if roleRow.Role != tenancy.RoleOwner && roleRow.Role != tenancy.RoleAdmin {
			continue
		}
		orgWorkspaces, err := s.dbWithContext(ctx).q.ListWorkspacesByOrganizationID(ctx, dbsqlc.ListWorkspacesByOrganizationIDParams{
			OrganizationID: pgtype.Int8{Int64: org.InternalID, Valid: true},
			RowLimit:       maxTenancyRows,
		})
		if err != nil {
			return nil, err
		}
		orgPublicID := org.PublicID
		for _, row := range orgWorkspaces {
			if _, ok := seen[row.PublicID]; ok {
				continue
			}
			seen[row.PublicID] = struct{}{}
			mapped = append(mapped, mapWorkspace(row, &orgPublicID))
		}
	}
	sort.Slice(mapped, func(i, j int) bool {
		if mapped[i].CreatedAt.Equal(mapped[j].CreatedAt) {
			return mapped[i].PublicID < mapped[j].PublicID
		}
		return mapped[i].CreatedAt.Before(mapped[j].CreatedAt)
	})
	// Closed Organizations suspend Workspace listing (#955): the
	// Organization itself stays visible for restore, its Workspaces do
	// not. Cardinality caps the distinct-org lookups at one; the org rows
	// above already carry closed_at, so the common path needs no extra
	// fetch (misses fall back to GetOrganization fail-closed).
	closedByPublic := make(map[string]bool, len(orgs))
	for _, org := range orgs {
		closedByPublic[org.PublicID] = org.ClosedAt.Valid
	}
	visible := mapped[:0]
	for _, w := range mapped {
		if w.OrganizationID == nil {
			visible = append(visible, w)
			continue
		}
		closed, known := closedByPublic[*w.OrganizationID]
		if !known {
			organization, err := s.GetOrganization(ctx, *w.OrganizationID)
			if err != nil {
				return nil, err
			}
			closed = organization.IsClosed()
			closedByPublic[*w.OrganizationID] = closed
		}
		if !closed {
			visible = append(visible, w)
		}
	}
	return visible, nil
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
		row, err := s.dbWithContext(ctx).q.GetOrganizationByInternalID(ctx, internalID)
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
// Billing scope (ADR-0038) is the creation scope, fixed permanently:
// organization workspaces share the organization's pool (ensured in the same
// transaction, never the creator personal pool); personal workspaces resolve
// to the owner's deterministic Personal account at read time (no
// per-workspace mint, no link row). Creating an Organization never moves
// existing Personal workspaces.
func (s *Store) CreateWorkspace(ctx context.Context, params tenancy.CreateWorkspaceParams) (*tenancy.Workspace, error) {
	if err := tenancy.ValidateName(params.Name); err != nil {
		return nil, err
	}
	if params.OrganizationID != nil {
		if params.IsDefault {
			return nil, tenancy.ErrDefaultWorkspaceConflict
		}
		return s.CreateOrganizationWorkspace(ctx, *params.OrganizationID, params.OwnerUserID, params.Name)
	}
	if params.OwnerUserID <= 0 {
		return nil, tenancy.ErrWorkspaceOwnerRequired
	}
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewWorkspacePublicID()
		if err != nil {
			return nil, err
		}
		var created dbsqlc.Workspace
		err = s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
			var err error
			created, err = createWorkspaceInTx(ctx, q, publicID, pgtype.Int8{}, params)
			return err
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
		return mapWorkspace(created, nil), nil
	}
	return nil, tenancy.ErrConflict
}

// createWorkspaceInTx is the shared persistence primitive for Personal and
// Organization creation. The caller owns validation, authorization, scope
// locking, and the transaction; insert, membership, and funds identity stay atomic.
func createWorkspaceInTx(ctx context.Context, q *dbsqlc.Queries, publicID string, orgInternal pgtype.Int8, params tenancy.CreateWorkspaceParams) (dbsqlc.Workspace, error) {
	ownerCol := nullInt64(params.OwnerUserID)
	if orgInternal.Valid {
		ownerCol = pgtype.Int8{}
	}
	row, err := q.CreateWorkspace(ctx, dbsqlc.CreateWorkspaceParams{
		PublicID:       publicID,
		OrganizationID: orgInternal,
		OwnerUserID:    ownerCol,
		Name:           strings.TrimSpace(params.Name),
		IsDefault:      params.IsDefault && !orgInternal.Valid,
	})
	if err != nil {
		return dbsqlc.Workspace{}, err
	}
	if params.OwnerUserID > 0 {
		if err := q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{
			WorkspaceID: row.InternalID, UserID: params.OwnerUserID, Role: tenancy.RoleOwner,
		}); err != nil {
			return dbsqlc.Workspace{}, err
		}
	}
	if err := createBillingInTx(ctx, q, row, orgInternal, params); err != nil {
		return dbsqlc.Workspace{}, err
	}
	return row, nil
}

// createBillingInTx provisions the creation-time funds identity inside the
// same workspace transaction (ADR-0038). Org workspaces ensure the shared
// org pool (no per-workspace account, no link row); Default rows mint
// their own deterministic personal account (the owner's Personal
// survivor); other personal workspaces need no billing write (they resolve
// to the owner's Personal survivor at read time).
func createBillingInTx(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.Workspace, orgInternal pgtype.Int8, params tenancy.CreateWorkspaceParams) error {
	if orgInternal.Valid {
		return ensureOrganizationAccountInTx(ctx, q, orgInternal.Int64, 0)
	}
	// Personal scope: only the Default row owns a billing write (the
	// Personal survivor anchor). Other personal workspaces resolve to it.
	if params.IsDefault {
		return ensureWorkspaceAccountWithQueries(ctx, q, row.InternalID, params.OwnerUserID)
	}
	return nil
}

// ensureWorkspaceAccountWithQueries Get-or-Creates the per-workspace
// account for a freshly minted workspace plus the creator billing-owner
// grant. The wallet row follows via the account-insert trigger. Only the
// Default workspace row owns such an anchor (the Personal survivor);
// business workspaces never call this.
func ensureWorkspaceAccountWithQueries(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID, ownerUserID int64) error {
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
// submissions/invites serialize here). It resolves the creation-scope
// payer without provisioning (deletion must never mint a wallet as a side
// effect): the Organization pool for org scopes, the owner's Personal
// survivor for personal scopes. Unsettled payment orders and live pending
// handover invites converge on the stored payer first. Lapsed (expired)
// invites never block: they move nothing and the owner may re-issue after
// the scope is gone. Wallets, Ledger, snapshots, and operation rows are
// never named here (red lines, retained by construction).
func guardWorkspaceDeleteFunds(ctx context.Context, tx pgx.Tx, workspaceID int64, lockedAccountIDs []int64) error {
	var payerID *int64
	var orgID, ownerID pgtype.Int8
	if err := tx.QueryRow(ctx, `SELECT organization_id, owner_user_id FROM workspaces WHERE internal_id = $1`, workspaceID).Scan(&orgID, &ownerID); err != nil {
		return fmt.Errorf("tenancy: workspace delete: resolve scope: %w", err)
	}
	if orgID.Valid && orgID.Int64 > 0 {
		var orgAccount int64
		if err := tx.QueryRow(ctx, `SELECT internal_id FROM billing_accounts WHERE organization_id = $1 LIMIT 1`, orgID.Int64).Scan(&orgAccount); err == nil {
			payerID = &orgAccount
		} else if !database.IsNotFound(err) {
			return fmt.Errorf("tenancy: workspace delete: resolve org account: %w", err)
		}
	} else if ownerID.Valid && ownerID.Int64 > 0 {
		// Personal survivor without provisioning: the Default-anchored
		// account when it exists, otherwise no payer and no guard.
		var survivor int64
		if err := tx.QueryRow(ctx, `SELECT ba.internal_id FROM billing_accounts AS ba
JOIN workspaces AS def ON def.internal_id = ba.workspace_id
WHERE def.owner_user_id = $1 AND def.organization_id IS NULL AND def.is_default LIMIT 1`, ownerID.Int64).Scan(&survivor); err == nil {
			payerID = &survivor
		} else if !database.IsNotFound(err) {
			return fmt.Errorf("tenancy: workspace delete: resolve personal survivor: %w", err)
		}
	}
	if payerID == nil {
		return nil
	}
	// The scope is fixed at creation, so the payer cannot change while
	// deletion waits for the Workspace lock. A payer outside the
	// pre-locked set still fails closed instead of deleting shared money.
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
  SELECT 1 FROM billing_account_transfer_invites WHERE billing_account_id = ANY($1::bigint[]) AND status = 'pending' AND expires_at > now()
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

// ensureDefaultWorkspaceWithQueries idempotently provisions the user's
// Default workspace plus its owner membership through the caller's
// queries (INSERT ... ON CONFLICT DO NOTHING plus read-back, so
// concurrent callers converge to a single Default row). The Personal
// survivor account follows via ensureWorkspaceAccountWithQueries at the
// call sites that need funds identity.
func ensureDefaultWorkspaceWithQueries(ctx context.Context, q *dbsqlc.Queries, userID int64) (dbsqlc.Workspace, error) {
	if userID <= 0 {
		return dbsqlc.Workspace{}, tenancy.ErrPersonalWorkspaceUserRequired
	}
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewWorkspacePublicID()
		if err != nil {
			return dbsqlc.Workspace{}, err
		}
		if _, insertErr := q.InsertDefaultWorkspaceIgnore(ctx, dbsqlc.InsertDefaultWorkspaceIgnoreParams{
			PublicID:    publicID,
			OwnerUserID: pgtype.Int8{Int64: userID, Valid: true},
			Name:        "My Workspace",
		}); insertErr != nil {
			if isPublicIDConflict(insertErr) {
				continue
			}
			return dbsqlc.Workspace{}, insertErr
		}
		selected, selectErr := q.GetDefaultWorkspaceByOwner(ctx, pgtype.Int8{Int64: userID, Valid: true})
		if selectErr != nil {
			return dbsqlc.Workspace{}, selectErr
		}
		if err := q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{
			WorkspaceID: selected.InternalID,
			UserID:      userID,
			Role:        tenancy.RoleOwner,
		}); err != nil {
			return dbsqlc.Workspace{}, err
		}
		return selected, nil
	}
	return dbsqlc.Workspace{}, tenancy.ErrConflict
}

// EnsureDefaultWorkspace idempotently provisions the user's Default
// workspace plus its deterministic Personal Billing Account (the survivor
// anchor, ADR-0038). Concurrent
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
	var row dbsqlc.Workspace
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		selected, err := ensureDefaultWorkspaceWithQueries(ctx, q, userID)
		if err != nil {
			return err
		}
		row = selected
		return ensureWorkspaceAccountWithQueries(ctx, q, row.InternalID, userID)
	})
	if err != nil {
		return nil, notFound(err)
	}
	return mapWorkspace(row, nil), nil
}

// GetWorkspaceRole reports the caller's effective workspace role
// (ADR-0038 #951): personal scopes use the direct row; organization
// scopes grant org Owner/Admin their org role automatically, org Members
// use only their explicit row, and non-org-members hold nothing
// (join-first, even with a stale direct row).
func (s *Store) GetWorkspaceRole(ctx context.Context, workspacePublicID string, userID int64) (string, bool, error) {
	workspace, err := s.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return "", false, err
	}
	// Closed Organizations suspend all Workspace content access (#955):
	// every member, including Owner/Admin, reads as non-member while
	// closed. Organization management (restore) stays on the org
	// endpoints via GetOrganizationRole, which this gate never touches.
	if !workspace.IsPersonal() && workspace.OrganizationID != nil {
		organization, err := s.GetOrganization(ctx, *workspace.OrganizationID)
		if err != nil {
			if err == tenancy.ErrNotFound {
				return "", false, nil
			}
			return "", false, err
		}
		if organization.IsClosed() {
			return "", false, nil
		}
	}
	directRole, directMember := "", false
	if row, err := s.dbWithContext(ctx).q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: userID,
	}); err == nil {
		directRole, directMember = row.Role, true
	} else if !database.IsNotFound(err) {
		return "", false, err
	}
	if workspace.IsPersonal() {
		return directRole, directMember, nil
	}
	orgRole, orgMember := "", false
	if workspace.OrganizationID != nil {
		if role, member, err := s.GetOrganizationRole(ctx, *workspace.OrganizationID, userID); err == nil {
			orgRole, orgMember = role, member
		} else if err != tenancy.ErrNotFound {
			return "", false, err
		}
	}
	role, ok := tenancy.EffectiveWorkspaceRole(workspace, orgRole, orgMember, directRole, directMember)
	return role, ok, nil
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
	externalIDs, err := userExternalIDs(ctx, s.dbWithContext(ctx).q, userIDs)
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

// rejectClosedOrgWorkspaceInTx fails workspace membership writes closed
// while the owning Organization is closed (#955). Personal scopes always
// pass; missing organization rows read as not found.
func rejectClosedOrgWorkspaceInTx(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID int64) error {
	workspaceRow, err := q.GetWorkspaceByInternalID(ctx, workspaceInternalID)
	if err != nil {
		return notFound(err)
	}
	if !workspaceRow.OrganizationID.Valid {
		return nil
	}
	organization, err := q.GetOrganizationByInternalID(ctx, workspaceRow.OrganizationID.Int64)
	if err != nil {
		return notFound(err)
	}
	if organization.ClosedAt.Valid {
		return tenancy.ErrOrganizationClosed
	}
	return nil
}

// AddWorkspaceMember grants a membership idempotently: re-adding keeps the
// existing role (parity with the in-memory Store).
//
// Personal collaboration (#952, ADR-0038): the Personal Default workspace
// is private and rejects invites with ErrDefaultWorkspaceProtected (fail
// closed, 403 workspace_default_protected). Non-Default Personal
// workspaces invite freely: Workspace Owner/Admin add and remove
// collaborators without per-invite account-holder approval, and no quota
// row is seeded here. Absent quota reads as unlimited everywhere
// (memberquotas.IsUnlimited); the Personal account holder sets per-member
// caps later via SetQuota without clearing accrued usage. Multi-space
// joins and rejoins preserve the existing quota row and immutable
// consumption history (quota rows are never written or deleted on this
// path; the old #947 fail-closed default gate is cut, not kept).
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
		workspaceRow, err := q.GetWorkspaceByInternalID(ctx, workspaceID)
		if err != nil {
			return notFound(err)
		}
		if err := rejectClosedOrgWorkspaceInTx(ctx, q, workspaceID); err != nil {
			return err
		}
		// Personal Default is private (#952): fail closed before any
		// membership or quota write.
		if !workspaceRow.OrganizationID.Valid && workspaceRow.IsDefault {
			return tenancy.ErrDefaultWorkspaceProtected
		}
		if workspaceRow.OrganizationID.Valid {
			if _, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
				OrganizationID: workspaceRow.OrganizationID.Int64, UserID: targetUserID,
			}); err != nil {
				return notFound(err)
			}
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

// UpdateWorkspaceMemberRole applies the agreed role-transition rules before
// persisting the new role.
func (s *Store) UpdateWorkspaceMemberRole(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, newRole string) (*tenancy.WorkspaceMember, error) {
	var updated *tenancy.WorkspaceMember
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		workspaceID, err := q.LockWorkspaceMemberships(ctx, strings.TrimSpace(workspacePublicID))
		if err != nil {
			return notFound(err)
		}
		if err := rejectClosedOrgWorkspaceInTx(ctx, q, workspaceID); err != nil {
			return err
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
		if err := rejectClosedOrgWorkspaceInTx(ctx, q, workspaceID); err != nil {
			return err
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
