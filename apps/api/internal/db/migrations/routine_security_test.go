package migrations

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMigrationVerificationRejectsSecurityDefinerSearchPathDrift(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	if _, err := conn.ExecContext(ctx, `
CREATE OR REPLACE FUNCTION execute_credit_reconciliation(p_run_id BIGINT)
RETURNS VOID AS $$
BEGIN
  PERFORM p_run_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog
`); err != nil {
		t.Fatalf("replace Reconciliation routine: %v", err)
	}

	err := upWithConn(ctx, conn)
	if err == nil {
		t.Fatal("migration verification accepted an unsafe SECURITY DEFINER routine")
	}
	for _, expected := range []string{"execute_credit_reconciliation(bigint)", "search_path"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("migration verification error %q does not identify %q", err, expected)
		}
	}
}

func TestMigrationVerificationRejectsSecurityInvokerOverwrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	schemaName := currentSchema(t, ctx, conn)
	if _, err := conn.ExecContext(ctx, `
CREATE OR REPLACE FUNCTION execute_credit_reconciliation(p_run_id BIGINT)
RETURNS VOID AS $$
BEGIN
  PERFORM p_run_id;
END;
$$ LANGUAGE plpgsql SECURITY INVOKER;
ALTER FUNCTION execute_credit_reconciliation(BIGINT)
  SET search_path TO `+quoteIdentifier(schemaName)+`, pg_catalog, pg_temp
`); err != nil {
		t.Fatalf("replace Reconciliation routine: %v", err)
	}

	err := upWithConn(ctx, conn)
	if err == nil {
		t.Fatal("migration verification accepted a SECURITY INVOKER overwrite")
	}
	for _, expected := range []string{"execute_credit_reconciliation(bigint)", "SECURITY DEFINER"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("migration verification error %q does not identify %q", err, expected)
		}
	}
}

func TestReconciliationRoutineRebuildKeepsSecurityInvariant(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const (
		file    = "20260729030000_reconciliation_task_fact_risk.sql"
		version = 20260729030000
	)
	rollbackTxMigrationForTest(t, ctx, conn, file, version)
	if err := verifySecurityDefinerRoutines(ctx, conn); err != nil {
		t.Fatalf("rolled-back routine violates security policy: %v", err)
	}

	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("reapply migration: %v", err)
	}
}

func TestMigrationReplayExecutesReconciliationRoutine(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var runID int64
	var enqueueRequired bool
	if err := conn.QueryRowContext(ctx, `
SELECT run_id, enqueue_required
FROM start_credit_reconciliation('manual', 1, 'migration replay verification', NULL, FALSE)
`).Scan(&runID, &enqueueRequired); err != nil {
		t.Fatalf("start Reconciliation run: %v", err)
	}
	if !enqueueRequired {
		t.Fatal("new Reconciliation run must require execution")
	}
	if _, err := conn.ExecContext(ctx, `SELECT execute_credit_reconciliation($1)`, runID); err != nil {
		t.Fatalf("execute Reconciliation routine: %v", err)
	}

	var status string
	if err := conn.QueryRowContext(ctx, `
SELECT status
FROM credit_reconciliation_runs
WHERE id = $1
`, runID).Scan(&status); err != nil {
		t.Fatalf("read Reconciliation run: %v", err)
	}
	if status != "completed" {
		t.Fatalf("Reconciliation run status=%q, want completed", status)
	}
}

func TestMigrationVerificationRejectsPublicRoutineExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	if _, err := conn.ExecContext(ctx, `
GRANT EXECUTE ON FUNCTION execute_credit_reconciliation(BIGINT) TO PUBLIC
`); err != nil {
		t.Fatalf("grant PUBLIC routine execution: %v", err)
	}

	err := upWithConn(ctx, conn)
	if err == nil {
		t.Fatal("migration verification accepted PUBLIC routine execution")
	}
	for _, expected := range []string{"execute_credit_reconciliation(bigint)", "PUBLIC"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("migration verification error %q does not identify %q", err, expected)
		}
	}
}

func TestMigrationReplaySecurityDefinerPolicyIsComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	rows, err := conn.QueryContext(ctx, `
SELECT functions.proname || '(' || pg_catalog.oidvectortypes(functions.proargtypes) || ')'
FROM pg_catalog.pg_proc AS functions
JOIN pg_catalog.pg_namespace AS schemas ON schemas.oid = functions.pronamespace
WHERE schemas.nspname = current_schema()
  AND functions.prosecdef
ORDER BY 1
`)
	if err != nil {
		t.Fatalf("inspect SECURITY DEFINER routines: %v", err)
	}
	defer rows.Close()

	var actual []string
	for rows.Next() {
		var signature string
		if err := rows.Scan(&signature); err != nil {
			t.Fatalf("read SECURITY DEFINER routine signature: %v", err)
		}
		actual = append(actual, signature)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("inspect SECURITY DEFINER routines: %v", err)
	}

	if !slices.Equal(actual, securityDefinerRoutineSignatures) {
		t.Fatalf(
			"SECURITY DEFINER policy=%v, migrated routines=%v",
			securityDefinerRoutineSignatures,
			actual,
		)
	}
}
