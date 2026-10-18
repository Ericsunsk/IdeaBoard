package boards

// Boundary pins for the unified Board access policy (issue #934,
// criterion 3+6).
//
//   - The Boards domain never depends on HTTP handlers or the runtime:
//     the policy is pure plus storage adapters, and handlers consume it
//     through the narrow RoleReader port.
//   - The role decision lives exactly once (ResolveRole in policy.go):
//     both store adapters delegate to it instead of reimplementing the
//     owner/explicit/default order.
//   - Media / Tasking / Events never copy the role mapping and never reach
//     into Boards storage. Media (#936) consumes the narrow ports
//     (RoleReader-shaped role fact + reference-listing fact) plus the pure
//     read helpers; Tasking / Events hold zero boards imports until
//     #937/#940 wire their ports.

import (
	"os"
	"strings"
	"testing"

	"ideaboard/api/internal/archtest"
)

func TestBoardsDomainHasNoHandlerOrRuntimeImports(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{
		"ideaboard/api/internal/runtime",
		"github.com/gin-gonic/gin",
		"net/http",
	})
}

func TestRoleAdaptersDelegateToUnifiedPolicy(t *testing.T) {
	adapters := map[string]string{
		"v1_workspace.go":       "boardRoleLocked",
		"postgres_workspace.go": "boardRoleTx",
	}
	for file, adapter := range adapters {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(raw)
		if !strings.Contains(text, adapter) {
			t.Fatalf("%s: adapter %s missing", file, adapter)
		}
		if !strings.Contains(text, "ResolveRole(") {
			t.Errorf("%s: %s must delegate to ResolveRole (single policy in policy.go)", file, adapter)
		}
		if strings.Contains(text, "RoleOwner, true\n\t}\n\tif member") ||
			strings.Contains(text, "case \"owner\":\n\t\treturn RoleOwner") {
			t.Errorf("%s: reimplements the role mapping instead of delegating to ResolveRole", file)
		}
	}
}

func TestBoardRoleRulesStayUncopiedOutsideBoards(t *testing.T) {
	for _, dir := range []string{"../tasking", "../events"} {
		archtest.AssertNoForbiddenImports(t, dir, []string{"ideaboard/api/internal/boards"})
		archtest.AssertNoForbiddenText(t, dir, []string{
			"DefaultBoardRoleForWorkspaceRole",
			"boards.ResolveRole",
			"boards.CheckCapability",
			"boards.AccessRole",
			"boards.RoleReader",
		})
	}
	// #936: Media consumes Board access facts through the narrow ports
	// (the RoleReader-shaped role fact plus the reference-listing fact)
	// and the pure read helpers (CheckCapability/CapRead/membership).
	// The role mapping itself is never recomputed outside Boards:
	// ResolveRole, explicit role literals, write/manage capabilities, and
	// Boards storage internals stay banned so the policy keeps one home.
	archtest.AssertNoForbiddenText(t, "../media", []string{
		"DefaultBoardRoleForWorkspaceRole",
		"boards.ResolveRole",
		"boards.RoleOwner",
		"boards.RoleEditor",
		"boards.RoleViewer",
		"boards.CapWrite",
		"boards.CapManage",
		"boards.WorkspaceBoardStore",
		"boards.PostgresWorkspaceBoardStore",
		"boards.InMemoryWorkspaceBoardStore",
		"boardRoleLocked",
		"boardRoleTx",
	})
}
