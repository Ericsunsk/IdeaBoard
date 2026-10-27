package workspacetasks

// Single-source guard for the v1 task status vocabulary.
//
// Three places must stay identical; a new status touches all three or the
// suites below go red:
//   - this package (Status constants + IsTerminalStatus, the terminal predicate)
//   - apps/web/src/api/workspaceTaskContract.ts (WORKSPACE_TASK_STATUSES +
//     isWorkspaceTaskTerminal, the contract single source)
//   - apps/web/src/features/admin/shared/core/taskMetricTokens.ts
//     (TaskStatus map + taskMetricDefinitions keys, project-token reuse only:
//     no new token system is introduced here)
//
// This file is the backend machine gate: it reads both frontend modules
// from disk and compares them with the pinned vocabulary. The frontend
// gate lives in `taskMetricTokens.unit.spec.ts` (direct import comparison)
// with local feedback in `workspaceTasks.contract.spec.ts`; both sides pin
// the same literals so either CI shard catches drift.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// pinnedWorkspaceTaskStatuses is the closed v1 status vocabulary in canonical
// lifecycle order. cancelled_with_results converges to cancelled at the v1
// surface and never enters this list.
var pinnedWorkspaceTaskStatuses = []string{"pending", "running", "completed", "failed", "cancelled"}

// pinnedTerminalStatuses must match isWorkspaceTaskTerminal in
// workspaceTaskContract.ts exactly.
var pinnedTerminalStatuses = []string{"completed", "failed", "cancelled"}

func TestIsTerminalStatusMatchesPinnedVocabulary(t *testing.T) {
	for _, status := range pinnedWorkspaceTaskStatuses {
		want := slices.Contains(pinnedTerminalStatuses, status)
		if got := IsTerminalStatus(Status(status)); got != want {
			t.Errorf("IsTerminalStatus(%q) = %v, want %v", status, got, want)
		}
	}
	for _, status := range []string{"", "ready", "archived", "cancelled_with_results"} {
		if IsTerminalStatus(Status(status)) {
			t.Errorf("IsTerminalStatus(%q) = true, want false (closed vocabulary)", status)
		}
	}
}

func findTaskStatusParityRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(filename)
	for {
		if fileExists(filepath.Join(dir, "apps", "api", "internal", "tasking", "workspacetasks", "tasks.go")) &&
			fileExists(filepath.Join(dir, "apps", "web", "src", "api", "workspaceTaskContract.ts")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", filename)
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func extractTSConstStringArray(t *testing.T, source, name string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?s)export\s+const\s+` + regexp.QuoteMeta(name) + `\s*:\s*readonly[^\=]*=\s*\[(.*?)\]`).FindStringSubmatch(source)
	if len(m) < 2 {
		t.Fatalf("%s declaration not found", name)
	}
	ms := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(m[1], -1)
	out := make([]string, 0, len(ms))
	for _, x := range ms {
		out = append(out, x[1])
	}
	if len(out) == 0 {
		t.Fatalf("%s parsed to an empty set", name)
	}
	return out
}

func TestTaskStatusVocabularyMatchesFrontend(t *testing.T) {
	root := findTaskStatusParityRepoRoot(t)

	contractRaw, err := os.ReadFile(filepath.Join(root, "apps", "web", "src", "api", "workspaceTaskContract.ts"))
	if err != nil {
		t.Fatalf("read frontend task contract: %v", err)
	}
	contractStatuses := extractTSConstStringArray(t, string(contractRaw), "WORKSPACE_TASK_STATUSES")
	if !slices.Equal(contractStatuses, pinnedWorkspaceTaskStatuses) {
		t.Fatalf("WORKSPACE_TASK_STATUSES=%v, want %v (canonical lifecycle order)", contractStatuses, pinnedWorkspaceTaskStatuses)
	}
	// The terminal predicate must name exactly the pinned terminal set.
	fn := regexp.MustCompile(`(?s)export function isWorkspaceTaskTerminal.*?\{.*?\}`).FindString(string(contractRaw))
	if fn == "" {
		t.Fatal("isWorkspaceTaskTerminal not found in workspaceTaskContract.ts")
	}
	for _, status := range pinnedTerminalStatuses {
		if !strings.Contains(fn, "'"+status+"'") {
			t.Fatalf("isWorkspaceTaskTerminal must cover terminal status %q", status)
		}
	}
	for _, status := range []string{"pending", "running"} {
		if strings.Contains(fn, "=== '"+status+"'") || strings.Contains(fn, `=== "`+status+`"`) {
			t.Fatalf("isWorkspaceTaskTerminal must not treat %q as terminal", status)
		}
	}

	tokensRaw, err := os.ReadFile(filepath.Join(root, "apps", "web", "src", "features", "admin", "shared", "core", "taskMetricTokens.ts"))
	if err != nil {
		t.Fatalf("read admin task metric tokens: %v", err)
	}
	// TaskStatus map values must equal the contract vocabulary.
	obj := regexp.MustCompile(`(?s)export\s+const\s+TaskStatus\s*=\s*\{(.*?)\}\s+as\s+const`).FindStringSubmatch(string(tokensRaw))
	if len(obj) < 2 {
		t.Fatal("TaskStatus map not found in taskMetricTokens.ts")
	}
	var tokenStatuses []string
	for _, x := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(obj[1], -1) {
		tokenStatuses = append(tokenStatuses, x[1])
	}
	slices.Sort(tokenStatuses)
	want := append([]string(nil), pinnedWorkspaceTaskStatuses...)
	slices.Sort(want)
	if !slices.Equal(tokenStatuses, want) {
		t.Fatalf("admin TaskStatus=%v, want %v", tokenStatuses, want)
	}
	// Metric definition keys must be exactly all + the contract vocabulary
	// in canonical order (single source: no divergent key set).
	var keys []string
	for _, x := range regexp.MustCompile(`key:\s*'([^']+)'`).FindAllStringSubmatch(string(tokensRaw), -1) {
		keys = append(keys, x[1])
	}
	if !slices.Equal(keys, append([]string{"all"}, pinnedWorkspaceTaskStatuses...)) {
		t.Fatalf("taskMetricDefinitions keys=%v, want [all %s]", keys, strings.Join(pinnedWorkspaceTaskStatuses, " "))
	}
}
