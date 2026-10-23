package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestRuntimeRoleCanUseTypedBillingCommandsButCannotMutateCatalogDirectly(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	runtimeRole := fmt.Sprintf("billing_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
	policies := []dbprivileges.Policy{dbprivileges.RuntimePrivilegePolicy(), RuntimePrivilegePolicy()}
	if err := dbprivileges.Configure(ctx, db, runtimeRole, policies...); err != nil {
		t.Fatalf("configure runtime privilege policies error = %v", err)
	}
	if err := dbprivileges.Verify(ctx, db, policies...); err == nil {
		t.Fatal("schema owner was accepted as Billing runtime role")
	}

	conn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("open runtime connection: %v", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanupCtx, `RESET ROLE`); err != nil {
			t.Errorf("RESET ROLE: %v", err)
		}
		conn.Release()
	}()
	if _, err := conn.Exec(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE: %v", err)
	}
	if err := dbprivileges.Verify(ctx, conn, policies...); err != nil {
		t.Fatalf("verify runtime privilege policies error = %v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT * FROM publish_billing_prices($1, $2, $3, $4::jsonb, $5)`, "test:runtime", "Runtime", "", `{"1k": 0}`, 1); err != nil {
		t.Fatalf("typed free price command: %v", err)
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM billing_skus WHERE sku = 'test:runtime'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("runtime SELECT count=%d error=%v", count, err)
	}

	for _, statement := range []string{
		`INSERT INTO billing_skus (sku, display_name, enabled, created_by, updated_by) VALUES ('test:bypass', 'Bypass', TRUE, 1, 1)`,
		`UPDATE billing_price_versions SET unit_credits = 99 WHERE sku = 'test:runtime'`,
		`DELETE FROM billing_skus WHERE sku = 'test:runtime'`,
		`SELECT * FROM billing_admin_actions`,
		`INSERT INTO billing_admin_actions (action_type, sku, price_version_id, actor_id) SELECT 'set_availability', sku, current_price_version_id, 1 FROM billing_skus WHERE sku = 'test:runtime'`,
		`SELECT prevent_billing_price_version_mutation()`,
	} {
		if _, err := conn.Exec(ctx, statement); err == nil {
			t.Fatalf("runtime bypass unexpectedly succeeded: %s", statement)
		}
	}

	for _, signature := range billingCommandFunctionSignatures {
		var securityDefiner bool
		var config string
		var trustedSchema string
		if err := db.QueryRow(ctx, `
SELECT prosecdef, COALESCE(array_to_string(proconfig, ','), ''), current_schema()
FROM pg_proc WHERE oid = $1::regprocedure`, signature).Scan(&securityDefiner, &config, &trustedSchema); err != nil {
			t.Fatalf("read function config for %s: %v", signature, err)
		}
		if !securityDefiner || !strings.Contains(config, "search_path="+trustedSchema+", pg_catalog, pg_temp") || strings.Contains(config, "$user") || strings.Contains(config, "public") {
			t.Fatalf("function %s security_definer=%v config=%q", signature, securityDefiner, config)
		}
		var publicCanExecute bool
		if err := db.QueryRow(ctx, `SELECT has_function_privilege('public', $1, 'EXECUTE')`, signature).Scan(&publicCanExecute); err != nil {
			t.Fatalf("read PUBLIC privilege for %s: %v", signature, err)
		}
		if publicCanExecute {
			t.Fatalf("PUBLIC can execute %s", signature)
		}
	}
}
