package payments

import (
	"context"
	"fmt"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
)

func (s *StripeFactsWorkflow) ReconcileDuePaymentOrders(ctx context.Context) error {
	if s == nil || s.reconciliation == nil {
		return fmt.Errorf("payments reconciliation unavailable")
	}
	return s.reconciliation.ReconcileDuePaymentOrders(ctx)
}

func (s *StripeFactsWorkflow) ListPaymentReconciliationAnomalies(ctx context.Context, limit int32) ([]paymentreconciliation.Anomaly, error) {
	if s == nil || s.reconciliation == nil {
		return nil, fmt.Errorf("payments reconciliation unavailable")
	}
	return s.reconciliation.ListAnomalies(ctx, limit)
}
