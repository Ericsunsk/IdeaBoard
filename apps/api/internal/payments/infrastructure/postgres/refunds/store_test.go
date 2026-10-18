package refunds_test

import (
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	"ideaboard/api/internal/payments/infrastructure/postgres/refunds"
)

var (
	_ paymentrefund.Store = (*refunds.Store)(nil)
)
