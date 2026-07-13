package worker

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var riverRuntimeTableNames = []string{
	"river_client",
	"river_client_queue",
	"river_job",
	"river_leader",
	"river_queue",
}

var riverRuntimeSequenceNames = []string{"river_job_id_seq"}

var riverRuntimeTablePrivileges = []string{"SELECT", "INSERT", "UPDATE", "DELETE"}
var riverForbiddenTablePrivileges = []string{"TRUNCATE", "REFERENCES", "TRIGGER"}

type riverPrivilegeDBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ConfigureRiverRuntimePrivileges(ctx context.Context, db riverPrivilegeDBTX, runtimeRole string) error {
	if db == nil {
		return fmt.Errorf("configure River runtime privileges: database is nil")
	}
	runtimeRole = strings.TrimSpace(runtimeRole)
	if runtimeRole == "" {
		return fmt.Errorf("configure River runtime privileges: runtime role is empty")
	}
	var schemaName string
	if err := db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schemaName); err != nil {
		return fmt.Errorf("configure River runtime privileges: load current schema: %w", err)
	}
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	statements := []string{
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ` + qualifiedRiverIdentifiers(schemaName, riverRuntimeTableNames) + ` TO ` + roleSQL,
		`GRANT USAGE, SELECT, UPDATE ON ` + qualifiedRiverIdentifiers(schemaName, riverRuntimeSequenceNames) + ` TO ` + roleSQL,
		`REVOKE ALL PRIVILEGES ON TABLE ` + schemaSQL + `.river_migration FROM ` + roleSQL,
		`GRANT SELECT ON ` + schemaSQL + `.river_migration TO ` + roleSQL,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure River runtime privileges: %w", err)
		}
	}
	return nil
}

func VerifyRiverRuntimePrivileges(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) error {
	if db == nil {
		return fmt.Errorf("verify River runtime privileges: database is nil")
	}
	var currentUser string
	if err := db.QueryRowContext(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		return fmt.Errorf("verify River runtime privileges: %w", err)
	}
	for _, tableName := range riverRuntimeTableNames {
		for _, privilege := range riverRuntimeTablePrivileges {
			if err := requireRiverTablePrivilege(ctx, db, currentUser, tableName, privilege); err != nil {
				return err
			}
		}
		for _, privilege := range riverForbiddenTablePrivileges {
			if err := forbidRiverTablePrivilege(ctx, db, currentUser, tableName, privilege); err != nil {
				return err
			}
		}
	}
	if err := requireRiverTablePrivilege(ctx, db, currentUser, "river_migration", "SELECT"); err != nil {
		return err
	}
	for _, privilege := range append(riverRuntimeTablePrivileges[1:], riverForbiddenTablePrivileges...) {
		if err := forbidRiverTablePrivilege(ctx, db, currentUser, "river_migration", privilege); err != nil {
			return err
		}
	}
	for _, sequenceName := range riverRuntimeSequenceNames {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_sequence_privilege(current_user, $1, 'USAGE')`, sequenceName).Scan(&allowed); err != nil {
			return fmt.Errorf("verify River runtime privileges: check USAGE on %s: %w", sequenceName, err)
		}
		if !allowed {
			return fmt.Errorf("verify River runtime privileges: role %q lacks required USAGE on %s", currentUser, sequenceName)
		}
	}
	return nil
}

func requireRiverTablePrivilege(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, currentUser string, tableName string, privilege string) error {
	var allowed bool
	if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, tableName, privilege).Scan(&allowed); err != nil {
		return fmt.Errorf("verify River runtime privileges: check %s on %s: %w", privilege, tableName, err)
	}
	if !allowed {
		return fmt.Errorf("verify River runtime privileges: role %q lacks required %s on %s", currentUser, privilege, tableName)
	}
	return nil
}

func forbidRiverTablePrivilege(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, currentUser string, tableName string, privilege string) error {
	var allowed bool
	if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, $1, $2)`, tableName, privilege).Scan(&allowed); err != nil {
		return fmt.Errorf("verify River runtime privileges: check %s on %s: %w", privilege, tableName, err)
	}
	if allowed {
		return fmt.Errorf("verify River runtime privileges: role %q has forbidden %s on %s", currentUser, privilege, tableName)
	}
	return nil
}

func qualifiedRiverIdentifiers(schemaName string, names []string) string {
	qualified := make([]string, len(names))
	for index, name := range names {
		qualified[index] = pgx.Identifier{schemaName, name}.Sanitize()
	}
	return strings.Join(qualified, ", ")
}
