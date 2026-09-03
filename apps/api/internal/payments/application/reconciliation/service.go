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
	GetStripeCredential(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
	GetStripeCredentialForOrder(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
}

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
	publish          func(int64, string)
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
	Publish          func(int64, string)
	Now              func() time.Time
}

func New(deps Dependencies) (*Service, error) {
	if deps.Configuration == nil || deps.Convergence == nil || deps.CheckoutRecovery == nil || deps.CheckoutStore == nil || deps.CheckoutGateway == nil || deps.RefundGateway == nil || deps.DisputeGateway == nil || deps.OrderLifecycle == nil || deps.CredentialAccess == nil || deps.OrderAccess == nil || deps.Now == nil {
		return nil, fmt.Errorf("reconciliation service requires complete dependencies")
	}
	return &Service{configuration: deps.Configuration, convergence: deps.Convergence, checkoutRecovery: deps.CheckoutRecovery, checkoutStore: deps.CheckoutStore, checkoutGateway: deps.CheckoutGateway, refundGateway: deps.RefundGateway, disputeGateway: deps.DisputeGateway, disputeExpiry: deps.DisputeExpiry, orderLifecycle: deps.OrderLifecycle, credentialAccess: deps.CredentialAccess, orderAccess: deps.OrderAccess, publish: deps.Publish, now: deps.Now}, nil
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

func reconciliationCredentialError(order paymentsdomain.PaymentOrder, credential paymentconfiguration.StoredStripeConfiguration, credentialErr error) error {
	if credentialErr != nil {
		return credentialErr
	}
	if credential.AccountID != order.StripeAccountID {
		return paymentsdomain.ErrPurchasesUnavailable
	}
	return nil
}
func terminal(now time.Time) Outcome {
	return Outcome{CheckedAt: now, NextAttemptAt: now.Add(Interval), ClearAnomaly: true}
}
func checkpoint(now time.Time, order paymentsdomain.PaymentOrder) Outcome {
	return Outcome{CheckedAt: now, NextAttemptAt: now.Add(Interval), Anomalous: !order.CreatedAt.After(now.Add(-AnomalyAge))}
}
func failureOutcome(now time.Time, order paymentsdomain.PaymentOrder, err error, transient bool) Outcome {
	code, summary := FailureDetail(err)
	delay := OperableDelay
	if transient {
		delay = 0
	}
	return Outcome{CheckedAt: now, NextAttemptAt: now.Add(delay), ErrorCode: code, ErrorSummary: summary, Anomalous: !order.CreatedAt.After(now.Add(-AnomalyAge))}
}

func (s *Service) reconcileCreating(ctx context.Context, credential paymentconfiguration.StoredStripeConfiguration, apiKey string, enabled bool, order paymentsdomain.PaymentOrder, now time.Time) (Outcome, error) {
	if !enabled {
		if _, err := s.orderLifecycle.ExpireOrder(ctx, order); err != nil {
			return failureOutcome(now, order, err, true), err
		}
		return terminal(now), nil
	}
	if _, err := s.checkoutRecovery.CreateSessionForOrder(ctx, credential, apiKey, order); err != nil {
		return failureOutcome(now, order, err, FailureIsTransient(err)), err
	}
	return checkpoint(now, order), nil
}
func (s *Service) reconcileBound(ctx context.Context, apiKey string, order paymentsdomain.PaymentOrder, now time.Time) (Outcome, error) {
	if order.StripeCheckoutSessionID == "" {
		err := paymentsdomain.ErrPurchasesUnavailable
		return failureOutcome(now, order, err, false), err
	}
	resolved, err := s.checkoutRecovery.RetrieveAndConverge(ctx, apiKey, order)
	if err != nil {
		return failureOutcome(now, order, err, FailureIsTransient(err)), err
	}
	if resolved.Status == paymentsdomain.PaymentOrderCheckoutOpen && !resolved.ExpiresAt.After(now) {
		resolved, err = s.orderLifecycle.ExpireOrder(ctx, resolved)
		if err != nil {
			return failureOutcome(now, order, err, true), err
		}
	}
	switch resolved.Status {
	case paymentsdomain.PaymentOrderCredited, paymentsdomain.PaymentOrderCancelled, paymentsdomain.PaymentOrderExpired, paymentsdomain.PaymentOrderFailed:
		return terminal(now), nil
	}
	return checkpoint(now, resolved), nil
}

func (s *Service) reconcileDuePaymentDisputes(ctx context.Context, now time.Time) error {
	disputes, err := s.convergence.ListDuePaymentDisputes(ctx, now, 100)
	if err != nil {
		return err
	}
	var retryErr error
	for _, dispute := range disputes {
		_, apiKey, accessErr := s.orderAccess(ctx, dispute.PaymentOrderID)
		if accessErr != nil {
			retryErr = errors.Join(retryErr, accessErr)
			continue
		}
		provider, err := s.disputeGateway.RetrieveDispute(ctx, apiKey, dispute.StripeDisputeID)
		if err != nil {
			retryErr = errors.Join(retryErr, err)
			continue
		}
		updated, changed, err := s.convergence.ConvergePaymentDispute(ctx, provider, now)
		if err != nil {
			retryErr = errors.Join(retryErr, err)
			continue
		}
		if changed && s.publish != nil {
			s.publish(updated.UserID, "dispute_updated")
		}
		if s.disputeExpiry != nil {
			if err := s.disputeExpiry.ExpireDisputeCheckoutSessions(ctx, updated); err != nil {
				retryErr = errors.Join(retryErr, err)
			}
		}
	}
	return retryErr
}
func (s *Service) reconcileDuePaymentRefunds(ctx context.Context, now time.Time) error {
	refunds, err := s.convergence.ListPendingPaymentRefunds(ctx, now.Add(-2*time.Minute), 100)
	if err != nil {
		return err
	}
	var retryErr error
	for _, refund := range refunds {
		_, apiKey, accessErr := s.orderAccess(ctx, refund.PaymentOrderID)
		if accessErr != nil {
			retryErr = errors.Join(retryErr, accessErr)
			continue
		}
		var provider paymentports.StripeRefund
		if refund.StripeRefundID == "" {
			provider, err = s.refundGateway.CreateRefund(ctx, paymentports.CreateStripeRefundInput{APIKey: apiKey, ChargeID: refund.StripeChargeID, AmountMinor: refund.AmountMinor, Currency: refund.Currency, IdempotencyKey: "ideaboard-refund-" + refund.ID})
			if err != nil {
				if FailureIsTransient(err) {
					retryErr = errors.Join(retryErr, err)
					continue
				}
				if _, markErr := s.convergence.MarkPaymentRefundFailed(ctx, refund.ID, OperationalErrorSummary(err, "Stripe refund request failed"), now); markErr != nil {
					retryErr = errors.Join(retryErr, markErr)
				}
				continue
			}
		} else {
			provider, err = s.refundGateway.RetrieveRefund(ctx, apiKey, refund.StripeRefundID)
			if err != nil {
				retryErr = errors.Join(retryErr, err)
				continue
			}
		}
		if err := s.convergeRefund(ctx, refund, provider, now); err != nil {
			retryErr = errors.Join(retryErr, err)
		}
	}
	return retryErr
}
func (s *Service) convergeRefund(ctx context.Context, refund paymentsdomain.PaymentRefund, provider paymentports.StripeRefund, now time.Time) error {
	if err := paymentrefundValidate(refund, provider); err != nil {
		return err
	}
	updated, err := s.convergence.MarkPaymentRefundProviderResult(ctx, refund.ID, provider.ID, provider.Status, now)
	if err != nil {
		return err
	}
	switch provider.Status {
	case "succeeded":
		_, err = s.convergence.MarkPaymentRefundSucceeded(ctx, updated.ID, now)
	case "failed", "canceled":
		_, err = s.convergence.MarkPaymentRefundFailed(ctx, updated.ID, "Stripe refund failed", now)
	}
	return err
}
func paymentrefundValidate(refund paymentsdomain.PaymentRefund, provider paymentports.StripeRefund) error {
	if refund.StripeRefundID != "" && refund.StripeRefundID != provider.ID {
		return paymentcontracts.ErrRefundProviderConflict
	}
	if refund.StripeChargeID != provider.ChargeID || refund.AmountMinor != provider.AmountMinor || refund.Currency != provider.Currency {
		return paymentcontracts.ErrRefundProviderConflict
	}
	return nil
}
