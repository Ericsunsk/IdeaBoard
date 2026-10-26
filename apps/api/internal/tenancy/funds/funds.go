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
// to the tenancy postgres ownerTypeOf mapping ("workspace" for the Personal
// survivor anchor, "organization" for the shared org pool).
const (
	OwnerTypeWorkspace    = "workspace"
	OwnerTypeOrganization = "organization"
)

// Identity is the stable funds identity for one workspace consumption scope.
// BillingAccountID is the internal join handle (database only, never on the
// wire); BillingAccountPublicID is the stable ba_ funds identity the API
// addresses. Workspace records the consumption location. Shared reports the
// org-pool fact (true) versus the Personal survivor (false).
type Identity struct {
	WorkspaceInternalID    int64
	WorkspacePublicID      string
	BillingAccountID       int64
	BillingAccountPublicID string
	Shared                 bool
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
		Shared:              i.Shared,
		CreatedAt:           i.CreatedAt,
		UpdatedAt:           i.UpdatedAt,
	}
}

// Resolver resolves the stable funds identity. Production uses Store; tests
// stub this interface. Resolution joins the caller transaction and fails
// closed; it never falls back to another wallet.
type Resolver interface {
	ResolveFunds(ctx context.Context, workspacePublicID string) (Identity, error)
	ResolveFundsInTx(ctx context.Context, tx pgx.Tx, workspacePublicID string) (Identity, error)
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

// ResolveFunds resolves workspace (ws_) to ba_ in one borrow-or-begin unit.
// When ctx carries an ambient carrier it joins it (no new transaction, no
// commit here); otherwise it begins, provisions, and commits its own unit so
// first-touch provisioning is durable. Missing or ownerless scopes fail
// closed; a zero account never returns success.
func (s *Store) ResolveFunds(ctx context.Context, workspacePublicID string) (Identity, error) {
	scope, err := dbutil.Begin(ctx, poolOf(s), pgx.TxOptions{})
	if err != nil {
		return Identity{}, err
	}
	defer func() {
		_ = scope.Rollback(ctx)
	}()
	identity, err := ResolveByPublicIDWithQueries(ctx, scope.Queries(), workspacePublicID)
	if err != nil {
		return Identity{}, err
	}
	if err := scope.Commit(ctx); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

// ResolveFundsInTx resolves workspace (ws_) to ba_ inside the caller's
// transaction. It runs statements only and never commits or rolls back tx;
// the caller owns the lifecycle. Task/Vision settlement uses this form so
// Quote/Hold/Task/River stay atomic with the funds-identity provisioning.
func (s *Store) ResolveFundsInTx(ctx context.Context, tx pgx.Tx, workspacePublicID string) (Identity, error) {
	if tx == nil {
		return Identity{}, tenancy.ErrBillingWorkspaceInvalid
	}
	return ResolveByPublicIDWithQueries(ctx, dbsqlc.New(tx), workspacePublicID)
}

func poolOf(s *Store) *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.db
}

// ResolveByPublicIDWithQueries is the single workspace (ws_) to ba_ seam
// over an existing query set (caller transaction or pool). It maps the public
// id to its internal id, then applies the fixed creation-scope order.
// Closed-organization gating stays with the callers (tasking/vision
// settlement, HTTP visibility); this seam never grants visibility, only the
// payer.
func ResolveByPublicIDWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspacePublicID string) (Identity, error) {
	trimmed := strings.TrimSpace(workspacePublicID)
	if queries == nil || !contract.IsWorkspaceID(trimmed) {
		return Identity{}, tenancy.ErrBillingWorkspaceInvalid
	}
	row, err := queries.GetWorkspaceByPublicID(ctx, trimmed)
	if err != nil {
		if database.IsNotFound(err) {
			return Identity{}, tenancy.ErrBillingWorkspaceInvalid
		}
		return Identity{}, err
	}
	return ResolveByInternalIDWithQueries(ctx, queries, row.InternalID, row.PublicID)
}

// ResolveAccountRowByInternalIDWithQueries is the row-level fixed-order seam
// for tenancy postgres adapters. It returns the stable dbsqlc row plus the
// shared fact; Identity projection stays in ResolveByInternalIDWithQueries.
// There is exactly one order implementation (this one).
func ResolveAccountRowByInternalIDWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.BillingAccount, bool, error) {
	return resolveBillingAccountRow(ctx, queries, workspaceInternalID)
}

// ResolveByInternalIDWithQueries applies the same fixed order to an already
// resolved workspace internal id. Tenancy postgres delegates here so there
// is exactly one order implementation. workspacePublicID rides the projection
// only (pass "" to leave it empty); the order never reads it.
func ResolveByInternalIDWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64, workspacePublicID string) (Identity, error) {
	row, shared, err := resolveBillingAccountRow(ctx, queries, workspaceInternalID)
	if err != nil {
		return Identity{}, err
	}
	if row.InternalID <= 0 || strings.TrimSpace(row.PublicID) == "" {
		// Creation path without a funds identity fails closed here instead
		// of leaving a row that later reads as unfunded.
		return Identity{}, tenancy.ErrBillingNotFound
	}
	return mapIdentity(row, workspaceInternalID, workspacePublicID, shared), nil
}

// resolveBillingAccountRow implements the fixed creation-scope order
// (ADR-0038): the Organization pool for org workspaces, else the owner's
// deterministic Personal survivor for personal workspaces (provisioned on
// demand, so first reads never fork per-workspace accounts). Ownerless
// personal scopes and missing rows fail closed; there is no link indirection
// and no per-workspace mint. Byte-identical to the retired postgres helper.
func resolveBillingAccountRow(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.BillingAccount, bool, error) {
	if queries == nil || workspaceInternalID <= 0 {
		return dbsqlc.BillingAccount{}, false, tenancy.ErrBillingWorkspaceInvalid
	}
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
	if !ws.OwnerUserID.Valid || ws.OwnerUserID.Int64 <= 0 {
		return dbsqlc.BillingAccount{}, false, tenancy.ErrBillingConflict
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
func resolveOrganizationAccount(ctx context.Context, queries *dbsqlc.Queries, organizationInternalID int64) (dbsqlc.BillingAccount, bool, error) {
	row, err := EnsureOrganizationBillingAccountInTx(ctx, queries, organizationInternalID, 0)
	if err != nil {
		return dbsqlc.BillingAccount{}, false, err
	}
	return row, true, nil
}

// resolvePersonalAccount returns the owner's deterministic Personal
// survivor (the Default-workspace-anchored account), provisioning the
// Default workspace, its membership, the account, and the owner grant on
// demand through the caller's queries (no SQL-function dependency, so it
// joins settlement transactions and works wherever the tenancy tables
// exist). Personal workspaces never mint their own account and never read
// link rows: every Personal scope of one user shares this pool.
func resolvePersonalAccount(ctx context.Context, queries *dbsqlc.Queries, ownerUserID int64) (dbsqlc.BillingAccount, bool, error) {
	if ownerUserID <= 0 {
		return dbsqlc.BillingAccount{}, false, tenancy.ErrBillingWorkspaceInvalid
	}
	def, err := EnsureDefaultWorkspaceWithQueries(ctx, queries, ownerUserID)
	if err != nil {
		return dbsqlc.BillingAccount{}, false, err
	}
	if err := EnsureWorkspaceAccountWithQueries(ctx, queries, def.InternalID, ownerUserID); err != nil {
		return dbsqlc.BillingAccount{}, false, err
	}
	row, err := queries.GetBillingAccountByWorkspaceID(ctx, pgtype.Int8{Int64: def.InternalID, Valid: true})
	if err != nil {
		if database.IsNotFound(err) {
			return dbsqlc.BillingAccount{}, false, tenancy.ErrBillingNotFound
		}
		return dbsqlc.BillingAccount{}, false, err
	}
	return row, false, nil
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
// survivor account follows via EnsureWorkspaceAccountWithQueries at the
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
// Default workspace row owns such an anchor (the Personal survivor);
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
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "uq_workspaces_public_id") ||
		strings.Contains(lowered, "uq_organizations_public_id")
}

func ownerTypeOf(row dbsqlc.BillingAccount) string {
	if row.OrganizationID.Valid {
		return OwnerTypeOrganization
	}
	return OwnerTypeWorkspace
}

func mapIdentity(row dbsqlc.BillingAccount, workspaceInternalID int64, workspacePublicID string, shared bool) Identity {
	return Identity{
		WorkspaceInternalID:    workspaceInternalID,
		WorkspacePublicID:      workspacePublicID,
		BillingAccountID:       row.InternalID,
		BillingAccountPublicID: row.PublicID,
		Shared:                 shared,
		OwnerType:              ownerTypeOf(row),
		CreatedAt:              dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:              dbutil.TimeValue(row.UpdatedAt),
	}
}
