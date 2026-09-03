package payments

import (
	"context"
	"fmt"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
)

func (s *CheckoutWorkflow) ListRecharges(ctx context.Context, userID int64, cursor string, limit int) (paymentcontracts.RechargeHistoryPage, error) {
	if s == nil || s.readModelService == nil {
		return paymentcontracts.RechargeHistoryPage{}, fmt.Errorf("payments read model service unavailable")
	}
	return s.readModelService.ListRecharges(ctx, userID, cursor, limit)
}
