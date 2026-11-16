// ci-api-test-plan is the canonical Go package-impact and source-contract planner.
// Run its tests with: go test scripts/ci-api-test-plan.go scripts/ci-api-test-plan_test.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type goPackage struct {
	Dir, ImportPath                              string
	GoFiles, CgoFiles, TestGoFiles, XTestGoFiles []string
	Imports, TestImports, XTestImports           []string
}
type testPlan struct {
	Mode             string   `json:"mode"`
	Reason           string   `json:"reason"`
	Packages         []string `json:"packages"`
	ContractPackages []string `json:"contract_packages"`
	ContractRun      string   `json:"contract_run"`
}

func command(dir, name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %v: %w", name, args, err)
	}
	return out, nil
}
func listPackages(root string) ([]goPackage, error) {
	out, err := command(filepath.Join(root, "apps/api"), "go", "list", "-json", "./...")
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var result []goPackage
	for {
		var p goPackage
		err = dec.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("empty Go package inventory")
	}
	return result, nil
}

// These source contracts inspect sibling contexts or non-Go frontend inputs.
// Local architecture/boundary files are discovered below; these off-convention
// owners remain explicit so DB-heavy packages only run their pure policy tests.
var contractTests = map[string][]string{
	"internal/boards/policy_test.go":                                  {"TestBoardsDomainHasNoHandlerOrRuntimeImports", "TestRoleAdaptersDelegateToUnifiedPolicy", "TestBoardRoleRulesStayUncopiedOutsideBoards"},
	"internal/platform/riverpolicy/policy_ownership_test.go":          nil,
	"internal/library/domain/validation_test.go":                      {"TestLibrarySourceFixturesContract"},
	"internal/boards/workspace_test.go":                               {"TestPreviewCatalogQueryNeverReadsDocuments"},
	"internal/platform/contract/contract_test.go":                     {"TestErrorCodeRegistryConsistency"},
	"internal/tasking/transaction_lock_ordering_test.go":              nil,
	"internal/tasking/application/runtime/compensation_guard_test.go": nil,
	"internal/runtime/interfaces/http/contract_inventory_test.go":     nil,
	"internal/runtime/interfaces/http/control/policy_parity_test.go":  nil,
	"internal/runtime/interfaces/http/control/guard_test.go":          {"TestControlNeverTouchesEventLayer"},
	"internal/workflow/features_test.go":                              {"TestAIImageFeatureSlugsMatchFrontendOpenAPIAndAdminContracts"},
	"internal/workflow/generation_contract_test.go":                   {"TestGenerationContractMatchesFrontendAndOpenAPI"},
	"internal/events/webhook/event_parity_test.go":                    nil,
}

func sourceContracts(root string, inventory []goPackage) ([]string, string, error) {
	owners := map[string]bool{}
	names := map[string]bool{}
	seen := map[string]bool{}
	for _, p := range inventory {
		for _, f := range append(append([]string{}, p.TestGoFiles...), p.XTestGoFiles...) {
			rel, err := filepath.Rel(filepath.Join(root, "apps/api"), filepath.Join(p.Dir, f))
			if err != nil {
				return nil, "", err
			}
			rel = filepath.ToSlash(rel)
			wanted, explicit := contractTests[rel]
			if !explicit && f != "architecture_test.go" && !strings.HasSuffix(f, "boundary_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(p.Dir, f), nil, 0)
			if err != nil {
				return nil, "", err
			}
			found := map[string]bool{}
			for _, d := range parsed.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
					continue
				}
				if len(wanted) > 0 {
					match := false
					for _, n := range wanted {
						if n == fn.Name.Name {
							match = true
						}
					}
					if !match {
						continue
					}
				}
				found[fn.Name.Name] = true
				names[fn.Name.Name] = true
				owners[p.ImportPath] = true
			}
			for _, n := range wanted {
				if !found[n] {
					return nil, "", fmt.Errorf("source contract %s missing in %s", n, rel)
				}
			}
			if explicit {
				seen[rel] = true
			}
		}
	}
	// Fixture modules deliberately have no IdeaBoard policy owners.
	if _, err := os.Stat(filepath.Join(root, "apps/api/internal/archtest")); err == nil {
		for p := range contractTests {
			if !seen[p] {
				return nil, "", fmt.Errorf("source contract owner missing: %s", p)
			}
		}
	}
	var pkgs, tests []string
	for p := range owners {
		pkgs = append(pkgs, p)
	}
	for n := range names {
		tests = append(tests, regexp.QuoteMeta(n))
	}
	sort.Strings(pkgs)
	sort.Strings(tests)
	run := ""
	if len(tests) > 0 {
		run = "^(" + strings.Join(tests, "|") + ")$"
	}
	return pkgs, run, nil
}
func allPlan(inventory []goPackage, reason string) testPlan {
	p := testPlan{Mode: "full", Reason: reason, Packages: []string{}, ContractPackages: []string{}}
	for _, g := range inventory {
		p.Packages = append(p.Packages, g.ImportPath)
	}
	sort.Strings(p.Packages)
	return p
}
func makePlan(root, base, head string, force bool, inventory []goPackage) (testPlan, error) {
	full := func(reason string) (testPlan, error) { return allPlan(inventory, reason), nil }
	if force {
		return full("explicit full verification")
	}
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	if !sha.MatchString(base) || !sha.MatchString(head) {
		return full("trusted base/head unavailable")
	}
	actual, err := command(root, "git", "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(actual)) != head {
		return full("checkout/head mismatch")
	}
	if _, err := command(root, "git", "merge-base", "--is-ancestor", base, head); err != nil {
		return full("base is not a reachable ancestor")
	}
	dirty, err := command(root, "git", "status", "--porcelain", "--untracked-files=all")
	if err != nil || len(dirty) > 0 {
		return full("worktree is not clean")
	}
	changed, err := command(root, "git", "diff", "--name-status", "--no-renames", "-z", base, head)
	if err != nil {
		return full("change inventory unavailable")
	}
	fields := strings.Split(string(changed), "\x00")
	selected := map[string]bool{}
	fileOwner := map[string]string{}
	reverse := map[string][]string{}
	for _, p := range inventory {
		for _, f := range append(append(append(append([]string{}, p.GoFiles...), p.CgoFiles...), p.TestGoFiles...), p.XTestGoFiles...) {
			rel, _ := filepath.Rel(root, filepath.Join(p.Dir, f))
			fileOwner[filepath.ToSlash(rel)] = p.ImportPath
		}
		for _, i := range append(append(append([]string{}, p.Imports...), p.TestImports...), p.XTestImports...) {
			reverse[i] = append(reverse[i], p.ImportPath)
		}
	}
	for i := 0; i < len(fields) && fields[i] != ""; i += 2 {
		if i+1 >= len(fields) {
			return full("malformed change inventory")
		}
		status, path := fields[i], fields[i+1]
		if status != "M" && status != "A" {
			return full("deleted or renamed input")
		}
		if strings.HasPrefix(path, "apps/api/") {
			if !strings.HasSuffix(path, ".go") || strings.HasPrefix(path, "apps/api/internal/testdb/") || strings.HasPrefix(path, "apps/api/internal/testsupport/") || strings.HasPrefix(path, "apps/api/internal/db/") {
				return full("shared API input: " + path)
			}
			pkg, ok := fileOwner[path]
			if !ok {
				return full("unmapped or build-constrained Go input: " + path)
			}
			selected[pkg] = true
		} else if strings.HasPrefix(path, "apps/web/") { // Cross-language parity is in the source-contract set.
		} else if strings.HasSuffix(path, ".md") || strings.HasPrefix(path, "docs/") {
		} else {
			return full("shared verification input: " + path)
		}
	}
	var queue []string
	for p := range selected {
		queue = append(queue, p)
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, consumer := range reverse[p] {
			if !selected[consumer] {
				selected[consumer] = true
				queue = append(queue, consumer)
			}
		}
	}
	contracts, run, err := sourceContracts(root, inventory)
	if err != nil {
		return testPlan{}, err
	}
	p := testPlan{Mode: "affected", Reason: "changed Go packages and transitive production/test consumers; source contracts retained", Packages: []string{}, ContractPackages: []string{}, ContractRun: run}
	for pkg := range selected {
		p.Packages = append(p.Packages, pkg)
	}
	sort.Strings(p.Packages)
	for _, pkg := range contracts {
		if !selected[pkg] {
			p.ContractPackages = append(p.ContractPackages, pkg)
		}
	}
	return p, nil
}
func readPlan(root, path string, inventory []goPackage) (testPlan, error) {
	var p testPlan
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	if p.Mode != "full" && p.Mode != "affected" {
		return p, fmt.Errorf("invalid plan mode")
	}
	valid := map[string]bool{}
	seen := map[string]bool{}
	for _, g := range inventory {
		valid[g.ImportPath] = true
	}
	for _, list := range [][]string{p.Packages, p.ContractPackages} {
		for _, name := range list {
			if !valid[name] || seen[name] {
				return p, fmt.Errorf("invalid or duplicate planned package: %s", name)
			}
			seen[name] = true
		}
	}
	if p.Mode == "full" && (len(p.Packages) != len(inventory) || len(p.ContractPackages) > 0) {
		return p, fmt.Errorf("incomplete full plan")
	}
	if len(p.ContractPackages) > 0 {
		if _, err := regexp.Compile(p.ContractRun); err != nil || p.ContractRun == "" {
			return p, fmt.Errorf("invalid source-contract filter")
		}
	}
	if p.Mode == "affected" {
		owners, run, err := sourceContracts(root, inventory)
		if err != nil {
			return p, err
		}
		for _, owner := range owners {
			if !seen[owner] {
				return p, fmt.Errorf("source-contract owner omitted: %s", owner)
			}
		}
		if len(p.ContractPackages) > 0 && p.ContractRun != run {
			return p, fmt.Errorf("source-contract filter differs from canonical plan")
		}
	}
	return p, nil
}
func main() {
	root := flag.String("root", ".", "repository root")
	base := flag.String("base", "", "trusted ancestor")
	head := flag.String("head", "", "checked-out commit")
	force := flag.Bool("full", false, "full verification")
	output := flag.String("output", "", "plan JSON path")
	read := flag.String("read", "", "validate and print planned rows")
	shard := flag.Int("shard", 0, "shard index")
	count := flag.Int("shards", 1, "shard count")
	flag.Parse()
	abs, err := filepath.Abs(*root)
	if err != nil {
		fatal(err)
	}
	inventory, err := listPackages(abs)
	if err != nil {
		fatal(err)
	}
	if *read != "" {
		if *count < 1 || *shard < 0 || *shard >= *count {
			fatal(fmt.Errorf("invalid shard"))
		}
		p, err := readPlan(abs, *read, inventory)
		if err != nil {
			fatal(err)
		}
		for i, pkg := range p.Packages {
			if i%*count == *shard {
				fmt.Printf("test\t%s\t\n", pkg)
			}
		}
		for i, pkg := range p.ContractPackages {
			if i%*count == *shard {
				fmt.Printf("contract\t%s\t%s\n", pkg, p.ContractRun)
			}
		}
		return
	}
	p, err := makePlan(abs, *base, *head, *force, inventory)
	if err != nil {
		fatal(err)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		fatal(err)
	}
	if *output == "" {
		fmt.Println(string(b))
	} else if err = os.WriteFile(*output, append(b, '\n'), 0600); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "ci-api-test-plan:", err); os.Exit(1) }
