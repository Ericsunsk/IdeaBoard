package rewards

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/database"
	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestRuntimeRoleLifecycleWritesRemainConstrainedByRewardsGuards(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := t.Context()
	service := newTestRewardService(t, pool, credits.NewService(pool))
	created, err := service.CreateRewardCode(ctx, CreateRewardCodeCommand{
		DisplayName: "Runtime 权限奖励", Code: "RUNTIME26", CreditAmount: 20,
		MaxRedemptions: 2, IdempotencyKey: "runtime-privileges", ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("create RewardCode fixture: %v", err)
	}

	runtimeRole := fmt.Sprintf("rewards_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DROP OWNED BY `+roleSQL)
		_, _ = db.ExecContext(cleanupCtx, `DROP ROLE IF EXISTS `+roleSQL)
	})

	policies := []dbprivileges.Policy{database.RuntimePrivilegePolicy(), RuntimePrivilegePolicy()}
	if err := dbprivileges.Configure(ctx, db, runtimeRole, policies...); err != nil {
		t.Fatalf("configure runtime privilege policies: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open runtime connection: %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `RESET ROLE`) }()
	if err := dbprivileges.Verify(ctx, conn, policies...); err != nil {
		t.Fatalf("verify runtime privilege policies: %v", err)
	}

	if _, err := conn.ExecContext(ctx, `UPDATE reward_codes SET enabled = FALSE WHERE id = $1`, created.RewardCode.ID); err != nil {
		t.Fatalf("disable RewardCode through guarded runtime privilege: %v", err)
	}
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_codes SET code_hint = 'ABCD' WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_codes SET credit_amount = credit_amount + 1 WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_codes SET redemption_count = redemption_count + 2 WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`DELETE FROM reward_codes WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_admin_actions SET metadata = '{}'::JSONB WHERE reward_code_id = $1`, created.RewardCode.ID)

	for _, signature := range []string{
		"prevent_reward_admin_action_mutation()",
		"guard_reward_code_identity()",
	} {
		var runtimeCanExecute, publicCanExecute bool
		if err := db.QueryRowContext(ctx, `
SELECT has_function_privilege($1, $2, 'EXECUTE'),
       has_function_privilege('public', $2, 'EXECUTE')`, runtimeRole, signature).Scan(
			&runtimeCanExecute,
			&publicCanExecute,
		); err != nil {
			t.Fatalf("read function privileges for %s: %v", signature, err)
		}
		if runtimeCanExecute || publicCanExecute {
			t.Fatalf("guard %s execute privileges runtime=%v public=%v", signature, runtimeCanExecute, publicCanExecute)
		}
	}
}

func assertRuntimeRewardStatementDenied(
	t *testing.T,
	ctx context.Context,
	conn *sql.Conn,
	statement string,
	args ...any,
) {
	t.Helper()
	if _, err := conn.ExecContext(ctx, statement, args...); err == nil {
		t.Fatalf("runtime Rewards bypass unexpectedly succeeded: %s", statement)
	}
}
