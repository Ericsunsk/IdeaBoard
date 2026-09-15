package privileges

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type compiledStatement struct {
	policyNames []string
	sql         string
}

func compileStatements(
	schemaName string,
	runtimeRole string,
	schemaPolicyNames []string,
	rules []resolvedRule,
) []compiledStatement {
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	statements := make([]compiledStatement, 0, len(rules)*3+1)
	if len(schemaPolicyNames) > 0 {
		statements = append(statements, compiledStatement{
			policyNames: schemaPolicyNames,
			sql:         `GRANT USAGE ON SCHEMA ` + pgx.Identifier{schemaName}.Sanitize() + ` TO ` + roleSQL,
		})
	}
	for _, rule := range rules {
		objectSQL := rule.sqlIdentity(schemaName)
		kindSQL := strings.ToUpper(string(rule.kind))
		if len(rule.publicForbidden) > 0 {
			statements = append(statements, compiledStatement{
				policyNames: rule.policyNames,
				sql:         `REVOKE ` + joinPrivileges(rule.publicForbidden) + ` ON ` + kindSQL + ` ` + objectSQL + ` FROM PUBLIC`,
			})
		}
		if len(rule.forbidden) > 0 {
			statements = append(statements, compiledStatement{
				policyNames: rule.policyNames,
				sql:         `REVOKE ` + joinPrivileges(rule.forbidden) + ` ON ` + kindSQL + ` ` + objectSQL + ` FROM ` + roleSQL,
			})
		}
		if len(rule.required) > 0 {
			statements = append(statements, compiledStatement{
				policyNames: rule.policyNames,
				sql:         `GRANT ` + joinPrivileges(rule.required) + ` ON ` + kindSQL + ` ` + objectSQL + ` TO ` + roleSQL,
			})
		}
	}
	return statements
}

func joinPrivileges(privileges []Privilege) string {
	values := make([]string, len(privileges))
	for index, privilege := range privileges {
		values[index] = string(privilege)
	}
	return strings.Join(values, ", ")
}

func (r resolvedRule) sqlIdentity(schemaName string) string {
	identity := pgx.Identifier{schemaName, r.name}.Sanitize()
	if r.kind == KindFunction {
		return identity + "(" + r.functionArguments + ")"
	}
	return identity
}

func (r resolvedRule) displayName(schemaName string) string {
	if schemaName == "" {
		return fmt.Sprintf("%s %q", r.kind, r.identityName())
	}
	return fmt.Sprintf("%s %s", r.kind, r.sqlIdentity(schemaName))
}

func formatPolicyNames(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("policy %q", name))
	}
	return strings.Join(quoted, ", ")
}

type verificationCheck struct {
	rule      resolvedRule
	privilege Privilege
	expected  bool
	isPublic  bool
}

type verificationPayload struct {
	CheckIndex int       `json:"check_index"`
	Name       string    `json:"name"`
	Arguments  string    `json:"arguments"`
	Privilege  Privilege `json:"privilege"`
	Expected   bool      `json:"expected"`
	IsPublic   bool      `json:"is_public"`
}

type verificationBatch struct {
	kind   ObjectKind
	query  string
	checks []verificationCheck
}

func verifyResolvedPrivileges(
	ctx context.Context,
	db DBTX,
	currentUser string,
	schemaName string,
	rules []resolvedRule,
) error {
	for _, batch := range buildVerificationBatches(rules) {
		if len(batch.checks) == 0 {
			continue
		}
		if err := verifyPrivilegeBatch(ctx, db, currentUser, schemaName, batch); err != nil {
			return err
		}
	}
	return nil
}

func buildVerificationBatches(rules []resolvedRule) []verificationBatch {
	batches := []verificationBatch{
		{kind: KindTable, query: verifyTablePrivilegesQuery},
		{kind: KindSequence, query: verifySequencePrivilegesQuery},
		{kind: KindFunction, query: verifyFunctionPrivilegesQuery},
	}
	for _, rule := range rules {
		var batch *verificationBatch
		for index := range batches {
			if batches[index].kind == rule.kind {
				batch = &batches[index]
				break
			}
		}
		if batch == nil {
			continue
		}
		for _, privilege := range rule.required {
			batch.checks = append(batch.checks, verificationCheck{rule: rule, privilege: privilege, expected: true})
		}
		for _, privilege := range rule.forbidden {
			batch.checks = append(batch.checks, verificationCheck{rule: rule, privilege: privilege})
		}
		for _, privilege := range rule.publicForbidden {
			batch.checks = append(batch.checks, verificationCheck{rule: rule, privilege: privilege, isPublic: true})
		}
	}
	return batches
}

func verifyPrivilegeBatch(
	ctx context.Context,
	db DBTX,
	currentUser string,
	schemaName string,
	batch verificationBatch,
) error {
	payload := make([]verificationPayload, len(batch.checks))
	var policyNames []string
	for index, check := range batch.checks {
		payload[index] = verificationPayload{
			CheckIndex: index,
			Name:       check.rule.name,
			Arguments:  check.rule.functionArguments,
			Privilege:  check.privilege,
			Expected:   check.expected,
			IsPublic:   check.isPublic,
		}
		policyNames = append(policyNames, check.rule.policyNames...)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("verify PostgreSQL privileges for %s: encode %s checks: %w", formatPolicyNames(sortedUnique(policyNames)), batch.kind, err)
	}
	rows, err := db.QueryContext(ctx, batch.query, string(encoded), schemaName, currentUser)
	if err != nil {
		return fmt.Errorf("verify PostgreSQL privileges for %s: query %s checks: %w", formatPolicyNames(sortedUnique(policyNames)), batch.kind, err)
	}
	if !rows.Next() {
		readErr := rows.Err()
		closeErr := rows.Close()
		if readErr != nil || closeErr != nil {
			return fmt.Errorf("verify PostgreSQL privileges for %s: read %s checks: %w", formatPolicyNames(sortedUnique(policyNames)), batch.kind, errors.Join(readErr, closeErr))
		}
		return nil
	}

	var (
		checkIndex   int
		objectExists bool
		allowed      sql.NullBool
	)
	if scanErr := rows.Scan(&checkIndex, &objectExists, &allowed); scanErr != nil {
		return closeVerificationRows(rows, fmt.Errorf("scan %s mismatch: %w", batch.kind, scanErr))
	}
	//nolint:sqlclosecheck // early close surfaces verification query errors with batch context
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("verify PostgreSQL privileges for %s: close %s checks: %w", formatPolicyNames(sortedUnique(policyNames)), batch.kind, closeErr)
	}
	if checkIndex < 0 || checkIndex >= len(batch.checks) {
		return fmt.Errorf("verify PostgreSQL privileges for %s: %s check returned invalid index %d", formatPolicyNames(sortedUnique(policyNames)), batch.kind, checkIndex)
	}
	check := batch.checks[checkIndex]
	if !objectExists {
		return fmt.Errorf(
			"verify PostgreSQL privileges for %s: %s no longer exists",
			formatPolicyNames(check.rule.policyNames), check.rule.displayName(schemaName),
		)
	}
	if !allowed.Valid {
		return fmt.Errorf(
			"verify PostgreSQL privileges for %s: %s check returned NULL on %s",
			formatPolicyNames(check.rule.policyNames), check.privilege, check.rule.displayName(schemaName),
		)
	}
	if allowed.Bool == check.expected {
		return fmt.Errorf(
			"verify PostgreSQL privileges for %s: %s check returned a non-mismatch on %s",
			formatPolicyNames(check.rule.policyNames), check.privilege, check.rule.displayName(schemaName),
		)
	}
	if check.expected {
		return fmt.Errorf(
			"verify PostgreSQL privileges for %s: role %q lacks required %s on %s",
			formatPolicyNames(check.rule.policyNames), currentUser, check.privilege, check.rule.displayName(schemaName),
		)
	}
	if check.isPublic {
		return fmt.Errorf(
			"verify PostgreSQL privileges for %s: PUBLIC has forbidden %s on %s",
			formatPolicyNames(check.rule.policyNames), check.privilege, check.rule.displayName(schemaName),
		)
	}
	return fmt.Errorf(
		"verify PostgreSQL privileges for %s: role %q has forbidden %s on %s",
		formatPolicyNames(check.rule.policyNames), currentUser, check.privilege, check.rule.displayName(schemaName),
	)
}

func closeVerificationRows(rows *sql.Rows, cause error) error {
	if closeErr := rows.Close(); closeErr != nil {
		return errors.Join(cause, closeErr)
	}
	return cause
}

const verifyTablePrivilegesQuery = `/* ideaboard:verify-table-privileges */
WITH checks AS (
  SELECT *
  FROM jsonb_to_recordset($1::JSONB) AS input(
    check_index INTEGER,
    name TEXT,
    arguments TEXT,
    privilege TEXT,
    expected BOOLEAN,
    is_public BOOLEAN
  )
), objects AS (
  SELECT checks.*,
         relation.oid,
         relation.relowner,
         relation.relacl
  FROM checks
  LEFT JOIN pg_catalog.pg_namespace AS namespace
    ON namespace.nspname = $2
  LEFT JOIN pg_catalog.pg_class AS relation
    ON relation.relnamespace = namespace.oid
   AND relation.relname = checks.name
   AND relation.relkind::TEXT = ANY(ARRAY['r', 'p', 'v', 'm', 'f']::TEXT[])
), evaluated AS (
  SELECT objects.*,
         CASE
           WHEN objects.oid IS NULL THEN NULL::BOOLEAN
           WHEN objects.is_public THEN EXISTS (
             SELECT 1
             FROM pg_catalog.aclexplode(
               COALESCE(objects.relacl, pg_catalog.acldefault('r'::"char", objects.relowner))
             ) AS acl
             WHERE acl.grantee = 0
               AND acl.privilege_type = objects.privilege
           )
           ELSE pg_catalog.has_table_privilege($3, objects.oid, objects.privilege)
         END AS allowed
  FROM objects
)
SELECT check_index, oid IS NOT NULL, allowed
FROM evaluated
WHERE oid IS NULL OR allowed IS DISTINCT FROM expected
ORDER BY check_index
LIMIT 1`

const verifySequencePrivilegesQuery = `/* ideaboard:verify-sequence-privileges */
WITH checks AS (
  SELECT *
  FROM jsonb_to_recordset($1::JSONB) AS input(
    check_index INTEGER,
    name TEXT,
    arguments TEXT,
    privilege TEXT,
    expected BOOLEAN,
    is_public BOOLEAN
  )
), objects AS (
  SELECT checks.*,
         sequence.oid,
         sequence.relowner,
         sequence.relacl
  FROM checks
  LEFT JOIN pg_catalog.pg_namespace AS namespace
    ON namespace.nspname = $2
  LEFT JOIN pg_catalog.pg_class AS sequence
    ON sequence.relnamespace = namespace.oid
   AND sequence.relname = checks.name
   AND sequence.relkind = 'S'
), evaluated AS (
  SELECT objects.*,
         CASE
           WHEN objects.oid IS NULL THEN NULL::BOOLEAN
           WHEN objects.is_public THEN EXISTS (
             SELECT 1
             FROM pg_catalog.aclexplode(
               COALESCE(objects.relacl, pg_catalog.acldefault('s'::"char", objects.relowner))
             ) AS acl
             WHERE acl.grantee = 0
               AND acl.privilege_type = objects.privilege
           )
           ELSE pg_catalog.has_sequence_privilege($3, objects.oid, objects.privilege)
         END AS allowed
  FROM objects
)
SELECT check_index, oid IS NOT NULL, allowed
FROM evaluated
WHERE oid IS NULL OR allowed IS DISTINCT FROM expected
ORDER BY check_index
LIMIT 1`

const verifyFunctionPrivilegesQuery = `/* ideaboard:verify-function-privileges */
WITH checks AS (
  SELECT *
  FROM jsonb_to_recordset($1::JSONB) AS input(
    check_index INTEGER,
    name TEXT,
    arguments TEXT,
    privilege TEXT,
    expected BOOLEAN,
    is_public BOOLEAN
  )
), objects AS (
  SELECT checks.*,
         function.oid,
         function.proowner,
         function.proacl
  FROM checks
  LEFT JOIN pg_catalog.pg_namespace AS namespace
    ON namespace.nspname = $2
  LEFT JOIN pg_catalog.pg_proc AS function
    ON function.pronamespace = namespace.oid
   AND function.proname = checks.name
   AND function.prokind IN ('f', 'w')
   AND pg_catalog.oidvectortypes(function.proargtypes) = checks.arguments
), evaluated AS (
  SELECT objects.*,
         CASE
           WHEN objects.oid IS NULL THEN NULL::BOOLEAN
           WHEN objects.is_public THEN EXISTS (
             SELECT 1
             FROM pg_catalog.aclexplode(
               COALESCE(objects.proacl, pg_catalog.acldefault('f'::"char", objects.proowner))
             ) AS acl
             WHERE acl.grantee = 0
               AND acl.privilege_type = objects.privilege
           )
           ELSE pg_catalog.has_function_privilege($3, objects.oid, objects.privilege)
         END AS allowed
  FROM objects
)
SELECT check_index, oid IS NOT NULL, allowed
FROM evaluated
WHERE oid IS NULL OR allowed IS DISTINCT FROM expected
ORDER BY check_index
LIMIT 1`
