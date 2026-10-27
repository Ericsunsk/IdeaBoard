package runtime

// Result-compensation fan-out guard: media ingestion runs outside the
// settlement transaction, so every result ingest path must converge on the
// unique detached compensation entry (compensateIngestedResultAssets), and
// no new ArchiveUnreferencedAsset call may appear beside it. A new ingest path that
// does not compensate fails here.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResultCompensationConvergesOnUniqueDetachedEntry proves the detached
// archive fan-out has exactly one production call site: the unique entry
// in renderer.go. A second ArchiveUnreferencedAsset beside it means a compensation
// path bypasses the bounded detached context and the warn-and-stay-live
// posture, so it must route through the entry instead.
func TestResultCompensationConvergesOnUniqueDetachedEntry(t *testing.T) {
	entries, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob runtime sources: %v", err)
	}
	sites := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry)
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, ".ArchiveAsset(") {
				t.Errorf("result cleanup bypasses reference guard: %s", entry)
			}
			if strings.Contains(line, ".ArchiveUnreferencedAsset(") {
				sites++
				if entry != "renderer.go" || !strings.Contains(string(raw), "func compensateIngestedResultAssets(") {
					t.Errorf("ArchiveUnreferencedAsset call outside the unique entry: %s", entry)
				}
			}
		}
	}
	if sites != 1 {
		t.Fatalf("ArchiveUnreferencedAsset call sites = %d, want exactly 1 (the unique detached entry)", sites)
	}
}

// TestNewResultIngestPathMustCompensate proves every production
// function that ingests result assets funnels through the unique compensation
// entry. A new ingest path must own cleanup of its partial result set.
func TestNewResultIngestPathMustCompensate(t *testing.T) {
	entries, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob runtime sources: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry)
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		name := ""
		body := &strings.Builder{}
		flush := func() {
			defer body.Reset()
			if name == "" || name == "persistWorkspaceTaskResultAsset" {
				return
			}
			text := body.String()
			if !strings.Contains(text, "persistWorkspaceTaskResultAsset(") {
				return
			}
			if strings.Contains(text, "compensateIngestedResultAssets") || strings.Contains(text, "CompensateRenderedOutputs") {
				return
			}
			t.Errorf("result ingest path %s in %s does not compensate: call the unique compensation entry (%s must archive its own ingested set on failure)",
				name, entry, name)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "func ") {
				flush()
				name = resultIngestFuncName(line)
				body.Reset()
				body.WriteString(line + "\n")
				continue
			}
			body.WriteString(line + "\n")
		}
		flush()
	}
}

// resultIngestFuncName extracts the function name from a top-level
// "func ..." declaration line (methods included).
func resultIngestFuncName(line string) string {
	rest := strings.TrimPrefix(line, "func ")
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "(") {
		if end := strings.Index(rest, ")"); end >= 0 {
			rest = strings.TrimSpace(rest[end+1:])
		}
	}
	if end := strings.Index(rest, "("); end >= 0 {
		return rest[:end]
	}
	return rest
}
