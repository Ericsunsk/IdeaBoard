package stripe

import (
	"context"
	"ideaboard/api/internal/platform/errorrender"
	"strings"
	"time"

	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/stripe/stripe-go/v86"
)

var _ paymentports.DisputeGateway = (*Adapter)(nil)

func (a *Adapter) RetrieveDispute(ctx context.Context, apiKey, disputeID string) (paymentports.StripeDispute, error) {
	disputeID = strings.TrimSpace(disputeID)
	if disputeID == "" {
		return paymentports.StripeDispute{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgRetrieveDispute, nil)
	}
	client, err := a.checkoutClient(apiKey)
	if err != nil {
		return paymentports.StripeDispute{}, err
	}
	startedAt := time.Now()
	dispute, err := client.V1Disputes.Retrieve(ctx, disputeID, nil)
	a.observe(err, startedAt)
	if err != nil {
		return paymentports.StripeDispute{}, mapStripeError(errorrender.MsgRetrieveDispute2, err)
	}
	fact := stripeDisputeFact(dispute)
	if fact.ID == "" || fact.Status == "" || fact.ChargeID == "" || fact.AmountMinor <= 0 ||
		fact.Currency == "" || fact.CreatedAt.Unix() <= 0 {
		return paymentports.StripeDispute{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgRetrieveDispute3, nil)
	}
	return fact, nil
}

func stripeDisputeFact(dispute *stripe.Dispute) paymentports.StripeDispute {
	if dispute == nil {
		return paymentports.StripeDispute{}
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
	return paymentports.StripeDispute{
		ID: strings.TrimSpace(dispute.ID), Status: status,
		ChargeID: chargeID, PaymentIntentID: paymentIntentID, AmountMinor: dispute.Amount,
		Currency:  strings.ToLower(strings.TrimSpace(string(dispute.Currency))),
		CreatedAt: time.Unix(dispute.Created, 0).UTC(),
	}
}
