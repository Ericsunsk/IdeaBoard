package workspacetasks

import (
	"errors"
	"testing"
)

func TestFundsGatePreflight(t *testing.T) {
	// Restricted denies first even when the balance would also be short.
	if err := CheckFundsGate(0, 100, true); !errors.Is(err, ErrWalletRestricted) {
		t.Fatalf("CheckFundsGate(restricted) = %v, want ErrWalletRestricted", err)
	}
	if err := CheckFundsGate(10, 100, false); !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("CheckFundsGate(short) = %v, want ErrInsufficientCredits", err)
	}
	if err := CheckFundsGate(100, 100, false); err != nil {
		t.Fatalf("CheckFundsGate(funded) = %v, want nil", err)
	}
}
