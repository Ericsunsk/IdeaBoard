package credits

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/database"
	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRuntimeRoleCanUseCreditCommandsButCannotWriteLedgerOrProjection(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	createProtectedAuthTables(t, ctx, db)
	runtimeRole := fmt.Sprintf("credits_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP OWNED BY `+roleSQL)
		_, _ = db.ExecContext(context.Background(), `DROP ROLE IF EXISTS `+roleSQL)
	})
	policies := []dbprivileges.Policy{database.RuntimePrivilegePolicy(), RuntimePrivilegePolicy()}
	if err := dbprivileges.Configure(ctx, db, runtimeRole, policies...); err != nil {
		t.Fatalf("configure runtime privilege policies error = %v", err)
	}
	if err := dbprivileges.Verify(ctx, db, policies...); err == nil {
		t.Fatal("schema owner was accepted as the API runtime role")
	}

	service := NewService(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(command) error = %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE(command) error = %v", err)
	}
	result, err := service.InTx(tx).Grant(ctx, GrantCommand{
		UserID:   42,
		Amount:   mustCreditAmount(t, 25),
		Metadata: testMetadata("runtime-role:grant", "runtime-role-grant"),
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("runtime role Grant() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("runtime role command commit error = %v", err)
	}
	if result.Wallet.Available != 25 {
		t.Fatalf("runtime role Grant() result = %+v", result)
	}
	denialTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(deduction denial) error = %v", err)
	}
	defer denialTx.Rollback(ctx)
	if _, err := denialTx.Exec(ctx, `SET LOCAL ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE(deduction denial) error = %v", err)
	}
	var ignored string
	err = denialTx.QueryRow(ctx, `
SELECT transaction_id FROM execute_credit_command_v2(
  'deduction', 42, 1, '', 'runtime-role:non-admin-deduction', 'service', 'runtime',
  'runtime_test', 'must be rejected', 'test', 'runtime-role-non-admin-deduction'
)`).Scan(&ignored)
	if err == nil || !strings.Contains(err.Error(), "invalid credit command") {
		t.Fatalf("runtime direct non-admin deduction error = %v, want invalid credit command", err)
	}
	runtimeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open runtime privilege verification connection error = %v", err)
	}
	defer runtimeConn.Close()
	if _, err := runtimeConn.ExecContext(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE(runtime verification) error = %v", err)
	}
	if err := dbprivileges.Verify(ctx, runtimeConn, policies...); err != nil {
		t.Fatalf("verify runtime privilege policies error = %v", err)
	}
	if _, err := runtimeConn.ExecContext(ctx, `RESET ROLE`); err != nil {
		t.Fatalf("RESET ROLE(runtime verification) error = %v", err)
	}

	assertRuntimeCreditStatementDenied(t, ctx, pool, roleSQL,
		`UPDATE credit_accounts SET balance = balance + 1 WHERE wallet_user_id = 42`)
	assertRuntimeCreditStatementDenied(t, ctx, pool, roleSQL,
		`INSERT INTO credit_postings (transaction_id, account_id, amount) VALUES ('ctx_missing', 1, 1)`)
	assertRuntimeCreditStatementDenied(t, ctx, pool, roleSQL,
		`INSERT INTO credit_holds (hold_transaction_id, wallet_user_id, amount) VALUES ('ctx_missing', 42, 1)`)
	assertRuntimeCreditStatementDenied(t, ctx, pool, roleSQL, `
INSERT INTO credit_transactions (
  transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type,
  business_reference_id, command_amount, posting_count,
  available_balance_after, reserved_balance_after
)
VALUES ('ctx_runtime_bypass', 42, 'grant', 'runtime-role:bypass', repeat('0', 64),
        'admin', 'spoofed', 'bypass', 'bypass', 'test', 'bypass', 1, 2, 26, 0)`)
	for _, tableName := range []string{"user", "session", "account", "verification"} {
		assertRuntimeCreditStatementDenied(t, ctx, pool, roleSQL,
			`INSERT INTO `+pgx.Identifier{tableName}.Sanitize()+` (id) VALUES ('api-bypass')`)
	}
	assertRuntimeCreditStatementDenied(t, ctx, pool, roleSQL,
		`DELETE FROM user_deletion_requests WHERE user_id = 42`)

	assertTemporaryCreditTablesCannotShadowLedger(t, ctx, pool, roleSQL)

	var canInsertLedger bool
	var canInsertAPIUser bool
	var canInsertAuthUser bool
	var canExecuteCommand bool
	if err := db.QueryRowContext(ctx, `
SELECT has_table_privilege($1, 'credit_transactions', 'INSERT'),
       has_table_privilege($1, 'users', 'INSERT'),
       has_table_privilege($1, '"user"', 'INSERT'),
       has_function_privilege(
         $1,
         'execute_credit_command_v2(text,bigint,bigint,text,text,text,text,text,text,text,text)',
         'EXECUTE'
	       )`, runtimeRole).Scan(&canInsertLedger, &canInsertAPIUser, &canInsertAuthUser, &canExecuteCommand); err != nil {
		t.Fatalf("read runtime privileges error = %v", err)
	}
	if canInsertLedger || !canInsertAPIUser || canInsertAuthUser || !canExecuteCommand {
		t.Fatalf(
			"runtime privileges insert_ledger=%v insert_api_user=%v insert_auth_user=%v execute_command=%v",
			canInsertLedger,
			canInsertAPIUser,
			canInsertAuthUser,
			canExecuteCommand,
		)
	}
	var securityDefiner bool
	var functionConfig string
	var trustedSchema string
	if err := db.QueryRowContext(ctx, `
SELECT prosecdef, COALESCE(array_to_string(proconfig, ','), ''), current_schema()
FROM pg_proc
WHERE oid = 'execute_credit_command_v2(text,bigint,bigint,text,text,text,text,text,text,text,text)'::regprocedure`,
	).Scan(&securityDefiner, &functionConfig, &trustedSchema); err != nil {
		t.Fatalf("read command function security config error = %v", err)
	}
	if !securityDefiner || !isTrustedFunctionSearchPath(functionConfig, trustedSchema) {
		t.Fatalf("command function security_definer=%v config=%v", securityDefiner, functionConfig)
	}
	var postingFunctionConfig string
	if err := db.QueryRowContext(ctx, `
SELECT COALESCE(array_to_string(proconfig, ','), '')
FROM pg_proc
WHERE oid = 'apply_credit_posting_balances()'::regprocedure`,
	).Scan(&postingFunctionConfig); err != nil {
		t.Fatalf("read posting function security config error = %v", err)
	}
	if !isTrustedFunctionSearchPath(postingFunctionConfig, trustedSchema) {
		t.Fatalf("posting function config=%v", postingFunctionConfig)
	}
}

func isTrustedFunctionSearchPath(config string, schemaName string) bool {
	return strings.Contains(config, "search_path="+schemaName+", pg_catalog, pg_temp") &&
		!strings.Contains(config, "$user") &&
		!strings.Contains(config, "public")
}

func createProtectedAuthTables(t *testing.T, ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}) {
	t.Helper()
	for _, tableName := range []string{"user", "session", "account", "verification"} {
		if _, err := db.ExecContext(ctx,
			`CREATE TABLE `+pgx.Identifier{tableName}.Sanitize()+` (id TEXT PRIMARY KEY)`); err != nil {
			t.Fatalf("create protected Auth table %s error = %v", tableName, err)
		}
	}
}

func assertTemporaryCreditTablesCannotShadowLedger(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	roleSQL string,
) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(temp shadow attack) error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE(temp shadow attack) error = %v", err)
	}
	var trustedSchema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&trustedSchema); err != nil {
		t.Fatalf("read trusted schema error = %v", err)
	}
	for _, tableName := range creditTableNames {
		qualifiedTable := pgx.Identifier{trustedSchema, tableName}.Sanitize()
		tempTable := pgx.Identifier{tableName}.Sanitize()
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE `+tempTable+` (LIKE `+qualifiedTable+` INCLUDING ALL)`); err != nil {
			t.Fatalf("create temp shadow table %s error = %v", tableName, err)
		}
	}
	result, err := NewService(pool).InTx(tx).Grant(ctx, GrantCommand{
		UserID:   42,
		Amount:   mustCreditAmount(t, 7),
		Metadata: testMetadata("runtime-role:temp-shadow", "runtime-role-temp-shadow"),
	})
	if err != nil {
		t.Fatalf("Grant() under temp shadow attack error = %v", err)
	}
	if result.Wallet.Available != 32 {
		t.Fatalf("Grant() under temp shadow attack result = %+v", result)
	}
	for _, tableName := range creditTableNames {
		var rowCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_temp.`+pgx.Identifier{tableName}.Sanitize()).Scan(&rowCount); err != nil {
			t.Fatalf("count temp shadow table %s error = %v", tableName, err)
		}
		if rowCount != 0 {
			t.Fatalf("temp shadow table %s was written: rows=%d", tableName, rowCount)
		}
	}
	var trustedTransactions int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{trustedSchema, "credit_transactions"}.Sanitize()+
		` WHERE idempotency_key = 'runtime-role:temp-shadow'`).Scan(&trustedTransactions); err != nil {
		t.Fatalf("count trusted Credit transaction error = %v", err)
	}
	if trustedTransactions != 1 {
		t.Fatalf("trusted Credit transactions = %d, want 1", trustedTransactions)
	}
}

func assertRuntimeCreditStatementDenied(
	t *testing.T,
	ctx context.Context,
	pool interface {
		Begin(context.Context) (pgx.Tx, error)
	},
	roleSQL string,
	statement string,
) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(denied statement) error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE(denied statement) error = %v", err)
	}
	if _, err := tx.Exec(ctx, statement); err == nil {
		t.Fatalf("runtime role statement unexpectedly succeeded: %s", statement)
	}
}
