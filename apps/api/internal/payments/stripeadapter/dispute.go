package stripeadapter

import (
	"context"
	"strings"
	"time"

	"ideaboard/api/internal/payments"

	"github.com/stripe/stripe-go/v86"
)

var _ payments.DisputeGateway = (*Adapter)(nil)

func (a *Adapter) RetrieveDispute(ctx context.Context, apiKey, disputeID string) (payments.StripeDispute, error) {
	disputeID = strings.TrimSpace(disputeID)
	if disputeID == "" {
		return payments.StripeDispute{}, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Dispute 参数无效", nil)
	}
	client, err := a.checkoutClient(apiKey)
	if err != nil {
		return payments.StripeDispute{}, err
	}
	dispute, err := client.V1Disputes.Retrieve(ctx, disputeID, nil)
	if err != nil {
		return payments.StripeDispute{}, mapStripeError("读取 Stripe Dispute", err)
	}
	fact := stripeDisputeFact(dispute)
	if fact.ID == "" || fact.Status == "" || fact.ChargeID == "" || fact.AmountMinor <= 0 ||
		fact.Currency == "" || fact.CreatedAt.Unix() <= 0 {
		return payments.StripeDispute{}, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Dispute 响应无效", nil)
	}
	return fact, nil
}

func stripeDisputeFact(dispute *stripe.Dispute) payments.StripeDispute {
	if dispute == nil {
		return payments.StripeDispute{}
	}
	chargeID := ""
	if dispute.Charge != nil {
		chargeID = strings.TrimSpace(dispute.Charge.ID)
	}
	paymentIntentID := ""
	if dispute.PaymentIntent != nil {
		paymentIntentID = strings.TrimSpace(dispute.PaymentIntent.ID)
	}
	status := strings.ToLower(strings.TrimSpace(string(dispute.Status)))
	return payments.StripeDispute{
		ID: strings.TrimSpace(dispute.ID), Status: status,
		ChargeID: chargeID, PaymentIntentID: paymentIntentID, AmountMinor: dispute.Amount,
		Currency:  strings.ToLower(strings.TrimSpace(string(dispute.Currency))),
		CreatedAt: time.Unix(dispute.Created, 0).UTC(),
	}
}
