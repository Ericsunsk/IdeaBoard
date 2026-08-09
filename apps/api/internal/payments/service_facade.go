package payments

import (
	"context"
	"fmt"
)

// Service is the transport-facing facade. Workflows own behavior and state;
// this type only preserves the stable API boundary used by HTTP and workers.
func (s *Service) unavailable() error { return fmt.Errorf("payments service unavailable") }

func (s *Service) GetStripeConfiguration(ctx context.Context) (StripeConfigurationSummary, error) {
	if s == nil || s.checkout == nil {
		return StripeConfigurationSummary{}, s.unavailable()
	}
	return s.checkout.GetStripeConfiguration(ctx)
}
func (s *Service) ConfigureStripe(ctx context.Context, command ConfigureStripeCommand) (StripeConfigurationSummary, error) {
	if s == nil || s.checkout == nil {
		return StripeConfigurationSummary{}, s.unavailable()
	}
	return s.checkout.ConfigureStripe(ctx, command)
}
func (s *Service) ReceiveStripeWebhook(ctx context.Context, raw []byte, signature string) (StripeWebhookReceipt, error) {
	if s == nil || s.facts == nil {
		return StripeWebhookReceipt{}, s.unavailable()
	}
	return s.facts.ReceiveStripeWebhook(ctx, raw, signature)
}
func (s *Service) ListPendingOrders(ctx context.Context, userID int64) (PendingPaymentOrderList, error) {
	if s == nil || s.checkout == nil {
		return PendingPaymentOrderList{}, s.unavailable()
	}
	return s.checkout.ListPendingOrders(ctx, userID)
}
func (s *Service) ContinuePendingOrder(ctx context.Context, userID int64, publicToken string) (PendingPaymentOrder, error) {
	if s == nil || s.checkout == nil {
		return PendingPaymentOrder{}, s.unavailable()
	}
	return s.checkout.ContinuePendingOrder(ctx, userID, publicToken)
}
func (s *Service) AbandonPendingOrder(ctx context.Context, userID int64, publicToken string) (PendingPaymentOrder, error) {
	if s == nil || s.checkout == nil {
		return PendingPaymentOrder{}, s.unavailable()
	}
	return s.checkout.AbandonPendingOrder(ctx, userID, publicToken)
}
func (s *Service) GetPurchaseOptions(ctx context.Context, userID int64) (PurchaseOptions, error) {
	if s == nil || s.checkout == nil {
		return PurchaseOptions{}, s.unavailable()
	}
	return s.checkout.GetPurchaseOptions(ctx, userID)
}
func (s *Service) CreateCheckout(ctx context.Context, command CreateCheckoutCommand) (CheckoutResult, error) {
	if s == nil || s.checkout == nil {
		return CheckoutResult{}, s.unavailable()
	}
	return s.checkout.CreateCheckout(ctx, command)
}
func (s *Service) GetPaymentOrderReturnStatus(ctx context.Context, userID int64, publicToken string) (PaymentOrderReturnStatus, error) {
	if s == nil || s.checkout == nil {
		return PaymentOrderReturnStatus{}, s.unavailable()
	}
	return s.checkout.GetPaymentOrderReturnStatus(ctx, userID, publicToken)
}
func (s *Service) ListRecharges(ctx context.Context, userID int64, cursor string, limit int) (RechargeHistoryPage, error) {
	if s == nil || s.checkout == nil {
		return RechargeHistoryPage{}, s.unavailable()
	}
	return s.checkout.ListRecharges(ctx, userID, cursor, limit)
}
func (s *Service) ListAdminPaymentOrders(ctx context.Context, input AdminPaymentOrderListInput) (AdminPaymentOrderPage, error) {
	if s == nil || s.checkout == nil {
		return AdminPaymentOrderPage{}, s.unavailable()
	}
	return s.checkout.ListAdminPaymentOrders(ctx, input)
}
func (s *Service) GetAdminPaymentOrder(ctx context.Context, orderNumber string) (AdminPaymentOrderDetail, error) {
	if s == nil || s.checkout == nil {
		return AdminPaymentOrderDetail{}, s.unavailable()
	}
	return s.checkout.GetAdminPaymentOrder(ctx, orderNumber)
}
func (s *Service) CreatePaymentRefund(ctx context.Context, command CreatePaymentRefundCommand) (PaymentRefund, error) {
	if s == nil || s.facts == nil {
		return PaymentRefund{}, s.unavailable()
	}
	return s.facts.CreatePaymentRefund(ctx, command)
}
func (s *Service) ReconcileDuePaymentOrders(ctx context.Context) error {
	if s == nil || s.facts == nil {
		return s.unavailable()
	}
	return s.facts.ReconcileDuePaymentOrders(ctx)
}
func (s *Service) ListPaymentReconciliationAnomalies(ctx context.Context, limit int32) ([]PaymentReconciliationAnomaly, error) {
	if s == nil || s.facts == nil {
		return nil, s.unavailable()
	}
	return s.facts.ListPaymentReconciliationAnomalies(ctx, limit)
}
func (s *Service) RunPaymentCheckoutShutdown(ctx context.Context, shutdownID string) error {
	if s == nil || s.checkout == nil {
		return s.unavailable()
	}
	return s.checkout.RunPaymentCheckoutShutdown(ctx, shutdownID)
}
func (s *Service) RunActivePaymentCheckoutShutdowns(ctx context.Context) error {
	if s == nil || s.checkout == nil {
		return s.unavailable()
	}
	return s.checkout.RunActivePaymentCheckoutShutdowns(ctx)
}
