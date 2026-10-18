package stripe

import (
	"context"
	"encoding/hex"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"strconv"
	"strings"
	"time"

	paymentports "ideaboard/api/internal/payments/ports"

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

var _ paymentports.CheckoutGateway = (*Adapter)(nil)

func (a *Adapter) CreateCustomer(ctx context.Context, input paymentports.CreateCheckoutCustomerInput) (paymentports.CheckoutCustomer, error) {
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return paymentports.CheckoutCustomer{}, err
	}

	params := &stripe.CustomerCreateParams{
		Description: stripe.String(fmt.Sprintf("IdeaBoard user %d", input.UserID)),
		Metadata:    map[string]string{"ideaboard_user_id": strconv.FormatInt(input.UserID, 10)},
	}
	params.SetIdempotencyKey(input.IdempotencyKey)
	startedAt := time.Now()
	customer, err := client.V1Customers.Create(ctx, params)
	a.observe(err, startedAt)
	if err != nil {
		return paymentports.CheckoutCustomer{}, mapStripeCreateError(errorrender.MsgCreateCustomer, err)
	}
	if customer == nil || strings.TrimSpace(customer.ID) == "" {
		return paymentports.CheckoutCustomer{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgCreateCustomer2, nil)
	}
	return paymentports.CheckoutCustomer{ID: customer.ID}, nil
}

func (a *Adapter) CreateCheckoutSession(ctx context.Context, input paymentports.CreateCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	if strings.TrimSpace(input.PaymentMethodConfigurationID) == "" {
		return paymentports.CheckoutSession{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgCreateCheckoutSession, nil)
	}
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return paymentports.CheckoutSession{}, err
	}

	metadata := checkoutMetadata(input)
	params := &stripe.CheckoutSessionCreateParams{
		Mode:                       stripe.String(string(stripe.CheckoutSessionModePayment)),
		PaymentMethodConfiguration: stripe.String(input.PaymentMethodConfigurationID),
		Customer:                   stripe.String(input.CustomerID),
		ClientReferenceID:          stripe.String(input.PublicOrderNumber),
		SuccessURL:                 stripe.String(input.SuccessURL),
		CancelURL:                  stripe.String(input.CancelURL),
		ExpiresAt:                  stripe.Int64(input.ExpiresAt.Unix()),
		Metadata:                   metadata,
		AdaptivePricing:            &stripe.CheckoutSessionCreateAdaptivePricingParams{Enabled: stripe.Bool(true)},
		PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{
			Metadata:                  metadata,
			StatementDescriptorSuffix: stripe.String(checkoutStatementSuffix),
		},
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   stripe.String(string(stripe.CurrencyUSD)),
				Product:    stripe.String(input.ProductID),
				UnitAmount: stripe.Int64(input.AmountMinor),
			},
		}},
	}
	params.SetIdempotencyKey(input.IdempotencyKey)
	startedAt := time.Now()
	session, err := client.V1CheckoutSessions.Create(ctx, params)
	a.observe(err, startedAt)
	if err != nil {
		return paymentports.CheckoutSession{}, mapStripeCreateError(errorrender.MsgCreateCheckoutSession2, err)
	}
	return checkoutSessionFact(session, true)
}

func (a *Adapter) RetrieveCheckoutSession(ctx context.Context, input paymentports.RetrieveCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return paymentports.CheckoutSession{}, err
	}
	session, err := a.retrieveCheckoutSession(ctx, client, input.SessionID)
	if err != nil {
		return paymentports.CheckoutSession{}, mapStripeError(errorrender.MsgRetrieveCheckoutSession, err)
	}
	return checkoutSessionFact(session, false)
}

func (a *Adapter) ExpireCheckoutSession(ctx context.Context, input paymentports.ExpireCheckoutSessionInput) (paymentports.ExpireCheckoutSessionResult, error) {
	client, err := a.checkoutClient(input.APIKey)
	if err != nil {
		return paymentports.ExpireCheckoutSessionResult{}, err
	}
	expireStartedAt := time.Now()
	session, err := client.V1CheckoutSessions.Expire(ctx, input.SessionID, &stripe.CheckoutSessionExpireParams{
		Expand: []*string{stripe.String("payment_intent.latest_charge")},
	})
	a.observe(err, expireStartedAt)
	if err != nil {
		expireErr := mapStripeError(errorrender.MsgExpireCheckoutSession, err)
		// A payment can complete between the local decision and Stripe's Expire
		// call. Resolve that provider race from a fresh canonical Session without
		// allowing a secondary read failure to hide the original operation error.
		session, err = a.retrieveCheckoutSession(ctx, client, input.SessionID)
		if err != nil {
			return paymentports.ExpireCheckoutSessionResult{}, expireErr
		}
	}
	return expireCheckoutSessionResult(session)
}

func (a *Adapter) retrieveCheckoutSession(ctx context.Context, client *stripe.Client, sessionID string) (*stripe.CheckoutSession, error) {
	startedAt := time.Now()
	session, err := client.V1CheckoutSessions.Retrieve(ctx, sessionID, &stripe.CheckoutSessionRetrieveParams{
		Expand: []*string{stripe.String("payment_intent.latest_charge")},
	})
	a.observe(err, startedAt)
	return session, err
}

func expireCheckoutSessionResult(session *stripe.CheckoutSession) (paymentports.ExpireCheckoutSessionResult, error) {
	fact, err := checkoutSessionFact(session, false)
	if err != nil {
		return paymentports.ExpireCheckoutSessionResult{}, err
	}
	outcome, err := checkoutSessionExpirationOutcome(session)
	if err != nil {
		return paymentports.ExpireCheckoutSessionResult{}, err
	}
	return paymentports.ExpireCheckoutSessionResult{Outcome: outcome, Session: fact}, nil
}

func (a *Adapter) checkoutClient(apiKey string) (*stripe.Client, error) {
	if a == nil || a.newClient == nil || strings.TrimSpace(apiKey) == "" {
		return nil, paymentports.NewGatewayError(paymentports.GatewayErrorProviderUnavailable, "Stripe client unavailable", nil)
	}
	return a.newClient(apiKey), nil
}

func checkoutMetadata(input paymentports.CreateCheckoutSessionInput) map[string]string {
	return map[string]string{
		checkoutMetadataOrderNumber:  input.PublicOrderNumber,
		checkoutMetadataPaymentOrder: input.PaymentOrderID,
		checkoutMetadataRequestHash:  input.RequestHash,
		checkoutMetadataIdeaQuantity: strconv.FormatInt(input.IdeaQuantity, 10),
	}
}

func checkoutSessionFact(session *stripe.CheckoutSession, requireURL bool) (paymentports.CheckoutSession, error) {
	if session == nil || strings.TrimSpace(session.ID) == "" || (requireURL && strings.TrimSpace(session.URL) == "") {
		return paymentports.CheckoutSession{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgCheckoutSessionFact, nil)
	}
	status, ok := checkoutSessionStatus(session.Status)
	if !ok {
		return paymentports.CheckoutSession{}, paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgCheckoutSessionFact2, nil)
	}

	fact := paymentports.CheckoutSession{
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
		return paymentports.CheckoutSession{}, err
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
	return paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgInvalidCheckoutBinding, nil)
}

func checkoutPaymentStatus(status stripe.CheckoutSessionPaymentStatus) paymentports.CheckoutPaymentStatus {
	if status == stripe.CheckoutSessionPaymentStatusPaid {
		return paymentports.CheckoutPaymentStatusPaid
	}
	return paymentports.CheckoutPaymentStatusUnpaid
}

func checkoutSessionStatus(status stripe.CheckoutSessionStatus) (paymentports.CheckoutSessionStatus, bool) {
	switch status {
	case stripe.CheckoutSessionStatusOpen:
		return paymentports.CheckoutSessionStatusOpen, true
	case stripe.CheckoutSessionStatusComplete:
		return paymentports.CheckoutSessionStatusComplete, true
	case stripe.CheckoutSessionStatusExpired:
		return paymentports.CheckoutSessionStatusExpired, true
	default:
		return "", false
	}
}

func checkoutSessionExpirationOutcome(session *stripe.CheckoutSession) (paymentports.CheckoutSessionExpirationOutcome, error) {
	if session == nil {
		return "", invalidCheckoutExpirationResponse()
	}
	switch {
	case session.Status == stripe.CheckoutSessionStatusComplete && session.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid:
		return paymentports.CheckoutSessionExpirationOutcomePaid, nil
	case session.Status == stripe.CheckoutSessionStatusExpired && session.PaymentStatus == stripe.CheckoutSessionPaymentStatusUnpaid:
		return paymentports.CheckoutSessionExpirationOutcomeExpired, nil
	case session.Status == stripe.CheckoutSessionStatusComplete && session.PaymentStatus == stripe.CheckoutSessionPaymentStatusUnpaid:
		return paymentports.CheckoutSessionExpirationOutcomeAsyncPaymentPending, nil
	case session.Status == stripe.CheckoutSessionStatusOpen && session.PaymentStatus == stripe.CheckoutSessionPaymentStatusUnpaid:
		return paymentports.CheckoutSessionExpirationOutcomeOpen, nil
	default:
		return "", invalidCheckoutExpirationResponse()
	}
}

func invalidCheckoutExpirationResponse() error {
	return paymentports.NewGatewayError(paymentports.GatewayErrorInvalidResponse, errorrender.MsgInvalidCheckoutExpirationResponse, nil)
}
