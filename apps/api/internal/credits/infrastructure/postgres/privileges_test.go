package postgres_test

import (
	"ideaboard/api/internal/credits/testhelpers"

	"ideaboard/api/internal/credits"

	"context"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/infrastructure/postgres"
	"ideaboard/api/internal/credits/ports"
	"os"
	"strings"
	"testing"
	"time"

	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// creditShadowTableNames mirrors the Credits runtime tables for the temp-shadow
// privilege test. The privilege policy itself remains the single source for
// runtime enforcement; this list only drives temp table shadowing.
var creditShadowTableNames = []string{
	"credit_wallets",
	"credit_accounts",
	"credit_transactions",
	"credit_postings",
	"credit_holds",
	"credit_charges",
	"credit_welcome_claims",
	"credit_welcome_identity_key_guard",
	"credit_wallet_restrictions",
	"payment_dispute_debit_requests",
	"credit_reconciliation_runs",
	"credit_reconciliation_findings",
	"credit_reconciliation_run_findings",
}

func TestRuntimeRoleCanUseCreditCommandsButCannotWriteLedgerOrProjection(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	createProtectedAuthTables(t, ctx, pool)
	runtimeRole := fmt.Sprintf("credits_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := pool.Exec(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP OWNED BY `+roleSQL)
		_, _ = pool.Exec(context.Background(), `DROP ROLE IF EXISTS `+roleSQL)
	})
	policies := []dbprivileges.Policy{dbprivileges.RuntimePrivilegePolicy(), postgres.RuntimePrivilegePolicy()}
	if err := dbprivileges.Configure(ctx, pool, runtimeRole, policies...); err != nil {
		t.Fatalf("configure runtime privilege policies error = %v", err)
	}
	if err := dbprivileges.Verify(ctx, pool, policies...); err == nil {
		t.Fatal("schema owner was accepted as the API runtime role")
	}

	service := credits.NewService(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(command) error = %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE(command) error = %v", err)
	}
	result, err := service.InTx(tx).Grant(ctx, ports.GrantCommand{
		UserID:   42,
		Amount:   testhelpers.MustCreditAmount(t, 25),
		Metadata: testhelpers.TestMetadata("runtime-role:grant", "runtime-role-grant"),
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("runtime role Grant() error = %v", err)
	}
	if result.Wallet.Available != 25 {
		t.Fatalf("runtime role Grant() result = %+v", result)
	}
	restriction, err := credits.RestrictionsInTx(tx).RestrictWallet(ctx, ports.RestrictWalletCommand{
		UserID:        42,
		Type:          "runtime_test",
		Source:        "privilege_test",
		RelatedObject: domain.BusinessReference{Type: "test", ID: "runtime-role"},
		Reason:        "verify restricted write capability",
		Actor:         domain.Actor{Type: domain.ActorService, ID: "runtime-test"},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("runtime role RestrictWallet() error = %v", err)
	}
	if _, err := credits.RestrictionsInTx(tx).ReleaseWalletRestriction(ctx, ports.ReleaseWalletRestrictionCommand{
		RestrictionID: restriction.RestrictionID,
		Reason:        "verify restriction release capability",
		Actor:         domain.Actor{Type: domain.ActorService, ID: "runtime-test"},
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("runtime role ReleaseWalletRestriction() error = %v", err)
	}
	purchase, err := service.InTx(tx).Purchase(ctx, ports.PurchaseCommand{
		UserID: 42, Amount: testhelpers.MustCreditAmount(t, 5),
		Metadata: testhelpers.PurchaseMetadata("runtime-role:purchase", "runtime-role-payment-order"),
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("runtime role Purchase() error = %v", err)
	}
	if purchase.Wallet.Available != 30 || purchase.CommandType != domain.CommandPurchaseCredit {
		t.Fatalf("runtime role Purchase() result = %+v", purchase)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("runtime role command commit error = %v", err)
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
	runtimeConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("open runtime privilege verification connection error = %v", err)
	}
	defer runtimeConn.Release()
	if _, err := runtimeConn.Exec(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE(runtime verification) error = %v", err)
	}
	if err := dbprivileges.Verify(ctx, runtimeConn, policies...); err != nil {
		t.Fatalf("verify runtime privilege policies error = %v", err)
	}
	if _, err := runtimeConn.Exec(ctx, `RESET ROLE`); err != nil {
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
	assertRuntimeCreditStatementDenied(t, ctx, pool, roleSQL, `
INSERT INTO credit_wallet_restrictions (
  wallet_user_id, restriction_type, source, related_object_type, related_object_id,
  reason, created_by_actor_type, created_by_actor_id
) VALUES (42, 'bypass', 'bypass', 'test', 'bypass', 'bypass', 'service', 'bypass')`)

	assertTemporaryCreditTablesCannotShadowLedger(t, ctx, pool, roleSQL)

	var canInsertLedger bool
	var canInsertAPIUser bool
	var canInsertAuthUser bool
	var canExecuteCommand bool
	var canExecutePurchase bool
	var canCreateRestriction bool
	var canReleaseRestriction bool
	if err := pool.QueryRow(ctx, `
SELECT has_table_privilege($1, 'credit_transactions', 'INSERT'),
       has_table_privilege($1, 'users', 'INSERT'),
       has_table_privilege($1, '"user"', 'INSERT'),
       has_function_privilege(
         $1,
         'execute_credit_command_v2(text,bigint,bigint,text,text,text,text,text,text,text,text)',
         'EXECUTE'
	       ),
	   has_function_privilege(
	     $1,
	     'execute_credit_purchase_v1(bigint,bigint,text,text,text,text,text,text,text)',
	     'EXECUTE'
	       ),
       has_function_privilege(
         $1,
         'create_credit_wallet_restriction(bigint,text,text,text,text,text,text,text)',
         'EXECUTE'
       ),
       has_function_privilege(
         $1,
         'release_credit_wallet_restriction(text,text,text,text)',
         'EXECUTE'
       )`, runtimeRole).Scan(
		&canInsertLedger,
		&canInsertAPIUser,
		&canInsertAuthUser,
		&canExecuteCommand,
		&canExecutePurchase,
		&canCreateRestriction,
		&canReleaseRestriction,
	); err != nil {
		t.Fatalf("read runtime privileges error = %v", err)
	}
	if canInsertLedger || !canInsertAPIUser || canInsertAuthUser || !canExecuteCommand || !canExecutePurchase ||
		!canCreateRestriction || !canReleaseRestriction {
		t.Fatalf(
			"runtime privileges insert_ledger=%v insert_api_user=%v insert_auth_user=%v execute_command=%v execute_purchase=%v create_restriction=%v release_restriction=%v",
			canInsertLedger,
			canInsertAPIUser,
			canInsertAuthUser,
			canExecuteCommand,
			canExecutePurchase,
			canCreateRestriction,
			canReleaseRestriction,
		)
	}
	var securityDefiner bool
	var functionConfig string
	var trustedSchema string
	if err := pool.QueryRow(ctx, `
SELECT prosecdef, COALESCE(array_to_string(proconfig, ','), ''), current_schema()
FROM pg_proc
WHERE oid = 'execute_credit_command_v2(text,bigint,bigint,text,text,text,text,text,text,text,text)'::regprocedure`,
	).Scan(&securityDefiner, &functionConfig, &trustedSchema); err != nil {
		t.Fatalf("read command function security config error = %v", err)
	}
	if !securityDefiner || !isTrustedFunctionSearchPath(functionConfig, trustedSchema) {
		t.Fatalf("command function security_definer=%v config=%v", securityDefiner, functionConfig)
	}
	if err := pool.QueryRow(ctx, `
SELECT prosecdef, COALESCE(array_to_string(proconfig, ','), '')
FROM pg_proc
WHERE oid = 'execute_credit_purchase_v1(bigint,bigint,text,text,text,text,text,text,text)'::regprocedure`,
	).Scan(&securityDefiner, &functionConfig); err != nil {
		t.Fatalf("read purchase function security config error = %v", err)
	}
	if !securityDefiner || !isTrustedFunctionSearchPath(functionConfig, trustedSchema) {
		t.Fatalf("purchase function security_definer=%v config=%v", securityDefiner, functionConfig)
	}
	for _, functionName := range []string{
		"create_credit_wallet_restriction(bigint,text,text,text,text,text,text,text)",
		"release_credit_wallet_restriction(text,text,text,text)",
	} {
		var restrictionSecurityDefiner bool
		var restrictionFunctionConfig string
		if err := pool.QueryRow(ctx, `
SELECT prosecdef, COALESCE(array_to_string(proconfig, ','), '')
FROM pg_proc
WHERE oid = $1::regprocedure`, functionName).Scan(&restrictionSecurityDefiner, &restrictionFunctionConfig); err != nil {
			t.Fatalf("read %s security config error = %v", functionName, err)
		}
		if !restrictionSecurityDefiner || !isTrustedFunctionSearchPath(restrictionFunctionConfig, trustedSchema) {
			t.Fatalf("restriction function=%s security_definer=%v config=%v", functionName, restrictionSecurityDefiner, restrictionFunctionConfig)
		}
	}
	var postingFunctionConfig string
	if err := pool.QueryRow(ctx, `
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
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}) {
	t.Helper()
	for _, tableName := range []string{"user", "session", "account", "verification"} {
		if _, err := db.Exec(ctx,
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
	for _, tableName := range creditShadowTableNames {
		qualifiedTable := pgx.Identifier{trustedSchema, tableName}.Sanitize()
		tempTable := pgx.Identifier{tableName}.Sanitize()
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE `+tempTable+` (LIKE `+qualifiedTable+` INCLUDING ALL)`); err != nil {
			t.Fatalf("create temp shadow table %s error = %v", tableName, err)
		}
	}
	result, err := credits.NewService(pool).InTx(tx).Grant(ctx, ports.GrantCommand{
		UserID:   42,
		Amount:   testhelpers.MustCreditAmount(t, 7),
		Metadata: testhelpers.TestMetadata("runtime-role:temp-shadow", "runtime-role-temp-shadow"),
	})
	if err != nil {
		t.Fatalf("Grant() under temp shadow attack error = %v", err)
	}
	if result.Wallet.Available != 37 {
		t.Fatalf("Grant() under temp shadow attack result = %+v", result)
	}
	restriction, err := credits.RestrictionsInTx(tx).RestrictWallet(ctx, ports.RestrictWalletCommand{
		UserID:        42,
		Type:          "temp_shadow",
		Source:        "privilege_test",
		RelatedObject: domain.BusinessReference{Type: "test", ID: "temp-shadow"},
		Reason:        "verify trusted restriction table",
		Actor:         domain.Actor{Type: domain.ActorService, ID: "runtime-test"},
	})
	if err != nil {
		t.Fatalf("RestrictWallet() under temp shadow attack error = %v", err)
	}
	if _, err := credits.RestrictionsInTx(tx).ReleaseWalletRestriction(ctx, ports.ReleaseWalletRestrictionCommand{
		RestrictionID: restriction.RestrictionID,
		Reason:        "complete temp shadow verification",
		Actor:         domain.Actor{Type: domain.ActorService, ID: "runtime-test"},
	}); err != nil {
		t.Fatalf("ReleaseWalletRestriction() under temp shadow attack error = %v", err)
	}
	for _, tableName := range creditShadowTableNames {
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
	var trustedRestrictions int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{trustedSchema, "credit_wallet_restrictions"}.Sanitize()+
		` WHERE related_object_id = 'temp-shadow'`).Scan(&trustedRestrictions); err != nil {
		t.Fatalf("count trusted Credit Wallet restrictions error = %v", err)
	}
	if trustedRestrictions != 1 {
		t.Fatalf("trusted Credit Wallet restrictions = %d, want 1", trustedRestrictions)
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
