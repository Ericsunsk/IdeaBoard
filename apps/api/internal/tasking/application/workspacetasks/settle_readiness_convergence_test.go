package workspacetasks

// Readiness/terminal convergence (#980, part of #973).
//
// Fresh-database proof that readiness/Quote and submission see the same
// actor payer and that every terminal converges with no hanging holds:
//   - Personal collaborator success/fail/cancel all settle on the actor's
//     own Personal account (owner untouched, reserved converges to 0).
//   - Member removal / role changes never rewrite row facts: in-flight
//     terminals converge on the submit-time account + actor.
//   - Retry mints a new execution token with no second Hold.
//   - Free (zero-amount) tasks ride the free path with no ledger.
//   - Idempotent replay returns the same task with a single Hold;
//     the stalled sweep releases once and stays idempotent (unchanged).
//   - Organization regression: shared-pool terminals + quota enforcement
//     are unchanged.
//
// Tests SKIP gracefully without API_TEST_DATABASE_DSN (house convention).

import (
	"context"
	"errors"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/tenancy/funds"
)

func mustCreateCollaboratorTask(t *testing.T, env *atomicEnv, actor int64, key string) *WorkspaceTask {
	t.Helper()
	created, err := env.service.Create(context.Background(), CreateInput{
		WorkspaceID: env.ws, OwnerUserID: actor,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, key)
	if err != nil {
		t.Fatalf("collaborator Create: %v", err)
	}
	_ = env.service.StagedEvents()
	return created
}

// TestReadinessConvergencePersonalThreeTerminals proves the acceptance
// core: success/fail/cancel all converge with no hanging reservation.
func TestReadinessConvergencePersonalThreeTerminals(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	success := mustCreateCollaboratorTask(t, env, collaboratorID, "980-conv-success-"+randomAtomicHex(t, 4))
	env.claimTask(t, success.ID, success.ExecutionToken)
	if _, err := env.service.Complete(ctx, env.ws, success.ID, atomicRemoteOutput()); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	_ = env.service.StagedEvents()
	if got := success.BillingAccountID; got != collaboratorAccount {
		// success was created with the actor payer; the stored row below
		// is the durable proof.
		t.Fatalf("success billing account = %d, want actor %d", got, collaboratorAccount)
	}

	failing := mustCreateCollaboratorTask(t, env, collaboratorID, "980-conv-fail-"+randomAtomicHex(t, 4))
	env.claimTask(t, failing.ID, failing.ExecutionToken)
	if _, err := env.service.Fail(ctx, env.ws, failing.ID, "boom"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	_ = env.service.StagedEvents()

	cancelling := mustCreateCollaboratorTask(t, env, collaboratorID, "980-conv-cancel-"+randomAtomicHex(t, 4))
	if _, _, err := env.service.Cancel(ctx, env.ws, cancelling.ID, collaboratorID, "user cancel"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	_ = env.service.StagedEvents()

	for _, id := range []string{success.ID, failing.ID, cancelling.ID} {
		stored, err := env.store.GetTask(ctx, env.ws, id)
		if err != nil {
			t.Fatal(err)
		}
		if stored.BillingAccountID != collaboratorAccount || stored.OwnerUserID != collaboratorID {
			t.Fatalf("task %s triple = (%d, %d), want (%d, %d)", id, stored.BillingAccountID, stored.OwnerUserID, collaboratorAccount, collaboratorID)
		}
	}
	// One success captured 10; fail + cancel released: 1000-10=990, 0 held.
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 990 || reserved != 0 {
		t.Fatalf("actor wallet = (%d, %d), want (990, 0) (no hanging holds)", available, reserved)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("owner wallet = (%d, %d), want (1000, 0) (untouched)", available, reserved)
	}
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type IN ('hold','capture','release')`, env.accountID); got != 0 {
		t.Fatalf("owner ledger rows = %d, want 0", got)
	}
	// No held credit_holds remain for the three tasks.
	var held int
	if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM credit_holds WHERE hold_transaction_id IN (SELECT hold_transaction_id FROM task_billing_snapshots WHERE task_id IN ($1,$2,$3)) AND status = 'held'`, success.ID, failing.ID, cancelling.ID).Scan(&held); err != nil {
		t.Fatalf("count held holds: %v", err)
	}
	if held != 0 {
		t.Fatalf("held holds = %d, want 0 (all terminals converged)", held)
	}
}

// TestReadinessConvergenceMemberRemovalKeepsRowFacts proves tenancy
// changes never rewrite row facts: after the collaborator is removed,
// in-flight terminals still converge on the original account + actor.
func TestReadinessConvergenceMemberRemovalKeepsRowFacts(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	inflight := mustCreateCollaboratorTask(t, env, collaboratorID, "980-conv-remove-"+randomAtomicHex(t, 4))
	env.claimTask(t, inflight.ID, inflight.ExecutionToken)
	// Remove the collaborator mid-flight (member departure + role loss).
	if _, err := env.pool.Exec(ctx, `DELETE FROM workspace_memberships WHERE workspace_id = (SELECT internal_id FROM workspaces WHERE public_id = $1) AND user_id = $2`, env.ws, collaboratorID); err != nil {
		t.Fatalf("remove collaborator: %v", err)
	}

	completed, err := env.service.Complete(ctx, env.ws, inflight.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("Complete after removal: %v", err)
	}
	_ = env.service.StagedEvents()
	if completed.BillingAccountID != collaboratorAccount || completed.OwnerUserID != collaboratorID {
		t.Fatalf("completed triple = (%d, %d), want original (%d, %d)", completed.BillingAccountID, completed.OwnerUserID, collaboratorAccount, collaboratorID)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 990 || reserved != 0 {
		t.Fatalf("actor wallet after post-removal complete = (%d, %d), want (990, 0)", available, reserved)
	}

	// A second in-flight task fails after removal: Release still lands on
	// the actor.
	// Re-add briefly to submit, then remove again before the terminal.
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspace_memberships(workspace_id,user_id,role) SELECT internal_id,$2,'member' FROM workspaces WHERE public_id=$1`, env.ws, collaboratorID); err != nil {
		t.Fatalf("re-add collaborator: %v", err)
	}
	failing := mustCreateCollaboratorTask(t, env, collaboratorID, "980-conv-remove-fail-"+randomAtomicHex(t, 4))
	env.claimTask(t, failing.ID, failing.ExecutionToken)
	if _, err := env.pool.Exec(ctx, `DELETE FROM workspace_memberships WHERE workspace_id = (SELECT internal_id FROM workspaces WHERE public_id = $1) AND user_id = $2`, env.ws, collaboratorID); err != nil {
		t.Fatalf("remove collaborator again: %v", err)
	}
	failed, err := env.service.Fail(ctx, env.ws, failing.ID, "boom")
	if err != nil {
		t.Fatalf("Fail after removal: %v", err)
	}
	_ = env.service.StagedEvents()
	if failed.BillingAccountID != collaboratorAccount {
		t.Fatalf("failed billing account = %d, want original actor %d", failed.BillingAccountID, collaboratorAccount)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 990 || reserved != 0 {
		t.Fatalf("actor wallet after post-removal fail = (%d, %d), want (990, 0) (second hold released, first captured)", available, reserved)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("owner wallet = (%d, %d), want (1000, 0)", available, reserved)
	}
}

// TestReadinessConvergenceCancelAfterRemoval proves the cancel terminal
// also converges on the original row facts after departure.
func TestReadinessConvergenceCancelAfterRemoval(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	pending := mustCreateCollaboratorTask(t, env, collaboratorID, "980-conv-cancel-remove-"+randomAtomicHex(t, 4))
	if _, err := env.pool.Exec(ctx, `DELETE FROM workspace_memberships WHERE workspace_id = (SELECT internal_id FROM workspaces WHERE public_id = $1) AND user_id = $2`, env.ws, collaboratorID); err != nil {
		t.Fatalf("remove collaborator: %v", err)
	}
	cancelled, already, err := env.service.Cancel(ctx, env.ws, pending.ID, collaboratorID, "user cancel")
	if err != nil || already {
		t.Fatalf("Cancel after removal = (%v, %v, %v), want fresh cancel", cancelled, already, err)
	}
	_ = env.service.StagedEvents()
	if cancelled.BillingAccountID != collaboratorAccount {
		t.Fatalf("cancelled billing account = %d, want original actor %d", cancelled.BillingAccountID, collaboratorAccount)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 1000 || reserved != 0 {
		t.Fatalf("actor wallet after post-removal cancel = (%d, %d), want (1000, 0)", available, reserved)
	}
}

// TestReadinessConvergenceRetryNoSecondHold proves retry requeues with a
// new execution token and no second Hold: the wallet and ledger are
// untouched by the retry itself.
func TestReadinessConvergenceRetryNoSecondHold(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	created := mustCreateCollaboratorTask(t, env, collaboratorID, "980-conv-retry-"+randomAtomicHex(t, 4))
	env.claimTask(t, created.ID, created.ExecutionToken)

	retried, err := env.service.Retry(ctx, env.ws, created.ID, true)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	_ = env.service.StagedEvents()
	if retried.BillingAccountID != collaboratorAccount || retried.OwnerUserID != collaboratorID {
		t.Fatalf("retried triple = (%d, %d), want (%d, %d)", retried.BillingAccountID, retried.OwnerUserID, collaboratorAccount, collaboratorID)
	}
	if retried.ExecutionToken == created.ExecutionToken {
		t.Fatal("retry kept the execution token, want a fresh token")
	}
	if retried.Status != StatusPending {
		t.Fatalf("retried status = %q, want pending", retried.Status)
	}
	// No second Hold: exactly one hold ledger row for the task key, wallet
	// still shows the single open Hold.
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type = 'hold'`, collaboratorAccount); got != 1 {
		t.Fatalf("actor hold ledger rows = %d, want 1 (no second Hold)", got)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 990 || reserved != 10 {
		t.Fatalf("actor wallet after retry = (%d, %d), want (990, 10) (hold untouched)", available, reserved)
	}
	// The retried task still completes on the original payer.
	env.claimTask(t, retried.ID, retried.ExecutionToken)
	if _, err := env.service.Complete(ctx, env.ws, retried.ID, atomicRemoteOutput()); err != nil {
		t.Fatalf("Complete after retry: %v", err)
	}
	_ = env.service.StagedEvents()
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 990 || reserved != 0 {
		t.Fatalf("actor wallet after post-retry complete = (%d, %d), want (990, 0)", available, reserved)
	}
}

// TestReadinessConvergenceFreePath proves zero-amount tasks ride the free
// path with no ledger and converge captured/released in the same terminal
// transaction.
func TestReadinessConvergenceFreePath(t *testing.T) {
	env := openAtomicEnv(t, 0)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	free, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "980-conv-free-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("free Create: %v", err)
	}
	_ = env.service.StagedEvents()
	if free.BillingSnapshot.TotalCredits != 0 || free.BillingSnapshot.SettlementState != "free" {
		t.Fatalf("free snapshot = %+v, want zero total with free state", free.BillingSnapshot)
	}
	if free.BillingAccountID != collaboratorAccount {
		t.Fatalf("free billing account = %d, want actor %d", free.BillingAccountID, collaboratorAccount)
	}
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type IN ('hold','capture','release')`, collaboratorAccount); got != 0 {
		t.Fatalf("free actor ledger rows = %d, want 0 (no Hold)", got)
	}
	completed, err := env.service.Complete(ctx, env.ws, free.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("free Complete: %v", err)
	}
	_ = env.service.StagedEvents()
	if completed.BillingSnapshot.SettlementState != "captured" {
		t.Fatalf("free settlement = %q, want captured", completed.BillingSnapshot.SettlementState)
	}
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type IN ('hold','capture','release')`, collaboratorAccount); got != 0 {
		t.Fatalf("free actor ledger rows after complete = %d, want 0", got)
	}

	freeFail, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "980-conv-free-fail-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("second free Create: %v", err)
	}
	_ = env.service.StagedEvents()
	env.claimTask(t, freeFail.ID, freeFail.ExecutionToken)
	failed, err := env.service.Fail(ctx, env.ws, freeFail.ID, "boom")
	if err != nil {
		t.Fatalf("free Fail: %v", err)
	}
	_ = env.service.StagedEvents()
	if failed.BillingSnapshot.SettlementState != "released" {
		t.Fatalf("free fail settlement = %q, want released", failed.BillingSnapshot.SettlementState)
	}
}

// TestReadinessConvergenceIdempotencySingleHold proves幂等 replay returns
// the same task with no second Hold (unchanged seam).
func TestReadinessConvergenceIdempotencySingleHold(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	key := "980-conv-idem-" + randomAtomicHex(t, 4)
	first, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, key)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_ = env.service.StagedEvents()
	replayed, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, key)
	if err != nil {
		t.Fatalf("replay Create: %v", err)
	}
	_ = env.service.StagedEvents()
	if replayed.ID != first.ID {
		t.Fatalf("replay id = %s, want %s (same task)", replayed.ID, first.ID)
	}
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type = 'hold'`, collaboratorAccount); got != 1 {
		t.Fatalf("actor hold rows after replay = %d, want 1", got)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 990 || reserved != 10 {
		t.Fatalf("actor wallet after replay = (%d, %d), want (990, 10)", available, reserved)
	}
}

// TestReadinessConvergenceSweepUnchanged proves the stalled sweep still
// releases exactly once on the actor payer (unchanged seam).
func TestReadinessConvergenceSweepUnchanged(t *testing.T) {
	env := openAtomicEnv(t, 10)
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)
	created, err := env.service.Create(context.Background(), CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "980-conv-sweep-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = env.service.StagedEvents()
	env.claimTask(t, created.ID, created.ExecutionToken)
	env.ageTaskPastStallThreshold(t, created.ID, StalledSettlementThreshold+time.Hour)

	first := env.runStalledSweep(t, time.Now())
	if first.Converged != 1 {
		t.Fatalf("first sweep = %+v, want converged=1", first)
	}
	second := env.runStalledSweep(t, time.Now())
	if second.Converged != 0 {
		t.Fatalf("second sweep = %+v, want converged=0 (idempotent)", second)
	}
	if got := env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID); got != 1 {
		t.Fatalf("release rows = %d, want 1", got)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 1000 || reserved != 0 {
		t.Fatalf("actor wallet after sweep = (%d, %d), want (1000, 0)", available, reserved)
	}
}

// TestReadinessConvergenceFundsSeesSameActor proves the funds seam the
// preflight and the settlement share: Personal collaborators resolve to
// their own account (owner differs), Organization members share the pool.
func TestReadinessConvergenceFundsSeesSameActor(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	queries := dbsqlc.New(env.pool)
	ownerIdentity, err := funds.ResolveByPublicIDForActorWithQueries(ctx, queries, env.ws, env.userID)
	if err != nil {
		t.Fatalf("owner funds: %v", err)
	}
	actorIdentity, err := funds.ResolveByPublicIDForActorWithQueries(ctx, queries, env.ws, collaboratorID)
	if err != nil {
		t.Fatalf("actor funds: %v", err)
	}
	if ownerIdentity.BillingAccountID == actorIdentity.BillingAccountID {
		t.Fatal("Personal owner and collaborator share one account, want per-actor accounts")
	}
	if actorIdentity.BillingAccountID != collaboratorAccount || actorIdentity.OwnerType != funds.OwnerTypeWorkspace {
		t.Fatalf("actor identity = %+v, want account %d workspace", actorIdentity, collaboratorAccount)
	}
	if ownerIdentity.BillingAccountID != env.accountID || ownerIdentity.OwnerType != funds.OwnerTypeWorkspace {
		t.Fatalf("owner identity = %+v, want account %d workspace", ownerIdentity, env.accountID)
	}
}

// TestReadinessConvergenceOrgRegression proves Organization readiness and
// terminals are unchanged: shared-pool settlement, quota enforced on both
// seams, terminals converge on the pool.
func TestReadinessConvergenceOrgRegression(t *testing.T) {
	env := openQuotaOrgEnv(t)
	quotas := memberquotas.NewStore(env.pool)
	env.serviceDeps.Quotas = quotas
	env.service = NewService(env.serviceDeps)
	ctx := context.Background()

	// Quota gate still enforced through the settlement on Org scopes.
	setDailyQuota(t, quotas, env.accountID, env.userID, 15)
	if _, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, "980-org-block-"+randomAtomicHex(t, 4)); !errors.Is(err, ErrMemberQuotaExhausted) {
		t.Fatalf("over-quota Org Create err = %v, want quota exhausted", err)
	}
	if got := taskCount(t, env); got != 0 {
		t.Fatalf("tasks after blocked Org submit = %d, want 0", got)
	}

	roomy := int64(1000)
	if _, err := quotas.SetQuota(ctx, env.accountID, env.userID, memberquotas.PeriodDaily, &roomy, env.userID); err != nil {
		t.Fatalf("set roomy Org quota: %v", err)
	}
	created := env.createPaidTaskKeyed(t, 1, "980-org-ok-"+randomAtomicHex(t, 4))
	if created.BillingAccountID != env.accountID {
		t.Fatalf("Org task billing account = %d, want shared %d", created.BillingAccountID, env.accountID)
	}
	env.claimTask(t, created.ID, created.ExecutionToken)
	completed, err := env.service.Complete(ctx, env.ws, created.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("Org Complete: %v", err)
	}
	_ = env.service.StagedEvents()
	if completed.BillingSnapshot.SettlementState != "captured" {
		t.Fatalf("Org settlement = %q, want captured", completed.BillingSnapshot.SettlementState)
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 0 {
		t.Fatalf("Org wallet = (%d, %d), want (990, 0)", available, reserved)
	}

	failing := env.createPaidTaskKeyed(t, 1, "980-org-fail-"+randomAtomicHex(t, 4))
	env.claimTask(t, failing.ID, failing.ExecutionToken)
	if _, err := env.service.Fail(ctx, env.ws, failing.ID, "boom"); err != nil {
		t.Fatalf("Org Fail: %v", err)
	}
	_ = env.service.StagedEvents()
	if available, reserved := env.wallet(t); available != 990 || reserved != 0 {
		t.Fatalf("Org wallet after fail = (%d, %d), want (990, 0) (second hold released)", available, reserved)
	}

	cancelling := env.createPaidTaskKeyed(t, 1, "980-org-cancel-"+randomAtomicHex(t, 4))
	if _, _, err := env.service.Cancel(ctx, env.ws, cancelling.ID, env.userID, "user cancel"); err != nil {
		t.Fatalf("Org Cancel: %v", err)
	}
	_ = env.service.StagedEvents()
	if available, reserved := env.wallet(t); available != 990 || reserved != 0 {
		t.Fatalf("Org wallet after cancel = (%d, %d), want (990, 0)", available, reserved)
	}
}
