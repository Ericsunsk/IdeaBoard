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
	emptyDraft := createRewardDraft(t, service, "可删除空 Draft")
	issuedCampaign, _ := createActivePublicCampaign(t, service, "RUNTIME26", 20, 2)

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

	if _, err := conn.ExecContext(ctx, `DELETE FROM reward_campaigns WHERE id = $1`, emptyDraft.ID); err != nil {
		t.Fatalf("delete empty Draft through guarded runtime privilege: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE reward_campaigns SET internal_note = 'runtime note' WHERE id = $1`, issuedCampaign.ID); err != nil {
		t.Fatalf("update mutable internal note through guarded runtime privilege: %v", err)
	}

	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_campaigns SET credit_amount = credit_amount + 1 WHERE id = $1`, issuedCampaign.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`DELETE FROM reward_campaigns WHERE id = $1`, issuedCampaign.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`UPDATE reward_codes SET code_hint = 'ABCD' WHERE campaign_id = $1`, issuedCampaign.ID)
	assertRuntimeRewardStatementDenied(t, ctx, conn,
		`DELETE FROM reward_codes WHERE campaign_id = $1`, issuedCampaign.ID)

	if _, err := conn.ExecContext(ctx, `
UPDATE reward_codes
SET revoked_at = clock_timestamp(), revoked_by = 1
WHERE campaign_id = $1`, issuedCampaign.ID); err != nil {
		t.Fatalf("revoke Code through guarded runtime privilege: %v", err)
	}
	assertRuntimeRewardStatementDenied(t, ctx, conn, `
UPDATE reward_codes
SET revoked_at = clock_timestamp(), revoked_by = 1
WHERE campaign_id = $1`, issuedCampaign.ID)

	for _, signature := range []string{
		"prevent_reward_admin_action_mutation()",
		"guard_reward_campaign_rule_mutation()",
		"guard_reward_code_mutation()",
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
