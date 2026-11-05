package workspacetasks

// Member cross-workspace quota settlement tests (#946).
//
// The quota check joins the submit settlement transaction after Quote
// and before Hold on Organization scopes only (#973/#974: Personal scopes
// bill the actor with no quota concept, so the settlement skips the
// check there): over-quota submits fail closed
// with ErrMemberQuotaExhausted (403 member_quota_exhausted, distinct from
// 402 insufficiency and 403 wallet
// restriction) and leave no task row, no Hold, and no queue entry.
// Terminal Fail
// releases converge usage down so a blocked member can submit after
// recovery.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/tasking/settlement"
)

func openQuotaAtomicEnv(t *testing.T) (*atomicEnv, *memberquotas.Store) {
	t.Helper()
	env := openQuotaOrgEnv(t)
	quotas := memberquotas.NewStore(env.pool)
	env.serviceDeps.Quotas = quotas
	env.service = NewService(env.serviceDeps)
	return env, quotas
}

// openQuotaOrgEnv re-scopes the atomic fixture onto an Organization
// workspace so the quota tests keep proving the unchanged Organization
// path (#974: Personal scopes skip the member quota check, Organization
// scopes keep enforcing it). The submitter (env.userID) joins as org
// admin: admins keep submit access without the Organization-Owner
// unlimited short-circuit, so the cap actually gates.
func openQuotaOrgEnv(t *testing.T) *atomicEnv {
	t.Helper()
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	ownerStorageUID := "T" + strings.ToUpper(randomAtomicHex(t, 3))[:5]
	var orgOwner int64
	if err := env.pool.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('quota-org-owner', $1, 'user', 1) RETURNING id`,
		ownerStorageUID,
	).Scan(&orgOwner); err != nil {
		t.Fatalf("seed quota org owner: %v", err)
	}
	orgPublicID := "org_quota" + randomAtomicHex(t, 4)
	var orgID int64
	if err := env.pool.QueryRow(ctx,
		`INSERT INTO organizations (public_id, name) VALUES ($1, 'quota-org') RETURNING internal_id`,
		orgPublicID,
	).Scan(&orgID); err != nil {
		t.Fatalf("seed quota organization: %v", err)
	}
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'owner'), ($1, $3, 'admin')`,
		orgID, orgOwner, env.userID); err != nil {
		t.Fatalf("seed quota org memberships: %v", err)
	}
	orgAccountPublicID := "ba_orgquota" + randomAtomicHex(t, 4)
	var orgAccount int64
	if err := env.pool.QueryRow(ctx,
		`INSERT INTO billing_accounts (public_id, workspace_id, organization_id) VALUES ($1, NULL, $2) RETURNING internal_id`,
		orgAccountPublicID, orgID,
	).Scan(&orgAccount); err != nil {
		t.Fatalf("seed quota org billing account: %v", err)
	}
	orgWS := "ws_orgquota" + randomAtomicHex(t, 4)
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO workspaces (public_id, organization_id, owner_user_id, name) VALUES ($1, $2, NULL, 'quota-org-ws')`,
		orgWS, orgID); err != nil {
		t.Fatalf("seed quota org workspace: %v", err)
	}
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.credits.LifecycleInTx(tx).ProvisionAccountWallet(ctx, orgAccount); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("provision quota org wallet: %v", err)
	}
	grantAmount, err := domain.NewCreditAmount(1000)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := env.credits.GrantsInTx(tx).Grant(ctx, ports.GrantCommand{BillingAccountID: orgAccount, Amount: grantAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "quota-org-seed-" + randomAtomicHex(t, 4),
		Actor:          domain.Actor{Type: domain.ActorSystem, ID: "test"},
		Source:         "test", Reason: "quota org seed",
		BusinessReference: domain.BusinessReference{Type: "test", ID: "quota-org-seed"},
	}}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("grant quota org funds: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	env.ws = orgWS
	env.accountID = orgAccount
	return env
}

func setDailyQuota(t *testing.T, quotas *memberquotas.Store, accountID, userID, limit int64) {
	t.Helper()
	if _, err := quotas.SetQuota(context.Background(), accountID, userID, memberquotas.PeriodDaily, &limit, userID); err != nil {
		t.Fatalf("set daily quota: %v", err)
	}
}

func taskCount(t *testing.T, env *atomicEnv) int {
	t.Helper()
	var count int
	if err := env.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM tasks WHERE workspace_id = (SELECT internal_id FROM workspaces WHERE public_id = $1)`, env.ws).Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	return count
}

// TestQuotaExhaustedBlocksSubmitWithoutResidue proves an over-quota submit
// fails before any write: no task row, no Hold ledger row, no reserved
// balance, no enqueue probe, no staged events.
func TestQuotaExhaustedBlocksSubmitWithoutResidue(t *testing.T) {
	env, quotas := openQuotaAtomicEnv(t)
	setDailyQuota(t, quotas, env.accountID, env.userID, 15)

	key := "quota-key-" + randomAtomicHex(t, 4)
	_, err := env.service.Create(context.Background(), CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, key)
	if !errors.Is(err, ErrMemberQuotaExhausted) {
		t.Fatalf("over-quota Create error = %v, want ErrMemberQuotaExhausted", err)
	}
	if got := taskCount(t, env); got != 0 {
		t.Fatalf("task rows = %d, want 0 (no residue)", got)
	}
	if got := env.ledgerCount(t, `idempotency_key = $1`, settlement.TaskHoldKey(env.ws, key)); got != 0 {
		t.Fatalf("hold ledger rows = %d, want 0 (no residue)", got)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0)", available, reserved)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("staged events non-empty after a rolled-back submit")
	}
}

// TestQuotaGateReportsOwnReason proves the quota gate constrains
// submits on its own: a submit blocked by the quota reports the quota
// reason, and a fitting submit settles on the funded wallet.
func TestQuotaGateReportsOwnReason(t *testing.T) {
	env, quotas := openQuotaAtomicEnv(t)
	ctx := context.Background()

	// Tight quota: the quota gate reports.
	setDailyQuota(t, quotas, env.accountID, env.userID, 15)
	_, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, "quota-key-"+randomAtomicHex(t, 4))
	if !errors.Is(err, ErrMemberQuotaExhausted) {
		t.Fatalf("quota-blocked Create error = %v, want ErrMemberQuotaExhausted", err)
	}

	// Roomy quota: the same funded wallet settles.
	roomyQuota := int64(1000)
	if _, err := quotas.SetQuota(ctx, env.accountID, env.userID, memberquotas.PeriodDaily, &roomyQuota, env.userID); err != nil {
		t.Fatalf("set roomy quota: %v", err)
	}
	fitting := env.createPaidTaskKeyed(t, 1, "quota-key-"+randomAtomicHex(t, 4))
	if fitting.BillingSnapshot.TotalCredits != 10 {
		t.Fatalf("fitting total = %d, want 10", fitting.BillingSnapshot.TotalCredits)
	}
}

// TestQuotaCancelReleasesForReuse proves a user-cancelled submit releases
// its Hold in the same terminal transaction, converging the member's
// usage back down: the member's next submit fits the recovered quota.
func TestQuotaCancelReleasesForReuse(t *testing.T) {
	env, quotas := openQuotaAtomicEnv(t)
	ctx := context.Background()
	setDailyQuota(t, quotas, env.accountID, env.userID, 15)

	first := env.createPaidTaskKeyed(t, 1, "quota-key-"+randomAtomicHex(t, 4))
	if _, _, err := env.service.Cancel(ctx, env.ws, first.ID, env.userID, "user cancel"); err != nil {
		t.Fatalf("cancel first task: %v", err)
	}
	_ = env.service.StagedEvents()
	retried := env.createPaidTaskKeyed(t, 1, "quota-key-"+randomAtomicHex(t, 4))
	if retried.Status != StatusPending {
		t.Fatalf("recovered task status = %q, want pending", retried.Status)
	}
	start, end := memberquotas.CurrentDayBounds(retried.CreatedAt)
	used, err := quotas.UsageForPeriod(ctx, env.accountID, env.userID, &start, &end)
	if err != nil {
		t.Fatalf("usage after cancel recovery: %v", err)
	}
	if used != 10 {
		t.Fatalf("usage after cancel recovery = %d, want 10 (second hold only)", used)
	}
}

// TestQuotaCrossWorkspaceFailRecovery proves one member's spend
// accumulates across spaces sharing one account through the settlement
// path, and a terminal Fail converges usage down: the member submits in
// the sibling space successfully after recovery.
func TestQuotaCrossWorkspaceFailRecovery(t *testing.T) {
	env, quotas := openQuotaAtomicEnv(t)
	ctx := context.Background()

	// Second workspace of the same Organization: it shares the
	// Organization pool, so quota accumulates across spaces.
	ws2 := "ws_orgquota2" + randomAtomicHex(t, 6)
	var orgID int64
	if err := env.pool.QueryRow(ctx, `SELECT organization_id FROM workspaces WHERE public_id = $1`, env.ws).Scan(&orgID); err != nil {
		t.Fatalf("resolve quota organization: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspaces (public_id, organization_id, owner_user_id, name) VALUES ($1, $2, NULL, 'atomic-settle-ws-2')`, ws2, orgID); err != nil {
		t.Fatalf("seed second workspace: %v", err)
	}
	setDailyQuota(t, quotas, env.accountID, env.userID, 15)

	// The member holds 10 in the first space...
	memberA := env.createPaidTaskKeyed(t, 1, "quota-key-"+randomAtomicHex(t, 4))
	// ...so the same member's next 10 in the sibling space exceeds the
	// 15 cap even though the wallet is funded: cross-space accumulation
	// blocks through the settlement path.
	_, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: ws2, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "quota-key-"+randomAtomicHex(t, 4))
	if !errors.Is(err, ErrMemberQuotaExhausted) {
		t.Fatalf("sibling-space Create error = %v, want ErrMemberQuotaExhausted", err)
	}
	// The first task fails: the Hold releases in the same terminal
	// transaction, converging the member's usage back down.
	if _, err := env.service.Fail(ctx, env.ws, memberA.ID, "boom"); err != nil {
		t.Fatalf("fail first task: %v", err)
	}
	_ = env.service.StagedEvents()
	// The member now fits the recovered quota in the sibling space.
	recovered := func() *WorkspaceTask {
		created, err := env.service.Create(ctx, CreateInput{
			WorkspaceID: ws2, OwnerUserID: env.userID,
			Service: "render", Quantity: 1, Resolution: "1k",
		}, "quota-key-"+randomAtomicHex(t, 4))
		if err != nil {
			t.Fatalf("sibling-space Create after recovery: %v", err)
		}
		return created
	}()
	_ = env.service.StagedEvents()
	if recovered.OwnerUserID != env.userID || recovered.Status != StatusPending {
		t.Fatalf("recovered task = %+v, want pending task owned by the member", recovered)
	}
	if recovered.BillingAccountID != env.accountID {
		t.Fatalf("recovered account = %d, want shared %d", recovered.BillingAccountID, env.accountID)
	}
	start, end := memberquotas.CurrentDayBounds(recovered.CreatedAt)
	used, err := quotas.UsageForPeriod(ctx, env.accountID, env.userID, &start, &end)
	if err != nil {
		t.Fatalf("usage after recovery: %v", err)
	}
	if used != 10 {
		t.Fatalf("usage after recovery = %d, want 10 (sibling hold only)", used)
	}
}
