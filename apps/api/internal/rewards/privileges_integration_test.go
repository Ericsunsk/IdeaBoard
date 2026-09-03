package rewards

import (
	"context"
	"database/sql"
	"fmt"
	creditpostgres "ideaboard/api/internal/credits/infrastructure/postgres"
	"os"
	"strings"
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
	if _, err := db.ExecContext(ctx, `
UPDATE reward_codes
SET expires_at = clock_timestamp() - INTERVAL '1 second'
WHERE id = $1`, created.RewardCode.ID); err != nil {
		t.Fatalf("expire RewardCode fixture: %v", err)
	}
	var reserved bool
	if err := conn.QueryRowContext(ctx, `SELECT reserve_reward_code_redemption_v1($1)`, created.RewardCode.ID).Scan(&reserved); err != nil {
		t.Fatalf("reserve expired RewardCode through runtime capability: %v", err)
	}
	if reserved {
		t.Fatal("runtime reserve bypassed database-authoritative expiry")
	}

	var changed bool
	if err := conn.QueryRowContext(ctx, `SELECT set_reward_code_enabled_v1($1, FALSE, $2)`,
		created.RewardCode.ID, int64(1)).Scan(&changed); err != nil || !changed {
		t.Fatalf("disable RewardCode through controlled command: changed=%t error=%v", changed, err)
	}
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_codes SET enabled = TRUE WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_codes SET code_hint = 'ABCD' WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_codes SET credit_amount = credit_amount + 1 WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_codes SET redemption_count = redemption_count + 2 WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`DELETE FROM reward_codes WHERE id = $1`, created.RewardCode.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`DELETE FROM reward_code_digest_aliases WHERE reward_code_id = $1`, created.RewardCode.ID)
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
	commandFunctions := []string{
		"update_reward_code_definition_v1(bigint,text,bigint,bigint,timestamp with time zone,bigint)",
		"set_reward_code_enabled_v1(bigint,boolean,bigint)",
		"rotate_reward_code_v1(bigint,text,bytea,text,text,bytea,bigint)",
		"delete_reward_code_v1(bigint,bigint)",
		"reserve_reward_code_redemption_v1(bigint)",
	}
	for _, signature := range commandFunctions {
		var securityDefiner bool
		var owner string
		var config string
		var trustedSchema string
		if err := db.QueryRowContext(ctx, `
SELECT functions.prosecdef,
       pg_get_userbyid(functions.proowner),
       COALESCE(array_to_string(functions.proconfig, ','), ''),
       current_schema()
FROM pg_proc AS functions
WHERE functions.oid = $1::regprocedure`, signature).Scan(
			&securityDefiner, &owner, &config, &trustedSchema,
		); err != nil {
			t.Fatalf("read command hardening for %s: %v", signature, err)
		}
		if !securityDefiner || owner == runtimeRole || !isTrustedRewardFunctionSearchPath(config, trustedSchema) {
			t.Fatalf("command %s security_definer=%v owner=%q runtime=%q config=%q", signature, securityDefiner, owner, runtimeRole, config)
		}
	}
	for _, signature := range commandFunctions {
		var runtimeCanExecute, publicCanExecute bool
		if err := db.QueryRowContext(ctx, `
SELECT has_function_privilege($1, $2, 'EXECUTE'),
       has_function_privilege('public', $2, 'EXECUTE')`, runtimeRole, signature).Scan(
			&runtimeCanExecute,
			&publicCanExecute,
		); err != nil {
			t.Fatalf("read command privileges for %s: %v", signature, err)
		}
		if !runtimeCanExecute || publicCanExecute {
			t.Fatalf("command %s execute privileges runtime=%v public=%v", signature, runtimeCanExecute, publicCanExecute)
		}
	}
}

func TestRuntimeRoleCanUseRewardAdminServiceCommands(t *testing.T) {
	db, ownerPool := testdb.OpenWithPool(t)
	ctx := t.Context()
	ownerService := newTestRewardService(t, ownerPool, credits.NewService(ownerPool))
	createFixture := func(name, code, idempotencyKey string) CreatedRewardCode {
		t.Helper()
		created, err := ownerService.CreateRewardCode(ctx, CreateRewardCodeCommand{
			DisplayName: name, Code: code, CreditAmount: 20,
			MaxRedemptions: 2, IdempotencyKey: idempotencyKey, ActorUserID: 1,
		})
		if err != nil {
			t.Fatalf("create RewardCode fixture %q: %v", name, err)
		}
		return created
	}
	deletable := createFixture("Runtime 删除奖励", "RUNTIME-DELETE-26", "runtime-delete")
	updatable := createFixture("Runtime 更新奖励", "RUNTIME-UPDATE-26", "runtime-update")
	redeemable := createFixture("Runtime 兑换奖励", "RUNTIME-REDEEM-26", "runtime-redeem")
	provisionRewardWallet(t, ownerPool, credits.NewService(ownerPool), 3)

	runtimeRole := testdb.CreateRestrictedRole(t, db, "rewards_service_runtime")
	policies := []dbprivileges.Policy{
		database.RuntimePrivilegePolicy(), creditpostgres.RuntimePrivilegePolicy(), RuntimePrivilegePolicy(),
	}
	if err := dbprivileges.Configure(ctx, db, runtimeRole, policies...); err != nil {
		t.Fatalf("configure runtime privilege policies: %v", err)
	}

	runtimePool := testdb.OpenPoolAsRole(t, ownerPool, runtimeRole)
	runtimeService := newTestRewardService(t, runtimePool, credits.NewService(runtimePool))

	if _, err := runtimeService.DeleteRewardCode(ctx, DeleteRewardCodeCommand{
		RewardCodeID: deletable.RewardCode.ID, ActorUserID: 1,
	}); err != nil {
		t.Fatalf("delete RewardCode through runtime service: %v", err)
	}
	if _, err := runtimeService.UpdateRewardCode(ctx, UpdateRewardCodeCommand{
		RewardCodeID: updatable.RewardCode.ID, ActorUserID: 1,
		DisplayName: "Runtime 已更新奖励", CreditAmount: 30, MaxRedemptions: 3,
	}); err != nil {
		t.Fatalf("update RewardCode through runtime service: %v", err)
	}
	if _, err := runtimeService.SetRewardCodeEnabled(ctx, SetRewardCodeEnabledCommand{
		RewardCodeID: updatable.RewardCode.ID, ActorUserID: 1, Enabled: false,
	}); err != nil {
		t.Fatalf("disable RewardCode through runtime service: %v", err)
	}
	if result, err := runtimeService.Redeem(ctx, RedeemCommand{
		UserID: 3, Code: "RUNTIME-REDEEM-26",
	}); err != nil || result.Redemption.RewardCodeID != redeemable.RewardCode.ID {
		t.Fatalf("redeem RewardCode through runtime service: result=%+v error=%v", result, err)
	}
}

func isTrustedRewardFunctionSearchPath(config, schema string) bool {
	return strings.Contains(config, "search_path="+schema+", pg_catalog, pg_temp") &&
		!strings.Contains(config, "$user") && !strings.Contains(config, "public")
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
