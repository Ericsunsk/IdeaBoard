package payments

import (
	"context"
	"fmt"
	"ideaboard/api/internal/credits/ports"
	paymentpersistence "ideaboard/api/internal/payments/application/persistence"
	"time"

	paymentcheckout "ideaboard/api/internal/payments/application/checkout"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/payments/application/credentials"
	paymentdispute "ideaboard/api/internal/payments/application/dispute"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentwebhook "ideaboard/api/internal/payments/application/webhook"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
	paymentports "ideaboard/api/internal/payments/ports"
)

// checkoutApplicationService gives the embedded checkout service a distinct field name
// from the read-model Service while keeping both application APIs promoted on
// CheckoutWorkflow.
type checkoutApplicationService = paymentcheckout.Service
type paymentRefundService = paymentrefund.Service
type paymentWebhookService = paymentwebhook.Service

// PaymentRuntime is intentionally shared by every Payments workflow. Event
// delivery is only a post-commit hint, while the repository remains the fact
// source. A nil clock is safe to default because it is not an external seam.
type PaymentRuntime struct {
	events paymentports.PaymentEventPublisher
	now    func() time.Time
}

func NewPaymentRuntime(events paymentports.PaymentEventPublisher, now func() time.Time) *PaymentRuntime {
	if events == nil {
		events = paymentevents.NewHub()
	}
	if now == nil {
		now = time.Now
	}
	return &PaymentRuntime{events: events, now: now}
}

// PaymentOrderTransitions centralizes the Payment Order transitions shared by
// Checkout, webhook processing, reversals, and reconciliation.
type PaymentOrderTransitions struct {
	orderLifecycle *paymentorder.PaymentOrderLifecycle
	runtime        *PaymentRuntime
}

func NewPaymentOrderTransitions(store paymentpersistence.PaymentOrderConvergenceStore, runtime *PaymentRuntime) (*PaymentOrderTransitions, error) {
	if store == nil {
		return nil, fmt.Errorf("payment order transitions require convergence store")
	}
	if runtime == nil {
		return nil, fmt.Errorf("payment order transitions require runtime")
	}
	return &PaymentOrderTransitions{
		orderLifecycle: paymentorder.NewLifecycle(store, runtime.events, runtime.now),
		runtime:        runtime,
	}, nil
}

type PaymentCheckoutRecovery interface {
	CreateSessionForOrder(context.Context, paymentconfiguration.StoredStripeConfiguration, string, paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error)
	RetrieveAndConverge(context.Context, string, paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error)
}

type CheckoutWorkflowDependencies struct {
	Configuration        paymentpersistence.StripeConfigurationStore
	ConfigurationGateway paymentports.StripeGateway
	ShutdownEnqueuer     paymentconfiguration.ShutdownEnqueuer
	Store                paymentpersistence.PaymentCheckoutStore
	CheckoutGateway      paymentports.CheckoutGateway
	Wallets              ports.WalletReader
	Convergence          paymentpersistence.PaymentConvergenceStore
	Transitions          *PaymentOrderTransitions
}

// CheckoutWorkflow owns Stripe configuration, Payment Order intent creation,
// Checkout customer/session lifecycle, and user-facing Payment Order reads.
type CheckoutWorkflow struct {
	runtime *PaymentRuntime
	*checkoutApplicationService
	*paymentreadmodel.Service
	configurationService *paymentconfiguration.Service
}

// GetStripeConfiguration delegates to the configuration application service;
// the workflow remains the composition boundary consumed by HTTP adapters.
func (s *CheckoutWorkflow) GetStripeConfiguration(ctx context.Context) (paymentcontracts.StripeConfigurationSummary, error) {
	if s == nil || s.configurationService == nil {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	return s.configurationService.GetStripeConfiguration(ctx)
}

func (s *CheckoutWorkflow) ConfigureStripe(ctx context.Context, command paymentcontracts.ConfigureStripeCommand) (paymentcontracts.StripeConfigurationSummary, error) {
	if s == nil || s.configurationService == nil {
		return paymentcontracts.StripeConfigurationSummary{}, fmt.Errorf("payments configuration service unavailable")
	}
	return s.configurationService.ConfigureStripe(ctx, command)
}

func NewCheckoutWorkflow(deps CheckoutWorkflowDependencies) (*CheckoutWorkflow, error) {
	if deps.Configuration == nil || deps.ConfigurationGateway == nil || deps.ShutdownEnqueuer == nil || deps.Store == nil || deps.CheckoutGateway == nil || deps.Wallets == nil || deps.Convergence == nil {
		return nil, fmt.Errorf("checkout workflow requires configuration, configuration gateway, shutdown enqueuer, store, Checkout gateway, wallet reader, and convergence store")
	}
	if deps.Transitions == nil || deps.Transitions.orderLifecycle == nil || deps.Transitions.runtime == nil {
		return nil, fmt.Errorf("checkout workflow requires shared payment order transitions")
	}
	resolver, err := newCredentialResolver(deps.Configuration)
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
	return &CheckoutWorkflow{
		runtime: deps.Transitions.runtime, checkoutApplicationService: checkoutService, Service: readModelService, configurationService: configurationService,
	}, nil
}

type StripeFactsWorkflowDependencies struct {
	Configuration    paymentpersistence.StripeConfigurationStore
	Inbox            paymentpersistence.StripeWebhookInboxStore
	Convergence      paymentpersistence.PaymentConvergenceStore
	Verifier         paymentports.StripeWebhookVerifier
	RefundGateway    paymentports.RefundGateway
	DisputeGateway   paymentports.DisputeGateway
	CheckoutStore    paymentpersistence.PaymentCheckoutStore
	CheckoutGateway  paymentports.CheckoutGateway
	CheckoutRecovery PaymentCheckoutRecovery
	Transitions      *PaymentOrderTransitions
}

// StripeFactsWorkflow owns verified Stripe facts and every durable reversal or
// recovery transition derived from them.
type StripeFactsWorkflow struct {
	*paymentreconciliation.Service
	*paymentRefundService
	runtime *PaymentRuntime
	*paymentWebhookService
}

// newCredentialResolver builds the single credential boundary shared by the
// Stripe facts workflow. All request-time credential lookup and plaintext
// normalization funnels through it; the deleted per-service assembly closures
// must not reappear.
func newCredentialResolver(store credentials.Store) (*credentials.Resolver, error) {
	return credentials.NewResolver(store)
}

func NewStripeFactsWorkflow(deps StripeFactsWorkflowDependencies) (*StripeFactsWorkflow, error) {
	if deps.Configuration == nil || deps.Inbox == nil || deps.Convergence == nil || deps.Verifier == nil || deps.RefundGateway == nil || deps.DisputeGateway == nil || deps.CheckoutStore == nil || deps.CheckoutGateway == nil || deps.CheckoutRecovery == nil {
		return nil, fmt.Errorf("stripe facts workflow requires configuration, inbox, convergence store, verifier, refund gateway, dispute gateway, Checkout store, Checkout gateway, and Checkout recovery")
	}
	if deps.Transitions == nil || deps.Transitions.orderLifecycle == nil || deps.Transitions.runtime == nil {
		return nil, fmt.Errorf("stripe facts workflow requires shared payment order transitions")
	}
	resolver, err := newCredentialResolver(deps.Configuration)
	if err != nil {
		return nil, err
	}
	refundService, err := paymentrefund.NewService(deps.Convergence, deps.RefundGateway, deps.Transitions.runtime.events, deps.Transitions.runtime.now, resolver)
	if err != nil {
		return nil, err
	}
	disputeService, err := paymentdispute.NewService(paymentdispute.Dependencies{
		CheckoutStore: deps.CheckoutStore, CheckoutGateway: deps.CheckoutGateway,
		OrderLifecycle:   deps.Transitions.orderLifecycle,
		CredentialAccess: resolver.ForCredentialID,
	})
	if err != nil {
		return nil, err
	}
	reconciliationService, err := paymentreconciliation.New(paymentreconciliation.Dependencies{
		Configuration: deps.Configuration, Convergence: deps.Convergence, CheckoutRecovery: deps.CheckoutRecovery,
		CheckoutStore: deps.CheckoutStore, CheckoutGateway: deps.CheckoutGateway, RefundGateway: deps.RefundGateway,
		DisputeGateway: deps.DisputeGateway, DisputeExpiry: disputeService, OrderLifecycle: deps.Transitions.orderLifecycle,
		CredentialAccess: resolver.ForCredentialID,
		OrderAccess:      resolver.ForOrder,
		Publish: func(userID int64, status paymentports.PaymentEventStatus) {
			deps.Transitions.runtime.events.Publish(userID, paymentports.PaymentEvent{Status: status})
		}, Now: deps.Transitions.runtime.now,
	})
	if err != nil {
		return nil, err
	}
	webhookService, err := paymentwebhook.NewService(paymentwebhook.Dependencies{
		Inbox: deps.Inbox, Convergence: deps.Convergence,
		Verifier: deps.Verifier, CheckoutStore: deps.CheckoutStore, Checkout: deps.Transitions.orderLifecycle,
		Refund: refundService, Dispute: disputeService, CheckoutGateway: deps.CheckoutGateway, DisputeGateway: deps.DisputeGateway,
		CredentialAccess:   resolver.ForCredentialID,
		WebhookCredentials: resolver.ListWebhookCredentials,
		Events:             deps.Transitions.runtime.events, Now: deps.Transitions.runtime.now,
	})
	if err != nil {
		return nil, err
	}
	return &StripeFactsWorkflow{Service: reconciliationService, paymentRefundService: refundService, runtime: deps.Transitions.runtime, paymentWebhookService: webhookService}, nil
}

// NewService is the only supported Payments root constructor. It accepts
// complete workflows, never a partially-populated dependency bag.
func NewService(checkout *CheckoutWorkflow, facts *StripeFactsWorkflow) (*Service, error) {
	if checkout == nil || facts == nil {
		return nil, fmt.Errorf("payments service requires Checkout and Stripe facts workflows")
	}
	runtime := checkout.runtime
	if runtime == nil || facts.runtime != runtime {
		return nil, fmt.Errorf("payments workflows must share one runtime")
	}
	return &Service{CheckoutWorkflow: checkout, StripeFactsWorkflow: facts, runtime: runtime}, nil
}
