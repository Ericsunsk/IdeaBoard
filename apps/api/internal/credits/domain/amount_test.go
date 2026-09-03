package domain

import (
	"math"
	"testing"
)

func TestNewCreditAmountAcceptsPositiveIntegers(t *testing.T) {
	amount, err := NewCreditAmount(125)
	if err != nil {
		t.Fatalf("NewCreditAmount() error = %v", err)
	}
	if amount.Int64() != 125 {
		t.Fatalf("amount = %d, want 125", amount.Int64())
	}
}

func TestNewCreditAmountRejectsNonPositiveValues(t *testing.T) {
	for _, value := range []int64{-1, 0} {
		if _, err := NewCreditAmount(value); err == nil {
			t.Fatalf("NewCreditAmount(%d) succeeded", value)
		}
	}
}

func TestNewCreditAmountRejectsValuesThatCannotBeSafelyAdded(t *testing.T) {
	if _, err := NewCreditAmount(math.MaxInt64); err == nil {
		t.Fatal("NewCreditAmount(math.MaxInt64) succeeded")
	}
}
