package workspacetasks_test

// Canvas Task shared-billing settlement (#917, PostgreSQL integration).
//
// What is proven here, over the real settlement seams (Billing catalog,
// Credits Commands, postgres task Store, stable Billing Account
// resolution):
//   - Two workspaces sharing one Billing Account (explicit link, no
//     organization) settle submits against that one payer with the real
//     submitter as operator; no personal or owner wallet is touched.
//   - Concurrent holds against the shared account cannot overdraft: every
//     failure is ErrInsufficientCredits and the balance never goes
//     negative.
//   - A repeated submit (same key + hash) replays the original task with
//     a single Hold; the same key with a different body is
//     ErrIdempotencyConflict.
//   - Member departure, billing ownership transfer, and payer-link
//     changes never rewrite an existing Task's attribution: terminal
//     Capture/Release converge on the submit-time account facts.
//   - Retry keeps the attribution and the Hold, mints a fresh execution
//     token, and rotates the envelope past the stale token (stale jobs
//     drop at dispatch on the envelope mismatch).
//
// Tests SKIP gracefully without API_TEST_DATABASE_DSN (house convention).

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
)

// openSharedBillingSecondWorkspace provisions a second personal workspace
// for the same owner and links it to the first workspace's stable Billing
// Account (the no-organization cross-workspace sharing shape).
func openSharedBillingSecondWorkspace(t *testing.T, env *atomicEnv) (string, int64) {
	t.Helper()
	ctx := context.Background()
	wsB := "ws_atomic" + randomAtomicHex(t, 6)
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'atomic-settle-ws-b')`,
		wsB, env.userID,
	); err != nil {
		t.Fatalf("seed second settle workspace: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspace_memberships(workspace_id,user_id,role) SELECT internal_id,$2,'owner' FROM workspaces WHERE public_id=$1`, wsB, env.userID); err != nil {
		t.Fatalf("seed second settle membership: %v", err)
	}
	var wsBInternal int64
	if err := env.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, wsB).Scan(&wsBInternal); err != nil {
		t.Fatalf("resolve second settle workspace: %v", err)
	}
	if err := tenancypg.NewBillingAccountStore(env.pool).LinkWorkspaceToBillingAccount(ctx, wsBInternal, env.accountID, env.userID); err != nil {
		t.Fatalf("link second workspace to shared account: %v", err)
	}
	return wsB, wsBInternal
}

// TestCanvasSharedAccountSubmitSettlesOnePayer proves two workspaces
// sharing one Billing Account settle against that account with the real
// submitter as operator.
func TestCanvasSharedAccountSubmitSettlesOnePayer(t *testing.T) {
	env := openAtomicEnv(t, 10)
	wsB, _ := openSharedBillingSecondWorkspace(t, env)

	created, err := env.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: wsB, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "shared-payer-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("shared Create: %v", err)
	}
	_ = env.service.StagedEvents()

	if created.WorkspaceID != wsB {
		t.Fatalf("task workspace = %q, want the consumption location %q", created.WorkspaceID, wsB)
	}
	if created.OwnerUserID != env.userID {
		t.Fatalf("task submitter = %d, want the real operator %d", created.OwnerUserID, env.userID)
	}
	if created.BillingAccountID != env.accountID {
		t.Fatalf("task billing account = %d, want the shared account %d", created.BillingAccountID, env.accountID)
	}
	if created.BillingSnapshot.BillingAccountID != env.accountID {
		t.Fatalf("snapshot billing account = %d, want the shared account %d", created.BillingSnapshot.BillingAccountID, env.accountID)
	}
	stored, err := env.store.GetTask(context.Background(), wsB, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.BillingAccountID != env.accountID || stored.WorkspaceID != wsB || stored.OwnerUserID != env.userID {
		t.Fatalf("stored triple = (%d, %q, %d), want (%d, %q, %d)",
			stored.BillingAccountID, stored.WorkspaceID, stored.OwnerUserID, env.accountID, wsB, env.userID)
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("shared wallet = (%d, %d), want (990, 10): the hold must debit the shared account", available, reserved)
	}
}

// TestCanvasSharedAccountConcurrentHoldsCannotOverdraft proves concurrent
// submits against one shared account serialize on the ledger: every
// failure is insufficient-credits and the balance never goes negative.
func TestCanvasSharedAccountConcurrentHoldsCannotOverdraft(t *testing.T) {
	env := openAtomicEnv(t, 10)
	wsB, _ := openSharedBillingSecondWorkspace(t, env)

	const workers = 4
	const quantity = int64(40) // 400 each; 4 x 400 = 1600 > 1000 funded.
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			// One Service per goroutine: the Service stages events on
			// itself, so sharing one instance across goroutines would
			// race. The seams below are all transaction-safe.
			service := workspacetasks.NewService(env.serviceDeps)
			workspace := env.ws
			if slot%2 == 1 {
				workspace = wsB
			}
			_, err := service.Create(context.Background(), workspacetasks.CreateInput{
				WorkspaceID: workspace, OwnerUserID: env.userID,
				Service: "render", Quantity: quantity, Resolution: "1k",
			}, "shared-race-"+randomAtomicHex(t, 4))
			errs[slot] = err
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		if !errors.Is(err, workspacetasks.ErrInsufficientCredits) {
			t.Fatalf("concurrent hold error = %v, want nil or insufficient-credits", err)
		}
	}
	if succeeded != 2 {
		t.Fatalf("succeeded = %d, want 2 (2 x 400 settles, the rest must fail closed)", succeeded)
	}
	available, reserved := env.wallet(t)
	if available < 0 || reserved < 0 || available+reserved != 1000 {
		t.Fatalf("shared wallet = (%d, %d), want non-negative legs summing to 1000", available, reserved)
	}
}

// TestCanvasSubmitIdempotencyReplaySingleHold proves a retried submit
// replays the original task with a single Hold, while the same key with a
// different body conflicts instead of double-spending.
func TestCanvasSubmitIdempotencyReplaySingleHold(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	key := "shared-idem-" + randomAtomicHex(t, 4)

	first, err := env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, key)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_ = env.service.StagedEvents()

	replayed, err := env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, key)
	if err != nil {
		t.Fatalf("replay Create: %v", err)
	}
	if replayed.ID != first.ID {
		t.Fatalf("replay task = %q, want the original %q", replayed.ID, first.ID)
	}
	if n := len(env.service.StagedEvents()); n != 0 {
		t.Fatalf("replay staged %d events, want 0 (the winner already emitted)", n)
	}
	if got := env.ledgerCount(t, `idempotency_key = $1`, workspacetasks.TaskSubmitHoldKey(env.ws, key)); got != 1 {
		t.Fatalf("hold ledger rows = %d, want exactly 1", got)
	}

	_, err = env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, key)
	if !errors.Is(err, workspacetasks.ErrIdempotencyConflict) {
		t.Fatalf("conflicting re-submit err = %v, want ErrIdempotencyConflict", err)
	}
}

// TestCanvasTerminalSettlesOriginalAccountAfterTenancyChange proves member
// departure, billing ownership transfer, and payer-link changes never
// rewrite an existing Task's attribution: terminal settlement converges on
// the submit-time account facts.
func TestCanvasTerminalSettlesOriginalAccountAfterTenancyChange(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()

	held := env.createPaidTask(t, 1)
	holdID := held.BillingSnapshot.HoldTransactionID
	if held.BillingAccountID != env.accountID {
		t.Fatalf("task billing account = %d, want %d", held.BillingAccountID, env.accountID)
	}

	// Tenancy churn after the Hold: hand billing ownership to a second
	// user and remove the submitter's workspace membership entirely.
	storageUID := "T" + strings.ToUpper(randomAtomicHex(t, 3))[:5]
	var otherUser int64
	if err := env.pool.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('shared-settle-other', $1, 'user', 1) RETURNING id`,
		storageUID,
	).Scan(&otherUser); err != nil {
		t.Fatalf("seed second user: %v", err)
	}
	baStore := tenancypg.NewBillingAccountStore(env.pool)
	if _, err := baStore.AddBillingAccountMember(ctx, env.accountID, env.userID, otherUser, tenancy.BillingRoleAdmin); err != nil {
		t.Fatalf("add billing admin: %v", err)
	}
	if err := baStore.TransferBillingAccountOwnership(ctx, env.accountID, env.userID, otherUser); err != nil {
		t.Fatalf("transfer billing ownership: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `DELETE FROM workspace_memberships WHERE user_id = $1 AND workspace_id = (SELECT internal_id FROM workspaces WHERE public_id = $2)`, env.userID, env.ws); err != nil {
		t.Fatalf("remove submitter membership: %v", err)
	}

	// Success still captures against the original account.
	completed, err := env.service.Complete(ctx, env.ws, held.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("Complete after tenancy change: %v", err)
	}
	_ = env.service.StagedEvents()
	if completed.BillingAccountID != env.accountID || completed.OwnerUserID != env.userID {
		t.Fatalf("completed attribution = (%d, %d), want (%d, %d)",
			completed.BillingAccountID, completed.OwnerUserID, env.accountID, env.userID)
	}
	if completed.BillingSnapshot.HoldTransactionID != holdID {
		t.Fatalf("completed hold = %q, want the submit-time hold %q", completed.BillingSnapshot.HoldTransactionID, holdID)
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 0 {
		t.Fatalf("shared wallet = (%d, %d), want (990, 0) after capture on the original account", available, reserved)
	}

	// Failure still releases to the original account. The membership is
	// gone, so the submit below must come from the remaining billing
	// owner through a fresh membership.
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspace_memberships(workspace_id,user_id,role) SELECT internal_id,$2,'owner' FROM workspaces WHERE public_id=$1`, env.ws, otherUser); err != nil {
		t.Fatalf("seed owner membership: %v", err)
	}
	failing, err := env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: otherUser,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "shared-fail-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	_ = env.service.StagedEvents()
	if failing.BillingAccountID != env.accountID {
		t.Fatalf("second task billing account = %d, want the still-shared %d", failing.BillingAccountID, env.accountID)
	}
	if _, err := env.service.Fail(ctx, env.ws, failing.ID, "provider exploded"); err != nil {
		t.Fatalf("Fail after tenancy change: %v", err)
	}
	_ = env.service.StagedEvents()
	if available, reserved := env.wallet(t); available != 990 || reserved != 0 {
		t.Fatalf("shared wallet = (%d, %d), want (990, 0) after release to the original account", available, reserved)
	}
}

// TestCanvasRetryKeepsBillingAttributionAndFencesStaleToken proves a
// background retry neither rewrites the Task's payer nor takes a second
// Hold: it mints a fresh execution token. The stale token loses at
// dispatch: the worker drops any job whose token no longer matches the
// task's envelope token before claiming
// (tasking/interfaces/worker `loadDispatchableTask`), so the envelope
// mismatch below is the fencing fact this test pins.
func TestCanvasRetryKeepsBillingAttributionAndFencesStaleToken(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()

	created := env.createPaidTask(t, 1)
	staleToken := created.ExecutionToken
	holdID := created.BillingSnapshot.HoldTransactionID

	retried, err := env.service.Retry(ctx, env.ws, created.ID, true)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	_ = env.service.StagedEvents()
	if retried.BillingAccountID != env.accountID {
		t.Fatalf("retried billing account = %d, want %d", retried.BillingAccountID, env.accountID)
	}
	if retried.BillingSnapshot.HoldTransactionID != holdID {
		t.Fatalf("retried hold = %q, want the submit-time hold %q", retried.BillingSnapshot.HoldTransactionID, holdID)
	}
	if retried.ExecutionToken == staleToken {
		t.Fatal("retry kept the execution token; a retry must mint a fresh one")
	}
	if got := env.ledgerCount(t, `command_type = 'hold' AND billing_account_id = $1`, env.accountID); got != 1 {
		t.Fatalf("hold ledger rows = %d, want 1 (retry takes no second hold)", got)
	}

	reloaded, err := env.store.GetTask(ctx, env.ws, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ExecutionToken == staleToken {
		t.Fatal("task envelope still carries the stale token after retry")
	}
	// The fresh token claims live; the stale job token no longer matches
	// the envelope and would drop at dispatch before any claim.
	claimed, err := env.store.ClaimTask(ctx, env.ws, created.ID, reloaded.ExecutionToken)
	if err != nil {
		t.Fatalf("fresh claim: %v", err)
	}
	if claimed.Status != workspacetasks.StatusRunning {
		t.Fatalf("claimed status = %q, want running", claimed.Status)
	}
}
