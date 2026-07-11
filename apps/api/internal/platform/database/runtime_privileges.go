package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type RuntimePrivilegeDBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

var apiOwnedTableNames = []string{
	"api_account_models",
	"api_account_secrets",
	"api_accounts",
	"board_members",
	"board_share_tokens",
	"boards",
	"feature_prompt_revisions",
	"feature_prompts",
	"line_draft_modes",
	"media_assets",
	"media_refs",
	"provider_endpoints",
	"storage_profile_secret_versions",
	"storage_profiles",
	"system_configs",
	"task_attempts",
	"task_billing_settlements",
	"task_billing_snapshots",
	"task_cancellations",
	"task_daily_stat_backfills",
	"task_daily_stats",
	"task_execution_metrics",
	"task_leases",
	"task_outputs",
	"task_submission_claims",
	"task_stat_events",
	"tasks",
	"users",
	"users_used_uids",
}

const userDeletionRequestsTable = "user_deletion_requests"

var apiOwnedSequenceNames = []string{
	"api_account_models_id_seq",
	"api_account_secrets_id_seq",
	"api_accounts_id_seq",
	"board_members_id_seq",
	"board_share_tokens_id_seq",
	"boards_id_seq",
	"feature_prompt_revisions_id_seq",
	"line_draft_modes_id_seq",
	"media_assets_id_seq",
	"media_refs_id_seq",
	"provider_endpoints_id_seq",
	"storage_profile_secret_versions_id_seq",
	"system_configs_id_seq",
	"task_attempts_id_seq",
	"task_daily_stat_backfills_id_seq",
	"task_daily_stats_id_seq",
	"task_leases_id_seq",
	"task_outputs_id_seq",
	"task_stat_events_id_seq",
	"tasks_id_seq",
	"users_id_seq",
	"users_used_uids_id_seq",
}

var protectedAuthTableNames = []string{"user", "session", "account", "verification"}

var forbiddenApplicationTablePrivileges = []string{"INSERT", "UPDATE", "DELETE", "TRUNCATE", "TRIGGER"}

func ConfigureRuntimePrivileges(ctx context.Context, db RuntimePrivilegeDBTX, runtimeRole string) error {
	if db == nil {
		return fmt.Errorf("configure runtime privileges: database is nil")
	}
	runtimeRole = strings.TrimSpace(runtimeRole)
	if runtimeRole == "" {
		return fmt.Errorf("configure runtime privileges: runtime role is empty")
	}
	var schemaName string
	if err := db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schemaName); err != nil {
		return fmt.Errorf("configure runtime privileges: load current schema: %w", err)
	}
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	statements := []string{
		`GRANT USAGE ON SCHEMA ` + schemaSQL + ` TO ` + roleSQL,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ` + qualifiedIdentifiers(schemaName, apiOwnedTableNames) + ` TO ` + roleSQL,
		`GRANT USAGE, SELECT, UPDATE ON ` + qualifiedIdentifiers(schemaName, apiOwnedSequenceNames) + ` TO ` + roleSQL,
		`REVOKE INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON ` + schemaSQL + `.schema_migrations FROM ` + roleSQL,
		`GRANT SELECT ON ` + schemaSQL + `.schema_migrations TO ` + roleSQL,
		`REVOKE ALL PRIVILEGES ON TABLE ` + schemaSQL + `.` + userDeletionRequestsTable + ` FROM ` + roleSQL,
		`GRANT SELECT, INSERT, UPDATE ON ` + schemaSQL + `.` + userDeletionRequestsTable + ` TO ` + roleSQL,
	}
	for _, tableName := range protectedAuthTableNames {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (
SELECT 1 FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2
)`, schemaName, tableName).Scan(&exists); err != nil {
			return fmt.Errorf("configure runtime privileges: inspect protected Auth table %s: %w", tableName, err)
		}
		if exists {
			statements = append(statements,
				`REVOKE ALL PRIVILEGES ON TABLE `+pgx.Identifier{schemaName, tableName}.Sanitize()+` FROM `+roleSQL,
			)
		}
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure runtime privileges: %w", err)
		}
	}
	return nil
}

func VerifyRuntimePrivileges(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) error {
	if db == nil {
		return fmt.Errorf("verify API runtime privileges: database is nil")
	}
	var currentUser string
	if err := db.QueryRowContext(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		return fmt.Errorf("verify API runtime privileges: %w", err)
	}
	for _, tableName := range protectedAuthTableNames {
		for _, privilege := range forbiddenApplicationTablePrivileges {
			var allowed bool
			if err := db.QueryRowContext(ctx, `
SELECT COALESCE(has_table_privilege(current_user, to_regclass($1), $2), FALSE)`,
				tableName, privilege).Scan(&allowed); err != nil {
				return fmt.Errorf("verify API runtime privileges: check Auth table %s %s: %w", tableName, privilege, err)
			}
			if allowed {
				return fmt.Errorf("verify API runtime privileges: role %q has forbidden %s on Auth table %s", currentUser, privilege, tableName)
			}
		}
	}
	for _, privilege := range []string{"SELECT", "INSERT", "UPDATE"} {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, userDeletionRequestsTable, privilege).Scan(&allowed); err != nil {
			return fmt.Errorf("verify API runtime privileges: check deletion requests %s: %w", privilege, err)
		}
		if !allowed {
			return fmt.Errorf("verify API runtime privileges: role %q lacks required %s on %s", currentUser, privilege, userDeletionRequestsTable)
		}
	}
	for _, privilege := range []string{"DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, userDeletionRequestsTable, privilege).Scan(&allowed); err != nil {
			return fmt.Errorf("verify API runtime privileges: check deletion requests %s: %w", privilege, err)
		}
		if allowed {
			return fmt.Errorf("verify API runtime privileges: role %q has forbidden %s on %s", currentUser, privilege, userDeletionRequestsTable)
		}
	}
	return nil
}

func qualifiedIdentifiers(schemaName string, names []string) string {
	qualified := make([]string, len(names))
	for index, name := range names {
		qualified[index] = pgx.Identifier{schemaName, name}.Sanitize()
	}
	return strings.Join(qualified, ", ")
}
