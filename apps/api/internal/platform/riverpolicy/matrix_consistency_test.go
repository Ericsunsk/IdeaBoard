package riverpolicy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRiverMatricesReferenceDeclaredConstants guards the triple-handwritten
// contract between the riverpolicy.go constant blocks and the two matrices:
// every matrix row must source its Queue/MaxAttempts/Timeout from a declared
// constant (no literals), and every declared constant must be covered by its
// matrix. Adding a queue or budget means touching the const block plus its
// matrix row(s); touching only one side fails here.
//
// The expected sets are parsed from riverpolicy.go itself (consts are
// invisible to reflect, same idiom as TestErrorCodeRegistryConsistency), so
// this test needs no update when a legitimate queue/budget is added: it
// adapts to the declared set instead of repeating it as a fourth copy.
func TestRiverMatricesReferenceDeclaredConstants(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	dir := filepath.Dir(thisFile)

	declaredQueues, declaredRetries, declaredTimeouts := declaredPolicyConsts(t, filepath.Join(dir, "riverpolicy.go"))

	retryRows := matrixRowIdents(t, filepath.Join(dir, "retry_matrix.go"), "QueueRetryPolicy")
	timeoutRows := matrixRowIdents(t, filepath.Join(dir, "timeout_matrix.go"), "QueueTimeoutPolicy")

	if len(retryRows) == 0 {
		t.Fatal("no QueueRetryPolicy rows found in retry_matrix.go")
	}
	if len(timeoutRows) == 0 {
		t.Fatal("no QueueTimeoutPolicy rows found in timeout_matrix.go")
	}

	// Forward: every matrix row must reference a declared constant.
	retryQueueRefs := map[string]bool{}
	retryBudgetRefs := map[string]bool{}
	for i, row := range retryRows {
		queue, ok := row["Queue"]
		if !ok || queue == "" {
			t.Errorf("retry row %d does not source Queue from a RiverQueue* constant", i)
		} else if !declaredQueues[queue] {
			t.Errorf("retry row %d Queue references undeclared const %q", i, queue)
		} else {
			retryQueueRefs[queue] = true
		}
		budget, ok := row["MaxAttempts"]
		if !ok || budget == "" {
			t.Errorf("retry row %d does not source MaxAttempts from a RiverMaxAttempts* constant", i)
		} else if !declaredRetries[budget] {
			t.Errorf("retry row %d MaxAttempts references undeclared const %q", i, budget)
		} else {
			retryBudgetRefs[budget] = true
		}
	}
	timeoutQueueRefs := map[string]bool{}
	timeoutBudgetRefs := map[string]bool{}
	for i, row := range timeoutRows {
		queue, ok := row["Queue"]
		if !ok || queue == "" {
			t.Errorf("timeout row %d does not source Queue from a RiverQueue* constant", i)
		} else if !declaredQueues[queue] {
			t.Errorf("timeout row %d Queue references undeclared const %q", i, queue)
		} else {
			timeoutQueueRefs[queue] = true
		}
		budget, ok := row["Timeout"]
		if !ok || budget == "" {
			t.Errorf("timeout row %d does not source Timeout from a RiverJobTimeout* constant", i)
		} else if !declaredTimeouts[budget] {
			t.Errorf("timeout row %d Timeout references undeclared const %q", i, budget)
		} else {
			timeoutBudgetRefs[budget] = true
		}
	}

	// Reverse: every declared constant must be covered by its matrix.
	for name := range declaredQueues {
		if !retryQueueRefs[name] {
			t.Errorf("riverpolicy.%s is declared but has no retry matrix row", name)
		}
		if !timeoutQueueRefs[name] {
			t.Errorf("riverpolicy.%s is declared but has no timeout matrix row", name)
		}
	}
	for name := range declaredRetries {
		if !retryBudgetRefs[name] {
			t.Errorf("riverpolicy.%s is declared but has no retry matrix row", name)
		}
	}
	for name := range declaredTimeouts {
		if !timeoutBudgetRefs[name] {
			t.Errorf("riverpolicy.%s is declared but has no timeout matrix row", name)
		}
	}
}

func declaredPolicyConsts(t *testing.T, path string) (queues, retries, timeouts map[string]bool) {
	t.Helper()
	node := parseGoFile(t, path)
	queues = map[string]bool{}
	retries = map[string]bool{}
	timeouts = map[string]bool{}
	for _, decl := range node.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				switch {
				case strings.HasPrefix(name.Name, "RiverQueue"):
					queues[name.Name] = true
				case strings.HasPrefix(name.Name, "RiverMaxAttempts"):
					retries[name.Name] = true
				case strings.HasPrefix(name.Name, "RiverJobTimeout"):
					timeouts[name.Name] = true
				}
			}
		}
	}
	if len(queues) == 0 || len(retries) == 0 || len(timeouts) == 0 {
		t.Fatalf("parsed no policy consts from %s (queues=%d retries=%d timeouts=%d)", path, len(queues), len(retries), len(timeouts))
	}
	return queues, retries, timeouts
}

// matrixRowIdents returns one entry per matrix composite literal, mapping the
// field name (Queue/MaxAttempts/Timeout) to the constant identifier used. A
// field written as a literal instead of a River* constant yields an empty
// name, which the caller reports as a missing source.
func matrixRowIdents(t *testing.T, path, litType string) []map[string]string {
	t.Helper()
	node := parseGoFile(t, path)
	// Matrix rows are written as elided composite literals inside the
	// `[]QueueRetryPolicy{...}` / `[]QueueTimeoutPolicy{...}` slice, so the
	// inner literals carry no type name. Identify rows by their keys instead:
	// a retry row sets Queue+MaxAttempts, a timeout row Queue+Timeout.
	budgetKey := "MaxAttempts"
	if litType == "QueueTimeoutPolicy" {
		budgetKey = "Timeout"
	}
	var rows []map[string]string
	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		row := map[string]string{}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			row[key.Name] = identName(kv.Value)
		}
		if _, ok := row["Queue"]; !ok {
			return true
		}
		if _, ok := row[budgetKey]; !ok {
			return true
		}
		rows = append(rows, row)
		return true
	})
	return rows
}

func identName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	default:
		return ""
	}
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	node, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return node
}
