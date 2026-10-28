package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixtureCommand(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := command(root, "git", args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
func writeFixture(t *testing.T, root, path, body string) {
	t.Helper()
	p := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func commitFixture(t *testing.T, root string) string {
	t.Helper()
	fixtureCommand(t, root, "add", ".")
	fixtureCommand(t, root, "-c", "user.name=CI Fixture", "-c", "user.email=ci@example.invalid", "commit", "-qm", "fixture")
	return fixtureCommand(t, root, "rev-parse", "HEAD")
}
func fixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	fixtureCommand(t, root, "init", "-q")
	writeFixture(t, root, "apps/api/go.mod", "module example.test/api\n\ngo 1.26.10\n")
	files := map[string]string{
		"a/a.go":                      "package a\nconst Value = 1\n",
		"b/b.go":                      "package b\nimport \"example.test/api/a\"\nvar Value = a.Value\n",
		"c/c.go":                      "package c\nimport \"example.test/api/b\"\nvar Value = b.Value\n",
		"sibling/sibling.go":          "package sibling\nconst Value = 1\n",
		"consumer/consumer.go":        "package consumer\n",
		"consumer/consumer_test.go":   "package consumer\nimport (\"testing\"; \"example.test/api/a\")\nfunc TestValue(t *testing.T){_ = a.Value}\n",
		"external/external.go":        "package external\n",
		"external/external_test.go":   "package external_test\nimport (\"testing\"; \"example.test/api/a\")\nfunc TestValue(t *testing.T){_ = a.Value}\n",
		"policy/policy.go":            "package policy\n",
		"policy/architecture_test.go": "package policy\nimport \"testing\"\nfunc TestSourcePolicy(t *testing.T){}\n",
	}
	for path, body := range files {
		writeFixture(t, root, "apps/api/"+path, body)
	}
	return root, commitFixture(t, root)
}
func TestActualImportAndTestImportReverseClosure(t *testing.T) {
	root, base := fixtureRepo(t)
	writeFixture(t, root, "apps/api/a/a.go", "package a\nconst Value = 2\n")
	head := commitFixture(t, root)
	inventory, err := listPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := makePlan(root, base, head, false, inventory)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"example.test/api/a", "example.test/api/b", "example.test/api/c", "example.test/api/consumer", "example.test/api/external"}
	if plan.Mode != "affected" || !reflect.DeepEqual(plan.Packages, want) {
		t.Fatalf("plan=%+v want=%v", plan, want)
	}
	if !reflect.DeepEqual(plan.ContractPackages, []string{"example.test/api/policy"}) || plan.ContractRun != "^(TestSourcePolicy)$" {
		t.Fatalf("contracts=%+v", plan)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	b, _ := json.Marshal(plan)
	os.WriteFile(path, b, 0600)
	if _, err = readPlan(root, path, inventory); err != nil {
		t.Fatal(err)
	}
	plan.ContractPackages = nil
	b, _ = json.Marshal(plan)
	os.WriteFile(path, b, 0600)
	if _, err = readPlan(root, path, inventory); err == nil {
		t.Fatal("omitted source contract accepted")
	}
}
func TestUnsafeChangesUseFullPlan(t *testing.T) {
	for _, path := range []string{"apps/api/a/testdata/input.json", "apps/api/internal/testdb/helper.go", "apps/api/internal/db/generated.go", "apps/api/go.mod", "scripts/ci-parity.sh", "apps/api/a/hidden_windows.go"} {
		t.Run(path, func(t *testing.T) {
			root, base := fixtureRepo(t)
			body := "fixture\n"
			if strings.HasSuffix(path, ".go") {
				body = "package a\n"
			}
			if path == "apps/api/go.mod" {
				body = "module example.test/api\n\ngo 1.26.10\n// dependency policy changed\n"
			}
			writeFixture(t, root, path, body)
			head := commitFixture(t, root)
			inventory, err := listPackages(root)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := makePlan(root, base, head, false, inventory)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Mode != "full" || len(plan.Packages) != len(inventory) {
				t.Fatalf("plan=%+v", plan)
			}
		})
	}
}
func TestMissingBaseDeletionDirtyTreeAndExplicitFull(t *testing.T) {
	root, base := fixtureRepo(t)
	inventory, err := listPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []struct {
		base, head string
		force      bool
	}{{"", base, false}, {strings.Repeat("a", 40), base, false}, {base, base, true}, {base, strings.Repeat("b", 40), false}} {
		p, err := makePlan(root, args.base, args.head, args.force, inventory)
		if err != nil || p.Mode != "full" {
			t.Fatalf("%+v err=%v", p, err)
		}
	}
	writeFixture(t, root, "untracked.txt", "dirty")
	p, err := makePlan(root, base, base, false, inventory)
	if err != nil || p.Mode != "full" {
		t.Fatalf("dirty=%+v err=%v", p, err)
	}
	os.Remove(filepath.Join(root, "untracked.txt"))
	os.Remove(filepath.Join(root, "apps/api/sibling/sibling.go"))
	head := commitFixture(t, root)
	inventory, err = listPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	p, err = makePlan(root, base, head, false, inventory)
	if err != nil || p.Mode != "full" {
		t.Fatalf("delete=%+v err=%v", p, err)
	}
}
func TestPlanRejectsMalformedPackagesAndFullCoverage(t *testing.T) {
	inventory := []goPackage{{ImportPath: "a"}, {ImportPath: "b"}}
	path := filepath.Join(t.TempDir(), "plan.json")
	for _, p := range []testPlan{{Mode: "full", Packages: []string{"a"}}, {Mode: "full", Packages: []string{"a", "a"}}, {Mode: "affected", Packages: []string{"-args"}}, {Mode: "bad"}} {
		b, _ := json.Marshal(p)
		os.WriteFile(path, b, 0600)
		if _, err := readPlan(t.TempDir(), path, inventory); err == nil {
			t.Fatalf("invalid plan accepted:%+v", p)
		}
	}
}
func TestActualFailedPackageInventoryFails(t *testing.T) {
	root, _ := fixtureRepo(t)
	writeFixture(t, root, "apps/api/a/a.go", "not valid go")
	if _, err := listPackages(root); err == nil {
		t.Fatal("failed Go metadata accepted")
	}
}
func TestPlannerCLIShardsCoverFullAndAffectedPlans(t *testing.T) {
	root, base := fixtureRepo(t)
	writeFixture(t, root, "apps/api/sibling/sibling.go", "package sibling\nconst Value = 2\n")
	head := commitFixture(t, root)
	inventory, err := listPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := makePlan(root, base, head, false, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Packages, []string{"example.test/api/sibling"}) {
		t.Fatalf("unrelated packages selected:%+v", plan)
	}
	binary := filepath.Join(t.TempDir(), "planner")
	cmd := exec.Command("go", "build", "-o", binary, "ci-api-test-plan.go")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build:%s %v", out, err)
	}
	for _, full := range []bool{false, true} {
		if full {
			plan = allPlan(inventory, "fixture")
		}
		path := filepath.Join(t.TempDir(), "plan.json")
		b, _ := json.Marshal(plan)
		os.WriteFile(path, b, 0600)
		var rows []string
		for _, shard := range []string{"0", "1"} {
			out, err := exec.Command(binary, "-root", root, "-read", path, "-shard", shard, "-shards", "2").Output()
			if err != nil {
				t.Fatal(err)
			}
			rows = append(rows, strings.Fields(string(out))...)
		}
		for _, p := range append(append([]string{}, plan.Packages...), plan.ContractPackages...) {
			n := 0
			for _, v := range rows {
				if v == p {
					n++
				}
			}
			if n != 1 {
				t.Fatalf("%s appeared%d in%v", p, n, rows)
			}
		}
	}
}
