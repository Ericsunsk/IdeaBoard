package taskbilling

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type privilegeDBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type privilegeQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ConfigureRuntimePrivileges(ctx context.Context, db privilegeDBTX, runtimeRole string) error {
	if db == nil || strings.TrimSpace(runtimeRole) == "" {
		return fmt.Errorf("configure Task Billing privileges: invalid input")
	}
	role := pgx.Identifier{strings.TrimSpace(runtimeRole)}.Sanitize()
	statements := []string{
		`REVOKE UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON task_submission_claims FROM ` + role,
		`GRANT SELECT, INSERT ON task_submission_claims TO ` + role,
		`REVOKE UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON task_billing_snapshots FROM ` + role,
		`GRANT SELECT, INSERT ON task_billing_snapshots TO ` + role,
		`REVOKE DELETE, TRUNCATE, REFERENCES, TRIGGER ON task_billing_settlements FROM ` + role,
		`GRANT SELECT, INSERT, UPDATE ON task_billing_settlements TO ` + role,
		`REVOKE EXECUTE ON FUNCTION prevent_task_billing_snapshot_mutation() FROM PUBLIC`,
		`REVOKE EXECUTE ON FUNCTION prevent_task_billing_snapshot_mutation() FROM ` + role,
		`REVOKE EXECUTE ON FUNCTION validate_task_billing_snapshot_insert() FROM PUBLIC`,
		`REVOKE EXECUTE ON FUNCTION validate_task_billing_snapshot_insert() FROM ` + role,
		`REVOKE EXECUTE ON FUNCTION guard_task_billing_settlement_transition() FROM PUBLIC`,
		`REVOKE EXECUTE ON FUNCTION guard_task_billing_settlement_transition() FROM ` + role,
		`REVOKE EXECUTE ON FUNCTION guard_task_submission_claim() FROM PUBLIC`,
		`REVOKE EXECUTE ON FUNCTION guard_task_submission_claim() FROM ` + role,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure Task Billing privileges: %w", err)
		}
	}
	return nil
}

func VerifyRuntimePrivileges(ctx context.Context, db privilegeQuerier) error {
	if db == nil {
		return fmt.Errorf("verify Task Billing privileges: database is nil")
	}
	required := map[string][]string{
		"task_submission_claims":   {"SELECT", "INSERT"},
		"task_billing_snapshots":   {"SELECT", "INSERT"},
		"task_billing_settlements": {"SELECT", "INSERT", "UPDATE"},
	}
	for table, privileges := range required {
		for _, privilege := range privileges {
			var allowed bool
			if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, table, privilege).Scan(&allowed); err != nil {
				return fmt.Errorf("verify Task Billing privileges: check %s on %s: %w", privilege, table, err)
			}
			if !allowed {
				return fmt.Errorf("verify Task Billing privileges: missing %s on %s", privilege, table)
			}
		}
	}
	for table, privileges := range map[string][]string{
		"task_submission_claims":   {"UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"},
		"task_billing_snapshots":   {"UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"},
		"task_billing_settlements": {"DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"},
	} {
		for _, privilege := range privileges {
			var allowed bool
			if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, table, privilege).Scan(&allowed); err != nil {
				return fmt.Errorf("verify Task Billing privileges: check %s on %s: %w", privilege, table, err)
			}
			if allowed {
				return fmt.Errorf("verify Task Billing privileges: forbidden %s on %s", privilege, table)
			}
		}
	}
	for _, signature := range []string{
		"validate_task_billing_snapshot_insert()",
		"prevent_task_billing_snapshot_mutation()",
		"guard_task_billing_settlement_transition()",
		"guard_task_submission_claim()",
	} {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_function_privilege(current_user, $1, 'EXECUTE')`, signature).Scan(&allowed); err != nil {
			return fmt.Errorf("verify Task Billing privileges: check function %s: %w", signature, err)
		}
		if allowed {
			return fmt.Errorf("verify Task Billing privileges: runtime can execute internal function %s", signature)
		}
	}
	return nil
}
