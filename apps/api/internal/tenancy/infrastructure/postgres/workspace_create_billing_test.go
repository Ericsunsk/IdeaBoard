package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgtype"
)

// Focused coverage for #944 creation-time explicit accounts: new-user
// idempotent provision (Default workspace + personal account, concurrent),
// personal default shared linkage (no new wallet), explicit independent
// accounts (isolated, same owner), org workspaces on the org pool (never
// personal), and fail-closed authorization with no half-linked workspaces.
func TestWorkspaceCreateDefaultProvisionIsIdempotent(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	userID := createTenancyTestUser(t, db)

	const callers = 8
	results := make([]*tenancy.Workspace, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			ws, err := store.EnsureDefaultWorkspace(ctx, userID)
			results[slot], errs[slot] = ws, err
		}(i)
	}
	wg.Wait()
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("ensure caller %d failed: %v", i, errs[i])
		}
	}
	for i := 1; i < callers; i++ {
		if results[i].PublicID != results[0].PublicID {
			t.Fatalf("concurrent default diverged: %q vs %q", results[i].PublicID, results[0].PublicID)
		}
	}
	// Single personal account across all callers (no duplicate wallets).
	first, err := baStore.EnsureBillingAccountForWorkspace(ctx, results[0].InternalID, results[0].PublicID)
	if err != nil {
		t.Fatalf("resolve personal failed: %v", err)
	}
	for i := 1; i < callers; i++ {
		again, err := baStore.EnsureBillingAccountForWorkspace(ctx, results[i].InternalID, results[i].PublicID)
		if err != nil || again.PublicID != first.PublicID || again.InternalID != first.InternalID {
			t.Fatalf("personal account diverged: %+v vs %+v (%v)", again, first, err)
		}
	}
	var accountRows int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_accounts WHERE workspace_id = $1`, results[0].InternalID).Scan(&accountRows); err != nil {
		t.Fatalf("count personal accounts failed: %v", err)
	}
	if accountRows != 1 {
		t.Fatalf("personal billing_accounts rows = %d, want 1", accountRows)
	}
}

func TestWorkspaceCreateSharedPersonalLinkCreatesNoWallet(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)

	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	personal, err := baStore.EnsureBillingAccountForWorkspace(ctx, def.InternalID, def.PublicID)
	if err != nil {
		t.Fatalf("resolve personal failed: %v", err)
	}
	var walletsBefore int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM credit_wallets`).Scan(&walletsBefore); err != nil {
		t.Fatalf("count wallets failed: %v", err)
	}

	// Two business spaces explicitly share the personal pool in one tx each.
	personalPublic := personal.PublicID
	shared := make([]*tenancy.Workspace, 2)
	for i := range shared {
		ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
			Name:                   "shared-biz",
			OwnerUserID:            owner,
			BillingAccountPublicID: &personalPublic,
		})
		if err != nil {
			t.Fatalf("create shared %d failed: %v", i, err)
		}
		shared[i] = ws
	}
	for i, ws := range shared {
		resolved, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
		if err != nil || resolved.PublicID != personal.PublicID {
			t.Fatalf("shared %d resolved = %+v %v, want %q", i, resolved, err, personal.PublicID)
		}
		if !resolved.Shared {
			t.Fatalf("shared %d must report shared=true: %+v", i, resolved)
		}
		// No per-workspace account row was minted for the sharing spaces.
		var rows int64
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_accounts WHERE workspace_id = $1`, ws.InternalID).Scan(&rows); err != nil {
			t.Fatalf("count studio account failed: %v", err)
		}
		if rows != 0 {
			t.Fatalf("shared workspace minted %d accounts, want 0 (link only)", rows)
		}
	}
	var walletsAfter int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM credit_wallets`).Scan(&walletsAfter); err != nil {
		t.Fatalf("count wallets failed: %v", err)
	}
	if walletsAfter != walletsBefore {
		t.Fatalf("wallets %d -> %d, want no new wallet for shared links", walletsBefore, walletsAfter)
	}
}

func TestWorkspaceCreateIndependentAccountStaysIsolated(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)

	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	personal, err := baStore.EnsureBillingAccountForWorkspace(ctx, def.InternalID, def.PublicID)
	if err != nil {
		t.Fatalf("resolve personal failed: %v", err)
	}
	independent, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name: "independent-biz", OwnerUserID: owner, NewBillingAccount: true,
	})
	if err != nil {
		t.Fatalf("create independent failed: %v", err)
	}
	resolved, err := baStore.EnsureBillingAccountForWorkspace(ctx, independent.InternalID, independent.PublicID)
	if err != nil {
		t.Fatalf("resolve independent failed: %v", err)
	}
	if resolved.PublicID == personal.PublicID {
		t.Fatal("independent account must not reuse the personal pool")
	}
	if resolved.Shared {
		t.Fatalf("independent account must report shared=false: %+v", resolved)
	}
	// Same owner manages both without merging: grants on both identities.
	for _, accountID := range []int64{personal.InternalID, resolved.InternalID} {
		role, ok, err := baStore.GetBillingAccountRole(ctx, accountID, owner)
		if err != nil || !ok || role != tenancy.BillingRoleOwner {
			t.Fatalf("owner role on %d = %q ok=%v err=%v, want owner", accountID, role, ok, err)
		}
	}
	// No link row for the independent space (its own per-workspace row).
	var links int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspace_billing_links WHERE workspace_id = $1`, independent.InternalID).Scan(&links); err != nil {
		t.Fatalf("count links failed: %v", err)
	}
	if links != 0 {
		t.Fatalf("independent links = %d, want 0", links)
	}
}

func TestWorkspaceCreateOrganizationUsesOrgPool(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)

	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	personal, err := baStore.EnsureBillingAccountForWorkspace(ctx, def.InternalID, def.PublicID)
	if err != nil {
		t.Fatalf("resolve personal failed: %v", err)
	}
	org, err := store.CreateOrganization(ctx, owner, "Acme Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	// Org creation eagerly provisions the pool (no lazy read needed).
	q := dbsqlc.New(db)
	orgKey := pgtype.Int8{Int64: org.InternalID, Valid: true}
	if _, err := q.GetBillingAccountByOrganizationID(ctx, orgKey); err != nil {
		t.Fatalf("org pool missing after create: %v", err)
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
	if accA.PublicID != accB.PublicID || accA.PublicID == personal.PublicID {
		t.Fatalf("org workspaces must share the org pool, not personal: A=%q B=%q personal=%q", accA.PublicID, accB.PublicID, personal.PublicID)
	}
	if accA.OwnerType != BillingOwnerOrganization {
		t.Fatalf("org account owner_type = %q, want organization", accA.OwnerType)
	}
}

func TestWorkspaceCreateUnauthorizedLeavesNoWorkspace(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)
	outsider := createTenancyTestUser(t, db)

	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	personal, err := baStore.EnsureBillingAccountForWorkspace(ctx, def.InternalID, def.PublicID)
	if err != nil {
		t.Fatalf("resolve personal failed: %v", err)
	}
	var before int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces`).Scan(&before); err != nil {
		t.Fatalf("count workspaces failed: %v", err)
	}
	personalPublic := personal.PublicID
	_, err = store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:                   "hijack",
		OwnerUserID:            outsider,
		BillingAccountPublicID: &personalPublic,
	})
	if !errors.Is(err, tenancy.ErrBillingForbidden) {
		t.Fatalf("unauthorized link err = %v, want ErrBillingForbidden", err)
	}
	// Unknown ba_ target fails closed without a workspace.
	unknown := "ba_missing_000001"
	_, err = store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:                   "ghost",
		OwnerUserID:            outsider,
		BillingAccountPublicID: &unknown,
	})
	if !errors.Is(err, tenancy.ErrBillingNotFound) {
		t.Fatalf("unknown target err = %v, want ErrBillingNotFound", err)
	}
	var after int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces`).Scan(&after); err != nil {
		t.Fatalf("count workspaces failed: %v", err)
	}
	if after != before {
		t.Fatalf("workspaces %d -> %d, want rollback with no half workspace", before, after)
	}
}

func TestWorkspaceCreateOrgBillingOverrideFailsClosed(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)

	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	personal, err := baStore.EnsureBillingAccountForWorkspace(ctx, def.InternalID, def.PublicID)
	if err != nil {
		t.Fatalf("resolve personal failed: %v", err)
	}
	org, err := store.CreateOrganization(ctx, owner, "Acme")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	var before int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces`).Scan(&before); err != nil {
		t.Fatalf("count workspaces failed: %v", err)
	}
	personalPublic := personal.PublicID
	orgPublic := org.PublicID
	// Personal pool must never back an org workspace.
	if _, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name: "org-hijack", OrganizationID: &orgPublic, OwnerUserID: owner,
		BillingAccountPublicID: &personalPublic,
	}); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("org+personal err = %v, want ErrBillingConflict", err)
	}
	if _, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name: "org-new", OrganizationID: &orgPublic, OwnerUserID: owner, NewBillingAccount: true,
	}); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("org+new err = %v, want ErrBillingConflict", err)
	}
	var after int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces`).Scan(&after); err != nil {
		t.Fatalf("count workspaces failed: %v", err)
	}
	if after != before {
		t.Fatalf("workspaces %d -> %d, want no half workspace on org conflict", before, after)
	}
}
