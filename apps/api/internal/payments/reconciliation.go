package payments

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	paymentReconciliationInterval      = 5 * time.Minute
	paymentReconciliationOperableDelay = time.Hour
	paymentReconciliationAnomalyAge    = 24 * time.Hour
	paymentReconciliationLease         = 5 * time.Minute
)

func PaymentReconciliationInterval() time.Duration { return paymentReconciliationInterval }

// ReconcileDuePaymentOrders is the durable recovery seam used by River. It only
// advances an order from canonical Stripe facts, never from redirects or job
// delivery. Claims are fenced in the repository before provider I/O begins.
func (s *StripeFactsWorkflow) ReconcileDuePaymentOrders(ctx context.Context) error {
	if s == nil || s.configuration == nil || s.checkoutStore == nil || s.convergenceStore == nil || s.orderLifecycle == nil || s.checkoutGateway == nil {
		return fmt.Errorf("payments reconciliation unavailable")
	}
	now := s.runtime.now().UTC()
	claims, err := s.convergenceStore.ClaimDuePaymentOrderReconciliations(ctx, now)
	if err != nil {
		return err
	}
	activeConfiguration, activeConfigurationErr := s.configuration.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(activeConfigurationErr) {
		activeConfigurationErr = ErrStripeConfigurationNotFound
	}
	var retryErr error
	for _, claim := range claims {
		credential, apiKey, credentialErr := s.stripeCredentialAccess(ctx, claim.Order.StripeCredentialID)
		outcome, processErr := s.reconcilePaymentOrderClaim(
			ctx,
			credential,
			apiKey,
			credentialErr,
			activeConfigurationErr == nil && activeConfiguration.PurchasesEnabled,
			activeConfigurationErr,
			claim,
			now,
		)
		completed, completeErr := s.convergenceStore.CompletePaymentOrderReconciliationClaim(ctx, claim, outcome)
		if completeErr != nil {
			retryErr = errors.Join(retryErr, completeErr)
			continue
		}
		if !completed {
			continue
		}
		if processErr != nil && reconciliationFailureIsTransient(processErr) {
			retryErr = errors.Join(retryErr, processErr)
		}
	}
	refundErr := s.reconcileDuePaymentRefunds(ctx, now)
	if refundErr != nil {
		retryErr = errors.Join(retryErr, refundErr)
	}
	disputeErr := s.reconcileDuePaymentDisputes(ctx, now)
	if disputeErr != nil {
		retryErr = errors.Join(retryErr, disputeErr)
	}
	return retryErr
}

func (s *StripeFactsWorkflow) reconcileDuePaymentDisputes(ctx context.Context, now time.Time) error {
	if s == nil || s.convergenceStore == nil || s.disputeGateway == nil {
		return nil
	}
	disputes, err := s.convergenceStore.ListDuePaymentDisputes(ctx, now, 100)
	if err != nil {
		return err
	}
	var retryErr error
	for _, dispute := range disputes {
		_, apiKey, configurationErr := s.stripeOrderAccess(ctx, dispute.PaymentOrderID)
		if configurationErr != nil {
			retryErr = errors.Join(retryErr, configurationErr)
			continue
		}
		provider, retrieveErr := s.disputeGateway.RetrieveDispute(ctx, apiKey, dispute.StripeDisputeID)
		if retrieveErr != nil {
			retryErr = errors.Join(retryErr, retrieveErr)
			continue
		}
		updated, changed, convergeErr := s.convergenceStore.ConvergePaymentDispute(ctx, provider, now)
		if convergeErr != nil {
			retryErr = errors.Join(retryErr, convergeErr)
			continue
		}
		if changed {
			s.publishDisputeUpdate(updated.UserID)
		}
		if expireErr := s.expireDisputeCheckoutSessions(ctx, updated); expireErr != nil {
			retryErr = errors.Join(retryErr, expireErr)
		}
	}
	return retryErr
}

func (s *StripeFactsWorkflow) reconcileDuePaymentRefunds(ctx context.Context, now time.Time) error {
	if s == nil || s.convergenceStore == nil || s.refundGateway == nil {
		return nil
	}
	refunds, err := s.convergenceStore.ListPendingPaymentRefunds(ctx, now.Add(-2*time.Minute), 100)
	if err != nil {
		return err
	}
	var retryErr error
	for _, refund := range refunds {
		_, apiKey, configurationErr := s.stripeOrderAccess(ctx, refund.PaymentOrderID)
		if configurationErr != nil {
			retryErr = errors.Join(retryErr, configurationErr)
			continue
		}
		var providerRefund StripeRefund
		if refund.StripeRefundID == "" {
			providerRefund, err = s.refundGateway.CreateRefund(ctx, CreateStripeRefundInput{
				APIKey: apiKey, ChargeID: refund.StripeChargeID, AmountMinor: refund.AmountMinor,
				Currency: refund.Currency, IdempotencyKey: "ideaboard-refund-" + refund.ID,
			})
			if err != nil {
				if reconciliationFailureIsTransient(err) {
					retryErr = errors.Join(retryErr, err)
					continue
				}
				if _, markErr := s.convergenceStore.MarkPaymentRefundFailed(ctx, refund.ID, operationalErrorSummary(err, "Stripe refund request failed"), now); markErr != nil {
					retryErr = errors.Join(retryErr, markErr)
				}
				continue
			}
		} else {
			providerRefund, err = s.refundGateway.RetrieveRefund(ctx, apiKey, refund.StripeRefundID)
			if err != nil {
				retryErr = errors.Join(retryErr, err)
				continue
			}
		}
		if _, err := s.convergePaymentRefund(ctx, refund, providerRefund); err != nil {
			retryErr = errors.Join(retryErr, err)
		}
	}
	return retryErr
}

func (s *StripeFactsWorkflow) reconcilePaymentOrderClaim(
	ctx context.Context,
	credential StoredStripeConfiguration,
	apiKey string,
	credentialErr error,
	purchasesEnabled bool,
	purchasePolicyErr error,
	claim PaymentReconciliationClaim,
	now time.Time,
) (PaymentReconciliationOutcome, error) {
	order := claim.Order
	if err := reconciliationCredentialError(order, credential, credentialErr); err != nil {
		return paymentReconciliationFailureOutcome(now, order, err, false), err
	}
	if order.Status == PaymentOrderCreating && purchasePolicyErr != nil {
		return paymentReconciliationFailureOutcome(now, order, purchasePolicyErr, false), purchasePolicyErr
	}
	switch order.Status {
	case PaymentOrderCreating:
		return s.reconcileCreatingPaymentOrder(ctx, credential, apiKey, purchasesEnabled, order, now)
	case PaymentOrderPaidPendingCredit:
		return s.reconcilePaidPendingCredit(ctx, order, now)
	default:
		return s.reconcileBoundPaymentOrder(ctx, apiKey, order, now)
	}
}

func reconciliationCredentialError(order PaymentOrder, credential StoredStripeConfiguration, credentialErr error) error {
	if credentialErr != nil {
		return credentialErr
	}
	if credential.AccountID != order.StripeAccountID {
		return ErrPurchasesUnavailable
	}
	return nil
}

func (s *StripeFactsWorkflow) reconcileCreatingPaymentOrder(
	ctx context.Context,
	credential StoredStripeConfiguration,
	apiKey string,
	purchasesEnabled bool,
	order PaymentOrder,
	now time.Time,
) (PaymentReconciliationOutcome, error) {
	if !purchasesEnabled {
		if _, err := s.orderLifecycle.expireOrder(ctx, order); err != nil {
			return paymentReconciliationFailureOutcome(now, order, err, true), err
		}
		return paymentReconciliationTerminal(now), nil
	}
	// A local creation deadline is not proof that Stripe did not accept the
	// initial request. Replay the public order idempotency key first.
	if _, err := s.checkoutRecovery.createCheckoutSessionForOrder(ctx, credential, apiKey, order); err != nil {
		return paymentReconciliationFailureOutcome(now, order, err, reconciliationFailureIsTransient(err)), err
	}
	return paymentReconciliationCheckpoint(now, order), nil
}

func (s *StripeFactsWorkflow) reconcilePaidPendingCredit(
	ctx context.Context,
	order PaymentOrder,
	now time.Time,
) (PaymentReconciliationOutcome, error) {
	if _, err := s.orderLifecycle.creditPaidOrder(ctx, order, now); err != nil {
		return paymentReconciliationFailureOutcome(now, order, err, true), err
	}
	return paymentReconciliationTerminal(now), nil
}

func (s *StripeFactsWorkflow) reconcileBoundPaymentOrder(
	ctx context.Context,
	apiKey string,
	order PaymentOrder,
	now time.Time,
) (PaymentReconciliationOutcome, error) {
	if order.StripeCheckoutSessionID == "" {
		err := ErrPurchasesUnavailable
		return paymentReconciliationFailureOutcome(now, order, err, false), err
	}
	resolved, err := s.checkoutRecovery.retrieveAndConvergeCheckout(ctx, apiKey, order)
	if err != nil {
		return paymentReconciliationFailureOutcome(now, order, err, reconciliationFailureIsTransient(err)), err
	}
	if resolved.Status == PaymentOrderCheckoutOpen && !resolved.ExpiresAt.After(now) {
		expired, err := s.orderLifecycle.expireOrder(ctx, resolved)
		if err != nil {
			return paymentReconciliationFailureOutcome(now, order, err, true), err
		}
		resolved = expired
	}
	if resolved.Status == PaymentOrderCredited || resolved.Status == PaymentOrderCancelled || resolved.Status == PaymentOrderExpired || resolved.Status == PaymentOrderFailed {
		return paymentReconciliationTerminal(now), nil
	}
	return paymentReconciliationCheckpoint(now, resolved), nil
}

func paymentReconciliationTerminal(now time.Time) PaymentReconciliationOutcome {
	return PaymentReconciliationOutcome{CheckedAt: now, NextAttemptAt: now.Add(paymentReconciliationInterval), ClearAnomaly: true}
}

func paymentReconciliationCheckpoint(now time.Time, order PaymentOrder) PaymentReconciliationOutcome {
	return PaymentReconciliationOutcome{
		CheckedAt: now, NextAttemptAt: now.Add(paymentReconciliationInterval),
		Anomalous: !order.CreatedAt.After(now.Add(-paymentReconciliationAnomalyAge)),
	}
}

func paymentReconciliationFailureOutcome(now time.Time, order PaymentOrder, err error, transient bool) PaymentReconciliationOutcome {
	code, summary := reconciliationFailureDetail(err)
	delay := paymentReconciliationOperableDelay
	if transient {
		// Releasing the claim immediately lets River's own retry/backoff perform
		// the next attempt. The periodic scan remains the recovery fallback.
		delay = 0
	}
	return PaymentReconciliationOutcome{CheckedAt: now, NextAttemptAt: now.Add(delay), ErrorCode: code, ErrorSummary: summary, Anomalous: !order.CreatedAt.After(now.Add(-paymentReconciliationAnomalyAge))}
}

func reconciliationFailureIsTransient(err error) bool {
	var gatewayErr *GatewayError
	if errors.As(err, &gatewayErr) {
		return gatewayErr.Code() == GatewayErrorProviderUnavailable
	}
	if errors.Is(err, ErrStripeProviderUnavailable) {
		return true
	}
	if errors.Is(err, ErrPurchasesUnavailable) || errors.Is(err, ErrCheckoutIdempotencyConflict) ||
		errors.Is(err, ErrInvalidIdeaQuantity) || errors.Is(err, ErrWalletNotRechargeable) ||
		errors.Is(err, ErrStripeConfigurationInvalid) || errors.Is(err, ErrStripeConfigurationNotFound) ||
		errors.Is(err, ErrInvalidStripeAPIKey) || errors.Is(err, ErrStripePermissionDenied) ||
		errors.Is(err, ErrStripeAccountUnavailable) || errors.Is(err, ErrStripeCapabilityUnavailable) {
		return false
	}
	// Credit command and database failures are retriable: CreditPaidPaymentOrder
	// preserves paid_pending_credit until its transaction can commit atomically.
	return true
}

func reconciliationFailureDetail(err error) (string, string) {
	var gatewayErr *GatewayError
	if errors.As(err, &gatewayErr) {
		return string(gatewayErr.Code()), boundedText(gatewayErr.Error(), 500)
	}
	if errors.Is(err, ErrStripeProviderUnavailable) {
		return string(GatewayErrorProviderUnavailable), "Stripe 服务暂时不可用"
	}
	if errors.Is(err, ErrStripeConfigurationInvalid) {
		return "configuration_invalid", "当前 Stripe 配置无效"
	}
	if errors.Is(err, ErrStripeConfigurationNotFound) {
		return "configuration_not_found", "当前 Stripe 配置尚未完成"
	}
	if errors.Is(err, ErrPurchasesUnavailable) {
		return "configuration_unavailable", "当前 Stripe 配置无法收敛该订单"
	}
	if errors.Is(err, ErrInvalidStripeAPIKey) {
		return string(GatewayErrorInvalidKey), "Stripe API Key 无效"
	}
	if errors.Is(err, ErrStripePermissionDenied) {
		return string(GatewayErrorPermissionDenied), "Stripe API Key 权限不足"
	}
	if errors.Is(err, ErrStripeAccountUnavailable) {
		return string(GatewayErrorAccountUnavailable), "Stripe 账户不可用"
	}
	if errors.Is(err, ErrStripeCapabilityUnavailable) {
		return string(GatewayErrorCapabilityMissing), "Stripe 账户暂不具备收款能力"
	}
	if errors.Is(err, ErrWalletNotRechargeable) {
		return "wallet_not_rechargeable", "当前账户暂不可充值"
	}
	if errors.Is(err, ErrCheckoutIdempotencyConflict) {
		return "identity_conflict", "Stripe Checkout Session 与订单绑定不一致"
	}
	return "ledger_retry", "账本写入暂时失败"
}

// ListPaymentReconciliationAnomalies is the internal Admin read seam. The HTTP
// presentation remains a separate Admin vertical concern.
func (s *StripeFactsWorkflow) ListPaymentReconciliationAnomalies(ctx context.Context, limit int32) ([]PaymentReconciliationAnomaly, error) {
	if s == nil || s.convergenceStore == nil {
		return nil, fmt.Errorf("payments reconciliation unavailable")
	}
	return s.convergenceStore.ListPaymentReconciliationAnomalies(ctx, limit)
}
