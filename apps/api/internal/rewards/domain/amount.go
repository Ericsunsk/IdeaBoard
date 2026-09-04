package domain

import "fmt"

const MaxCreditAmount int64 = 9_000_000_000_000_000

func ValidateCreditAmount(value int64) error {
	if value <= 0 || value > MaxCreditAmount {
		return fmt.Errorf("credit amount must be between 1 and %d", MaxCreditAmount)
	}
	return nil
}
