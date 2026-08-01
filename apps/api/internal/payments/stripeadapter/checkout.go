package stripeadapter

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/payments"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v86"
)

const (
	checkoutMetadataOrderNumber  = "ideaboard_public_order_number"
	checkoutMetadataPaymentOrder = "ideaboard_payment_order_id"
	checkoutMetadataRequestHash  = "ideaboard_request_hash"
	checkoutMetadataIdeaQuantity = "ideaboard_idea_quantity"
	checkoutStatementSuffix      = "IDEABOARD"
)

var _ payments.CheckoutGateway = (*Adapter)(nil)

func (a *Adapter) CreateCustomer(ctx context.Context, input payments.CreateCheckoutCustomerInput) (payments.CheckoutCustomer, error) {
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return payments.CheckoutCustomer{}, err
	}

	params := &stripe.CustomerCreateParams{
		Description: stripe.String(fmt.Sprintf("IdeaBoard user %d", input.UserID)),
		Metadata:    map[string]string{"ideaboard_user_id": strconv.FormatInt(input.UserID, 10)},
	}
	params.SetIdempotencyKey(input.IdempotencyKey)
	customer, err := client.V1Customers.Create(ctx, params)
	if err != nil {
		return payments.CheckoutCustomer{}, mapStripeError("创建 Stripe Customer", err)
	}
	if customer == nil || strings.TrimSpace(customer.ID) == "" {
		return payments.CheckoutCustomer{}, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Customer 响应无效", nil)
	}
	return payments.CheckoutCustomer{ID: customer.ID}, nil
}

func (a *Adapter) CreateCheckoutSession(ctx context.Context, input payments.CreateCheckoutSessionInput) (payments.CheckoutSession, error) {
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return payments.CheckoutSession{}, err
	}

	metadata := checkoutMetadata(input)
	params := &stripe.CheckoutSessionCreateParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
		Customer:          stripe.String(input.CustomerID),
		ClientReferenceID: stripe.String(input.PublicOrderNumber),
		SuccessURL:        stripe.String(input.SuccessURL),
		CancelURL:         stripe.String(input.CancelURL),
		ExpiresAt:         stripe.Int64(input.ExpiresAt.Unix()),
		Metadata:          metadata,
		AutomaticTax:      &stripe.CheckoutSessionCreateAutomaticTaxParams{Enabled: stripe.Bool(true)},
		AdaptivePricing:   &stripe.CheckoutSessionCreateAdaptivePricingParams{Enabled: stripe.Bool(true)},
		PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{
			Metadata:                  metadata,
			StatementDescriptorSuffix: stripe.String(checkoutStatementSuffix),
		},
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:    stripe.String(string(stripe.CurrencyUSD)),
				Product:     stripe.String(input.ProductID),
				UnitAmount:  stripe.Int64(input.AmountMinor),
				TaxBehavior: stripe.String("exclusive"),
			},
		}},
	}
	params.SetIdempotencyKey(input.IdempotencyKey)
	session, err := client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return payments.CheckoutSession{}, mapStripeError("创建 Stripe Checkout Session", err)
	}
	return checkoutSessionFact(session, true)
}

func (a *Adapter) RetrieveCheckoutSession(ctx context.Context, input payments.RetrieveCheckoutSessionInput) (payments.CheckoutSession, error) {
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return payments.CheckoutSession{}, err
	}
	session, err := retrieveCheckoutSession(ctx, client, input.SessionID)
	if err != nil {
		return payments.CheckoutSession{}, mapStripeError("读取 Stripe Checkout Session", err)
	}
	return checkoutSessionFact(session, false)
}

func (a *Adapter) ExpireCheckoutSession(ctx context.Context, input payments.ExpireCheckoutSessionInput) (payments.ExpireCheckoutSessionResult, error) {
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return payments.ExpireCheckoutSessionResult{}, err
	}
	session, err := client.V1CheckoutSessions.Expire(ctx, input.SessionID, &stripe.CheckoutSessionExpireParams{
		Expand: []*string{stripe.String("payment_intent.latest_charge")},
	})
	if err != nil {
		expireErr := mapStripeError("关闭 Stripe Checkout Session", err)
		// A payment can complete between the local decision and Stripe's Expire
		// call. Resolve that provider race from a fresh canonical Session without
		// allowing a secondary read failure to hide the original operation error.
		session, err = retrieveCheckoutSession(ctx, client, input.SessionID)
		if err != nil {
			return payments.ExpireCheckoutSessionResult{}, expireErr
		}
	}
	return expireCheckoutSessionResult(session)
}

func retrieveCheckoutSession(ctx context.Context, client *stripe.Client, sessionID string) (*stripe.CheckoutSession, error) {
	return client.V1CheckoutSessions.Retrieve(ctx, sessionID, &stripe.CheckoutSessionRetrieveParams{
		Expand: []*string{stripe.String("payment_intent.latest_charge")},
	})
}

func expireCheckoutSessionResult(session *stripe.CheckoutSession) (payments.ExpireCheckoutSessionResult, error) {
	fact, err := checkoutSessionFact(session, false)
	if err != nil {
		return payments.ExpireCheckoutSessionResult{}, err
	}
	outcome, err := checkoutSessionExpirationOutcome(session)
	if err != nil {
		return payments.ExpireCheckoutSessionResult{}, err
	}
	return payments.ExpireCheckoutSessionResult{Outcome: outcome, Session: fact}, nil
}

func (a *Adapter) checkoutClient(apiKey string) (*stripe.Client, error) {
	if a == nil || a.newClient == nil || strings.TrimSpace(apiKey) == "" {
		return nil, payments.NewGatewayError(payments.GatewayErrorProviderUnavailable, "Stripe client unavailable", nil)
	}
	return a.newClient(apiKey), nil
}

func checkoutMetadata(input payments.CreateCheckoutSessionInput) map[string]string {
	return map[string]string{
		checkoutMetadataOrderNumber:  input.PublicOrderNumber,
		checkoutMetadataPaymentOrder: input.PaymentOrderID,
		checkoutMetadataRequestHash:  input.RequestHash,
		checkoutMetadataIdeaQuantity: strconv.FormatInt(input.IdeaQuantity, 10),
	}
}

func checkoutSessionFact(session *stripe.CheckoutSession, requireURL bool) (payments.CheckoutSession, error) {
	if session == nil || strings.TrimSpace(session.ID) == "" || (requireURL && strings.TrimSpace(session.URL) == "") {
		return payments.CheckoutSession{}, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Checkout Session 响应无效", nil)
	}
	status, ok := checkoutSessionStatus(session.Status)
	if !ok {
		return payments.CheckoutSession{}, payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Checkout Session 状态无效", nil)
	}

	fact := payments.CheckoutSession{
		ID:                  session.ID,
		URL:                 session.URL,
		Status:              status,
		PaymentStatus:       checkoutPaymentStatus(session.PaymentStatus),
		AmountSubtotalMinor: session.AmountSubtotal,
		AmountTotalMinor:    session.AmountTotal,
		Currency:            string(session.Currency),
		ExpiresAt:           time.Unix(session.ExpiresAt, 0).UTC(),
	}
	binding, err := checkoutSessionBinding(session)
	if err != nil {
		return payments.CheckoutSession{}, err
	}
	fact.PublicOrderNumber = binding.publicOrderNumber
	fact.PaymentOrderID = binding.paymentOrderID
	fact.RequestHash = binding.requestHash
	fact.IdeaQuantity = binding.ideaQuantity
	if session.Customer != nil {
		fact.CustomerID = session.Customer.ID
	}
	if session.TotalDetails != nil {
		fact.AmountTaxMinor = session.TotalDetails.AmountTax
	}
	if session.PaymentIntent != nil {
		fact.PaymentIntentID = session.PaymentIntent.ID
		if session.PaymentIntent.LatestCharge != nil {
			fact.ChargeID = session.PaymentIntent.LatestCharge.ID
			fact.ReceiptURL = session.PaymentIntent.LatestCharge.ReceiptURL
		}
	}
	return fact, nil
}

type checkoutBinding struct {
	publicOrderNumber string
	paymentOrderID    string
	requestHash       string
	ideaQuantity      int64
}

func checkoutSessionBinding(session *stripe.CheckoutSession) (checkoutBinding, error) {
	if session == nil || strings.TrimSpace(session.ClientReferenceID) == "" || session.Metadata == nil {
		return checkoutBinding{}, invalidCheckoutBinding()
	}
	publicOrderNumber := strings.TrimSpace(session.Metadata[checkoutMetadataOrderNumber])
	if publicOrderNumber == "" || publicOrderNumber != strings.TrimSpace(session.ClientReferenceID) {
		return checkoutBinding{}, invalidCheckoutBinding()
	}
	paymentOrderID, err := uuid.Parse(strings.TrimSpace(session.Metadata[checkoutMetadataPaymentOrder]))
	if err != nil {
		return checkoutBinding{}, invalidCheckoutBinding()
	}
	requestHash := strings.TrimSpace(session.Metadata[checkoutMetadataRequestHash])
	if len(requestHash) != 64 {
		return checkoutBinding{}, invalidCheckoutBinding()
	}
	decodedHash, err := hex.DecodeString(requestHash)
	if err != nil || len(decodedHash) != 32 {
		return checkoutBinding{}, invalidCheckoutBinding()
	}
	ideaQuantity, err := strconv.ParseInt(strings.TrimSpace(session.Metadata[checkoutMetadataIdeaQuantity]), 10, 64)
	if err != nil || ideaQuantity <= 0 {
		return checkoutBinding{}, invalidCheckoutBinding()
	}
	return checkoutBinding{
		publicOrderNumber: publicOrderNumber,
		paymentOrderID:    paymentOrderID.String(),
		requestHash:       requestHash,
		ideaQuantity:      ideaQuantity,
	}, nil
}

func invalidCheckoutBinding() error {
	return payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Checkout Session 订单绑定无效", nil)
}

func checkoutPaymentStatus(status stripe.CheckoutSessionPaymentStatus) payments.CheckoutPaymentStatus {
	if status == stripe.CheckoutSessionPaymentStatusPaid {
		return payments.CheckoutPaymentStatusPaid
	}
	return payments.CheckoutPaymentStatusUnpaid
}

func checkoutSessionStatus(status stripe.CheckoutSessionStatus) (payments.CheckoutSessionStatus, bool) {
	switch status {
	case stripe.CheckoutSessionStatusOpen:
		return payments.CheckoutSessionStatusOpen, true
	case stripe.CheckoutSessionStatusComplete:
		return payments.CheckoutSessionStatusComplete, true
	case stripe.CheckoutSessionStatusExpired:
		return payments.CheckoutSessionStatusExpired, true
	default:
		return "", false
	}
}

func checkoutSessionExpirationOutcome(session *stripe.CheckoutSession) (payments.CheckoutSessionExpirationOutcome, error) {
	if session == nil {
		return "", invalidCheckoutExpirationResponse()
	}
	switch {
	case session.Status == stripe.CheckoutSessionStatusComplete && session.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid:
		return payments.CheckoutSessionExpirationOutcomePaid, nil
	case session.Status == stripe.CheckoutSessionStatusExpired && session.PaymentStatus == stripe.CheckoutSessionPaymentStatusUnpaid:
		return payments.CheckoutSessionExpirationOutcomeExpired, nil
	case session.Status == stripe.CheckoutSessionStatusComplete && session.PaymentStatus == stripe.CheckoutSessionPaymentStatusUnpaid:
		return payments.CheckoutSessionExpirationOutcomeAsyncPaymentPending, nil
	case session.Status == stripe.CheckoutSessionStatusOpen && session.PaymentStatus == stripe.CheckoutSessionPaymentStatusUnpaid:
		return payments.CheckoutSessionExpirationOutcomeOpen, nil
	default:
		return "", invalidCheckoutExpirationResponse()
	}
}

func invalidCheckoutExpirationResponse() error {
	return payments.NewGatewayError(payments.GatewayErrorInvalidResponse, "Stripe Checkout Session 关闭结果无效", nil)
}
