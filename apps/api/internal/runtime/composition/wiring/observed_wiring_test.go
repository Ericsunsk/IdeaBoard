package wiring

import (
	"errors"
	"testing"

	"ideaboard/api/internal/billing"
)

// Base pairing test for observed_wiring.go.
// A nil catalog must degrade to the billing unavailable sentinel, never
// to an empty list that looks like "no SKUs on sale".
func TestBillingSKUCatalogAdapterNilCatalogIsUnavailable(t *testing.T) {
	adapter := newBillingSKUCatalogAdapter(nil)
	if _, err := adapter.ListSKUCatalog(t.Context()); !errors.Is(err, billing.ErrServiceUnavailable) {
		t.Fatalf("ListSKUCatalog with nil catalog error = %v, want ErrServiceUnavailable", err)
	}
}
