package stripe

import (
	"context"
	"strings"

	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/stripe/stripe-go/v86"
)

var _ paymentports.RefundGateway = (*Adapter)(nil)

func (a *Adapter) CreateRefund(ctx context.Context, input paymentports.CreateStripeRefundInput) (paymentports.StripeRefund, error) {
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return paymentports.StripeRefund{}, err
	}
	if strings.TrimSpace(input.ChargeID) == "" || input.AmountMinor <= 0 || strings.TrimSpace(input.Currency) == "" {
		return paymentports.StripeRefund{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe Refund 参数无效", nil)
	}
	params := &stripe.RefundCreateParams{Charge: stripe.String(input.ChargeID), Amount: stripe.Int64(input.AmountMinor), Currency: stripe.String(input.Currency)}
	params.SetIdempotencyKey(input.IdempotencyKey)
	refund, err := client.V1Refunds.Create(ctx, params)
	if err != nil {
		return paymentports.StripeRefund{}, mapStripeError("创建 Stripe Refund", err)
	}
	if refund == nil || strings.TrimSpace(refund.ID) == "" {
		return paymentports.StripeRefund{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe Refund 响应无效", nil)
	}
	return stripeRefundFact(refund), nil
}

func (a *Adapter) RetrieveRefund(ctx context.Context, apiKey, refundID string) (paymentports.StripeRefund, error) {
	client, err := a.checkoutClient(apiKey)
	if err != nil {
		return paymentports.StripeRefund{}, err
	}
	refund, err := client.V1Refunds.Retrieve(ctx, refundID, nil)
	if err != nil {
		return paymentports.StripeRefund{}, mapStripeError("读取 Stripe Refund", err)
	}
	if refund == nil || strings.TrimSpace(refund.ID) == "" {
		return paymentports.StripeRefund{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, "Stripe Refund 响应无效", nil)
	}
	return stripeRefundFact(refund), nil
}

func stripeRefundFact(refund *stripe.Refund) paymentports.StripeRefund {
	chargeID := ""
	if refund.Charge != nil {
		chargeID = refund.Charge.ID
	}
	return paymentports.StripeRefund{
		ID: refund.ID, Status: string(refund.Status), ChargeID: chargeID,
		AmountMinor: refund.Amount, Currency: strings.ToLower(string(refund.Currency)),
	}
}
