package runtime_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeRootHasNoImplementationFiles(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read current dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			t.Errorf("found implementation file %s at runtime root", name)
		}
	}
}

func TestNoBusinessPackageImportsRuntime(t *testing.T) {
	internalRoot := ".."
	err := filepath.WalkDir(internalRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipArchitectureDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceCodeFile(d.Name()) {
			return nil
		}
		checkFileRuntimeImports(t, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}

func shouldSkipArchitectureDir(name string) bool {
	return name == "runtime" || name == ".git" || name == "vendor"
}

func isSourceCodeFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func checkFileRuntimeImports(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if parseErr != nil {
		t.Fatalf("parse %s: %v", path, parseErr)
	}
	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if strings.HasPrefix(importPath, "ideaboard/api/internal/runtime") {
			t.Errorf("business file %s imports forbidden runtime package %q", path, importPath)
		}
	}
}

func TestRuntimeLayerAndAdapterIsolation(t *testing.T) {
	// Domain must not depend on higher runtime layers.
	checkPackageForbiddenImports(t, "domain", []string{
		"ideaboard/api/internal/runtime/ports",
		"ideaboard/api/internal/runtime/application",
		"ideaboard/api/internal/runtime/interfaces",
		"ideaboard/api/internal/runtime/composition",
		"ideaboard/api/internal/runtime/adapters",
	})

	// Ports must not depend on application, interfaces, or composition.
	checkPackageForbiddenImports(t, "ports", []string{
		"ideaboard/api/internal/runtime/application",
		"ideaboard/api/internal/runtime/interfaces",
		"ideaboard/api/internal/runtime/composition",
		"ideaboard/api/internal/runtime/adapters",
	})

	// Application must not depend on interfaces, composition, or adapters.
	checkPackageForbiddenImports(t, "application", []string{
		"ideaboard/api/internal/runtime/interfaces",
		"ideaboard/api/internal/runtime/composition",
		"ideaboard/api/internal/runtime/adapters",
	})

	// Interfaces must remain isolated from each other and composition root.
	checkPackageForbiddenImports(t, "interfaces/http", []string{
		"ideaboard/api/internal/runtime/interfaces/worker",
		"ideaboard/api/internal/runtime/interfaces/events",
		"ideaboard/api/internal/runtime/composition",
	})

	checkPackageForbiddenImports(t, "interfaces/worker", []string{
		"ideaboard/api/internal/runtime/interfaces/http",
		"ideaboard/api/internal/runtime/interfaces/events",
		"ideaboard/api/internal/runtime/composition",
	})

	checkPackageForbiddenImports(t, "interfaces/events", []string{
		"ideaboard/api/internal/runtime/interfaces/http",
		"ideaboard/api/internal/runtime/interfaces/worker",
		"ideaboard/api/internal/runtime/composition",
	})

	// Adapters must not depend on interfaces or composition.
	checkPackageForbiddenImports(t, "adapters/billingregistry", []string{
		"ideaboard/api/internal/runtime/interfaces",
		"ideaboard/api/internal/runtime/composition",
	})

	checkPackageForbiddenImports(t, "adapters/webdist", []string{
		"ideaboard/api/internal/runtime/interfaces",
		"ideaboard/api/internal/runtime/composition",
	})
}

func TestCompositionSubpackageIsolation(t *testing.T) {
	// P1 composition split: composition/ is the sole assembly root.
	// control/ and wiring/ are leaf subpackages (package control, package
	// wiring). The dependency arrow is root -> child only: leaves must
	// never import the root, and control must never import wiring.
	// wiring -> control is the single allowed sibling edge (HTTP wiring
	// assembles the guard sinks); the reverse would be a cycle.
	checkPackageForbiddenImports(t, "composition/control", []string{
		"ideaboard/api/internal/runtime/composition",
	})
	checkCompositionWiringImports(t)
}

func checkCompositionWiringImports(t *testing.T) {
	t.Helper()
	const root = "ideaboard/api/internal/runtime/composition"
	err := filepath.WalkDir("composition/wiring", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isSourceCodeFile(d.Name()) {
			return nil
		}
		fset := token.NewFileSet()
		node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == root {
				t.Errorf("package composition/wiring file %s imports forbidden composition root %q (root -> child only)", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk composition/wiring: %v", err)
	}
}

func checkPackageForbiddenImports(t *testing.T, pkgDir string, forbidden []string) {
	t.Helper()
	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isSourceCodeFile(d.Name()) {
			return nil
		}
		checkFileForbiddenImports(t, pkgDir, path, forbidden)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", pkgDir, err)
	}
}

func checkFileForbiddenImports(t *testing.T, pkgDir, path string, forbidden []string) {
	t.Helper()
	fset := token.NewFileSet()
	node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if parseErr != nil {
		t.Fatalf("parse %s: %v", path, parseErr)
	}
	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		for _, f := range forbidden {
			if strings.HasPrefix(importPath, f) {
				t.Errorf("package %s file %s imports forbidden package %q", pkgDir, path, importPath)
			}
		}
	}
}
