package domain

import "fmt"

// MaxCreditAmount keeps command arithmetic comfortably inside signed BIGINT.
const MaxCreditAmount int64 = 9_000_000_000_000_000

// CreditAmount is the smallest indivisible unit shown to users as Idea.
// Credits never use floating-point arithmetic.
type CreditAmount int64

func NewCreditAmount(value int64) (CreditAmount, error) {
	if value <= 0 || value > MaxCreditAmount {
		return 0, fmt.Errorf("credit amount must be between 1 and %d", MaxCreditAmount)
	}
	return CreditAmount(value), nil
}

func (a CreditAmount) Int64() int64 {
	return int64(a)
}
