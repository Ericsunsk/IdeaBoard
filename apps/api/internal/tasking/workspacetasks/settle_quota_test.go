package workspacetasks_test

// Member cross-workspace quota settlement tests (#946).
//
// The quota check joins the submit settlement transaction after Quote
// and before Hold: over-quota submits fail closed
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
	"testing"

	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/tasking/settlement"
	"ideaboard/api/internal/tasking/workspacetasks"
)

func openQuotaAtomicEnv(t *testing.T) (*atomicEnv, *memberquotas.Store) {
	t.Helper()
	env := openAtomicEnv(t, 10)
	quotas := memberquotas.NewStore(env.pool)
	env.serviceDeps.Quotas = quotas
	env.service = workspacetasks.NewService(env.serviceDeps)
	return env, quotas
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
	_, err := env.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, key)
	if !errors.Is(err, workspacetasks.ErrMemberQuotaExhausted) {
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
	_, err := env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, "quota-key-"+randomAtomicHex(t, 4))
	if !errors.Is(err, workspacetasks.ErrMemberQuotaExhausted) {
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
	if retried.Status != workspacetasks.StatusPending {
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

	// Second workspace of the same owner: it shares the Personal
	// survivor by creation scope (no link), so quota accumulates.
	ws2 := "ws_atomic2" + randomAtomicHex(t, 6)
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'atomic-settle-ws-2')`, ws2, env.userID); err != nil {
		t.Fatalf("seed second workspace: %v", err)
	}
	var ws2ID int64
	if err := env.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, ws2).Scan(&ws2ID); err != nil {
		t.Fatalf("resolve second workspace: %v", err)
	}
	var wsID int64
	if err := env.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, env.ws).Scan(&wsID); err != nil {
		t.Fatalf("resolve first workspace: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING`, ws2ID, env.userID); err != nil {
		t.Fatalf("seed sibling membership: %v", err)
	}
	setDailyQuota(t, quotas, env.accountID, env.userID, 15)

	// The member holds 10 in the first space...
	memberA := env.createPaidTaskKeyed(t, 1, "quota-key-"+randomAtomicHex(t, 4))
	// ...so the same member's next 10 in the sibling space exceeds the
	// 15 cap even though the wallet is funded: cross-space accumulation
	// blocks through the settlement path.
	_, err := env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: ws2, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "quota-key-"+randomAtomicHex(t, 4))
	if !errors.Is(err, workspacetasks.ErrMemberQuotaExhausted) {
		t.Fatalf("sibling-space Create error = %v, want ErrMemberQuotaExhausted", err)
	}
	// The first task fails: the Hold releases in the same terminal
	// transaction, converging the member's usage back down.
	if _, err := env.service.Fail(ctx, env.ws, memberA.ID, "boom"); err != nil {
		t.Fatalf("fail first task: %v", err)
	}
	_ = env.service.StagedEvents()
	// The member now fits the recovered quota in the sibling space.
	recovered := func() *workspacetasks.WorkspaceTask {
		created, err := env.service.Create(ctx, workspacetasks.CreateInput{
			WorkspaceID: ws2, OwnerUserID: env.userID,
			Service: "render", Quantity: 1, Resolution: "1k",
		}, "quota-key-"+randomAtomicHex(t, 4))
		if err != nil {
			t.Fatalf("sibling-space Create after recovery: %v", err)
		}
		return created
	}()
	_ = env.service.StagedEvents()
	if recovered.OwnerUserID != env.userID || recovered.Status != workspacetasks.StatusPending {
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
