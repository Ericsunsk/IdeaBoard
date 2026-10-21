package workspacetasks_test

// Workspace shared budget settlement tests (#945).
//
// The budget check joins the submit settlement transaction after Quote
// and before Hold: over-cap submits fail closed with
// ErrWorkspaceBudgetExhausted (403 workspace_budget_exhausted, distinct
// from 402 insufficiency and 403 wallet restriction) and leave no task
// row, no Hold, and no queue entry. Cross-member Holds accumulate in one
// workspace period; terminal Fail releases converge usage down so a
// blocked member can submit after recovery.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/workspacebudgets"
)

func budgetInternalID(t *testing.T, env *atomicEnv) int64 {
	t.Helper()
	var wsID int64
	if err := env.pool.QueryRow(context.Background(), `SELECT internal_id FROM workspaces WHERE public_id = $1`, env.ws).Scan(&wsID); err != nil {
		t.Fatalf("resolve budget workspace: %v", err)
	}
	return wsID
}

func openBudgetAtomicEnv(t *testing.T) (*atomicEnv, *workspacebudgets.Store) {
	t.Helper()
	env := openAtomicEnv(t, 10)
	budgets := workspacebudgets.NewStore(env.pool)
	env.serviceDeps.Budgets = budgets
	env.service = workspacetasks.NewService(env.serviceDeps)
	return env, budgets
}

func setMonthlyBudget(t *testing.T, budgets *workspacebudgets.Store, wsID int64, limit int64) {
	t.Helper()
	if _, err := budgets.SetBudget(context.Background(), wsID, workspacebudgets.PeriodMonthly, &limit, 0); err != nil {
		t.Fatalf("set monthly budget: %v", err)
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

// TestBudgetExhaustedBlocksSubmitWithoutResidue proves an over-cap submit
// fails before any write: no task row, no Hold ledger row, no reserved
// balance, no enqueue probe, no staged events.
func TestBudgetExhaustedBlocksSubmitWithoutResidue(t *testing.T) {
	env, budgets := openBudgetAtomicEnv(t)
	setMonthlyBudget(t, budgets, budgetInternalID(t, env), 15)

	key := "budget-key-" + randomAtomicHex(t, 4)
	_, err := env.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, key)
	if !errors.Is(err, workspacetasks.ErrWorkspaceBudgetExhausted) {
		t.Fatalf("over-cap Create error = %v, want ErrWorkspaceBudgetExhausted", err)
	}
	if got := taskCount(t, env); got != 0 {
		t.Fatalf("task rows = %d, want 0 (no residue)", got)
	}
	if got := env.ledgerCount(t, `idempotency_key = $1`, workspacetasks.TaskSubmitHoldKey(env.ws, key)); got != 0 {
		t.Fatalf("hold ledger rows = %d, want 0 (no residue)", got)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0)", available, reserved)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("staged events non-empty after a rolled-back submit")
	}
}

// TestBudgetSignalsStayDistinct proves the three funding gates stay
// separate: balance shortfall is insufficiency (402), an active budget
// cap is exhaustion (403 workspace_budget_exhausted), and a fitting
// submit still settles.
func TestBudgetSignalsStayDistinct(t *testing.T) {
	env, budgets := openBudgetAtomicEnv(t)

	// No budget row: unlimited. A 2000-quote over the 1000 balance is
	// insufficiency, never budget exhaustion.
	_, err := env.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 200, Resolution: "1k",
	}, "budget-key-"+randomAtomicHex(t, 4))
	if !errors.Is(err, workspacetasks.ErrInsufficientCredits) {
		t.Fatalf("shortfall Create error = %v, want ErrInsufficientCredits", err)
	}
	if errors.Is(err, workspacetasks.ErrWorkspaceBudgetExhausted) {
		t.Fatalf("shortfall misreports as budget exhaustion: %v", err)
	}

	setMonthlyBudget(t, budgets, budgetInternalID(t, env), 15)
	// A fitting submit settles under both gates.
	fitting := env.createPaidTaskKeyed(t, 1, "budget-key-"+randomAtomicHex(t, 4))
	if fitting.BillingSnapshot.TotalCredits != 10 {
		t.Fatalf("fitting total = %d, want 10", fitting.BillingSnapshot.TotalCredits)
	}
	// The same funded wallet with an exhausted budget reports exhaustion,
	// never insufficiency.
	_, err = env.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, "budget-key-"+randomAtomicHex(t, 4))
	if !errors.Is(err, workspacetasks.ErrWorkspaceBudgetExhausted) {
		t.Fatalf("over-cap Create error = %v, want ErrWorkspaceBudgetExhausted", err)
	}
	if errors.Is(err, workspacetasks.ErrInsufficientCredits) {
		t.Fatalf("over-cap misreports as insufficiency: %v", err)
	}
}

// TestBudgetCrossMemberFailRecovery proves member Holds accumulate in one
// workspace period and a terminal Fail converges usage down: the blocked
// member submits successfully after recovery.
func TestBudgetCrossMemberFailRecovery(t *testing.T) {
	env, budgets := openBudgetAtomicEnv(t)
	wsID := budgetInternalID(t, env)
	setMonthlyBudget(t, budgets, wsID, 15)
	ctx := context.Background()

	memberA := env.createPaidTaskKeyed(t, 1, "budget-key-"+randomAtomicHex(t, 4))

	var userB int64
	if err := env.pool.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('budget-member-b', $1, 'user', 1) RETURNING id`, "M"+strings.ToUpper(randomAtomicHex(t, 3))[:5]).Scan(&userB); err != nil {
		t.Fatalf("seed budget member b: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, 'member')`, wsID, userB); err != nil {
		t.Fatalf("seed budget member membership: %v", err)
	}
	// 10 (A) + 10 (B) exceeds the 15 cap: B is blocked while funded.
	_, err := env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.ws, OwnerUserID: userB,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "budget-key-"+randomAtomicHex(t, 4))
	if !errors.Is(err, workspacetasks.ErrWorkspaceBudgetExhausted) {
		t.Fatalf("member-b Create error = %v, want ErrWorkspaceBudgetExhausted", err)
	}
	// A's task fails: the Hold releases in the same terminal
	// transaction, converging workspace usage back down.
	if _, err := env.service.Fail(ctx, env.ws, memberA.ID, "boom"); err != nil {
		t.Fatalf("fail member-a task: %v", err)
	}
	_ = env.service.StagedEvents()
	// B now fits the recovered budget.
	memberB := func() *workspacetasks.WorkspaceTask {
		created, err := env.service.Create(ctx, workspacetasks.CreateInput{
			WorkspaceID: env.ws, OwnerUserID: userB,
			Service: "render", Quantity: 1, Resolution: "1k",
		}, "budget-key-"+randomAtomicHex(t, 4))
		if err != nil {
			t.Fatalf("member-b Create after recovery: %v", err)
		}
		return created
	}()
	_ = env.service.StagedEvents()
	if memberB.OwnerUserID != userB || memberB.Status != workspacetasks.StatusPending {
		t.Fatalf("member-b task = %+v, want pending task owned by b", memberB)
	}
	start, end := workspacebudgets.CurrentMonthBounds(memberB.CreatedAt)
	used, err := budgets.UsageForPeriod(ctx, wsID, &start, &end)
	if err != nil {
		t.Fatalf("usage after recovery: %v", err)
	}
	if used != 10 {
		t.Fatalf("usage after recovery = %d, want 10 (b's hold only)", used)
	}
}
