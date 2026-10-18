package postgres_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"

	"ideaboard/api/internal/payments/infrastructure/postgres"
)

func TestRuntimePrivilegePolicyIncludesPaymentCheckoutShutdowns(t *testing.T) {
	policy := postgres.RuntimePrivilegePolicy()
	for _, rule := range policy.Objects {
		if rule.Kind != dbprivileges.KindTable || !slices.Contains(rule.Names, "payment_checkout_shutdowns") {
			continue
		}
		for _, required := range []dbprivileges.Privilege{
			dbprivileges.PrivilegeSelect,
			dbprivileges.PrivilegeInsert,
			dbprivileges.PrivilegeUpdate,
		} {
			if !slices.Contains(rule.Required, required) {
				t.Fatalf("payment checkout shutdowns policy lacks required privilege %s", required)
			}
		}
		for _, forbidden := range []dbprivileges.Privilege{
			dbprivileges.PrivilegeDelete,
			dbprivileges.PrivilegeTruncate,
			dbprivileges.PrivilegeReferences,
			dbprivileges.PrivilegeTrigger,
		} {
			if !slices.Contains(rule.Forbidden, forbidden) {
				t.Fatalf("payment checkout shutdowns policy lacks forbidden privilege %s", forbidden)
			}
		}
		return
	}
	t.Fatal("payment checkout shutdowns table is missing from the Payments runtime privilege policy")
}

func TestRuntimeRoleCanUsePaymentsTablesWithoutMutatingAuditOrDeletingConfiguration(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	runtimeRole := fmt.Sprintf("payments_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.Exec(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.Exec(cleanupCtx, `DROP OWNED BY `+roleSQL); err != nil {
			t.Errorf("drop runtime role ownership: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DROP ROLE IF EXISTS `+roleSQL); err != nil {
			t.Errorf("drop runtime role: %v", err)
		}
	})

	policies := []dbprivileges.Policy{dbprivileges.RuntimePrivilegePolicy(), postgres.RuntimePrivilegePolicy()}
	if err := dbprivileges.Configure(ctx, db, runtimeRole, policies...); err != nil {
		t.Fatalf("configure runtime privilege policies: %v", err)
	}
	conn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("open runtime connection: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("set runtime role: %v", err)
	}
	defer func() {
		if _, err := conn.Exec(context.Background(), `RESET ROLE`); err != nil {
			t.Errorf("reset runtime role: %v", err)
		}
	}()
	if err := dbprivileges.Verify(ctx, conn, policies...); err != nil {
		t.Fatalf("verify runtime privilege policies: %v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT 1 FROM payment_checkout_shutdowns LIMIT 0`); err != nil {
		t.Fatalf("read payment checkout shutdowns through runtime role: %v", err)
	}

	if _, err := conn.Exec(ctx, `
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
		if _, err := conn.Exec(ctx, statement); err == nil {
			t.Fatalf("runtime bypass unexpectedly succeeded: %s", statement)
		}
	}
}
