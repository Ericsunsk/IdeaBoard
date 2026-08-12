package payments

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/credits"
)

// PaymentRuntime is intentionally shared by every Payments workflow. Event
// delivery is only a post-commit hint, while the repository remains the fact
// source. A nil clock is safe to default because it is not an external seam.
type PaymentRuntime struct {
	events *EventHub
	now    func() time.Time
}

func NewPaymentRuntime(events *EventHub, now func() time.Time) *PaymentRuntime {
	if events == nil {
		events = NewEventHub()
	}
	if now == nil {
		now = time.Now
	}
	return &PaymentRuntime{events: events, now: now}
}

// PaymentOrderTransitions centralizes the Payment Order transitions shared by
// Checkout, webhook processing, reversals, and reconciliation.
type PaymentOrderTransitions struct {
	orderLifecycle *paymentOrderLifecycle
	runtime        *PaymentRuntime
}

func NewPaymentOrderTransitions(store PaymentOrderConvergenceStore, runtime *PaymentRuntime) (*PaymentOrderTransitions, error) {
	if store == nil {
		return nil, fmt.Errorf("payment order transitions require convergence store")
	}
	if runtime == nil {
		return nil, fmt.Errorf("payment order transitions require runtime")
	}
	return &PaymentOrderTransitions{
		orderLifecycle: newPaymentOrderLifecycle(store, runtime.events, runtime.now),
		runtime:        runtime,
	}, nil
}

type PaymentCheckoutRecovery interface {
	createCheckoutSessionForOrder(context.Context, StoredStripeConfiguration, string, PaymentOrder) (PaymentOrder, error)
	retrieveAndConvergeCheckout(context.Context, string, PaymentOrder) (PaymentOrder, error)
}

type CheckoutWorkflowDependencies struct {
	Configuration        StripeConfigurationStore
	ConfigurationGateway StripeGateway
	ShutdownEnqueuer     PaymentCheckoutShutdownEnqueuer
	Store                PaymentCheckoutStore
	CheckoutGateway      CheckoutGateway
	Wallets              credits.WalletReader
	Convergence          PaymentConvergenceStore
	Transitions          *PaymentOrderTransitions
}

// CheckoutWorkflow owns Stripe configuration, Payment Order intent creation,
// Checkout customer/session lifecycle, and user-facing Payment Order reads.
type CheckoutWorkflow struct {
	configuration        StripeConfigurationStore
	configurationGateway StripeGateway
	shutdownEnqueuer     PaymentCheckoutShutdownEnqueuer
	checkoutStore        PaymentCheckoutStore
	checkoutGateway      CheckoutGateway
	wallets              credits.WalletReader
	convergenceStore     PaymentConvergenceStore
	orderLifecycle       *paymentOrderLifecycle
	runtime              *PaymentRuntime
}

func NewCheckoutWorkflow(deps CheckoutWorkflowDependencies) (*CheckoutWorkflow, error) {
	if deps.Configuration == nil || deps.ConfigurationGateway == nil || deps.ShutdownEnqueuer == nil || deps.Store == nil || deps.CheckoutGateway == nil || deps.Wallets == nil || deps.Convergence == nil {
		return nil, fmt.Errorf("checkout workflow requires configuration, configuration gateway, shutdown enqueuer, store, Checkout gateway, wallet reader, and convergence store")
	}
	if deps.Transitions == nil || deps.Transitions.orderLifecycle == nil || deps.Transitions.runtime == nil {
		return nil, fmt.Errorf("checkout workflow requires shared payment order transitions")
	}
	return &CheckoutWorkflow{
		configuration: deps.Configuration, configurationGateway: deps.ConfigurationGateway, shutdownEnqueuer: deps.ShutdownEnqueuer,
		checkoutStore: deps.Store, checkoutGateway: deps.CheckoutGateway, wallets: deps.Wallets, convergenceStore: deps.Convergence,
		orderLifecycle: deps.Transitions.orderLifecycle, runtime: deps.Transitions.runtime,
	}, nil
}

type StripeFactsWorkflowDependencies struct {
	Configuration    StripeConfigurationStore
	Inbox            StripeWebhookInboxStore
	Convergence      PaymentConvergenceStore
	Verifier         StripeWebhookVerifier
	RefundGateway    RefundGateway
	DisputeGateway   DisputeGateway
	CheckoutStore    PaymentCheckoutStore
	CheckoutGateway  CheckoutGateway
	CheckoutRecovery PaymentCheckoutRecovery
	Transitions      *PaymentOrderTransitions
}

// StripeFactsWorkflow owns verified Stripe facts and every durable reversal or
// recovery transition derived from them.
type StripeFactsWorkflow struct {
	configuration    StripeConfigurationStore
	webhookInbox     StripeWebhookInboxStore
	convergenceStore PaymentConvergenceStore
	refundGateway    RefundGateway
	disputeGateway   DisputeGateway
	checkoutStore    PaymentCheckoutStore
	checkoutGateway  CheckoutGateway
	orderLifecycle   *paymentOrderLifecycle
	checkoutRecovery PaymentCheckoutRecovery
	verifier         StripeWebhookVerifier
	runtime          *PaymentRuntime
}

func NewStripeFactsWorkflow(deps StripeFactsWorkflowDependencies) (*StripeFactsWorkflow, error) {
	if deps.Configuration == nil || deps.Inbox == nil || deps.Convergence == nil || deps.Verifier == nil || deps.RefundGateway == nil || deps.DisputeGateway == nil || deps.CheckoutStore == nil || deps.CheckoutGateway == nil || deps.CheckoutRecovery == nil {
		return nil, fmt.Errorf("stripe facts workflow requires configuration, inbox, convergence store, verifier, refund gateway, dispute gateway, Checkout store, Checkout gateway, and Checkout recovery")
	}
	if deps.Transitions == nil || deps.Transitions.orderLifecycle == nil || deps.Transitions.runtime == nil {
		return nil, fmt.Errorf("stripe facts workflow requires shared payment order transitions")
	}
	return &StripeFactsWorkflow{configuration: deps.Configuration, webhookInbox: deps.Inbox, convergenceStore: deps.Convergence, refundGateway: deps.RefundGateway, disputeGateway: deps.DisputeGateway, checkoutStore: deps.CheckoutStore, checkoutGateway: deps.CheckoutGateway, orderLifecycle: deps.Transitions.orderLifecycle, checkoutRecovery: deps.CheckoutRecovery, verifier: deps.Verifier, runtime: deps.Transitions.runtime}, nil
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
