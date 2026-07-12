package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var billingTableNames = []string{
	"billing_skus",
	"billing_price_versions",
	"billing_price_version_states",
}

var billingNoDirectAccessTableNames = []string{
	"billing_admin_actions",
}

var billingCommandFunctionSignatures = []string{
	"create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)",
	"update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)",
	"create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT)",
	"set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT)",
}

var billingInternalFunctionSignatures = []string{
	"prevent_billing_sku_identity_mutation()",
	"prevent_billing_price_version_mutation()",
	"prevent_billing_price_state_delete()",
	"prevent_billing_admin_action_mutation()",
}

type privilegeDBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type runtimePrivilegeQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ConfigureRuntimePrivileges(ctx context.Context, db privilegeDBTX, runtimeRole string) error {
	if db == nil {
		return fmt.Errorf("configure Billing privileges: database is nil")
	}
	runtimeRole = strings.TrimSpace(runtimeRole)
	if runtimeRole == "" {
		return fmt.Errorf("configure Billing privileges: runtime role is empty")
	}
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	tables := make([]string, len(billingTableNames))
	for index, tableName := range billingTableNames {
		tables[index] = pgx.Identifier{tableName}.Sanitize()
	}
	joinedTables := strings.Join(tables, ", ")
	statements := []string{
		`REVOKE INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON ` + joinedTables + ` FROM ` + roleSQL,
		`GRANT SELECT ON ` + joinedTables + ` TO ` + roleSQL,
	}
	for _, tableName := range billingNoDirectAccessTableNames {
		statements = append(statements, `REVOKE ALL PRIVILEGES ON TABLE `+pgx.Identifier{tableName}.Sanitize()+` FROM `+roleSQL)
	}
	for _, functionSignature := range billingInternalFunctionSignatures {
		statements = append(statements,
			`REVOKE EXECUTE ON FUNCTION `+functionSignature+` FROM PUBLIC`,
			`REVOKE EXECUTE ON FUNCTION `+functionSignature+` FROM `+roleSQL,
		)
	}
	for _, functionSignature := range billingCommandFunctionSignatures {
		statements = append(statements,
			`REVOKE EXECUTE ON FUNCTION `+functionSignature+` FROM PUBLIC`,
			`GRANT EXECUTE ON FUNCTION `+functionSignature+` TO `+roleSQL,
		)
	}
	rows, err := db.QueryContext(ctx, `
SELECT sequence_name
FROM information_schema.sequences
WHERE sequence_schema = current_schema()
  AND sequence_name LIKE 'billing\_%' ESCAPE '\'`)
	if err != nil {
		return fmt.Errorf("configure Billing privileges: list sequences: %w", err)
	}
	var sequences []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			scanErr := fmt.Errorf("configure Billing privileges: scan sequence: %w", err)
			if closeErr := rows.Close(); closeErr != nil {
				return errors.Join(scanErr, fmt.Errorf("configure Billing privileges: close sequence rows: %w", closeErr))
			}
			return scanErr
		}
		sequences = append(sequences, pgx.Identifier{name}.Sanitize())
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("configure Billing privileges: close sequence rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("configure Billing privileges: read sequences: %w", err)
	}
	if len(sequences) > 0 {
		statements = append(statements, `REVOKE ALL PRIVILEGES ON SEQUENCE `+strings.Join(sequences, ", ")+` FROM `+roleSQL)
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure Billing privileges: %w", err)
		}
	}
	return nil
}

func VerifyRuntimePrivileges(ctx context.Context, db runtimePrivilegeQuerier) error {
	if db == nil {
		return fmt.Errorf("verify Billing runtime privileges: database is nil")
	}
	var currentUser string
	if err := db.QueryRowContext(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		return fmt.Errorf("verify Billing runtime privileges: %w", err)
	}
	for _, tableName := range billingTableNames {
		var canSelect bool
		if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, 'SELECT')`, tableName).Scan(&canSelect); err != nil {
			return fmt.Errorf("verify Billing runtime privileges: check SELECT on %s: %w", tableName, err)
		}
		if !canSelect {
			return fmt.Errorf("verify Billing runtime privileges: role %q cannot read %s", currentUser, tableName)
		}
		for _, privilege := range []string{"INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
			var allowed bool
			if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, tableName, privilege).Scan(&allowed); err != nil {
				return fmt.Errorf("verify Billing runtime privileges: check %s on %s: %w", privilege, tableName, err)
			}
			if allowed {
				return fmt.Errorf("verify Billing runtime privileges: role %q has forbidden %s on %s", currentUser, privilege, tableName)
			}
		}
	}
	for _, tableName := range billingNoDirectAccessTableNames {
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
			var allowed bool
			if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, tableName, privilege).Scan(&allowed); err != nil {
				return fmt.Errorf("verify Billing runtime privileges: check %s on %s: %w", privilege, tableName, err)
			}
			if allowed {
				return fmt.Errorf("verify Billing runtime privileges: role %q has forbidden %s on %s", currentUser, privilege, tableName)
			}
		}
	}
	for _, functionSignature := range billingCommandFunctionSignatures {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_function_privilege(current_user, $1, 'EXECUTE')`, functionSignature).Scan(&allowed); err != nil {
			return fmt.Errorf("verify Billing runtime privileges: check command %s: %w", functionSignature, err)
		}
		if !allowed {
			return fmt.Errorf("verify Billing runtime privileges: role %q cannot execute %s", currentUser, functionSignature)
		}
	}
	for _, functionSignature := range billingInternalFunctionSignatures {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_function_privilege(current_user, $1, 'EXECUTE')`, functionSignature).Scan(&allowed); err != nil {
			return fmt.Errorf("verify Billing runtime privileges: check internal function %s: %w", functionSignature, err)
		}
		if allowed {
			return fmt.Errorf("verify Billing runtime privileges: role %q can execute internal function %s", currentUser, functionSignature)
		}
	}
	return nil
}
