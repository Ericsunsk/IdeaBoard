package funds

import (
	"context"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mintAttempts bounds public_id regeneration after a random collision.
const mintAttempts = 5

// Owner types for the funds identity projection. Values stay byte-identical
// to the tenancy postgres mapping ("workspace" for the Personal
// account anchor, "organization" for the shared org pool).
const (
	OwnerTypeWorkspace    = "workspace"
	OwnerTypeOrganization = "organization"
)

// Identity is the stable funds identity for one workspace consumption scope.
// BillingAccountID is the internal join handle (database only, never on the
// wire); BillingAccountPublicID is the stable ba_ funds identity the API
// addresses. Workspace records the consumption location. OwnerType reports
// the org-pool fact (organization) versus the Personal account (workspace).
type Identity struct {
	WorkspaceInternalID    int64
	WorkspacePublicID      string
	BillingAccountID       int64
	BillingAccountPublicID string
	OwnerType              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// BA returns the wire-stable ba_ funds identity. Handlers render this and
// treat BillingAccountID as an opaque join handle.
func (i Identity) BA() string { return i.BillingAccountPublicID }

// ToBillingAccount projects the deep identity onto the agreed tenancy shape
// for downstream joins. The wire field stays PublicID (ba_); InternalID is
// the opaque join handle. Callers must not branch on the mapping that
// produced it.
func (i Identity) ToBillingAccount() tenancy.BillingAccount {
	return tenancy.BillingAccount{
		InternalID:          i.BillingAccountID,
		PublicID:            i.BillingAccountPublicID,
		OwnerType:           i.OwnerType,
		WorkspaceInternalID: i.WorkspaceInternalID,
		WorkspacePublicID:   i.WorkspacePublicID,
		CreatedAt:           i.CreatedAt,
		UpdatedAt:           i.UpdatedAt,
	}
}

// Resolver resolves the stable funds identity. Production uses Store; tests
// stub this interface. Resolution joins the caller transaction and fails
// closed; it never falls back to another wallet.
type Resolver interface {
	ResolveFunds(ctx context.Context, workspacePublicID string) (Identity, error)
	ResolveFundsForActor(ctx context.Context, workspacePublicID string, actorUserID int64) (Identity, error)
}

// Store provisions and resolves the stable funds identity over pool.
// Pass nil pool in tests that stub the interface instead. Reads and
// Get-or-Create writes join the caller transaction via dbutil borrow-or-begin.
type Store struct {
	db *pgxpool.Pool
}

// NewStore builds a funds resolver over pool. A nil pool still borrows an
// ambient carrier when one is staged (no pool connection is needed); without
// a carrier it fails closed with the historical "dbutil: database is nil".
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Compile-time convergence with the agreed seam.
var _ Resolver = (*Store)(nil)

// withBorrow runs fn in one borrow-or-begin unit: it joins the ambient
// carrier when one is staged, else opens its own commit-or-rollback unit
// so first-touch provisioning is durable. Callers never manage the
// lifecycle; fn receives only the query set.
func withBorrow(ctx context.Context, db *pgxpool.Pool, fn func(queries *dbsqlc.Queries) (Identity, error)) (Identity, error) {
	scope, err := dbutil.Begin(ctx, db, pgx.TxOptions{})
	if err != nil {
		return Identity{}, err
	}
	defer func() {
		_ = scope.Rollback(ctx)
	}()
	identity, err := fn(scope.Queries())
	if err != nil {
		return Identity{}, err
	}
	if err := scope.Commit(ctx); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

// ResolveFunds resolves workspace (ws_) to ba_ in one borrow-or-begin unit.
// When ctx carries an ambient carrier it joins it (no new transaction, no
// commit here); otherwise it begins, provisions, and commits its own unit so
// first-touch provisioning is durable. Missing or ownerless scopes fail
// closed; a zero account never returns success.
func (s *Store) ResolveFunds(ctx context.Context, workspacePublicID string) (Identity, error) {
	return withBorrow(ctx, poolOf(s), func(queries *dbsqlc.Queries) (Identity, error) {
		return ResolveByPublicIDWithQueries(ctx, queries, workspacePublicID)
	})
}

// ResolveFundsForActor resolves workspace (ws_) to ba_ for one viewer in
// one borrow-or-begin unit (payer = actor, #973/#978): the Organization
// pool for org workspaces (actor-independent), else the viewer's own
// deterministic Personal account (provisioned on demand, no fallback to
// the workspace owner). Ownerless personal scopes, unknown workspaces,
// and a non-positive actor all fail closed. Billing overview, balance,
// usage, and billing-records reads use this seam so Personal members
// each see their own (BillingAccount, User) column with zero cross
// pollution; Org reads keep the shared pool.
func (s *Store) ResolveFundsForActor(ctx context.Context, workspacePublicID string, actorUserID int64) (Identity, error) {
	return withBorrow(ctx, poolOf(s), func(queries *dbsqlc.Queries) (Identity, error) {
		return ResolveByPublicIDForActorWithQueries(ctx, queries, workspacePublicID, actorUserID)
	})
}

func poolOf(s *Store) *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.db
}

// loadWorkspaceRowByPublicID maps a ws_ public id to its workspace row:
// the single trim/validate/lookup seam behind both ByPublicID orders.
// Unknown or malformed ids fail closed as ErrBillingWorkspaceInvalid.
func loadWorkspaceRowByPublicID(ctx context.Context, queries *dbsqlc.Queries, workspacePublicID string) (dbsqlc.Workspace, error) {
	trimmed := strings.TrimSpace(workspacePublicID)
	if queries == nil || !contract.IsWorkspaceID(trimmed) {
		return dbsqlc.Workspace{}, tenancy.ErrBillingWorkspaceInvalid
	}
	row, err := queries.GetWorkspaceByPublicID(ctx, trimmed)
	if err != nil {
		if database.IsNotFound(err) {
			return dbsqlc.Workspace{}, tenancy.ErrBillingWorkspaceInvalid
		}
		return dbsqlc.Workspace{}, err
	}
	return row, nil
}

// ResolveByPublicIDWithQueries is the single workspace (ws_) to ba_ seam
// over an existing query set (caller transaction or pool). It maps the public
// id to its internal id, then applies the fixed creation-scope order.
// Closed-organization gating stays with the callers (tasking/vision
// settlement, HTTP visibility); this seam never grants visibility, only the
// payer.
func ResolveByPublicIDWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspacePublicID string) (Identity, error) {
	row, err := loadWorkspaceRowByPublicID(ctx, queries, workspacePublicID)
	if err != nil {
		return Identity{}, err
	}
	return ResolveByInternalIDWithQueries(ctx, queries, row.InternalID, row.PublicID)
}

// ResolveAccountRowByInternalIDWithQueries is the row-level fixed-order seam
// for tenancy postgres adapters. Identity projection stays in
// ResolveByInternalIDWithQueries. There is exactly one order implementation
// (resolveBillingAccountRowForScope with useActor=false).
func ResolveAccountRowByInternalIDWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.BillingAccount, error) {
	return resolveBillingAccountRowForScope(ctx, queries, workspaceInternalID, 0, false)
}

// ResolveByInternalIDWithQueries applies the same fixed order to an already
// resolved workspace internal id. Tenancy postgres delegates here so there
// is exactly one order implementation. workspacePublicID rides the projection
// only (pass "" to leave it empty); the order never reads it.
//
// This is the owner-based order for non-settlement readers. Settlement
// uses ResolveByInternalIDForActorWithQueries so Personal scopes bill the
// actor (payer = actor, #973/#974) instead of the workspace owner.
func ResolveByInternalIDWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64, workspacePublicID string) (Identity, error) {
	row, err := resolveBillingAccountRowForScope(ctx, queries, workspaceInternalID, 0, false)
	if err != nil {
		return Identity{}, err
	}
	if row.InternalID <= 0 || strings.TrimSpace(row.PublicID) == "" {
		// Creation path without a funds identity fails closed here instead
		// of leaving a row that later reads as unfunded.
		return Identity{}, tenancy.ErrBillingNotFound
	}
	return mapIdentity(row, workspaceInternalID, workspacePublicID), nil
}

// ResolveByPublicIDForActorWithQueries is the settlement payer seam
// (payer = actor, #973/#974): the Organization pool for org workspaces
// (actor-independent, same as the owner-based order), else the actor's
// own deterministic Personal account (provisioned on demand, no fallback
// to the workspace owner or any other wallet). Ownerless personal scopes,
// unknown workspaces, and a non-positive actor all fail closed.
func ResolveByPublicIDForActorWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspacePublicID string, actorUserID int64) (Identity, error) {
	row, err := loadWorkspaceRowByPublicID(ctx, queries, workspacePublicID)
	if err != nil {
		return Identity{}, err
	}
	return ResolveByInternalIDForActorWithQueries(ctx, queries, row.InternalID, row.PublicID, actorUserID)
}

// ResolveByInternalIDForActorWithQueries applies the settlement payer
// order to an already resolved workspace internal id. workspacePublicID
// rides the projection only (pass "" to leave it empty); the order never
// reads it.
func ResolveByInternalIDForActorWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64, workspacePublicID string, actorUserID int64) (Identity, error) {
	row, err := resolveBillingAccountRowForScope(ctx, queries, workspaceInternalID, actorUserID, true)
	if err != nil {
		return Identity{}, err
	}
	if row.InternalID <= 0 || strings.TrimSpace(row.PublicID) == "" {
		// Creation path without a funds identity fails closed here instead
		// of leaving a row that later reads as unfunded.
		return Identity{}, tenancy.ErrBillingNotFound
	}
	return mapIdentity(row, workspaceInternalID, workspacePublicID), nil
}

// resolveBillingAccountRowForScope implements the fixed creation-scope
// order behind both reader orders (ADR-0038 owner order, #973/#974
// settlement payer order): the Organization pool for org workspaces
// (actor-independent), else a deterministic Personal account provisioned on
// demand. useActor=false reads the workspace owner's account (ownerless
// personal scopes fail closed with ErrBillingConflict); useActor=true reads
// the actor's own account (a non-positive actor fails closed with
// ErrBillingWorkspaceInvalid, never falling back to the owner). There is no
// link indirection and no per-workspace mint. Byte-identical to the retired
// postgres helper.
func resolveBillingAccountRowForScope(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID, actorUserID int64, useActor bool) (dbsqlc.BillingAccount, error) {
	if queries == nil || workspaceInternalID <= 0 {
		return dbsqlc.BillingAccount{}, tenancy.ErrBillingWorkspaceInvalid
	}
	ws, err := queries.GetWorkspaceByInternalID(ctx, workspaceInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return dbsqlc.BillingAccount{}, tenancy.ErrBillingWorkspaceInvalid
		}
		return dbsqlc.BillingAccount{}, err
	}
	if ws.OrganizationID.Valid {
		return resolveOrganizationAccount(ctx, queries, ws.OrganizationID.Int64)
	}
	if useActor {
		if actorUserID <= 0 {
			return dbsqlc.BillingAccount{}, tenancy.ErrBillingWorkspaceInvalid
		}
		return resolvePersonalAccount(ctx, queries, actorUserID)
	}
	if !ws.OwnerUserID.Valid || ws.OwnerUserID.Int64 <= 0 {
		return dbsqlc.BillingAccount{}, tenancy.ErrBillingConflict
	}
	return resolvePersonalAccount(ctx, queries, ws.OwnerUserID.Int64)
}

// EnsureOrganizationBillingAccountInTx is the single internal-only
// Get-or-Create seam for the shared Organization pool. Existing accounts
// are read without row locks: settlement may already hold the Workspace
// lock, whereas lifecycle mutations lock the account before the Workspace.
// Financial mutations take their own explicit account lock. This helper
// mints the stub row once on first use and converges mint races on the
// duplicate-key read-back. CreateBillingAccountStub remains the sole
// billing_accounts insert primitive: all Organization scopes resolve
// through this helper, and the Personal Default anchor path is the only
// other stub caller. ownerUserID > 0 grants that user billing ownership
// (creation path); 0 falls back to the discoverable org Owner via
// ensureBillingOwnerForOrg (resolution path).
func EnsureOrganizationBillingAccountInTx(ctx context.Context, queries *dbsqlc.Queries, organizationInternalID, ownerUserID int64) (dbsqlc.BillingAccount, error) {
	if organizationInternalID <= 0 {
		return dbsqlc.BillingAccount{}, tenancy.ErrBillingWorkspaceInvalid
	}
	orgKey := pgtype.Int8{Int64: organizationInternalID, Valid: true}
	if row, err := queries.GetBillingAccountByOrganizationID(ctx, orgKey); err == nil {
		return row, nil
	} else if !database.IsNotFound(err) {
		return dbsqlc.BillingAccount{}, err
	}
	publicID, err := tenancy.NewBillingAccountPublicID()
	if err != nil {
		return dbsqlc.BillingAccount{}, err
	}
	created, err := queries.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		PublicID: publicID, WorkspaceID: pgtype.Int8{},
		OrganizationID: orgKey,
	})
	if err != nil {
		if database.IsDuplicateConstraintError(err) {
			if row, readErr := queries.GetBillingAccountByOrganizationID(ctx, orgKey); readErr == nil {
				grantOrganizationBillingOwner(ctx, queries, row.InternalID, organizationInternalID, ownerUserID)
				return row, nil
			}
		}
		return dbsqlc.BillingAccount{}, err
	}
	grantOrganizationBillingOwner(ctx, queries, created.InternalID, organizationInternalID, ownerUserID)
	return created, nil
}

// grantOrganizationBillingOwner seeds the explicit billing owner when known,
// else the discoverable org Owner. Empty pools stay manageable once a member
// is granted; existing grants are never overwritten here.
func grantOrganizationBillingOwner(ctx context.Context, queries *dbsqlc.Queries, billingAccountID, organizationInternalID, ownerUserID int64) {
	if ownerUserID > 0 {
		_, _ = queries.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: billingAccountID, UserID: ownerUserID, Role: tenancy.BillingRoleOwner,
		})
		return
	}
	ensureBillingOwnerForOrg(ctx, queries, billingAccountID, organizationInternalID)
}

// resolveOrganizationAccount returns the shared org pool, provisioning once.
func resolveOrganizationAccount(ctx context.Context, queries *dbsqlc.Queries, organizationInternalID int64) (dbsqlc.BillingAccount, error) {
	row, err := EnsureOrganizationBillingAccountInTx(ctx, queries, organizationInternalID, 0)
	if err != nil {
		return dbsqlc.BillingAccount{}, err
	}
	return row, nil
}

// resolvePersonalAccount returns the deterministic Personal account for
// one user (their Default-workspace-anchored account), provisioning the
// Default workspace, its membership, the account, and the owner grant on
// demand through the caller's queries (no SQL-function dependency, so it
// joins settlement transactions and works wherever the tenancy tables
// exist). Callers pass the workspace owner for the owner-based order and
// the submitter for the settlement payer order (payer = actor, #973/#974).
// Personal workspaces never mint their own account and never read
// link rows: every Personal scope of one user shares this pool.
func resolvePersonalAccount(ctx context.Context, queries *dbsqlc.Queries, userID int64) (dbsqlc.BillingAccount, error) {
	if userID <= 0 {
		return dbsqlc.BillingAccount{}, tenancy.ErrBillingWorkspaceInvalid
	}
	def, err := EnsureDefaultWorkspaceWithQueries(ctx, queries, userID)
	if err != nil {
		return dbsqlc.BillingAccount{}, err
	}
	if err := EnsureWorkspaceAccountWithQueries(ctx, queries, def.InternalID, userID); err != nil {
		return dbsqlc.BillingAccount{}, err
	}
	row, err := queries.GetBillingAccountByWorkspaceID(ctx, pgtype.Int8{Int64: def.InternalID, Valid: true})
	if err != nil {
		if database.IsNotFound(err) {
			return dbsqlc.BillingAccount{}, tenancy.ErrBillingNotFound
		}
		return dbsqlc.BillingAccount{}, err
	}
	return row, nil
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

// EnsureDefaultWorkspaceWithQueries idempotently provisions the user's
// Default workspace plus its owner membership through the caller's
// queries (INSERT ... ON CONFLICT DO NOTHING plus read-back, so
// concurrent callers converge to a single Default row). The Personal
// account follows via EnsureWorkspaceAccountWithQueries at the
// call sites that need funds identity.
func EnsureDefaultWorkspaceWithQueries(ctx context.Context, q *dbsqlc.Queries, userID int64) (dbsqlc.Workspace, error) {
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

// EnsureWorkspaceAccountWithQueries Get-or-Creates the per-workspace
// account for a freshly minted workspace plus the creator billing-owner
// grant. The wallet row follows via the account-insert trigger. Only the
// Default workspace row owns such an anchor (the Personal account anchor);
// business workspaces never call this. Internal-only: this is the Personal
// counterpart to EnsureOrganizationBillingAccountInTx; both resolve through
// CreateBillingAccountStub, the sole billing_accounts insert primitive.
func EnsureWorkspaceAccountWithQueries(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID, ownerUserID int64) error {
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

func isPublicIDConflict(err error) bool {
	return database.IsConstraintViolation(err, "uq_workspaces_public_id") ||
		database.IsConstraintViolation(err, "uq_organizations_public_id")
}

func mapIdentity(row dbsqlc.BillingAccount, workspaceInternalID int64, workspacePublicID string) Identity {
	ownerType := OwnerTypeWorkspace
	if row.OrganizationID.Valid {
		ownerType = OwnerTypeOrganization
	}
	return Identity{
		WorkspaceInternalID:    workspaceInternalID,
		WorkspacePublicID:      workspacePublicID,
		BillingAccountID:       row.InternalID,
		BillingAccountPublicID: row.PublicID,
		OwnerType:              ownerType,
		CreatedAt:              dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:              dbutil.TimeValue(row.UpdatedAt),
	}
}
