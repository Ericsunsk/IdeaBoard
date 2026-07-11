package billing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBillingContextDoesNotDependOnProviderRouting(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read billing package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		content, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(content), "internal/accountpool") {
			t.Fatalf("%s imports Provider Account routing; billing prices must depend only on SKU", entry.Name())
		}
	}
}
