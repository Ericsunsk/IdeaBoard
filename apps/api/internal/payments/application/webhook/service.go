package webhook

import (
	"context"
	"fmt"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	"ideaboard/api/internal/payments/application/credentials"
	paymentdispute "ideaboard/api/internal/payments/application/dispute"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

type InboxStore interface {
	InsertStripeWebhookEvent(context.Context, paymentports.StripeWebhookEvent, string, *time.Time) (bool, error)
	MarkStripeWebhookEventProcessed(context.Context, string, string, string, time.Time) error
}

type CheckoutStore interface {
	GetPaymentOrderByCheckoutSession(context.Context, string, string) (paymentsdomain.PaymentOrder, error)
}

type ConvergenceStore interface {
	GetPaymentRefundByStripeID(context.Context, string) (paymentsdomain.PaymentRefund, error)
	CreateExternalPaymentRefund(context.Context, paymentports.StripeRefund, time.Time) (paymentsdomain.PaymentRefund, error)
	RecordPaymentRefundAnomaly(context.Context, paymentports.StripeRefund, string, time.Time) (int64, error)
	ConvergePaymentDispute(context.Context, paymentports.StripeDispute, time.Time) (paymentsdomain.PaymentDispute, bool, error)
}

// WebhookProcessingResult is the closed inbox processing_result
// vocabulary, verified against the SQL guard
// ck_payment_webhook_processing_result
// (processing_result IN ('received','ignored','processed','failed')).
// The wire/DB words are unchanged; the type only documents the allowed
// words at compile time. The inbox seam keeps string params (the
// postgres boundary binds the words directly).
type WebhookProcessingResult string

const (
	WebhookProcessingReceived  WebhookProcessingResult = "received"
	WebhookProcessingIgnored   WebhookProcessingResult = "ignored"
	WebhookProcessingProcessed WebhookProcessingResult = "processed"
	WebhookProcessingFailed    WebhookProcessingResult = "failed"
)

type CredentialAccess func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error)
type WebhookCredentialAccess func(context.Context) ([]credentials.WebhookCredential, error)

type Dependencies struct {
	Inbox              InboxStore
	Convergence        ConvergenceStore
	Verifier           paymentports.StripeWebhookVerifier
	CheckoutStore      CheckoutStore
	Checkout           CheckoutLifecycle
	Refund             *paymentrefund.Service
	Dispute            *paymentdispute.Service
	CheckoutGateway    paymentports.CheckoutGateway
	DisputeGateway     paymentports.DisputeGateway
	CredentialAccess   CredentialAccess
	WebhookCredentials WebhookCredentialAccess
	Events             paymentports.PaymentEventPublisher
	Now                func() time.Time
}

type Service struct {
	inbox              InboxStore
	convergence        ConvergenceStore
	verifier           paymentports.StripeWebhookVerifier
	checkoutStore      CheckoutStore
	checkout           CheckoutLifecycle
	refund             *paymentrefund.Service
	dispute            *paymentdispute.Service
	checkoutGateway    paymentports.CheckoutGateway
	disputeGateway     paymentports.DisputeGateway
	credentialAccess   CredentialAccess
	webhookCredentials WebhookCredentialAccess
	events             paymentports.PaymentEventPublisher
	now                func() time.Time
}

type CheckoutLifecycle interface {
	ConvergeCheckoutSession(context.Context, paymentsdomain.PaymentOrder, paymentports.CheckoutSession, paymentorder.CheckoutSignal) (paymentsdomain.PaymentOrder, error)
}

func NewService(deps Dependencies) (*Service, error) {
	if deps.Inbox == nil || deps.Convergence == nil || deps.Verifier == nil || deps.CheckoutStore == nil || deps.Checkout == nil || deps.Refund == nil || deps.Dispute == nil || deps.CheckoutGateway == nil || deps.DisputeGateway == nil || deps.CredentialAccess == nil || deps.WebhookCredentials == nil {
		return nil, fmt.Errorf("webhook service requires inbox, convergence, verifier, checkout, refund, dispute, gateways, and credential access")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Service{inbox: deps.Inbox, convergence: deps.Convergence, verifier: deps.Verifier, checkoutStore: deps.CheckoutStore, checkout: deps.Checkout, refund: deps.Refund, dispute: deps.Dispute, checkoutGateway: deps.CheckoutGateway, disputeGateway: deps.DisputeGateway, credentialAccess: deps.CredentialAccess, webhookCredentials: deps.WebhookCredentials, events: deps.Events, now: deps.Now}, nil
}
