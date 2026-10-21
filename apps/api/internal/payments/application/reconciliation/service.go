package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

type ConfigurationStore interface {
	GetStripeConfiguration(context.Context) (paymentconfiguration.StoredStripeConfiguration, error)
}

// Runner is the canonical sweep seam for the River payment-reconciliation
// worker: converge every due order/refund/dispute claim in one pass. Workers
// depend on this interface, never on *Service.
type Runner interface {
	ReconcileDuePaymentOrders(context.Context) error
}

var _ Runner = (*Service)(nil)

type ConvergenceStore interface {
	ClaimDuePaymentOrderReconciliations(context.Context, time.Time) ([]Claim, error)
	CompletePaymentOrderReconciliationClaim(context.Context, Claim, Outcome) (bool, error)
	ListPaymentReconciliationAnomalies(context.Context, int32) ([]Anomaly, error)
	ListDuePaymentDisputes(context.Context, time.Time, int) ([]paymentsdomain.PaymentDispute, error)
	ConvergePaymentDispute(context.Context, paymentports.StripeDispute, time.Time) (paymentsdomain.PaymentDispute, bool, error)
	ListPendingPaymentRefunds(context.Context, time.Time, int) ([]paymentsdomain.PaymentRefund, error)
	MarkPaymentRefundFailed(context.Context, string, string, time.Time) (paymentsdomain.PaymentRefund, error)
	MarkPaymentRefundProviderResult(context.Context, string, string, string, time.Time) (paymentsdomain.PaymentRefund, error)
	MarkPaymentRefundSucceeded(context.Context, string, time.Time) (paymentsdomain.PaymentRefund, error)
	GetPaymentRefundByStripeID(context.Context, string) (paymentsdomain.PaymentRefund, error)
	GetPaymentOrderPayerAccount(context.Context, string) (int64, error)
}

type CheckoutRecovery interface {
	CreateSessionForOrder(context.Context, paymentconfiguration.StoredStripeConfiguration, string, paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error)
	RetrieveAndConverge(context.Context, string, paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error)
}

type CheckoutStore interface {
	WithUserCheckoutLock(context.Context, int64, func(context.Context) error) error
	ListActivePaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error)
}

type DisputeCheckoutExpiry interface {
	ExpireDisputeCheckoutSessions(context.Context, paymentsdomain.PaymentDispute) error
}

type Service struct {
	configuration    ConfigurationStore
	convergence      ConvergenceStore
	checkoutRecovery CheckoutRecovery
	checkoutStore    CheckoutStore
	checkoutGateway  paymentports.CheckoutGateway
	refundGateway    paymentports.RefundGateway
	disputeGateway   paymentports.DisputeGateway
	disputeExpiry    DisputeCheckoutExpiry
	orderLifecycle   *paymentorder.PaymentOrderLifecycle
	credentialAccess func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error)
	orderAccess      func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error)
	hints            paymentports.PaymentHintPublisher
	now              func() time.Time
}

type Dependencies struct {
	Configuration    ConfigurationStore
	Convergence      ConvergenceStore
	CheckoutRecovery CheckoutRecovery
	CheckoutStore    CheckoutStore
	CheckoutGateway  paymentports.CheckoutGateway
	RefundGateway    paymentports.RefundGateway
	DisputeGateway   paymentports.DisputeGateway
	DisputeExpiry    DisputeCheckoutExpiry
	OrderLifecycle   *paymentorder.PaymentOrderLifecycle
	CredentialAccess func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error)
	OrderAccess      func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error)
	Hints            paymentports.PaymentHintPublisher
	Now              func() time.Time
}

func New(deps Dependencies) (*Service, error) {
	if deps.Configuration == nil || deps.Convergence == nil || deps.CheckoutRecovery == nil || deps.CheckoutStore == nil || deps.CheckoutGateway == nil || deps.RefundGateway == nil || deps.DisputeGateway == nil || deps.OrderLifecycle == nil || deps.CredentialAccess == nil || deps.OrderAccess == nil || deps.Now == nil {
		return nil, fmt.Errorf("reconciliation service requires complete dependencies")
	}
	return &Service{configuration: deps.Configuration, convergence: deps.Convergence, checkoutRecovery: deps.CheckoutRecovery, checkoutStore: deps.CheckoutStore, checkoutGateway: deps.CheckoutGateway, refundGateway: deps.RefundGateway, disputeGateway: deps.DisputeGateway, disputeExpiry: deps.DisputeExpiry, orderLifecycle: deps.OrderLifecycle, credentialAccess: deps.CredentialAccess, orderAccess: deps.OrderAccess, hints: deps.Hints, now: deps.Now}, nil
}

func (s *Service) ListAnomalies(ctx context.Context, limit int32) ([]Anomaly, error) {
	return s.convergence.ListPaymentReconciliationAnomalies(ctx, limit)
}

func (s *Service) ReconcileDuePaymentOrders(ctx context.Context) error {
	now := s.now().UTC()
	claims, err := s.convergence.ClaimDuePaymentOrderReconciliations(ctx, now)
	if err != nil {
		return err
	}
	active, policyErr := s.configuration.GetStripeConfiguration(ctx)
	if paymentconfiguration.IsNotFound(policyErr) {
		policyErr = paymentcontracts.ErrStripeConfigurationNotFound
	}
	var retryErr error
	for _, claim := range claims {
		credential, apiKey, credentialErr := s.credentialAccess(ctx, claim.Order.StripeCredentialID)
		outcome, processErr := s.reconcilePaymentOrderClaim(ctx, credential, apiKey, credentialErr, policyErr == nil && active.PurchasesEnabled, policyErr, claim, now)
		completed, completeErr := s.convergence.CompletePaymentOrderReconciliationClaim(ctx, claim, outcome)
		if completeErr != nil {
			retryErr = errors.Join(retryErr, completeErr)
			continue
		}
		if !completed {
			continue
		}
		if processErr != nil && FailureIsTransient(processErr) {
			retryErr = errors.Join(retryErr, processErr)
		}
	}
	if err := s.reconcileDuePaymentRefunds(ctx, now); err != nil {
		retryErr = errors.Join(retryErr, err)
	}
	if err := s.reconcileDuePaymentDisputes(ctx, now); err != nil {
		retryErr = errors.Join(retryErr, err)
	}
	return retryErr
}

// ReconcileDuePaymentRefunds runs the refund recovery pass independently.
// It is exposed for composition-level orchestration and focused integration tests.
func (s *Service) ReconcileDuePaymentRefunds(ctx context.Context, now time.Time) error {
	return s.reconcileDuePaymentRefunds(ctx, now)
}

// ReconcileDuePaymentDisputes runs the dispute recovery pass independently.
// It mirrors ReconcileDuePaymentRefunds so each recovery state machine stays
// testable without driving the full batch runner.
func (s *Service) ReconcileDuePaymentDisputes(ctx context.Context, now time.Time) error {
	return s.reconcileDuePaymentDisputes(ctx, now)
}

// ReconcilePaymentOrderClaim evaluates one claimed order and returns the
// durable outcome. The batch runner uses the same operation; exposing it keeps
// focused integration tests on the application boundary instead of reaching
// into workflow internals.
func (s *Service) ReconcilePaymentOrderClaim(ctx context.Context, credential paymentconfiguration.StoredStripeConfiguration, apiKey string, credentialErr error, purchasesEnabled bool, policyErr error, claim Claim, now time.Time) (Outcome, error) {
	return s.reconcilePaymentOrderClaim(ctx, credential, apiKey, credentialErr, purchasesEnabled, policyErr, claim, now)
}

func (s *Service) reconcilePaymentOrderClaim(ctx context.Context, credential paymentconfiguration.StoredStripeConfiguration, apiKey string, credentialErr error, purchasesEnabled bool, policyErr error, claim Claim, now time.Time) (Outcome, error) {
	order := claim.Order
	if err := reconciliationCredentialError(order, credential, credentialErr); err != nil {
		return failureOutcome(now, order, err, false), err
	}
	if order.Status == paymentsdomain.PaymentOrderCreating && policyErr != nil {
		return failureOutcome(now, order, policyErr, false), policyErr
	}
	switch order.Status {
	case paymentsdomain.PaymentOrderCreating:
		return s.reconcileCreating(ctx, credential, apiKey, purchasesEnabled, order, now)
	case paymentsdomain.PaymentOrderPaidPendingCredit:
		if _, err := s.orderLifecycle.CreditPaidOrder(ctx, order, now); err != nil {
			return failureOutcome(now, order, err, true), err
		}
		return terminal(now), nil
	default:
		return s.reconcileBound(ctx, apiKey, order, now)
	}
}
