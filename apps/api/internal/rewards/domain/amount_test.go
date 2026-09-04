package domain

import "testing"

func TestValidateCreditAmount(t *testing.T) {
	for _, value := range []int64{0, -1, MaxCreditAmount + 1} {
		if err := ValidateCreditAmount(value); err == nil {
			t.Errorf("ValidateCreditAmount(%d) succeeded", value)
		}
	}
	for _, value := range []int64{1, MaxCreditAmount} {
		if err := ValidateCreditAmount(value); err != nil {
			t.Errorf("ValidateCreditAmount(%d) error = %v", value, err)
		}
	}
}
