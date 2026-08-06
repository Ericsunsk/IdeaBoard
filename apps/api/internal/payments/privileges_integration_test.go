package payments

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"ideaboard/api/internal/platform/database"
	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestRuntimeRoleCanUsePaymentsTablesWithoutMutatingAuditOrDeletingConfiguration(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	runtimeRole := fmt.Sprintf("payments_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.ExecContext(cleanupCtx, `DROP OWNED BY `+roleSQL); err != nil {
			t.Errorf("drop runtime role ownership: %v", err)
		}
		if _, err := db.ExecContext(cleanupCtx, `DROP ROLE IF EXISTS `+roleSQL); err != nil {
			t.Errorf("drop runtime role: %v", err)
		}
	})

	policies := []dbprivileges.Policy{database.RuntimePrivilegePolicy(), RuntimePrivilegePolicy()}
	if err := dbprivileges.Configure(ctx, db, runtimeRole, policies...); err != nil {
		t.Fatalf("configure runtime privilege policies: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open runtime connection: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close runtime connection: %v", err)
		}
	}()
	if _, err := conn.ExecContext(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("set runtime role: %v", err)
	}
	defer func() {
		if _, err := conn.ExecContext(context.Background(), `RESET ROLE`); err != nil {
			t.Errorf("reset runtime role: %v", err)
		}
	}()
	if err := dbprivileges.Verify(ctx, conn, policies...); err != nil {
		t.Fatalf("verify runtime privilege policies: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT 1 FROM payment_checkout_shutdowns LIMIT 0`); err != nil {
		t.Fatalf("read payment checkout shutdowns through runtime role: %v", err)
	}

	if _, err := conn.ExecContext(ctx, `
INSERT INTO payment_admin_actions (
  action_type, actor_id, target_type, target_id, outcome
) VALUES ('configure_stripe', 1, 'stripe_configuration', 'current', 'failed')`); err != nil {
		t.Fatalf("insert payment audit through runtime role: %v", err)
	}
	for _, statement := range []string{
		`UPDATE payment_admin_actions SET outcome = 'succeeded'`,
		`DELETE FROM payment_admin_actions`,
		`DELETE FROM payment_stripe_configurations`,
		`DELETE FROM payment_customers`,
		`DELETE FROM payment_orders`,
		`TRUNCATE payment_webhook_events`,
		`SELECT prevent_payment_admin_action_mutation()`,
		`SELECT guard_payment_order_lifecycle()`,
	} {
		if _, err := conn.ExecContext(ctx, statement); err == nil {
			t.Fatalf("runtime bypass unexpectedly succeeded: %s", statement)
		}
	}
}
