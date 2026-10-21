package payments

import (
	"fmt"
	"ideaboard/api/internal/credits/ports"
	paymentpersistence "ideaboard/api/internal/payments/application/persistence"
	"time"

	paymentcheckout "ideaboard/api/internal/payments/application/checkout"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	"ideaboard/api/internal/payments/application/credentials"
	paymentdispute "ideaboard/api/internal/payments/application/dispute"
	"ideaboard/api/internal/payments/application/hints"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentwebhook "ideaboard/api/internal/payments/application/webhook"
	"ideaboard/api/internal/payments/infrastructure/events"
	paymentports "ideaboard/api/internal/payments/ports"
)

// PaymentRuntime is intentionally shared by every Payments service. Event
// delivery is only a post-commit hint, while the repository remains the fact
// source. A nil clock is safe to default because it is not an external seam.
type PaymentRuntime struct {
	events paymentports.PaymentEventPublisher
	now    func() time.Time
}

func NewPaymentRuntime(publisher paymentports.PaymentEventPublisher, now func() time.Time) *PaymentRuntime {
	if publisher == nil {
		publisher = events.NewHub()
	}
	if now == nil {
		now = time.Now
	}
	return &PaymentRuntime{events: publisher, now: now}
}

// PaymentOrderTransitions centralizes the Payment Order transitions shared by
// Checkout, webhook processing, reversals, and reconciliation.
type PaymentOrderTransitions struct {
	orderLifecycle *paymentorder.PaymentOrderLifecycle
	runtime        *PaymentRuntime
}

// NewPaymentOrderTransitions wires the shared order lifecycle. When the
// convergence store also resolves the SSE fanout audience (the Postgres
// repository does via ListBillingAccountManagerIDs), hints fan out to the
// persisted payer account's current managers (#919); otherwise only the
// creating operator is notified.
func NewPaymentOrderTransitions(store paymentpersistence.PaymentOrderConvergenceStore, runtime *PaymentRuntime) (*PaymentOrderTransitions, error) {
	if store == nil {
		return nil, fmt.Errorf("payment order transitions require convergence store")
	}
	if runtime == nil {
		return nil, fmt.Errorf("payment order transitions require runtime")
	}
	audience, _ := store.(hints.BillingAccountAudience)
	lifecycle := paymentorder.NewLifecycle(store, hints.NewPublisher(runtime.events, audience), runtime.now)
	return &PaymentOrderTransitions{
		orderLifecycle: lifecycle,
		runtime:        runtime,
	}, nil
}

// ServiceDependencies is the complete dependency bag for the Payments
// composition root. Callers provide every store, gateway, and reader; the
// constructor wires the single shared credential resolver and the dispute
// helper internally instead of accepting partially-populated workflows.
type ServiceDependencies struct {
	Configuration        paymentpersistence.StripeConfigurationStore
	ConfigurationGateway paymentports.StripeGateway
	ShutdownEnqueuer     paymentconfiguration.ShutdownEnqueuer
	Store                paymentpersistence.PaymentCheckoutStore
	CheckoutGateway      paymentports.CheckoutGateway
	Wallets              ports.WalletReader
	Convergence          paymentpersistence.PaymentConvergenceStore
	Inbox                paymentpersistence.StripeWebhookInboxStore
	Verifier             paymentports.StripeWebhookVerifier
	RefundGateway        paymentports.RefundGateway
	DisputeGateway       paymentports.DisputeGateway
	Transitions          *PaymentOrderTransitions
}

// NewService is the only supported Payments root constructor. It accepts a
// complete dependency bag, never a partially-populated workflow pair.
func NewService(deps ServiceDependencies) (*Service, error) {
	if deps.Configuration == nil || deps.ConfigurationGateway == nil || deps.ShutdownEnqueuer == nil || deps.Store == nil || deps.CheckoutGateway == nil || deps.Wallets == nil || deps.Convergence == nil || deps.Inbox == nil || deps.Verifier == nil || deps.RefundGateway == nil || deps.DisputeGateway == nil {
		return nil, fmt.Errorf("payments service requires configuration, configuration gateway, shutdown enqueuer, store, Checkout gateway, wallet reader, convergence store, inbox, verifier, refund gateway, and dispute gateway")
	}
	if deps.Transitions == nil || deps.Transitions.orderLifecycle == nil || deps.Transitions.runtime == nil {
		return nil, fmt.Errorf("payments service requires shared payment order transitions")
	}
	// The credential resolver is the single Stripe credential read boundary.
	// All request-time credential lookup and plaintext normalization funnels
	// through it; per-service assembly closures must not reappear.
	resolver, err := credentials.NewResolver(deps.Configuration)
	if err != nil {
		return nil, err
	}
	checkoutService, err := paymentcheckout.NewService(deps.Configuration, deps.Store, deps.CheckoutGateway, deps.Wallets, deps.Transitions.orderLifecycle, deps.Transitions.runtime.now, resolver)
	if err != nil {
		return nil, err
	}
	readModelService, err := paymentreadmodel.NewService(deps.Configuration, deps.Store, deps.Convergence, deps.Wallets, deps.Transitions.runtime.now)
	if err != nil {
		return nil, err
	}
	configurationService, err := paymentconfiguration.NewService(paymentconfiguration.Dependencies{
		Store:    deps.Configuration,
		Gateway:  deps.ConfigurationGateway,
		Shutdown: deps.ShutdownEnqueuer,
		Replace:  deps.Configuration.ReplaceStripeConfiguration,
	})
	if err != nil {
		return nil, err
	}
	audience, _ := deps.Convergence.(hints.BillingAccountAudience)
	hintPublisher := hints.NewPublisher(deps.Transitions.runtime.events, audience)
	refundService, err := paymentrefund.NewService(deps.Convergence, deps.RefundGateway, hintPublisher, deps.Transitions.runtime.now, resolver)
	if err != nil {
		return nil, err
	}
	disputeService, err := paymentdispute.NewService(paymentdispute.Dependencies{
		CheckoutStore: deps.Store, CheckoutGateway: deps.CheckoutGateway,
		OrderLifecycle:   deps.Transitions.orderLifecycle,
		CredentialAccess: resolver.ForCredentialID,
	})
	if err != nil {
		return nil, err
	}
	reconciliationService, err := paymentreconciliation.New(paymentreconciliation.Dependencies{
		Configuration: deps.Configuration, Convergence: deps.Convergence, CheckoutRecovery: checkoutService,
		CheckoutStore: deps.Store, CheckoutGateway: deps.CheckoutGateway, RefundGateway: deps.RefundGateway,
		DisputeGateway: deps.DisputeGateway, DisputeExpiry: disputeService, OrderLifecycle: deps.Transitions.orderLifecycle,
		CredentialAccess: resolver.ForCredentialID,
		OrderAccess:      resolver.ForOrder,
		Hints:            hintPublisher, Now: deps.Transitions.runtime.now,
	})
	if err != nil {
		return nil, err
	}
	webhookService, err := paymentwebhook.NewService(paymentwebhook.Dependencies{
		Inbox: deps.Inbox, Convergence: deps.Convergence,
		Verifier: deps.Verifier, CheckoutStore: deps.Store, Checkout: deps.Transitions.orderLifecycle,
		Refund: refundService, Dispute: disputeService, CheckoutGateway: deps.CheckoutGateway, DisputeGateway: deps.DisputeGateway,
		CredentialAccess:   resolver.ForCredentialID,
		WebhookCredentials: resolver.ListWebhookCredentials,
		Hints:              hintPublisher, Now: deps.Transitions.runtime.now,
	})
	if err != nil {
		return nil, err
	}
	return &Service{
		checkoutApplicationService:   checkoutService,
		paymentReadModelService:      readModelService,
		paymentReconciliationService: reconciliationService,
		paymentRefundService:         refundService,
		paymentWebhookService:        webhookService,
		configurationService:         configurationService,
		runtime:                      deps.Transitions.runtime,
	}, nil
}
