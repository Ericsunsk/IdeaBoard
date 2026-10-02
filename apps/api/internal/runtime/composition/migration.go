package composition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrCleanBreakNotAllowed is returned when the database records Atlas
// revisions that the shipped migration directory does not contain, and the
// operator has not explicitly opted into the destructive reset. Callers must
// treat it as fatal: the deploy is pointed at a different migration history
// (wrong image, rollback to an older image, renamed/removed migration file,
// or a misconfigured migrations directory), and the database must not be
// touched.
var ErrCleanBreakNotAllowed = errors.New("clean break reset is not allowed")

// ErrCleanBreakRevisionsAmbiguous is returned when the catalog holds more than
// one table named atlas_schema_revisions. Picking one would make the recorded
// version list of an arbitrary schema authoritative and could justify dropping
// the public schema on the strength of another schema's history, so
// reconciliation refuses to guess and requires an operator to decide which
// schema Atlas is meant to own.
var ErrCleanBreakRevisionsAmbiguous = errors.New("multiple atlas schema revisions tables found")

// ErrCleanBreakRevisionsUnresolved is returned when the database still carries
// an object named atlas_schema_revisions but the catalog lookup produced no
// resolvable revisions table. That combination means the catalog answered
// something the reconciliation does not understand, and treating it as a fresh
// database would skip detection entirely: the deploy would fall through to
// `atlas migrate apply` and die with "connected database is not clean" while
// ATLAS_CLEAN_BREAK_ALLOW_RESET never gets evaluated.
var ErrCleanBreakRevisionsUnresolved = errors.New("atlas schema revisions table could not be resolved")

// cleanBreakRevisionsTableName is the fixed name Atlas gives its revisions
// table. Only the schema holding it varies between deployments.
const cleanBreakRevisionsTableName = "atlas_schema_revisions"

// cleanBreakRevisionsCandidateQuery lists every schema that holds a table named
// atlas_schema_revisions.
//
// The schema is deliberately not hardcoded. Atlas lets the revisions schema be
// configured — atlas.hcl's env.<name>.migration supports a `schema` field, the
// CLI accepts --schema, and a connection role with an unexpected search_path
// silently relocates otherwise unqualified DDL. A lookup that only probes
// "atlas_schema_revisions" and "public" therefore misses real deployments, and
// it misses them silently: the lookup returns NULL, reconciliation concludes
// "fresh database", and the following `atlas migrate apply` fails with
// "connected database is not clean" no matter what the operator set
// ATLAS_CLEAN_BREAK_ALLOW_RESET to, because detection never ran.
//
// pg_namespace is joined through relnamespace rather than compared by name,
// because the table inside the atlas_schema_revisions schema is itself named
// atlas_schema_revisions: matching the schema name alone would treat the schema
// as if it were the table.
const cleanBreakRevisionsCandidateQuery = `
	SELECT n.nspname::text
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE c.relname = $1
		AND c.relkind IN ('r', 'p')
		AND n.nspname <> 'pg_catalog'
		AND n.nspname <> 'information_schema'
	ORDER BY n.nspname ASC
`

// cleanBreakRevisionsMarkerQuery reports whether anything at all is still named
// atlas_schema_revisions, in any relation kind or as a schema. It is only
// consulted when cleanBreakRevisionsCandidateQuery finds no table, to tell a
// genuinely fresh database apart from a catalog state that reconciliation
// failed to interpret.
const cleanBreakRevisionsMarkerQuery = `
	SELECT
		EXISTS (SELECT 1 FROM pg_class WHERE relname = $1),
		EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)
`

// cleanBreakRowCountQuery summarises every table in one schema so the
// destructive reset leaves an auditable record of what it is about to drop.
// query_to_xml runs the per-table count because a single statement cannot
// reference a table name chosen at runtime. The schema is a parameter because
// the reset may destroy more than public when Atlas wrote its revisions into a
// stray schema.
const cleanBreakRowCountQuery = `
	WITH counts AS (
		SELECT c.relname,
			(xpath('/row/cnt/text()', query_to_xml(
				format('SELECT count(*)::bigint AS cnt FROM %I.%I', $1, c.relname),
				false, true, ''
			)))[1]::text::bigint AS row_count
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relkind = 'r'
	)
	SELECT COALESCE(string_agg(format('%s.%s=%s', $1, relname, row_count), ', ' ORDER BY relname), ''),
		COALESCE(sum(row_count), 0)
	FROM counts
`

// Migrate applies Atlas migrations with the schema-owner role, then performs
// the runtime-only River and privilege finalization step.
func Migrate(ctx context.Context, cfg *config.Config, atlasBinary, migrationsDirectory string) error {
	if cfg == nil {
		return fmt.Errorf("migration config is nil")
	}
	if err := cfg.ValidateDatabase(); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}
	migrationConfig, err := separatedMigrationDatabaseConfig(cfg.Database)
	if err != nil {
		return err
	}
	if strings.TrimSpace(atlasBinary) == "" {
		atlasBinary = config.DefaultAtlasBinary
	}
	if strings.TrimSpace(migrationsDirectory) == "" {
		migrationsDirectory = config.DefaultMigrationDirectory
	}
	if err := reconcileCleanBreakDatabase(ctx, migrationConfig, migrationsDirectory, cfg.Migration.CleanBreakAllowReset); err != nil {
		return fmt.Errorf("clean break reconciliation failed: %w", err)
	}
	cmd := exec.CommandContext(ctx, atlasBinary, "migrate", "apply", "--dir", migrationsDirectory, "--url", database.PostgresDSN(migrationConfig))
	var output bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("atlas schema migration failed: %w (output: %s)", err, strings.TrimSpace(output.String()))
	}
	if err := finalizeAppDatabases(ctx, cfg); err != nil {
		return fmt.Errorf("runtime migration finalization failed: %w", err)
	}
	return nil
}

// reconcileCleanBreakDatabase drops and recreates the public schema when the
// database records Atlas revisions that the shipped migration directory does
// not contain. This is a pre-launch clean break, not a compatibility
// mechanism: it discards every row in the database.
//
// The reset is therefore opt-in. When missing revisions are detected and
// allowCleanBreakReset is false the function returns ErrCleanBreakNotAllowed
// with the offending versions and drops nothing, so a deploy that accidentally
// ships a different migration history fails instead of destroying production
// data.
//
// allowCleanBreakReset comes from ATLAS_CLEAN_BREAK_ALLOW_RESET. Callers other
// than Migrate must pass an explicit value; there is no default-true path.
func reconcileCleanBreakDatabase(ctx context.Context, migrationConfig config.DatabaseConfig, migrationsDirectory string, allowCleanBreakReset bool) error {
	db, err := database.NewPostgres(ctx, migrationConfig)
	if err != nil {
		return fmt.Errorf("connect migration database: %w", err)
	}
	defer db.Close()

	return reconcileCleanBreak(ctx, db, migrationConfig, migrationsDirectory, allowCleanBreakReset)
}

// cleanBreakDatabase is the narrow database surface the clean break
// reconciliation needs. Keeping it an interface lets tests exercise the
// destructive branch without dropping a real schema.
type cleanBreakDatabase interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// resolveCleanBreakRevisionsSchema locates the schema holding Atlas' revisions
// table and returns its name. An empty string means the database really has no
// revisions table (fresh database); the caller is responsible for turning the
// name into an identifier with pgx.Identifier.Sanitize.
//
// The lookup is fail-closed in both directions: a catalog error is propagated
// rather than downgraded to "fresh", and more than one candidate is an error
// rather than an arbitrary pick.
func resolveCleanBreakRevisionsSchema(ctx context.Context, db cleanBreakDatabase) (string, error) {
	rows, err := db.Query(ctx, cleanBreakRevisionsCandidateQuery, cleanBreakRevisionsTableName)
	if err != nil {
		return "", fmt.Errorf("search catalog for %s: %w", cleanBreakRevisionsTableName, err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return "", fmt.Errorf("scan %s candidate: %w", cleanBreakRevisionsTableName, err)
		}
		if schema != "" {
			schemas = append(schemas, schema)
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate %s candidates: %w", cleanBreakRevisionsTableName, err)
	}

	if len(schemas) > 1 {
		slog.Error("clean break blocked: the database holds more than one atlas schema revisions table; refusing to guess which schema owns the recorded history",
			"candidate_schemas", schemas,
			"opt_in_env", "ATLAS_CLEAN_BREAK_ALLOW_RESET",
		)
		return "", fmt.Errorf("%w: found tables named %s in schemas [%s]. "+
			"Treating an arbitrary candidate as authoritative could read the wrong recorded version history and use it to justify dropping the public schema. "+
			"Drop the stale revisions table from the schema Atlas no longer uses (or align the migration schema), then redeploy. "+
			"Setting ATLAS_CLEAN_BREAK_ALLOW_RESET does not resolve this: the ambiguity is about which history is real, not about permission to reset.",
			ErrCleanBreakRevisionsAmbiguous, cleanBreakRevisionsTableName, strings.Join(schemas, ", "))
	}
	if len(schemas) == 1 {
		return schemas[0], nil
	}

	var hasRelation, hasSchema bool
	if err := db.QueryRow(ctx, cleanBreakRevisionsMarkerQuery, cleanBreakRevisionsTableName).Scan(&hasRelation, &hasSchema); err != nil {
		return "", fmt.Errorf("check for leftover %s objects: %w", cleanBreakRevisionsTableName, err)
	}
	if hasRelation || hasSchema {
		slog.Error("clean break blocked: the database still holds an object named atlas_schema_revisions but no resolvable revisions table",
			"relation_exists", hasRelation,
			"schema_exists", hasSchema,
			"opt_in_env", "ATLAS_CLEAN_BREAK_ALLOW_RESET",
		)
		return "", fmt.Errorf("%w: the database holds an object named %s (relation=%v, schema=%v) but the catalog lookup returned no revisions table. "+
			"Refusing to treat this as a fresh database, because detection would silently not run and `atlas migrate apply` would then fail with a dirty database that ATLAS_CLEAN_BREAK_ALLOW_RESET cannot unblock. "+
			"Inspect the database manually and drop the stale object, then redeploy.",
			ErrCleanBreakRevisionsUnresolved, cleanBreakRevisionsTableName, hasRelation, hasSchema)
	}
	return "", nil
}

func reconcileCleanBreak(ctx context.Context, db cleanBreakDatabase, migrationConfig config.DatabaseConfig, migrationsDirectory string, allowCleanBreakReset bool) error {
	revisionsSchema, err := resolveCleanBreakRevisionsSchema(ctx, db)
	if err != nil {
		return err
	}
	if revisionsSchema == "" {
		// Fresh database: Atlas has not recorded anything yet.
		return nil
	}
	revisionsTable := pgx.Identifier{revisionsSchema, cleanBreakRevisionsTableName}.Sanitize()

	rows, err := db.Query(ctx, fmt.Sprintf("SELECT version::text FROM %s ORDER BY version ASC", revisionsTable))
	if err != nil {
		return fmt.Errorf("query atlas schema revisions: %w", err)
	}
	defer rows.Close()

	var recordedVersions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err == nil && v != "" {
			recordedVersions = append(recordedVersions, v)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate atlas schema revisions: %w", err)
	}
	if len(recordedVersions) == 0 {
		return nil
	}

	dirPath := strings.TrimPrefix(migrationsDirectory, "file://")
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("read migrations directory %s: %w", dirPath, err)
	}

	availableVersions := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".sql") {
			parts := strings.SplitN(name, "_", 2)
			if len(parts) > 0 {
				availableVersions[parts[0]] = true
			}
		}
	}

	var missingVersions []string
	for _, rec := range recordedVersions {
		if !availableVersions[rec] {
			missingVersions = append(missingVersions, rec)
		}
	}
	if len(missingVersions) == 0 {
		return nil
	}

	var databaseName string
	if err := db.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		databaseName = migrationConfig.Database
	}

	if !allowCleanBreakReset {
		slog.Error("clean break blocked: recorded Atlas revisions are missing from the migrations directory; refusing to drop the public schema",
			"database", databaseName,
			"migrations_directory", dirPath,
			"missing_versions", missingVersions,
			"recorded_version_count", len(recordedVersions),
			"opt_in_env", "ATLAS_CLEAN_BREAK_ALLOW_RESET",
		)
		return fmt.Errorf("%w: database %q records %d Atlas revision(s) absent from %s: %s. "+
			"This deploy ships a different migration history than the database (wrong image, image rollback, or a renamed/removed migration file). "+
			"Deploy the image that matches the recorded history, or restore a matching migration directory. "+
			"Only if this is an intended pre-launch clean break, after exporting the data you need to keep, set ATLAS_CLEAN_BREAK_ALLOW_RESET=true to allow the public schema to be dropped and recreated.",
			ErrCleanBreakNotAllowed, databaseName, len(missingVersions), dirPath, strings.Join(missingVersions, ", "))
	}

	// Every schema the reset destroys, so the audit log below describes the
	// whole blast radius and not just public.
	schemasToDrop := []string{"public"}
	if revisionsSchema != "public" {
		schemasToDrop = append(schemasToDrop, revisionsSchema)
	}

	auditSummaries := make([]string, 0, len(schemasToDrop))
	var totalRows int64
	for _, schemaName := range schemasToDrop {
		var summary string
		var schemaRows int64
		if err := db.QueryRow(ctx, cleanBreakRowCountQuery, schemaName).Scan(&summary, &schemaRows); err != nil {
			// The audit trail is best-effort; losing it must not silently change
			// the decision the operator already made by opting in.
			slog.Warn("clean break: could not summarize schema row counts", "database", databaseName, "schema", schemaName, "err", err)
			continue
		}
		if summary != "" {
			auditSummaries = append(auditSummaries, summary)
		}
		totalRows += schemaRows
	}
	tableRowCounts := strings.Join(auditSummaries, "; ")

	slog.Warn("clean break: dropping schemas because recorded Atlas revisions are missing from the migrations directory",
		"database", databaseName,
		"schemas_dropped", schemasToDrop,
		"revisions_schema", revisionsSchema,
		"migrations_directory", dirPath,
		"missing_versions", missingVersions,
		"recorded_version_count", len(recordedVersions),
		"available_migration_count", len(availableVersions),
		"table_row_counts", tableRowCounts,
		"total_rows", totalRows,
		"opt_in_env", "ATLAS_CLEAN_BREAK_ALLOW_RESET",
	)

	owner := strings.TrimSpace(migrationConfig.User)
	if owner == "" {
		owner = "ideaboard"
	}
	sanitizedOwner := pgx.Identifier{owner}.Sanitize()

	// Dropping the revisions table is not enough on its own. `atlas migrate apply`
	// only runs its clean-database check when it reads zero revisions, and that
	// check rejects the database when it holds ANY schema other than public, even
	// an empty one. So when Atlas wrote its revisions into a stray schema, that
	// schema's mere existence is what produces
	// "connected database is not clean: found schema ...", and leaving it behind
	// would keep the deploy failing after the operator opted in. Dropping it
	// CASCADE also removes the business tables a broken search_path parked
	// there, which is what this opt-in already promises.
	//
	// Both identifiers are produced by pgx.Identifier.Sanitize, and every drop
	// is guarded with IF EXISTS, so re-dropping one of the fixed locations is
	// harmless.
	extraRevisionsSchemaDrop := ""
	if revisionsSchema != "public" {
		extraRevisionsSchemaDrop = fmt.Sprintf("\n\t\tDROP SCHEMA IF EXISTS %s CASCADE;", pgx.Identifier{revisionsSchema}.Sanitize())
	}
	stmt := fmt.Sprintf(`
		DROP SCHEMA IF EXISTS atlas_schema_revisions CASCADE;
		DROP TABLE IF EXISTS public.atlas_schema_revisions CASCADE;
		DROP TABLE IF EXISTS %s CASCADE;%s
		DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;
		ALTER SCHEMA public OWNER TO %s;
		GRANT ALL ON SCHEMA public TO %s;
		GRANT ALL ON SCHEMA public TO public;
	`, revisionsTable, extraRevisionsSchemaDrop, sanitizedOwner, sanitizedOwner)
	if _, err := db.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("reset clean break schema: %w", err)
	}

	return nil
}
