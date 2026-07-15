package taskbilling

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

func TestRuntimeRoleCanPersistTaskBillingFactsWithoutBypassingGuards(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	runtimeRole := fmt.Sprintf("task_billing_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
		t.Fatalf("configure runtime privilege policies error = %v", err)
	}
	if err := dbprivileges.Verify(ctx, db, policies...); err == nil {
		t.Fatal("schema owner was accepted as Task Billing runtime role")
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open runtime connection: %v", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanupCtx, `RESET ROLE`); err != nil {
			t.Errorf("RESET ROLE: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Errorf("close runtime connection: %v", err)
		}
	}()
	if _, err := conn.ExecContext(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE: %v", err)
	}
	if err := dbprivileges.Verify(ctx, conn, policies...); err != nil {
		t.Fatalf("verify runtime privilege policies error = %v", err)
	}
	for _, privilege := range []string{"SELECT", "INSERT"} {
		var allowed bool
		if err := conn.QueryRowContext(ctx, `
SELECT has_column_privilege(current_user, 'task_billing_snapshots', 'display_name', $1)`, privilege).Scan(&allowed); err != nil {
			t.Fatalf("check display_name %s privilege: %v", privilege, err)
		}
		if !allowed {
			t.Fatalf("runtime role is missing display_name %s privilege", privilege)
		}
	}
	for _, statement := range []string{
		`UPDATE task_billing_snapshots SET total_credits = total_credits`,
		`DELETE FROM task_billing_snapshots`,
		`DELETE FROM task_billing_settlements`,
		`SELECT validate_task_billing_snapshot_insert()`,
		`SELECT prevent_task_billing_snapshot_mutation()`,
		`SELECT guard_task_billing_settlement_transition()`,
	} {
		if _, err := conn.ExecContext(ctx, statement); err == nil {
			t.Fatalf("runtime bypass unexpectedly succeeded: %s", statement)
		}
	}
}
