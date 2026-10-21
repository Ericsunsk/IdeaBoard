package postgres

import (
	"context"
	"errors"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Focused coverage for #915 resolution: no-org sharing, multi-org isolation,
// same-user multi-account management, non-org admin + handover, tx-joinable
// resolution, and fail-closed missing/ambiguous/conflict paths.
func TestBillingResolutionSharesOrgPool(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)

	org, err := store.CreateOrganization(ctx, owner, "Acme")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	wsA, err := store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "Design")
	if err != nil {
		t.Fatalf("create org workspace A failed: %v", err)
	}
	wsB, err := store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "Marketing")
	if err != nil {
		t.Fatalf("create org workspace B failed: %v", err)
	}
	accA, err := baStore.EnsureBillingAccountForWorkspace(ctx, wsA.InternalID, wsA.PublicID)
	if err != nil {
		t.Fatalf("resolve A failed: %v", err)
	}
	accB, err := baStore.EnsureBillingAccountForWorkspace(ctx, wsB.InternalID, wsB.PublicID)
	if err != nil {
		t.Fatalf("resolve B failed: %v", err)
	}
	// Same org pool: identical stable payer, different resource scopes.
	if accA.PublicID == "" || accA.PublicID != accB.PublicID {
		t.Fatalf("org workspaces share pool: A=%q B=%q, want identical ba_", accA.PublicID, accB.PublicID)
	}
	if accA.InternalID != accB.InternalID {
		t.Fatalf("org internal ids differ: %d vs %d", accA.InternalID, accB.InternalID)
	}
	if wsA.InternalID == wsB.InternalID {
		t.Fatal("workspaces must stay distinct resource scopes")
	}
	if accA.OwnerType != BillingOwnerOrganization || !accA.Shared {
		t.Fatalf("org account view = %+v, want owner_type=organization shared=true", accA)
	}
	// Second org stays isolated.
	owner2 := createTenancyTestUser(t, db)
	org2, err := store.CreateOrganization(ctx, owner2, "Other")
	if err != nil {
		t.Fatalf("create org2 failed: %v", err)
	}
	wsC, err := store.CreateOrganizationWorkspace(ctx, org2.PublicID, owner2, "Design")
	if err != nil {
		t.Fatalf("create org2 workspace failed: %v", err)
	}
	accC, err := baStore.EnsureBillingAccountForWorkspace(ctx, wsC.InternalID, wsC.PublicID)
	if err != nil {
		t.Fatalf("resolve C failed: %v", err)
	}
	if accC.PublicID == accA.PublicID {
		t.Fatalf("multi-org isolation violated: org2 account %q == org1 %q", accC.PublicID, accA.PublicID)
	}
}

func TestBillingResolutionNoOrgExplicitSharing(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)

	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	studio, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "Brand", OwnerUserID: owner})
	if err != nil {
		t.Fatalf("create studio workspace failed: %v", err)
	}
	defAcc, err := baStore.EnsureBillingAccountForWorkspace(ctx, def.InternalID, def.PublicID)
	if err != nil {
		t.Fatalf("resolve default failed: %v", err)
	}
	// Explicitly associate the second business space with the studio pool.
	// The actor is both workspace owner and billing owner (seeded at provision).
	if err := baStore.LinkWorkspaceToBillingAccount(ctx, studio.InternalID, defAcc.InternalID, owner); err != nil {
		t.Fatalf("explicit link failed: %v", err)
	}
	linked, err := baStore.EnsureBillingAccountForWorkspace(ctx, studio.InternalID, studio.PublicID)
	if err != nil {
		t.Fatalf("resolve linked failed: %v", err)
	}
	if linked.PublicID != defAcc.PublicID || linked.InternalID != defAcc.InternalID {
		t.Fatalf("no-org sharing: linked=%+v want %+v", linked, defAcc)
	}
	if !linked.Shared {
		t.Fatal("linked resolution must report shared=true")
	}
	// Independent account stays isolated.
	other := createTenancyTestUser(t, db)
	otherWS, err := store.EnsureDefaultWorkspace(ctx, other)
	if err != nil {
		t.Fatalf("ensure other default failed: %v", err)
	}
	otherAcc, err := baStore.EnsureBillingAccountForWorkspace(ctx, otherWS.InternalID, otherWS.PublicID)
	if err != nil {
		t.Fatalf("resolve other failed: %v", err)
	}
	if otherAcc.PublicID == defAcc.PublicID {
		t.Fatal("independent accounts must stay isolated")
	}
	// Same user manages independent accounts without merging balances:
	// the owner holds manager grants on both distinct ba_ identities.
	if _, err := baStore.AddBillingAccountMember(ctx, otherAcc.InternalID, other, owner, tenancy.BillingRoleAdmin); err != nil {
		// other is the billing owner of otherAcc (seeded); adding owner as
		// admin must succeed and keep the two accounts distinct.
		t.Fatalf("same user second account grant failed: %v", err)
	}
	again, err := baStore.EnsureBillingAccountForWorkspace(ctx, studio.InternalID, studio.PublicID)
	if err != nil || again.PublicID != defAcc.PublicID {
		t.Fatalf("sharing stable after second grant: %+v %v", again, err)
	}
}

func TestBillingNonOrgAdminAndHandover(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)
	admin := createTenancyTestUser(t, db)

	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	acc, err := baStore.EnsureBillingAccountForWorkspace(ctx, def.InternalID, def.PublicID)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	// Non-org account gains an authorized admin (not lifetime-bound).
	added, err := baStore.AddBillingAccountMember(ctx, acc.InternalID, owner, admin, tenancy.BillingRoleAdmin)
	if err != nil || added.Role != tenancy.BillingRoleAdmin {
		t.Fatalf("add admin = %+v %v, want admin/nil", added, err)
	}
	// Admin manages (list succeeds); space-only standing would fail closed
	// at the HTTP layer (covered in v1 tests), here the store gate is the
	// billing role itself.
	if _, err := baStore.ListBillingAccountMembers(ctx, acc.InternalID, admin); err != nil {
		t.Fatalf("admin list failed: %v", err)
	}
	outsider := createTenancyTestUser(t, db)
	if _, err := baStore.ListBillingAccountMembers(ctx, acc.InternalID, outsider); !errors.Is(err, tenancy.ErrBillingForbidden) {
		t.Fatalf("outsider list err = %v, want ErrBillingForbidden", err)
	}
	// Handover: ownership moves, identity/history stay stable.
	if err := baStore.TransferBillingAccountOwnership(ctx, acc.InternalID, owner, admin); err != nil {
		t.Fatalf("transfer failed: %v", err)
	}
	role, ok, err := baStore.GetBillingAccountRole(ctx, acc.InternalID, admin)
	if err != nil || !ok || role != tenancy.BillingRoleOwner {
		t.Fatalf("new owner role = %q ok=%v err=%v, want owner/true/nil", role, ok, err)
	}
	role, ok, err = baStore.GetBillingAccountRole(ctx, acc.InternalID, owner)
	if err != nil || !ok || role != tenancy.BillingRoleAdmin {
		t.Fatalf("previous owner role = %q ok=%v err=%v, want admin/true/nil", role, ok, err)
	}
	// Previous owner (now admin) cannot transfer again.
	if err := baStore.TransferBillingAccountOwnership(ctx, acc.InternalID, owner, outsider); !errors.Is(err, tenancy.ErrBillingForbidden) {
		t.Fatalf("ex-owner transfer err = %v, want ErrBillingForbidden", err)
	}
}

func TestBillingResolutionJoinsCallerTx(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	owner := createTenancyTestUser(t, db)
	ws, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbsqlc.New(tx)
	resolved, err := EnsureBillingAccountForWorkspaceWithQueries(ctx, q, ws.InternalID)
	if err != nil {
		t.Fatalf("tx resolution failed: %v", err)
	}
	if resolved.PublicID == "" || resolved.InternalID <= 0 {
		t.Fatalf("tx resolved = %+v, want stable payer identity", resolved)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	// Committed provisioning is visible outside the tx (same stable payer).
	baStore := NewBillingAccountStore(db)
	again, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil || again.PublicID != resolved.PublicID {
		t.Fatalf("post-tx resolve = %+v %v, want %q", again, err, resolved.PublicID)
	}
}

func TestBillingResolutionFailClosed(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)

	// Missing workspace fails closed (never silently picks another wallet).
	if _, err := baStore.EnsureBillingAccountForWorkspace(ctx, 0, "ws_missing"); !errors.Is(err, tenancy.ErrBillingWorkspaceInvalid) {
		t.Fatalf("zero workspace err = %v, want ErrBillingWorkspaceInvalid", err)
	}
	if _, err := EnsureBillingAccountForWorkspaceWithQueries(ctx, dbsqlc.New(db), -1); !errors.Is(err, tenancy.ErrBillingWorkspaceInvalid) {
		t.Fatalf("negative workspace err = %v, want ErrBillingWorkspaceInvalid", err)
	}

	// Conflicting ownership: org workspace cannot link to a personal pool.
	org, err := store.CreateOrganization(ctx, owner, "Acme")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	orgWS, err := store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "Design")
	if err != nil {
		t.Fatalf("create org workspace failed: %v", err)
	}
	personal, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "Solo", OwnerUserID: owner})
	if err != nil {
		t.Fatalf("create personal failed: %v", err)
	}
	personalAcc, err := baStore.EnsureBillingAccountForWorkspace(ctx, personal.InternalID, personal.PublicID)
	if err != nil {
		t.Fatalf("resolve personal failed: %v", err)
	}
	if err := baStore.LinkWorkspaceToBillingAccount(ctx, orgWS.InternalID, personalAcc.InternalID, owner); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("org->personal link err = %v, want ErrBillingConflict", err)
	}

	// Conflicting ownership: personal workspace cannot link to an org pool.
	orgAcc, err := baStore.EnsureBillingAccountForWorkspace(ctx, orgWS.InternalID, orgWS.PublicID)
	if err != nil {
		t.Fatalf("resolve org failed: %v", err)
	}
	if err := baStore.LinkWorkspaceToBillingAccount(ctx, personal.InternalID, orgAcc.InternalID, owner); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("personal->org link err = %v, want ErrBillingConflict", err)
	}

	// Payer switches are out of scope: relinking to a different personal
	// pool fails closed instead of silently moving funds.
	second, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "Second", OwnerUserID: owner})
	if err != nil {
		t.Fatalf("create second failed: %v", err)
	}
	if err := baStore.LinkWorkspaceToBillingAccount(ctx, second.InternalID, personalAcc.InternalID, owner); err != nil {
		t.Fatalf("first link failed: %v", err)
	}
	third, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "Third", OwnerUserID: owner})
	if err != nil {
		t.Fatalf("create third failed: %v", err)
	}
	thirdAcc, err := baStore.EnsureBillingAccountForWorkspace(ctx, third.InternalID, third.PublicID)
	if err != nil {
		t.Fatalf("resolve third failed: %v", err)
	}
	if err := baStore.LinkWorkspaceToBillingAccount(ctx, second.InternalID, thirdAcc.InternalID, owner); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("payer switch err = %v, want ErrBillingConflict", err)
	}

	// Unknown ba_ target fails closed without leaking workspace existence.
	if err := baStore.LinkWorkspaceToBillingAccount(ctx, second.InternalID, 999999999, owner); !errors.Is(err, tenancy.ErrBillingNotFound) {
		t.Fatalf("unknown target err = %v, want ErrBillingNotFound", err)
	}
}

func TestBillingMemberRoleChanges(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)
	admin := createTenancyTestUser(t, db)
	other := createTenancyTestUser(t, db)

	ws, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	acc, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if _, err := baStore.AddBillingAccountMember(ctx, acc.InternalID, owner, admin, tenancy.BillingRoleAdmin); err != nil {
		t.Fatalf("add admin failed: %v", err)
	}
	// Admin cannot promote to owner (owner-only promotion).
	if _, err := baStore.AddBillingAccountMember(ctx, acc.InternalID, admin, other, tenancy.BillingRoleOwner); !errors.Is(err, tenancy.ErrBillingForbidden) {
		t.Fatalf("admin grant owner err = %v, want ErrBillingForbidden", err)
	}
	// Owner promotes admin's peer, then demotes back (role changes stick).
	if _, err := baStore.AddBillingAccountMember(ctx, acc.InternalID, owner, other, tenancy.BillingRoleAdmin); err != nil {
		t.Fatalf("owner grant admin failed: %v", err)
	}
	updated, err := baStore.UpdateBillingAccountMemberRole(ctx, acc.InternalID, owner, other, tenancy.BillingRoleOwner)
	if err != nil || updated.Role != tenancy.BillingRoleOwner {
		t.Fatalf("promote to owner = %+v %v", updated, err)
	}
	// Last-owner guard: demoting the only remaining owner fails closed.
	// First hand the account fully to other (owner->admin), then try to
	// demote other while it is the last owner.
	if err := baStore.TransferBillingAccountOwnership(ctx, acc.InternalID, owner, other); err != nil {
		t.Fatalf("transfer to other failed: %v", err)
	}
	if _, err := baStore.UpdateBillingAccountMemberRole(ctx, acc.InternalID, other, other, tenancy.BillingRoleAdmin); !errors.Is(err, tenancy.ErrBillingLastOwner) {
		t.Fatalf("last owner demote err = %v, want ErrBillingLastOwner", err)
	}
	_ = pgtype.Int8{}
	_ = pgx.Tx(nil)
	_ = database.IsNotFound
}
