package backendconvergence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ideaboard/api/internal/archtest"
)

// These architecture checks keep backend convergence rules in go test ./....

// TestV1NoOffsetOrBareCursor bans OFFSET SQL and legacy cursor dialects in
// the v1 surface: only cursor + page_size via the shared v1OffsetPage seam.
func TestV1NoOffsetOrBareCursor(t *testing.T) {
	archtest.AssertNoForbiddenText(t, "../runtime/interfaces/http/v1", []string{
		"OFFSET",
		"BeforeIDFromGin",
		"ParseBeforeID",
		"LimitFromGin",
		"ParseLimit",
		"BeforeIDParam",
		"LimitParam",
		`Query("cursor"`,
	})
}

// TestNoNewFakeTxStruct bans new FakeTx struct definitions: settlement
// fakes converge on db/dbutil/testscope.Tx via type aliases.
func TestNoNewFakeTxStruct(t *testing.T) {
	archtest.AssertNoForbiddenText(t, "..", []string{
		"FakeTx struct",
		"fakeTx struct",
	})
}

// TestControlAuditWritesConverge bans direct control_audit_records writes
// outside the durable sink (platform/audit) and generated sqlc.
func TestControlAuditWritesConverge(t *testing.T) {
	assertNoForbiddenTextOutside(t, []string{
		"INSERT INTO control_audit_records",
		"UPDATE control_audit_records",
		"DELETE FROM control_audit_records",
	}, "platform/audit", "db/sqlc")
}

// TestNoNewPoolNotConfiguredLiteral keeps the pool error text in its sentinels.
func TestNoNewPoolNotConfiguredLiteral(t *testing.T) {
	assertNoForbiddenTextOutside(t, []string{"database pool not configured"},
		"platform/database/errors.go", "billingrecords/ports/types.go")
}

func assertNoForbiddenTextOutside(t *testing.T, patterns []string, allowedPaths ...string) {
	t.Helper()
	root := ".."
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		for _, allowed := range allowedPaths {
			allowed = filepath.Join(root, allowed)
			if path == allowed || strings.HasPrefix(path, allowed+string(os.PathSeparator)) {
				return nil
			}
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, pattern := range patterns {
			if strings.Contains(string(raw), pattern) {
				t.Errorf("production file %s contains forbidden text %q", path, pattern)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan %s: %v", root, walkErr)
	}
}
