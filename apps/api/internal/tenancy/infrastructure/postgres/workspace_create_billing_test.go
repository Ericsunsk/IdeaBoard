package postgres

import (
	"context"
	"sync"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgtype"
)

// Focused coverage for ADR-0038 creation-time funds identities: new-user
// idempotent provision (Default workspace + personal account, concurrent),
// Personal business workspaces sharing the owner's account (no new wallet),
// and org workspaces sharing the organization pool (never Personal).
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

func TestWorkspaceCreatePersonalSharesSurvivorNoWallet(t *testing.T) {
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

	// Two business spaces share the Personal survivor by creation scope
	// alone: no billing option exists, no new wallet, no link row.
	shared := make([]*tenancy.Workspace, 2)
	for i := range shared {
		ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
			Name: "shared-biz", OwnerUserID: owner,
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
		// No per-workspace account row was minted for the sharing spaces.
		var rows int64
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_accounts WHERE workspace_id = $1`, ws.InternalID).Scan(&rows); err != nil {
			t.Fatalf("count studio account failed: %v", err)
		}
		if rows != 0 {
			t.Fatalf("shared workspace minted %d accounts, want 0 (survivor only)", rows)
		}
		var links int64
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspace_billing_links WHERE workspace_id = $1`, ws.InternalID).Scan(&links); err != nil {
			t.Fatalf("count links failed: %v", err)
		}
		if links != 0 {
			t.Fatalf("shared workspace wrote %d links, want 0", links)
		}
	}
	var walletsAfter int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM credit_wallets`).Scan(&walletsAfter); err != nil {
		t.Fatalf("count wallets failed: %v", err)
	}
	if walletsAfter != walletsBefore {
		t.Fatalf("wallets %d -> %d, want no new wallet for shared scope", walletsBefore, walletsAfter)
	}
}

func TestWorkspaceCreateHasNoIndependentAccount(t *testing.T) {
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
	// The independent-account option is removed (ADR-0038): a second
	// business workspace resolves to the same Personal survivor.
	second, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name: "second-biz", OwnerUserID: owner,
	})
	if err != nil {
		t.Fatalf("create second failed: %v", err)
	}
	resolved, err := baStore.EnsureBillingAccountForWorkspace(ctx, second.InternalID, second.PublicID)
	if err != nil {
		t.Fatalf("resolve second failed: %v", err)
	}
	if resolved.PublicID != personal.PublicID || resolved.InternalID != personal.InternalID {
		t.Fatalf("second scope = %+v, want survivor %+v", resolved, personal)
	}
	role, ok, err := baStore.GetBillingAccountRole(ctx, resolved.InternalID, owner)
	if err != nil || !ok || role != tenancy.BillingRoleOwner {
		t.Fatalf("owner role on survivor = %q ok=%v err=%v, want owner", role, ok, err)
	}
	// No link row for the second space (scope resolution, not linkage).
	var links int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspace_billing_links WHERE workspace_id = $1`, second.InternalID).Scan(&links); err != nil {
		t.Fatalf("count links failed: %v", err)
	}
	if links != 0 {
		t.Fatalf("second links = %d, want 0", links)
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

func TestWorkspaceCreateScopeFixedAtCreation(t *testing.T) {
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
	var before int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces`).Scan(&before); err != nil {
		t.Fatalf("count workspaces failed: %v", err)
	}
	// Personal creation carries no billing choice: it resolves to the
	// survivor and writes no link or account row.
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name: "scoped", OwnerUserID: owner,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	resolved, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil || resolved.PublicID != personal.PublicID {
		t.Fatalf("scoped resolve = %+v %v, want survivor %q", resolved, err, personal.PublicID)
	}
	var after int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces`).Scan(&after); err != nil {
		t.Fatalf("count workspaces failed: %v", err)
	}
	if after != before+1 {
		t.Fatalf("workspaces %d -> %d, want exactly one new row", before, after)
	}
}

func TestWorkspaceCreateOrgScopeFixed(t *testing.T) {
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
	// Creating the Organization never moves existing Personal workspaces:
	// they keep resolving to the Personal survivor while org workspaces
	// use the org pool.
	orgWs, err := store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "org-space")
	if err != nil {
		t.Fatalf("create org workspace failed: %v", err)
	}
	orgAcc, err := baStore.EnsureBillingAccountForWorkspace(ctx, orgWs.InternalID, orgWs.PublicID)
	if err != nil {
		t.Fatalf("resolve org workspace failed: %v", err)
	}
	if orgAcc.PublicID == personal.PublicID {
		t.Fatalf("org scope must not reuse personal pool: %q", orgAcc.PublicID)
	}
	still, err := baStore.EnsureBillingAccountForWorkspace(ctx, def.InternalID, def.PublicID)
	if err != nil || still.PublicID != personal.PublicID {
		t.Fatalf("personal survivor moved after org create: %+v %v", still, err)
	}
	var after int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces`).Scan(&after); err != nil {
		t.Fatalf("count workspaces failed: %v", err)
	}
	if after != before+1 {
		t.Fatalf("workspaces %d -> %d, want exactly one new row", before, after)
	}
}
