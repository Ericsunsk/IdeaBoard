package payments

import (
	"context"
	"fmt"
	paymentpersistence "ideaboard/api/internal/payments/application/persistence"
	"time"

	"ideaboard/api/internal/credits"
	paymentcheckout "ideaboard/api/internal/payments/application/checkout"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentdispute "ideaboard/api/internal/payments/application/dispute"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentwebhook "ideaboard/api/internal/payments/application/webhook"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/secrets"
)

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
	ShutdownEnqueuer     PaymentCheckoutShutdownEnqueuer
	Store                paymentpersistence.PaymentCheckoutStore
	CheckoutGateway      paymentports.CheckoutGateway
	Wallets              credits.WalletReader
	Convergence          paymentpersistence.PaymentConvergenceStore
	Transitions          *PaymentOrderTransitions
}

// CheckoutWorkflow owns Stripe configuration, Payment Order intent creation,
// Checkout customer/session lifecycle, and user-facing Payment Order reads.
type CheckoutWorkflow struct {
	configuration        paymentpersistence.StripeConfigurationStore
	configurationGateway paymentports.StripeGateway
	shutdownEnqueuer     PaymentCheckoutShutdownEnqueuer
	checkoutStore        paymentpersistence.PaymentCheckoutStore
	checkoutGateway      paymentports.CheckoutGateway
	wallets              credits.WalletReader
	convergenceStore     paymentpersistence.PaymentConvergenceStore
	orderLifecycle       *paymentorder.PaymentOrderLifecycle
	runtime              *PaymentRuntime
	*paymentcheckout.Service
	configurationService *paymentconfiguration.Service
	readModelService     *paymentreadmodel.Service
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
	checkoutService, err := paymentcheckout.NewService(deps.Configuration, deps.Store, deps.CheckoutGateway, deps.Wallets, deps.Transitions.orderLifecycle, deps.Transitions.runtime.now)
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
		configuration: deps.Configuration, configurationGateway: deps.ConfigurationGateway, shutdownEnqueuer: deps.ShutdownEnqueuer,
		checkoutStore: deps.Store, checkoutGateway: deps.CheckoutGateway, wallets: deps.Wallets, convergenceStore: deps.Convergence,
		orderLifecycle: deps.Transitions.orderLifecycle, runtime: deps.Transitions.runtime, Service: checkoutService, configurationService: configurationService, readModelService: readModelService,
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
	configuration    paymentpersistence.StripeConfigurationStore
	webhookInbox     paymentpersistence.StripeWebhookInboxStore
	convergenceStore paymentpersistence.PaymentConvergenceStore
	refundGateway    paymentports.RefundGateway
	refundService    *paymentrefund.Service
	disputeGateway   paymentports.DisputeGateway
	disputeService   *paymentdispute.Service
	checkoutStore    paymentpersistence.PaymentCheckoutStore
	checkoutGateway  paymentports.CheckoutGateway
	orderLifecycle   *paymentorder.PaymentOrderLifecycle
	checkoutRecovery PaymentCheckoutRecovery
	verifier         paymentports.StripeWebhookVerifier
	runtime          *PaymentRuntime
	reconciliation   *paymentreconciliation.Service
	webhook          *paymentwebhook.Service
}

func NewStripeFactsWorkflow(deps StripeFactsWorkflowDependencies) (*StripeFactsWorkflow, error) {
	if deps.Configuration == nil || deps.Inbox == nil || deps.Convergence == nil || deps.Verifier == nil || deps.RefundGateway == nil || deps.DisputeGateway == nil || deps.CheckoutStore == nil || deps.CheckoutGateway == nil || deps.CheckoutRecovery == nil {
		return nil, fmt.Errorf("stripe facts workflow requires configuration, inbox, convergence store, verifier, refund gateway, dispute gateway, Checkout store, Checkout gateway, and Checkout recovery")
	}
	if deps.Transitions == nil || deps.Transitions.orderLifecycle == nil || deps.Transitions.runtime == nil {
		return nil, fmt.Errorf("stripe facts workflow requires shared payment order transitions")
	}
	refundService, err := paymentrefund.NewService(deps.Convergence, deps.Configuration, deps.RefundGateway, deps.Transitions.runtime.events, deps.Transitions.runtime.now)
	if err != nil {
		return nil, err
	}
	disputeService, err := paymentdispute.NewService(paymentdispute.Dependencies{
		CheckoutStore: deps.CheckoutStore, CheckoutGateway: deps.CheckoutGateway,
		OrderLifecycle: deps.Transitions.orderLifecycle,
		CredentialAccess: func(ctx context.Context, id string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
			c, err := deps.Configuration.GetStripeCredential(ctx, id)
			if err != nil {
				return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
			}
			key, err := secrets.DecryptMaterialString(c.APIKeyCiphertext, c.SecretMaterialKind)
			if err != nil {
				return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
			}
			return c, key, nil
		},
	})
	if err != nil {
		return nil, err
	}
	reconciliationService, err := paymentreconciliation.New(paymentreconciliation.Dependencies{
		Configuration: deps.Configuration, Convergence: deps.Convergence, CheckoutRecovery: deps.CheckoutRecovery,
		CheckoutStore: deps.CheckoutStore, CheckoutGateway: deps.CheckoutGateway, RefundGateway: deps.RefundGateway,
		DisputeGateway: deps.DisputeGateway, DisputeExpiry: disputeService, OrderLifecycle: deps.Transitions.orderLifecycle,
		CredentialAccess: func(ctx context.Context, id string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
			c, err := deps.Configuration.GetStripeCredential(ctx, id)
			if err != nil {
				return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
			}
			key, err := secrets.DecryptMaterialString(c.APIKeyCiphertext, c.SecretMaterialKind)
			if err != nil {
				return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
			}
			return c, key, nil
		},
		OrderAccess: func(ctx context.Context, id string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
			c, err := deps.Configuration.GetStripeCredentialForOrder(ctx, id)
			if err != nil {
				return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
			}
			key, err := secrets.DecryptMaterialString(c.APIKeyCiphertext, c.SecretMaterialKind)
			if err != nil {
				return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
			}
			return c, key, nil
		},
		Publish: func(userID int64, status string) {
			deps.Transitions.runtime.events.Publish(userID, paymentports.PaymentEvent{Status: status})
		}, Now: deps.Transitions.runtime.now,
	})
	if err != nil {
		return nil, err
	}
	webhookService, err := paymentwebhook.NewService(paymentwebhook.Dependencies{
		Configuration: deps.Configuration, Inbox: deps.Inbox, Convergence: deps.Convergence,
		Verifier: deps.Verifier, CheckoutStore: deps.CheckoutStore, Checkout: deps.Transitions.orderLifecycle,
		Refund: refundService, Dispute: disputeService, CheckoutGateway: deps.CheckoutGateway, DisputeGateway: deps.DisputeGateway,
		CredentialAccess: func(ctx context.Context, id string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
			c, err := deps.Configuration.GetStripeCredential(ctx, id)
			if err != nil {
				return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
			}
			key, err := secrets.DecryptMaterialString(c.APIKeyCiphertext, c.SecretMaterialKind)
			if err != nil {
				return paymentconfiguration.StoredStripeConfiguration{}, "", paymentcontracts.ErrStripeConfigurationInvalid
			}
			return c, key, nil
		},
		Events: deps.Transitions.runtime.events, Now: deps.Transitions.runtime.now,
	})
	if err != nil {
		return nil, err
	}
	return &StripeFactsWorkflow{configuration: deps.Configuration, webhookInbox: deps.Inbox, convergenceStore: deps.Convergence, refundGateway: deps.RefundGateway, refundService: refundService, disputeGateway: deps.DisputeGateway, disputeService: disputeService, checkoutStore: deps.CheckoutStore, checkoutGateway: deps.CheckoutGateway, orderLifecycle: deps.Transitions.orderLifecycle, checkoutRecovery: deps.CheckoutRecovery, verifier: deps.Verifier, runtime: deps.Transitions.runtime, reconciliation: reconciliationService, webhook: webhookService}, nil
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
