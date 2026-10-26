package migration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// cleanBreakStubDB is a scripted cleanBreakDatabase. It records every Exec so
// tests can assert the destructive statement is or is not issued.
type cleanBreakStubDB struct {
	// revisionsSchemas is what the catalog lookup reports: one entry per schema
	// holding a table named atlas_schema_revisions. Nil or empty means the
	// lookup found no revisions table.
	revisionsSchemas []string
	// leftoverMarker marks an object still named atlas_schema_revisions that the
	// catalog lookup did not resolve to a table.
	leftoverMarker bool
	// versions are the Atlas revisions recorded in the database.
	versions []string
	// rowCountSummary and totalRows are returned by the audit query.
	rowCountSummary string
	totalRows       int64
	// catalogQueryErr / markerQueryErr / queryRowErr / queryErr / execErr inject
	// failures per stage.
	catalogQueryErr error
	markerQueryErr  error
	queryRowErr     error
	queryErr        error
	execErr         error

	queries  []string
	executed []string
}

func (db *cleanBreakStubDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	db.queries = append(db.queries, sql)
	if db.queryRowErr != nil {
		return cleanBreakStubRow{err: db.queryRowErr}
	}
	switch {
	case strings.Contains(sql, "FROM pg_namespace WHERE nspname"):
		if db.markerQueryErr != nil {
			return cleanBreakStubRow{err: db.markerQueryErr}
		}
		// Atlas marker probe: relation named atlas_schema_revisions, and schema
		// named atlas_schema_revisions.
		return cleanBreakStubRow{bools: []bool{db.leftoverMarker, db.leftoverMarker}}
	case strings.Contains(sql, "current_database"):
		return cleanBreakStubRow{text: "ideaboard"}
	case strings.Contains(sql, "query_to_xml"):
		return cleanBreakStubRow{summary: db.rowCountSummary, total: db.totalRows}
	}
	return cleanBreakStubRow{err: errors.New("unexpected QueryRow: " + sql)}
}

func (db *cleanBreakStubDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	db.queries = append(db.queries, sql)
	switch {
	case strings.Contains(sql, "relkind IN"):
		if db.catalogQueryErr != nil {
			return nil, db.catalogQueryErr
		}
		return &cleanBreakStubRows{schemas: db.revisionsSchemas}, nil
	case strings.Contains(sql, "atlas_schema_revisions"):
		if db.queryErr != nil {
			return nil, db.queryErr
		}
		return &cleanBreakStubRows{versions: db.versions}, nil
	}
	return nil, errors.New("unexpected Query: " + sql)
}

func (db *cleanBreakStubDB) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	db.executed = append(db.executed, sql)
	if db.execErr != nil {
		return pgconn.CommandTag{}, db.execErr
	}
	return pgconn.CommandTag{}, nil
}

// consultedMarkerProbe reports whether the code under test ran the fallback
// probe that separates a genuinely fresh database from a catalog state it
// could not interpret.
func (db *cleanBreakStubDB) consultedMarkerProbe() bool {
	for _, q := range db.queries {
		if strings.Contains(q, "FROM pg_namespace WHERE nspname") {
			return true
		}
	}
	return false
}

type cleanBreakStubRow struct {
	text    string
	summary string
	total   int64
	bools   []bool
	err     error
}

func (r cleanBreakStubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) == 2 {
		if r.bools != nil {
			// Atlas marker probe: two EXISTS flags.
			for i, target := range dest {
				flag, ok := target.(*bool)
				if !ok {
					return errors.New("unexpected marker scan target")
				}
				*flag = r.bools[i]
			}
			return nil
		}
		// Row-count audit query: summary string plus total row count.
		summary, ok := dest[0].(*string)
		if !ok {
			return errors.New("unexpected row-count scan target")
		}
		*summary = r.summary
		total, ok := dest[1].(*int64)
		if !ok {
			return errors.New("unexpected row-count scan target")
		}
		*total = r.total
		return nil
	}
	switch target := dest[0].(type) {
	case *string:
		*target = r.text
		return nil
	}
	return errors.New("unexpected scan target type")
}

// cleanBreakStubRows serves the two row shapes the reconciliation reads: schema
// names from the catalog lookup, then recorded versions.
type cleanBreakStubRows struct {
	pgx.Rows
	schemas  []string
	versions []string
	cursor   int
}

func (r *cleanBreakStubRows) Next() bool {
	if r.cursor >= len(r.schemas)+len(r.versions) {
		return false
	}
	r.cursor++
	return true
}

func (r *cleanBreakStubRows) Scan(dest ...any) error {
	target, ok := dest[0].(*string)
	if !ok {
		return errors.New("unexpected string scan target")
	}
	if r.cursor <= len(r.schemas) {
		*target = r.schemas[r.cursor-1]
		return nil
	}
	*target = r.versions[r.cursor-len(r.schemas)-1]
	return nil
}

func (r *cleanBreakStubRows) Err() error { return nil }
func (r *cleanBreakStubRows) Close()     {}

// writeCleanBreakMigrationsDir creates a migrations directory containing the
// given version prefixes.
func writeCleanBreakMigrationsDir(t *testing.T, versions ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, version := range versions {
		name := version + "_fixture.sql"
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;\n"), 0o600); err != nil {
			t.Fatalf("write migration fixture %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "atlas.sum"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatalf("write atlas.sum fixture: %v", err)
	}
	return dir
}

func cleanBreakMigrationConfig() config.DatabaseConfig {
	return config.DatabaseConfig{User: "ideaboard", Database: "ideaboard"}
}

func containsDropPublicSchema(executed []string) bool {
	for _, stmt := range executed {
		if strings.Contains(stmt, "DROP SCHEMA public CASCADE") {
			return true
		}
	}
	return false
}

func TestCleanBreakSkipsResetWhenAllRevisionsAvailable(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001", "20260928000002")
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"atlas_schema_revisions"},
		versions:         []string{"20260925000001", "20260928000002"},
	}

	if err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, false); err != nil {
		t.Fatalf("reconcileCleanBreak() error = %v, want nil", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
}

func TestCleanBreakBlocksMissingRevisionsWithoutOptIn(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"atlas_schema_revisions"},
		versions:         []string{"20260925000001", "20260928000002", "20260930000003"},
	}

	err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, false)
	if err == nil {
		t.Fatal("reconcileCleanBreak() error = nil, want ErrCleanBreakNotAllowed")
	}
	if !errors.Is(err, ErrCleanBreakNotAllowed) {
		t.Fatalf("reconcileCleanBreak() error = %v, want ErrCleanBreakNotAllowed", err)
	}
	// The diagnostic must name every missing version and the opt-in env var.
	for _, missing := range []string{"20260928000002", "20260930000003"} {
		if !strings.Contains(err.Error(), missing) {
			t.Fatalf("error %q does not name missing version %s", err, missing)
		}
	}
	if !strings.Contains(err.Error(), "ATLAS_CLEAN_BREAK_ALLOW_RESET") {
		t.Fatalf("error %q does not mention the opt-in env var", err)
	}
	if containsDropPublicSchema(db.executed) {
		t.Fatal("public schema was dropped without opt-in")
	}
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
}

func TestCleanBreakDropsSchemaWhenOptedIn(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"atlas_schema_revisions"},
		versions:         []string{"20260925000001", "20260928000002"},
		rowCountSummary:  "boards=16, users=3",
		totalRows:        19,
	}

	if err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, true); err != nil {
		t.Fatalf("reconcileCleanBreak() error = %v, want nil", err)
	}
	if !containsDropPublicSchema(db.executed) {
		t.Fatalf("executed statements = %v, want the clean break reset", db.executed)
	}
	drop := db.executed[0]
	for _, fragment := range []string{
		"DROP SCHEMA IF EXISTS atlas_schema_revisions CASCADE",
		"DROP SCHEMA public CASCADE",
		"CREATE SCHEMA public",
	} {
		if !strings.Contains(drop, fragment) {
			t.Fatalf("reset statement missing %q:\n%s", fragment, drop)
		}
	}
}

func TestCleanBreakSkipsFreshDatabaseWithoutRevisions(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	// No revisions table at all: the first deploy of a fresh database.
	db := &cleanBreakStubDB{}

	for _, allowReset := range []bool{false, true} {
		if err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, allowReset); err != nil {
			t.Fatalf("reconcileCleanBreak(allowReset=%v) error = %v, want nil", allowReset, err)
		}
		if len(db.executed) != 0 {
			t.Fatalf("executed statements = %v, want none", db.executed)
		}
	}
	// The fresh verdict must be earned: an empty catalog alone is not enough,
	// the marker probe has to confirm no object named atlas_schema_revisions is
	// left over either.
	if !db.consultedMarkerProbe() {
		t.Fatal("fresh database was accepted without probing for leftover atlas_schema_revisions objects")
	}
}

func TestCleanBreakFindsRevisionsTableInUnexpectedSchema(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001", "20260928000002")
	// Atlas wrote its revisions into a schema nobody hardcoded, the way a role
	// with a broken search_path does.
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"srmig"},
		versions:         []string{"20260925000001", "20260928000002"},
	}

	if err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, false); err != nil {
		t.Fatalf("reconcileCleanBreak() error = %v, want nil", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
	if db.consultedMarkerProbe() {
		t.Fatal("marker probe ran even though the catalog resolved a revisions table")
	}
	// The recorded history of srmig must be the one consulted. pgx.Identifier
	// sanitizes, hence the quoting.
	if !strings.Contains(db.queries[1], `FROM "srmig"."atlas_schema_revisions"`) {
		t.Fatalf("version query = %q, want it to read srmig.atlas_schema_revisions", db.queries[1])
	}
}

func TestCleanBreakBlocksUnexpectedSchemaWithoutOptIn(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"srmig"},
		versions:         []string{"20260925000001", "20260928000002"},
	}

	err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, false)
	if !errors.Is(err, ErrCleanBreakNotAllowed) {
		t.Fatalf("reconcileCleanBreak() error = %v, want ErrCleanBreakNotAllowed", err)
	}
	if !strings.Contains(err.Error(), "20260928000002") {
		t.Fatalf("error %q does not name the missing version", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
}

func TestCleanBreakDropsWhenRevisionsLiveInUnexpectedSchema(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"srmig"},
		versions:         []string{"20260925000001", "20260928000002"},
		rowCountSummary:  "boards=16",
		totalRows:        16,
	}

	if err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, true); err != nil {
		t.Fatalf("reconcileCleanBreak() error = %v, want nil", err)
	}
	if !containsDropPublicSchema(db.executed) {
		t.Fatalf("executed statements = %v, want the clean break reset", db.executed)
	}
	// Dropping only the two historical locations would leave the dirty revisions
	// table behind and `atlas migrate apply` would still refuse to run.
	if !strings.Contains(db.executed[0], `DROP TABLE IF EXISTS "srmig"."atlas_schema_revisions" CASCADE`) {
		t.Fatalf("reset statement does not drop the discovered revisions table:\n%s", db.executed[0])
	}
	// The schema itself is the blocker: Atlas rejects a database that holds any
	// schema other than public, even an empty one, so the reset has to remove it
	// or the deploy keeps failing after the operator opted in.
	if !strings.Contains(db.executed[0], `DROP SCHEMA IF EXISTS "srmig" CASCADE`) {
		t.Fatalf("reset statement does not drop the discovered revisions schema:\n%s", db.executed[0])
	}
	// Both schemas about to be destroyed must be audited, not just public.
	var audited int
	for _, q := range db.queries {
		if strings.Contains(q, "query_to_xml") {
			audited++
		}
	}
	if audited != 2 {
		t.Fatalf("audit queries = %d, want 2 (public plus the discovered schema); queries = %v", audited, db.queries)
	}
}

func TestCleanBreakDropsNoStraySchemaWhenRevisionsAreInPublic(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	// public is already destroyed wholesale, so no extra schema drop is needed.
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"public"},
		versions:         []string{"20260925000001", "20260928000002"},
	}

	if err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, true); err != nil {
		t.Fatalf("reconcileCleanBreak() error = %v, want nil", err)
	}
	drop := db.executed[0]
	if strings.Count(drop, "DROP SCHEMA public CASCADE") != 1 {
		t.Fatalf("reset statement drops public more than once:\n%s", drop)
	}
	var audited int
	for _, q := range db.queries {
		if strings.Contains(q, "query_to_xml") {
			audited++
		}
	}
	if audited != 1 {
		t.Fatalf("audit queries = %d, want 1 (public only); queries = %v", audited, db.queries)
	}
}

func TestCleanBreakRejectsMultipleRevisionsCandidates(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	// Two histories are on record; choosing one would treat an arbitrary schema
	// as authoritative and could license a reset nobody intended.
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"atlas_schema_revisions", "public", "srmig"},
		versions:         []string{"20260801000000"},
	}

	err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, true)
	if !errors.Is(err, ErrCleanBreakRevisionsAmbiguous) {
		t.Fatalf("reconcileCleanBreak() error = %v, want ErrCleanBreakRevisionsAmbiguous", err)
	}
	for _, schema := range []string{"public", "srmig"} {
		if !strings.Contains(err.Error(), schema) {
			t.Fatalf("error %q does not name candidate schema %s", err, schema)
		}
	}
	// Opting in must not paper over the ambiguity.
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
}

func TestCleanBreakPropagatesCatalogLookupFailure(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	db := &cleanBreakStubDB{catalogQueryErr: errors.New("permission denied for table pg_class")}

	err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, true)
	if !errors.Is(err, db.catalogQueryErr) {
		t.Fatalf("reconcileCleanBreak() error = %v, want the catalog failure to propagate", err)
	}
	// A failed lookup must not be downgraded to "fresh database".
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
	if errors.Is(err, ErrCleanBreakNotAllowed) {
		t.Fatalf("error = %v, want it not to masquerade as a clean break verdict", err)
	}
}

func TestCleanBreakRejectsLeftoverRevisionsObjectWithoutTable(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	// Something is still named atlas_schema_revisions but the lookup resolved no
	// revisions table. Calling this fresh would skip detection entirely.
	db := &cleanBreakStubDB{leftoverMarker: true}

	err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, true)
	if !errors.Is(err, ErrCleanBreakRevisionsUnresolved) {
		t.Fatalf("reconcileCleanBreak() error = %v, want ErrCleanBreakRevisionsUnresolved", err)
	}
	if !strings.Contains(err.Error(), "ATLAS_CLEAN_BREAK_ALLOW_RESET") {
		t.Fatalf("error %q does not explain why the opt-in cannot help", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
}

func TestCleanBreakPropagatesMarkerProbeFailure(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	db := &cleanBreakStubDB{markerQueryErr: errors.New("connection reset")}

	err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, false)
	if !errors.Is(err, db.markerQueryErr) {
		t.Fatalf("reconcileCleanBreak() error = %v, want the marker probe failure to propagate", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
}

func TestCleanBreakSkipsDatabaseWithEmptyRevisions(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	// Revisions table exists but holds no rows yet.
	db := &cleanBreakStubDB{revisionsSchemas: []string{"atlas_schema_revisions"}}

	if err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, false); err != nil {
		t.Fatalf("reconcileCleanBreak() error = %v, want nil", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("executed statements = %v, want none", db.executed)
	}
}

func TestCleanBreakOptInPropagatesResetFailure(t *testing.T) {
	dir := writeCleanBreakMigrationsDir(t, "20260925000001")
	db := &cleanBreakStubDB{
		revisionsSchemas: []string{"atlas_schema_revisions"},
		versions:         []string{"20260801000000"},
		execErr:          errors.New("permission denied for schema public"),
	}

	err := reconcileCleanBreak(t.Context(), db, cleanBreakMigrationConfig(), dir, true)
	if err == nil || !strings.Contains(err.Error(), "reset clean break schema") {
		t.Fatalf("reconcileCleanBreak() error = %v, want reset failure", err)
	}
}
