package schemarelease

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ideaboard/api/internal/db/migrations"
	"ideaboard/api/internal/db/schemaassembly"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Options struct {
	APIRoot       string
	ManifestPath  string
	OutputPath    string
	WriteArtifact bool
	BaseSHA       string
	HeadSHA       string
	DatabaseDSN   string
}

type commandRunner interface {
	Run(context.Context, string, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, directory, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}

func Check(ctx context.Context, options Options) error {
	return check(ctx, options, execRunner{})
}

func check(ctx context.Context, options Options, runner commandRunner) error {
	apiRoot, manifestPath, outputPath, err := normalizeOptions(options)
	if err != nil {
		return err
	}
	manifest, err := schemaassembly.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("schema release manifest: %w", err)
	}
	if outputPath == "" {
		outputPath = filepath.Join(filepath.Dir(manifestPath), manifest.Output)
	}
	assembled, err := schemaassembly.Assemble(manifest, filepath.Dir(manifestPath))
	if err != nil {
		return fmt.Errorf("schema release assembly: %w", err)
	}
	repeated, err := schemaassembly.Assemble(manifest, filepath.Dir(manifestPath))
	if err != nil {
		return fmt.Errorf("schema release deterministic assembly: %w", err)
	}
	if !bytes.Equal(assembled, repeated) {
		return errors.New("schema release deterministic assembly: repeated assembly produced different output")
	}
	if !options.WriteArtifact {
		committed, err := os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("schema release artifact drift: read %q: %w", outputPath, err)
		}
		if !bytes.Equal(assembled, committed) {
			return fmt.Errorf("schema release artifact drift: %s does not match manifest assembly", outputPath)
		}
	}
	if err := checkMigrationImmutability(ctx, runner, apiRoot, options.BaseSHA, options.HeadSHA); err != nil {
		return fmt.Errorf("schema release migration immutability: %w", err)
	}
	if err := checkSQLCGeneration(ctx, runner, apiRoot, assembled); err != nil {
		return fmt.Errorf("schema release sqlc generation: %w", err)
	}
	if err := verifyCatalogSemantics(ctx, strings.TrimSpace(options.DatabaseDSN), assembled); err != nil {
		return fmt.Errorf("schema release semantic verification: %w", err)
	}
	if options.WriteArtifact {
		if err := writeArtifact(outputPath, assembled); err != nil {
			return fmt.Errorf("schema release artifact generation: %w", err)
		}
	}
	return nil
}

func normalizeOptions(options Options) (string, string, string, error) {
	apiRoot := strings.TrimSpace(options.APIRoot)
	if apiRoot == "" {
		apiRoot = "."
	}
	absoluteRoot, err := filepath.Abs(apiRoot)
	if err != nil {
		return "", "", "", fmt.Errorf("schema release API root: %w", err)
	}
	manifestPath := resolvePath(absoluteRoot, options.ManifestPath, "internal/db/schema/manifest.json")
	outputPath := ""
	if strings.TrimSpace(options.OutputPath) != "" {
		outputPath = resolvePath(absoluteRoot, options.OutputPath, "")
	}
	if strings.TrimSpace(options.BaseSHA) == "" {
		return "", "", "", errors.New("schema release migration immutability: base SHA is required")
	}
	if strings.TrimSpace(options.DatabaseDSN) == "" {
		return "", "", "", errors.New("schema release semantic verification: database DSN is required")
	}
	return absoluteRoot, manifestPath, outputPath, nil
}

func resolvePath(root, value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(root, filepath.Clean(value))
}

func checkMigrationImmutability(ctx context.Context, runner commandRunner, apiRoot, baseSHA, headSHA string) error {
	baseSHA = strings.TrimSpace(baseSHA)
	headSHA = strings.TrimSpace(headSHA)
	if headSHA == "" {
		headSHA = "HEAD"
	}
	pathspec := "internal/db/migrations/*.sql"
	committed, err := runner.Run(ctx, apiRoot, "git", "diff", "--no-renames", "--name-status", baseSHA, headSHA, "--", pathspec)
	if err != nil {
		return fmt.Errorf("compare %s to %s: %w", baseSHA, headSHA, err)
	}
	working, err := runner.Run(ctx, apiRoot, "git", "diff", "--no-renames", "--name-status", headSHA, "--", pathspec)
	if err != nil {
		return fmt.Errorf("compare working tree to %s: %w", headSHA, err)
	}
	return validateMigrationChanges(string(committed) + string(working))
}

func validateMigrationChanges(raw string) error {
	violations := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			violations = append(violations, line)
			continue
		}
		if fields[0] != "A" {
			violations = append(violations, line)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("released migrations are append-only; invalid changes: %s", strings.Join(violations, ", "))
	}
	return nil
}

func checkSQLCGeneration(ctx context.Context, runner commandRunner, apiRoot string, assembled []byte) error {
	temporaryRoot, err := os.MkdirTemp("", "ideaboard-schema-release-")
	if err != nil {
		return fmt.Errorf("create temporary sqlc workspace: %w", err)
	}
	defer os.RemoveAll(temporaryRoot)

	if err := copyFile(filepath.Join(apiRoot, "sqlc.yaml"), filepath.Join(temporaryRoot, "sqlc.yaml")); err != nil {
		return err
	}
	if err := writeArtifact(filepath.Join(temporaryRoot, "internal/db/schema/schema.sql"), assembled); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(apiRoot, "internal/db/queries"), filepath.Join(temporaryRoot, "internal/db/queries")); err != nil {
		return err
	}
	configPath := filepath.Join(temporaryRoot, "sqlc.yaml")
	if output, err := runner.Run(ctx, apiRoot, "go", "tool", "sqlc", "generate", "-f", configPath); err != nil {
		return fmt.Errorf("generate sqlc output: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return compareTrees(filepath.Join(apiRoot, "internal/db/sqlc"), filepath.Join(temporaryRoot, "internal/db/sqlc"))
}

func writeArtifact(path string, content []byte) (resultErr error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create artifact directory %q: %w", directory, err)
	}
	temporary, err := os.CreateTemp(directory, ".schema-release-*")
	if err != nil {
		return fmt.Errorf("create temporary artifact for %q: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove temporary artifact for %q: %w", path, err))
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return closeArtifactAfterError(temporary, path, fmt.Errorf("set temporary artifact permissions for %q: %w", path, err))
	}
	if _, err := temporary.Write(content); err != nil {
		return closeArtifactAfterError(temporary, path, fmt.Errorf("write temporary artifact for %q: %w", path, err))
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary artifact for %q: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish artifact %q: %w", path, err)
	}
	return nil
}

func closeArtifactAfterError(artifact *os.File, path string, operationErr error) error {
	if err := artifact.Close(); err != nil {
		return errors.Join(operationErr, fmt.Errorf("close temporary artifact for %q: %w", path, err))
	}
	return operationErr
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %q: %w", source, err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create %q: %w", filepath.Dir(target), err)
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create %q: %w", target, err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	return nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(target, relative), 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse symlink in sqlc source: %s", path)
		}
		return copyFile(path, filepath.Join(target, relative))
	})
}

func compareTrees(expectedRoot, actualRoot string) error {
	expected, err := readTree(expectedRoot)
	if err != nil {
		return err
	}
	actual, err := readTree(actualRoot)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(expected)+len(actual))
	seen := make(map[string]struct{}, len(expected)+len(actual))
	for path := range expected {
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	for path := range actual {
		if _, exists := seen[path]; !exists {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		expectedContent, expectedExists := expected[path]
		actualContent, actualExists := actual[path]
		if expectedExists && actualExists && bytes.Equal(expectedContent, actualContent) {
			continue
		}
		return fmt.Errorf("generated sqlc drift: %s differs", path)
	}
	return nil
}

func readTree(root string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse symlink in generated output: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = content
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read generated tree %q: %w", root, err)
	}
	return result, nil
}

func verifyCatalogSemantics(ctx context.Context, dsn string, snapshot []byte) (err error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()

	snapshotConn, snapshotCleanup, err := openIsolatedSchema(ctx, db, "schema_release_snapshot")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, snapshotCleanup()) }()
	migrationConn, migrationCleanup, err := openIsolatedSchema(ctx, db, "schema_release_migration")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, migrationCleanup()) }()

	return migrations.VerifySchemaSnapshotContract(ctx, snapshotConn, migrationConn, snapshot)
}

func openIsolatedSchema(ctx context.Context, db *sql.DB, prefix string) (*sql.Conn, func() error, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return nil, nil, fmt.Errorf("generate isolated schema name: %w", err)
	}
	schemaName := fmt.Sprintf("%s_%x", prefix, random)
	quoted := quoteIdentifier(schemaName)
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+quoted); err != nil {
		return nil, nil, fmt.Errorf("create isolated schema: %w", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		_, _ = db.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quoted+" CASCADE")
		return nil, nil, fmt.Errorf("open isolated schema connection: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "SET search_path TO "+quoted+", public"); err != nil {
		_ = conn.Close()
		_, _ = db.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quoted+" CASCADE")
		return nil, nil, fmt.Errorf("set isolated schema search_path: %w", err)
	}
	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		closeErr := conn.Close()
		_, dropErr := db.ExecContext(cleanupCtx, "DROP SCHEMA IF EXISTS "+quoted+" CASCADE")
		return errors.Join(closeErr, dropErr)
	}
	return conn, cleanup, nil
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
