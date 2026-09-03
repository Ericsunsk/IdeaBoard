package payments

import (
	"context"
	"fmt"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
)

func (s *CheckoutWorkflow) ListAdminPaymentOrders(ctx context.Context, input paymentcontracts.AdminPaymentOrderListInput) (paymentcontracts.AdminPaymentOrderPage, error) {
	if s == nil || s.readModelService == nil {
		return paymentcontracts.AdminPaymentOrderPage{}, fmt.Errorf("payments admin read service unavailable")
	}
	return s.readModelService.ListAdminPaymentOrders(ctx, input)
}

func (s *CheckoutWorkflow) GetAdminPaymentOrder(ctx context.Context, orderNumber string) (paymentcontracts.AdminPaymentOrderDetail, error) {
	if s == nil || s.readModelService == nil {
		return paymentcontracts.AdminPaymentOrderDetail{}, fmt.Errorf("payments admin read service unavailable")
	}
	return s.readModelService.GetAdminPaymentOrder(ctx, orderNumber)
}
