package credits

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var creditTableNames = []string{
	"credit_wallets",
	"credit_accounts",
	"credit_transactions",
	"credit_postings",
	"credit_holds",
	"credit_charges",
	"credit_welcome_claims",
	"credit_welcome_identity_key_guard",
	"credit_reconciliation_runs",
	"credit_reconciliation_findings",
	"credit_reconciliation_run_findings",
	"credit_reconciliation_alerts",
}

var creditInternalFunctionSignatures = []string{
	"prevent_credit_ledger_mutation()",
	"guard_credit_account_projection()",
	"apply_credit_posting_balances()",
	"validate_credit_transaction_balance()",
	"guard_credit_posting_transaction_scope()",
}

type runtimePrivilegeQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const (
	creditCommandFunctionSignature = "execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT)"
)

var creditLifecycleFunctionSignatures = []string{
	"bind_credit_welcome_identity_key(TEXT)",
	"provision_credit_wallet(BIGINT)",
	"set_credit_wallet_status(BIGINT, TEXT)",
	"claim_credit_welcome(TEXT, BIGINT)",
}

var creditReconciliationFunctionSignatures = []string{
	"start_credit_reconciliation(BIGINT, BIGINT, TEXT)",
	"execute_credit_reconciliation(BIGINT)",
	"fail_credit_reconciliation(BIGINT, TEXT)",
}

type privilegeDBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func ConfigureRuntimePrivileges(ctx context.Context, db privilegeDBTX, runtimeRole string) error {
	if db == nil {
		return fmt.Errorf("configure Credits privileges: database is nil")
	}
	runtimeRole = strings.TrimSpace(runtimeRole)
	if runtimeRole == "" {
		return fmt.Errorf("configure Credits privileges: runtime role is empty")
	}
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	tableSQL := make([]string, len(creditTableNames))
	for index, tableName := range creditTableNames {
		tableSQL[index] = pgx.Identifier{tableName}.Sanitize()
	}
	joinedTables := strings.Join(tableSQL, ", ")
	statements := []string{
		`REVOKE INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON ` + joinedTables + ` FROM ` + roleSQL,
		`GRANT SELECT ON ` + joinedTables + ` TO ` + roleSQL,
	}
	for _, functionSignature := range creditInternalFunctionSignatures {
		statements = append(statements, `REVOKE EXECUTE ON FUNCTION `+functionSignature+` FROM PUBLIC`)
		statements = append(statements, `REVOKE EXECUTE ON FUNCTION `+functionSignature+` FROM `+roleSQL)
	}
	statements = append(statements, `REVOKE EXECUTE ON FUNCTION `+creditCommandFunctionSignature+` FROM PUBLIC`)
	statements = append(statements, `GRANT EXECUTE ON FUNCTION `+creditCommandFunctionSignature+` TO `+roleSQL)
	for _, functionSignature := range creditLifecycleFunctionSignatures {
		statements = append(statements, `REVOKE EXECUTE ON FUNCTION `+functionSignature+` FROM PUBLIC`)
		statements = append(statements, `GRANT EXECUTE ON FUNCTION `+functionSignature+` TO `+roleSQL)
	}
	for _, functionSignature := range creditReconciliationFunctionSignatures {
		statements = append(statements, `REVOKE EXECUTE ON FUNCTION `+functionSignature+` FROM PUBLIC`)
		statements = append(statements, `GRANT EXECUTE ON FUNCTION `+functionSignature+` TO `+roleSQL)
	}

	rows, err := db.QueryContext(ctx, `
SELECT sequence_name
FROM information_schema.sequences
WHERE sequence_schema = current_schema()
  AND sequence_name LIKE 'credit\_%' ESCAPE '\'`)
	if err != nil {
		return fmt.Errorf("configure Credits privileges: list sequences: %w", err)
	}
	var sequenceSQL []string
	for rows.Next() {
		var sequenceName string
		if err := rows.Scan(&sequenceName); err != nil {
			_ = rows.Close()
			return fmt.Errorf("configure Credits privileges: scan sequence: %w", err)
		}
		sequenceSQL = append(sequenceSQL, pgx.Identifier{sequenceName}.Sanitize())
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("configure Credits privileges: close sequence rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("configure Credits privileges: read sequences: %w", err)
	}
	if len(sequenceSQL) > 0 {
		statements = append(statements, `REVOKE ALL PRIVILEGES ON SEQUENCE `+strings.Join(sequenceSQL, ", ")+` FROM `+roleSQL)
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure Credits privileges: %w", err)
		}
	}
	return nil
}

func VerifyRuntimePrivileges(ctx context.Context, db runtimePrivilegeQuerier) error {
	if db == nil {
		return fmt.Errorf("verify Credits runtime privileges: database is nil")
	}
	var currentUser string
	var canExecuteCommand bool
	err := db.QueryRowContext(ctx, `
SELECT current_user,
       has_function_privilege(
         current_user,
         'execute_credit_command_v2(text,bigint,bigint,text,text,text,text,text,text,text,text)',
         'EXECUTE'
       )`).Scan(&currentUser, &canExecuteCommand)
	if err != nil {
		return fmt.Errorf("verify Credits runtime privileges: %w", err)
	}
	if !canExecuteCommand {
		return fmt.Errorf("verify Credits runtime privileges: role %q cannot execute typed Credit Commands", currentUser)
	}
	for _, functionSignature := range creditLifecycleFunctionSignatures {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_function_privilege(current_user, $1, 'EXECUTE')`, functionSignature).Scan(&allowed); err != nil {
			return fmt.Errorf("verify Credits runtime privileges: check lifecycle function %s: %w", functionSignature, err)
		}
		if !allowed {
			return fmt.Errorf("verify Credits runtime privileges: role %q cannot execute wallet lifecycle function %s", currentUser, functionSignature)
		}
	}
	for _, functionSignature := range creditReconciliationFunctionSignatures {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_function_privilege(current_user, $1, 'EXECUTE')`, functionSignature).Scan(&allowed); err != nil {
			return fmt.Errorf("verify Credits runtime privileges: check reconciliation function %s: %w", functionSignature, err)
		}
		if !allowed {
			return fmt.Errorf("verify Credits runtime privileges: role %q cannot execute reconciliation function %s", currentUser, functionSignature)
		}
	}
	for _, tableName := range creditTableNames {
		for _, privilege := range []string{"INSERT", "UPDATE", "DELETE", "TRUNCATE", "TRIGGER"} {
			var allowed bool
			if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, tableName, privilege).Scan(&allowed); err != nil {
				return fmt.Errorf("verify Credits runtime privileges: check %s %s: %w", tableName, privilege, err)
			}
			if allowed {
				return fmt.Errorf("verify Credits runtime privileges: role %q has forbidden %s on %s", currentUser, privilege, tableName)
			}
		}
	}
	for _, functionSignature := range creditInternalFunctionSignatures {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_function_privilege(current_user, $1, 'EXECUTE')`, functionSignature).Scan(&allowed); err != nil {
			return fmt.Errorf("verify Credits runtime privileges: check function %s: %w", functionSignature, err)
		}
		if allowed {
			return fmt.Errorf("verify Credits runtime privileges: role %q can execute internal function %s", currentUser, functionSignature)
		}
	}
	return nil
}
