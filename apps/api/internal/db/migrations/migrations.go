package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const migrationLockReleaseTimeout = 5 * time.Second
const migrationTxLockTimeout = "5s"
const migrationTxStatementTimeout = "30min"
const migrationNoTxLockTimeout = "5s"
const migrationNoTxStatementTimeout = "2h"
const statementNoTx = "-- statement:no-tx"
const statementIndexNamePrefix = "-- statement:index-name="
const baselineMigrationVersion int64 = 20260609000000
const migrationLockNamespace = "ideaboard:migrations:v1"

var baselineBusinessTableNames = map[string]struct{}{
	"api_account_models":              {},
	"api_account_secrets":             {},
	"api_accounts":                    {},
	"boards":                          {},
	"board_members":                   {},
	"board_share_tokens":              {},
	"credit_accounts":                 {},
	"credit_charges":                  {},
	"credit_holds":                    {},
	"credit_postings":                 {},
	"credit_transactions":             {},
	"credit_wallets":                  {},
	"provider_endpoints":              {},
	"feature_prompts":                 {},
	"line_draft_modes":                {},
	"media_assets":                    {},
	"media_refs":                      {},
	"storage_profile_secret_versions": {},
	"storage_profiles":                {},
	"system_configs":                  {},
	"task_attempts":                   {},
	"task_cancellations":              {},
	"task_daily_stat_backfills":       {},
	"task_daily_stats":                {},
	"task_execution_metrics":          {},
	"task_leases":                     {},
	"task_outputs":                    {},
	"task_stat_events":                {},
	"tasks":                           {},
	"users":                           {},
	"users_used_uids":                 {},
}

//go:embed *.sql
var migrationFS embed.FS

type migrationStatement struct {
	SQL       string
	NoTx      bool
	IndexName string
}

func Up(ctx context.Context, db *sql.DB) (err error) {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close migration connection: %w", closeErr))
		}
	}()

	release, err := acquireMigrationLock(ctx, conn)
	if err != nil {
		return err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), migrationLockReleaseTimeout)
		defer cancel()
		if releaseErr := release(releaseCtx); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
	}()

	return upWithConn(ctx, conn)
}

func upWithConn(ctx context.Context, conn *sql.Conn) error {
	if conn == nil {
		return fmt.Errorf("migration connection is nil")
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  applied_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
)`); err != nil {
		return err
	}

	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	embeddedVersions, err := embeddedMigrationVersions(files)
	if err != nil {
		return err
	}
	if err := ensureAppliedMigrationsAreEmbedded(ctx, conn, embeddedVersions); err != nil {
		return err
	}

	for _, file := range files {
		version, err := migrationVersion(file)
		if err != nil {
			return err
		}
		applied, err := migrationApplied(ctx, conn, version)
		if err != nil {
			return err
		}
		if applied {
			if version == baselineMigrationVersion {
				if err := ensureBaselineSchemaIsPresent(ctx, conn, version); err != nil {
					return err
				}
			}
			continue
		}
		if version == baselineMigrationVersion {
			if err := ensureBaselineMigrationCanApply(ctx, conn, version); err != nil {
				return err
			}
		}

		raw, err := migrationFS.ReadFile(file)
		if err != nil {
			return err
		}
		statements := parseSQLStatements(extractUpSQL(string(raw)))
		if len(statements) == 0 {
			return fmt.Errorf("migration %s has no statements", file)
		}
		if err := validateMigrationStatements(file, statements); err != nil {
			return err
		}
		if err := applyMigration(ctx, conn, file, statements, version); err != nil {
			return err
		}
	}
	if err := verifySecurityDefinerRoutines(ctx, conn); err != nil {
		return fmt.Errorf("verify SECURITY DEFINER routines: %w", err)
	}
	return nil
}

func embeddedMigrationVersions(files []string) (map[int64]struct{}, error) {
	versions := make(map[int64]struct{}, len(files))
	for _, file := range files {
		version, err := migrationVersion(file)
		if err != nil {
			return nil, err
		}
		versions[version] = struct{}{}
	}
	return versions, nil
}

func ensureAppliedMigrationsAreEmbedded(ctx context.Context, conn *sql.Conn, embeddedVersions map[int64]struct{}) error {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var unknown []string
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return err
		}
		if _, ok := embeddedVersions[version]; !ok {
			unknown = append(unknown, strconv.FormatInt(version, 10))
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(unknown) > 0 {
		return fmt.Errorf(
			"database has migration version(s) not present in this binary: %s; reset the database or restore a backup created for this release",
			strings.Join(unknown, ", "),
		)
	}
	return nil
}

func ensureBaselineMigrationCanApply(ctx context.Context, conn *sql.Conn, version int64) error {
	rows, err := conn.QueryContext(ctx, `
SELECT c.relname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema()
  AND c.relkind IN ('r', 'p')
  AND c.relname <> 'schema_migrations'
ORDER BY c.relname`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var existingBusinessTables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return err
		}
		if _, ok := baselineBusinessTableNames[tableName]; ok {
			existingBusinessTables = append(existingBusinessTables, tableName)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(existingBusinessTables) > 0 {
		return fmt.Errorf(
			"cannot apply baseline migration %d: current schema already contains API table(s): %s; reset the database or restore a backup created for this release",
			version,
			strings.Join(existingBusinessTables, ", "),
		)
	}
	return nil
}

func ensureBaselineSchemaIsPresent(ctx context.Context, conn *sql.Conn, version int64) error {
	rows, err := conn.QueryContext(ctx, `
SELECT c.relname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema()
  AND c.relkind IN ('r', 'p')
ORDER BY c.relname`)
	if err != nil {
		return err
	}
	defer rows.Close()

	present := make(map[string]struct{}, len(baselineBusinessTableNames))
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return err
		}
		if _, ok := baselineBusinessTableNames[tableName]; ok {
			present[tableName] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var missing []string
	for tableName := range baselineBusinessTableNames {
		if _, ok := present[tableName]; !ok {
			missing = append(missing, tableName)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf(
			"baseline migration %d is marked applied, but API table(s) are missing: %s; reset the database or restore a backup created for this release",
			version,
			strings.Join(missing, ", "),
		)
	}
	return nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, file string, statements []migrationStatement, version int64) error {
	if hasNoTxStatements(statements) {
		return applyNoTxMigration(ctx, conn, file, statements, version)
	}
	return applyTransactionalMigration(ctx, conn, file, statements, version)
}

func applyTransactionalMigration(ctx context.Context, conn *sql.Conn, file string, statements []migrationStatement, version int64) error {
	return withMigrationTx(ctx, conn, file, func(tx *sql.Tx) error {
		if err := execMigrationStatements(ctx, tx, file, statements); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return fmt.Errorf("record migration %s: %w", file, err)
		}
		return nil
	})
}

func applyNoTxMigration(ctx context.Context, conn *sql.Conn, file string, statements []migrationStatement, version int64) error {
	for _, statement := range statements {
		if !statement.NoTx {
			return fmt.Errorf("migration %s has transactional statement in no-tx path: %s", file, firstSQLLine(statement.SQL))
		}
		if err := applyNoTxStatement(ctx, conn, file, statement); err != nil {
			return err
		}
	}
	return recordMigrationVersion(ctx, conn, file, version)
}

func applyNoTxStatement(ctx context.Context, conn *sql.Conn, file string, statement migrationStatement) error {
	isConcurrentCreate := isCreateIndexConcurrently(statement.SQL)
	return withNoTxStatementTimeouts(ctx, conn, file, func() error {
		if isConcurrentCreate && statement.IndexName != "" {
			if err := dropInvalidIndexIfNeeded(ctx, conn, statement.IndexName); err != nil {
				return fmt.Errorf("prepare no-tx migration %s index %s: %w", file, statement.IndexName, err)
			}
		}
		if _, err := conn.ExecContext(ctx, statement.SQL); err != nil {
			return fmt.Errorf("apply migration %s no-tx statement: %w", file, err)
		}
		if isConcurrentCreate {
			if err := ensureIndexValid(ctx, conn, statement.IndexName); err != nil {
				return fmt.Errorf("verify migration %s index %s: %w", file, statement.IndexName, err)
			}
		}
		return nil
	})
}

func dropInvalidIndexIfNeeded(ctx context.Context, conn *sql.Conn, indexName string) error {
	var invalid bool
	if err := conn.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_class c
  JOIN pg_index i ON i.indexrelid = c.oid
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = current_schema()
    AND c.relname = $1
    AND i.indisvalid = false
)`, indexName).Scan(&invalid); err != nil {
		return err
	}
	if !invalid {
		return nil
	}
	quotedIndexName, err := quoteSQLIdentifier(indexName)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DROP INDEX CONCURRENTLY IF EXISTS `+quotedIndexName); err != nil {
		return err
	}
	return nil
}

func quoteSQLIdentifier(identifier string) (string, error) {
	if strings.TrimSpace(identifier) == "" {
		return "", fmt.Errorf("identifier is required")
	}
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`, nil
}

func ensureIndexValid(ctx context.Context, conn *sql.Conn, indexName string) error {
	var valid bool
	if err := conn.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_class c
  JOIN pg_index i ON i.indexrelid = c.oid
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = current_schema()
    AND c.relname = $1
    AND i.indisvalid = true
)`, indexName).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("index is missing or invalid")
	}
	return nil
}

func validateMigrationStatements(file string, statements []migrationStatement) error {
	hasTx := false
	hasNoTx := false
	for _, statement := range statements {
		if statement.NoTx {
			hasNoTx = true
		} else {
			hasTx = true
		}
		normalized := normalizedStatementSQL(statement.SQL)
		usesConcurrently := strings.Contains(normalized, "CONCURRENTLY")
		if usesConcurrently && !statement.NoTx {
			return fmt.Errorf("migration %s has CONCURRENTLY statement without %s", file, statementNoTx)
		}
		if !statement.NoTx {
			continue
		}
		if !isAllowedNoTxStatement(normalized) {
			return fmt.Errorf("migration %s has unsupported no-tx statement: %s", file, firstSQLLine(statement.SQL))
		}
		if isCreateIndexConcurrently(normalized) {
			if !strings.Contains(normalized, "IF NOT EXISTS") {
				return fmt.Errorf("migration %s concurrent index must use IF NOT EXISTS", file)
			}
			if statement.IndexName == "" {
				return fmt.Errorf("migration %s concurrent index must declare %s<name>", file, statementIndexNamePrefix)
			}
			if err := validateStatementIndexName(statement.IndexName); err != nil {
				return fmt.Errorf("migration %s has invalid concurrent index name %q: %w", file, statement.IndexName, err)
			}
			actualIndexName, ok := createIndexStatementName(statement.SQL)
			if !ok {
				return fmt.Errorf("migration %s cannot parse concurrent index statement name", file)
			}
			if actualIndexName != statement.IndexName {
				return fmt.Errorf("migration %s concurrent index declares %s but creates %s", file, statement.IndexName, actualIndexName)
			}
		}
		if isDropIndexConcurrently(normalized) && !strings.Contains(normalized, "IF EXISTS") {
			return fmt.Errorf("migration %s concurrent index drop must use IF EXISTS", file)
		}
	}
	if hasTx && hasNoTx {
		return fmt.Errorf("migration %s must not mix transactional statements with %s statements", file, statementNoTx)
	}
	return nil
}

func isAllowedNoTxStatement(normalized string) bool {
	return isCreateIndexConcurrently(normalized) ||
		isDropIndexConcurrently(normalized)
}

func isCreateIndexConcurrently(sql string) bool {
	normalized := normalizedStatementSQL(sql)
	return strings.HasPrefix(normalized, "CREATE INDEX CONCURRENTLY") ||
		strings.HasPrefix(normalized, "CREATE UNIQUE INDEX CONCURRENTLY")
}

func createIndexStatementName(sql string) (string, bool) {
	fields := strings.Fields(meaningfulSQL(sql))
	if len(fields) < 4 || !strings.EqualFold(fields[0], "CREATE") {
		return "", false
	}
	next := 1
	if strings.EqualFold(fields[next], "UNIQUE") {
		next++
	}
	if next+2 >= len(fields) ||
		!strings.EqualFold(fields[next], "INDEX") ||
		!strings.EqualFold(fields[next+1], "CONCURRENTLY") {
		return "", false
	}
	next += 2
	if next+3 < len(fields) &&
		strings.EqualFold(fields[next], "IF") &&
		strings.EqualFold(fields[next+1], "NOT") &&
		strings.EqualFold(fields[next+2], "EXISTS") {
		next += 3
	}
	if next >= len(fields) {
		return "", false
	}
	return strings.Trim(fields[next], `"`), true
}

func isDropIndexConcurrently(sql string) bool {
	return strings.HasPrefix(normalizedStatementSQL(sql), "DROP INDEX CONCURRENTLY")
}

func normalizedStatementSQL(stmt string) string {
	return strings.ToUpper(meaningfulSQL(stmt))
}

func firstSQLLine(stmt string) string {
	for _, line := range strings.Split(meaningfulSQL(stmt), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func hasNoTxStatements(statements []migrationStatement) bool {
	for _, statement := range statements {
		if statement.NoTx {
			return true
		}
	}
	return false
}

func withMigrationTx(ctx context.Context, conn *sql.Conn, file string, fn func(*sql.Tx) error) (err error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", file, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := setLocalMigrationTimeouts(ctx, tx, file); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", file, err)
	}
	committed = true
	return nil
}

func execMigrationStatements(ctx context.Context, tx *sql.Tx, file string, statements []migrationStatement) error {
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.SQL); err != nil {
			return fmt.Errorf("apply migration %s statement %q: %w", file, firstSQLLine(statement.SQL), err)
		}
	}
	return nil
}

func recordMigrationVersion(ctx context.Context, conn *sql.Conn, file string, version int64) error {
	return withMigrationTx(ctx, conn, file, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return fmt.Errorf("record migration %s: %w", file, err)
		}
		return nil
	})
}

func setLocalMigrationTimeouts(ctx context.Context, tx *sql.Tx, file string) error {
	if _, err := tx.ExecContext(ctx, `SELECT set_config('lock_timeout', $1, true)`, migrationTxLockTimeout); err != nil {
		return fmt.Errorf("set migration %s lock timeout: %w", file, err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('statement_timeout', $1, true)`, migrationTxStatementTimeout); err != nil {
		return fmt.Errorf("set migration %s statement timeout: %w", file, err)
	}
	return nil
}

func withNoTxStatementTimeouts(ctx context.Context, conn *sql.Conn, file string, fn func() error) (err error) {
	if _, err := conn.ExecContext(ctx, `SELECT set_config('lock_timeout', $1, false)`, migrationNoTxLockTimeout); err != nil {
		return fmt.Errorf("set migration %s no-tx lock timeout: %w", file, err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT set_config('statement_timeout', $1, false)`, migrationNoTxStatementTimeout); err != nil {
		return fmt.Errorf("set migration %s no-tx statement timeout: %w", file, err)
	}
	defer func() {
		resetErr := resetNoTxStatementTimeouts(ctx, conn, file)
		if err == nil && resetErr != nil {
			err = resetErr
		}
	}()
	return fn()
}

func resetNoTxStatementTimeouts(ctx context.Context, conn *sql.Conn, file string) error {
	if _, err := conn.ExecContext(ctx, `RESET lock_timeout`); err != nil {
		return fmt.Errorf("reset migration %s no-tx lock timeout: %w", file, err)
	}
	if _, err := conn.ExecContext(ctx, `RESET statement_timeout`); err != nil {
		return fmt.Errorf("reset migration %s no-tx statement timeout: %w", file, err)
	}
	return nil
}

func acquireMigrationLock(ctx context.Context, conn *sql.Conn) (func(context.Context) error, error) {
	const (
		lockTimeout       = 30 * time.Second
		lockRetryInterval = 250 * time.Millisecond
	)
	if conn == nil {
		return nil, fmt.Errorf("migration lock connection is nil")
	}
	scope, err := resolveMigrationLockScope(ctx, conn)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		var locked bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, scope.key).Scan(&locked); err != nil {
			return nil, fmt.Errorf("acquire migration lock (%s): %w", scope, err)
		}
		if locked {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("acquire migration lock (%s): lock is busy", scope)
		}
		sleep := lockRetryInterval
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("acquire migration lock (%s): %w", scope, ctx.Err())
		case <-timer.C:
		}
	}
	return func(ctx context.Context) error {
		var released bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, scope.key).Scan(&released); err != nil {
			return fmt.Errorf("release migration lock (%s): %w", scope, err)
		}
		if !released {
			return fmt.Errorf("release migration lock (%s): lock was not held", scope)
		}
		return nil
	}, nil
}

type migrationLockScope struct {
	database string
	schema   string
	key      int64
}

func (s migrationLockScope) String() string {
	return fmt.Sprintf("database=%q schema=%q key=%d", s.database, s.schema, s.key)
}

func resolveMigrationLockScope(ctx context.Context, conn *sql.Conn) (migrationLockScope, error) {
	var scope migrationLockScope
	if err := conn.QueryRowContext(ctx, `SELECT current_database(), current_schema()`).Scan(&scope.database, &scope.schema); err != nil {
		return migrationLockScope{}, fmt.Errorf("resolve migration lock scope (database=unknown schema=unknown key=unknown): %w", err)
	}
	if scope.database == "" || scope.schema == "" {
		return migrationLockScope{}, fmt.Errorf("resolve migration lock scope (database=%q schema=%q key=unknown): database and schema are required", scope.database, scope.schema)
	}
	scope.key = deriveMigrationLockKey(scope.database, scope.schema)
	return scope, nil
}

func deriveMigrationLockKey(database, schema string) int64 {
	digest := sha256.Sum256([]byte(migrationLockNamespace + "\x00" + database + "\x00" + schema))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func migrationVersion(file string) (int64, error) {
	name := path.Base(file)
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %s must start with numeric version", file)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse migration version %s: %w", file, err)
	}
	return version, nil
}

func migrationApplied(ctx context.Context, conn *sql.Conn, version int64) (bool, error) {
	var count int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = $1`, version).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func extractUpSQL(raw string) string {
	raw = strings.Replace(raw, "-- migrate:up", "", 1)
	if before, _, ok := strings.Cut(raw, "-- migrate:down"); ok {
		return before
	}
	return raw
}

func splitSQLStatements(raw string) []string {
	parsed := parseSQLStatements(raw)
	statements := make([]string, 0, len(parsed))
	for _, statement := range parsed {
		statements = append(statements, statement.SQL)
	}
	return statements
}

func parseSQLStatements(raw string) []migrationStatement {
	const (
		blockStart = "-- statement:begin"
		blockEnd   = "-- statement:end"
	)
	var statements []migrationStatement
	var regular strings.Builder
	for len(raw) > 0 {
		start := strings.Index(raw, blockStart)
		if start < 0 {
			regular.WriteString(raw)
			break
		}
		regular.WriteString(raw[:start])
		statements = append(statements, splitSemicolonStatements(regular.String())...)
		regular.Reset()

		blockBodyStart := start + len(blockStart)
		rest := raw[blockBodyStart:]
		end := strings.Index(rest, blockEnd)
		if end < 0 {
			regular.WriteString(raw[start:])
			break
		}
		block := strings.TrimSpace(rest[:end])
		if meaningfulSQL(block) != "" {
			statements = append(statements, newMigrationStatement(block))
		}
		raw = rest[end+len(blockEnd):]
	}
	statements = append(statements, splitSemicolonStatements(regular.String())...)
	return statements
}

func splitSemicolonStatements(raw string) []migrationStatement {
	const (
		sqlCode = iota
		sqlLineComment
		sqlBlockComment
		sqlSingleQuoted
		sqlDoubleQuoted
		sqlDollarQuoted
	)

	statements := make([]migrationStatement, 0)
	var statement strings.Builder
	state := sqlCode
	blockCommentDepth := 0
	dollarQuoteDelimiter := ""

	appendStatement := func() {
		stmt := strings.TrimSpace(statement.String())
		statement.Reset()
		if meaningfulSQL(stmt) == "" {
			return
		}
		statements = append(statements, newMigrationStatement(stmt))
	}

	for offset := 0; offset < len(raw); {
		switch state {
		case sqlCode:
			switch {
			case raw[offset] == ';':
				appendStatement()
				offset++
			case strings.HasPrefix(raw[offset:], "--"):
				statement.WriteString("--")
				offset += 2
				state = sqlLineComment
			case strings.HasPrefix(raw[offset:], "/*"):
				statement.WriteString("/*")
				offset += 2
				blockCommentDepth = 1
				state = sqlBlockComment
			case raw[offset] == '\'':
				statement.WriteByte(raw[offset])
				offset++
				state = sqlSingleQuoted
			case raw[offset] == '"':
				statement.WriteByte(raw[offset])
				offset++
				state = sqlDoubleQuoted
			default:
				delimiter, ok := sqlDollarQuoteDelimiter(raw, offset)
				if ok {
					statement.WriteString(delimiter)
					offset += len(delimiter)
					dollarQuoteDelimiter = delimiter
					state = sqlDollarQuoted
					continue
				}
				statement.WriteByte(raw[offset])
				offset++
			}
		case sqlLineComment:
			statement.WriteByte(raw[offset])
			if raw[offset] == '\n' {
				state = sqlCode
			}
			offset++
		case sqlBlockComment:
			switch {
			case strings.HasPrefix(raw[offset:], "/*"):
				statement.WriteString("/*")
				offset += 2
				blockCommentDepth++
			case strings.HasPrefix(raw[offset:], "*/"):
				statement.WriteString("*/")
				offset += 2
				blockCommentDepth--
				if blockCommentDepth == 0 {
					state = sqlCode
				}
			default:
				statement.WriteByte(raw[offset])
				offset++
			}
		case sqlSingleQuoted:
			statement.WriteByte(raw[offset])
			if raw[offset] == '\'' {
				if offset+1 < len(raw) && raw[offset+1] == '\'' {
					statement.WriteByte(raw[offset+1])
					offset += 2
					continue
				}
				state = sqlCode
			}
			offset++
		case sqlDoubleQuoted:
			statement.WriteByte(raw[offset])
			if raw[offset] == '"' {
				if offset+1 < len(raw) && raw[offset+1] == '"' {
					statement.WriteByte(raw[offset+1])
					offset += 2
					continue
				}
				state = sqlCode
			}
			offset++
		case sqlDollarQuoted:
			if strings.HasPrefix(raw[offset:], dollarQuoteDelimiter) {
				statement.WriteString(dollarQuoteDelimiter)
				offset += len(dollarQuoteDelimiter)
				dollarQuoteDelimiter = ""
				state = sqlCode
				continue
			}
			statement.WriteByte(raw[offset])
			offset++
		}
	}

	appendStatement()
	return statements
}

func sqlDollarQuoteDelimiter(raw string, offset int) (string, bool) {
	if offset >= len(raw) || raw[offset] != '$' {
		return "", false
	}
	end := offset + 1
	if end < len(raw) && raw[end] == '$' {
		return "$$", true
	}
	if end >= len(raw) || !isSQLIdentifierStart(raw[end]) {
		return "", false
	}
	for end++; end < len(raw) && isSQLIdentifierPart(raw[end]); end++ {
	}
	if end >= len(raw) || raw[end] != '$' {
		return "", false
	}
	return raw[offset : end+1], true
}

func isSQLIdentifierStart(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isSQLIdentifierPart(value byte) bool {
	return isSQLIdentifierStart(value) || value >= '0' && value <= '9'
}

func newMigrationStatement(stmt string) migrationStatement {
	return migrationStatement{
		SQL:       stmt,
		NoTx:      statementHasNoTx(stmt),
		IndexName: statementIndexName(stmt),
	}
}

func statementHasNoTx(stmt string) bool {
	for _, line := range strings.Split(stmt, "\n") {
		if strings.TrimSpace(line) == statementNoTx {
			return true
		}
	}
	return false
}

func statementIndexName(stmt string) string {
	for _, line := range strings.Split(stmt, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, statementIndexNamePrefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, statementIndexNamePrefix))
		}
	}
	return ""
}

func validateStatementIndexName(name string) error {
	if name == "" {
		return fmt.Errorf("index name is required")
	}
	for i, r := range name {
		if i == 0 && (r < 'a' || r > 'z') && r != '_' {
			return fmt.Errorf("must start with a letter or underscore")
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("only lowercase letters, digits, and underscore are supported")
	}
	return nil
}

func meaningfulSQL(stmt string) string {
	lines := strings.Split(stmt, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "\n")
}
