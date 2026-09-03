package payments

import (
	"context"
	"fmt"

	paymentports "ideaboard/api/internal/payments/ports"
)

// ReceiveStripeWebhook is the composition boundary exposed to HTTP handlers.
// Webhook business orchestration lives in application/webhook.
func (s *StripeFactsWorkflow) ReceiveStripeWebhook(ctx context.Context, raw []byte, signature string) (paymentports.StripeWebhookReceipt, error) {
	if s == nil || s.webhook == nil {
		return paymentports.StripeWebhookReceipt{}, fmt.Errorf("payments webhook service unavailable")
	}
	return s.webhook.ReceiveStripeWebhook(ctx, raw, signature)
}
