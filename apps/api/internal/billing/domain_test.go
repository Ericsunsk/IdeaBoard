package billing

import (
	"errors"
	"math"
	"testing"
)

func TestCanonicalizeSKUDefinesGlobalNamespaceContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    SKU
		wantErr bool
	}{
		{name: "canonicalizes ASCII case and whitespace", input: "  IdeaBoard:Image.Generate  ", want: "ideaboard:image.generate"},
		{name: "allows product namespace", input: "canvas-pro:export_hd", want: "canvas-pro:export_hd"},
		{name: "requires namespace separator", input: "image.generate", wantErr: true},
		{name: "rejects empty local key", input: "ideaboard:", wantErr: true},
		{name: "rejects unicode identity", input: "点子:image", wantErr: true},
		{name: "rejects overlong identity", input: "namespace:" + string(make([]byte, 120)), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalizeSKU(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidSKU) {
					t.Fatalf("CanonicalizeSKU(%q) error = %v, want ErrInvalidSKU", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CanonicalizeSKU(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("CanonicalizeSKU(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPriceSnapshotTotalFailsClosed(t *testing.T) {
	t.Parallel()

	free := PriceSnapshot{UnitCredits: 0}
	if total, err := free.Total(7); err != nil || total != 0 {
		t.Fatalf("free Total(7) = %d, %v, want 0, nil", total, err)
	}

	priced := PriceSnapshot{UnitCredits: 25}
	if total, err := priced.Total(4); err != nil || total != 100 {
		t.Fatalf("priced Total(4) = %d, %v, want 100, nil", total, err)
	}

	for _, quantity := range []int64{0, -1} {
		if _, err := priced.Total(quantity); !errors.Is(err, ErrInvalidQuantity) {
			t.Fatalf("Total(%d) error = %v, want ErrInvalidQuantity", quantity, err)
		}
	}

	overflow := PriceSnapshot{UnitCredits: MaxQuoteCredits}
	if _, err := overflow.Total(math.MaxInt64); !errors.Is(err, ErrQuoteTotalExceeded) {
		t.Fatalf("overflow Total() error = %v, want ErrQuoteTotalExceeded", err)
	}
}
