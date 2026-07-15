package privileges

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// DBTX is the shared database seam used by privilege configuration and
// verification. *sql.DB, *sql.Conn, and *sql.Tx all satisfy it.
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type ObjectKind string

const (
	KindTable    ObjectKind = "table"
	KindSequence ObjectKind = "sequence"
	KindFunction ObjectKind = "function"
)

type Privilege string

const (
	PrivilegeSelect     Privilege = "SELECT"
	PrivilegeInsert     Privilege = "INSERT"
	PrivilegeUpdate     Privilege = "UPDATE"
	PrivilegeDelete     Privilege = "DELETE"
	PrivilegeTruncate   Privilege = "TRUNCATE"
	PrivilegeReferences Privilege = "REFERENCES"
	PrivilegeTrigger    Privilege = "TRIGGER"
	PrivilegeUsage      Privilege = "USAGE"
	PrivilegeExecute    Privilege = "EXECUTE"
)

// Policy keeps privilege ownership with the calling context while the engine
// owns validation, resolution, SQL execution, and verification.
type Policy struct {
	Name               string
	RequireSchemaUsage bool
	Objects            []ObjectRule
}

// ObjectRule selects either explicit Names or every object beginning with
// NamePrefix. Function names use PostgreSQL identity-signature form, such as
// execute_command(text, bigint). Optional only tolerates missing objects for
// rules without Required privileges; a missing required object always fails.
type ObjectRule struct {
	Kind            ObjectKind
	Names           []string
	NamePrefix      string
	Optional        bool
	Required        []Privilege
	Forbidden       []Privilege
	PublicForbidden []Privilege
}

// Configure applies all policies as one validated policy set. Callers retain
// their own error context by wrapping the returned error.
func Configure(ctx context.Context, db DBTX, runtimeRole string, policies ...Policy) error {
	validated, err := validatePolicies(policies)
	if err != nil {
		return fmt.Errorf("configure PostgreSQL privileges: %w", err)
	}
	if db == nil {
		return fmt.Errorf("configure PostgreSQL privileges: database is nil")
	}
	runtimeRole = strings.TrimSpace(runtimeRole)
	if runtimeRole == "" {
		return fmt.Errorf("configure PostgreSQL privileges: runtime role is empty")
	}
	if strings.ContainsRune(runtimeRole, 0) {
		return fmt.Errorf("configure PostgreSQL privileges: runtime role contains a null byte")
	}

	schemaName, _, err := loadDatabaseIdentity(ctx, db)
	if err != nil {
		return fmt.Errorf("configure PostgreSQL privileges: %w", err)
	}
	resolved, schemaPolicies, err := resolvePolicies(ctx, db, schemaName, validated)
	if err != nil {
		return fmt.Errorf("configure PostgreSQL privileges: %w", err)
	}
	for _, statement := range compileStatements(schemaName, runtimeRole, schemaPolicies, resolved) {
		if _, err := db.ExecContext(ctx, statement.sql); err != nil {
			return fmt.Errorf(
				"configure PostgreSQL privileges for %s: %w",
				formatPolicyNames(statement.policyNames),
				err,
			)
		}
	}
	return nil
}

// Verify checks all required, forbidden, and PUBLIC-forbidden privileges for
// current_user. Every database or policy error fails verification closed.
func Verify(ctx context.Context, db DBTX, policies ...Policy) error {
	validated, err := validatePolicies(policies)
	if err != nil {
		return fmt.Errorf("verify PostgreSQL privileges: %w", err)
	}
	if db == nil {
		return fmt.Errorf("verify PostgreSQL privileges: database is nil")
	}

	schemaName, currentUser, schemaAllowed, err := loadVerificationIdentity(ctx, db, policiesRequireSchemaUsage(validated))
	if err != nil {
		return fmt.Errorf("verify PostgreSQL privileges: %w", err)
	}
	resolved, schemaPolicies, err := resolvePolicies(ctx, db, schemaName, validated)
	if err != nil {
		return fmt.Errorf("verify PostgreSQL privileges: %w", err)
	}
	if len(schemaPolicies) > 0 {
		if !schemaAllowed {
			return fmt.Errorf(
				"verify PostgreSQL privileges for %s: role %q lacks required USAGE on schema %q",
				formatPolicyNames(schemaPolicies), currentUser, schemaName,
			)
		}
	}
	return verifyResolvedPrivileges(ctx, db, currentUser, schemaName, resolved)
}

func policiesRequireSchemaUsage(policies []validatedPolicy) bool {
	for _, policy := range policies {
		if policy.requireSchemaUsage {
			return true
		}
	}
	return false
}

func loadVerificationIdentity(
	ctx context.Context,
	db DBTX,
	requireSchemaUsage bool,
) (schemaName string, currentUser string, schemaAllowed bool, err error) {
	err = db.QueryRowContext(ctx, `/* ideaboard:verify-schema-privileges */
WITH identity AS (
  SELECT current_schema() AS schema_name, current_user::TEXT AS role_name
)
SELECT schema_name,
       role_name,
       CASE
         WHEN $1 THEN has_schema_privilege(role_name, schema_name, 'USAGE')
         ELSE TRUE
       END
FROM identity`, requireSchemaUsage).Scan(&schemaName, &currentUser, &schemaAllowed)
	if err != nil {
		return "", "", false, fmt.Errorf("load current schema, user, and schema privileges: %w", err)
	}
	if strings.TrimSpace(schemaName) == "" || strings.TrimSpace(currentUser) == "" {
		return "", "", false, fmt.Errorf("current schema or user is empty")
	}
	return schemaName, currentUser, schemaAllowed, nil
}

func loadDatabaseIdentity(ctx context.Context, db DBTX) (schemaName string, currentUser string, err error) {
	err = db.QueryRowContext(ctx, `SELECT current_schema(), current_user`).Scan(&schemaName, &currentUser)
	if err != nil {
		return "", "", fmt.Errorf("load current schema and user: %w", err)
	}
	if strings.TrimSpace(schemaName) == "" || strings.TrimSpace(currentUser) == "" {
		return "", "", fmt.Errorf("current schema or user is empty")
	}
	return schemaName, currentUser, nil
}
