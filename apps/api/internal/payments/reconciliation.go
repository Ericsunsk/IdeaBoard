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

// ReconcileDuePaymentOrders is the durable recovery seam used by River. It only
// advances an order from canonical Stripe facts, never from redirects or job
// delivery. Claims are fenced in the repository before provider I/O begins.
func (s *Service) ReconcileDuePaymentOrders(ctx context.Context) error {
	if s == nil || s.repo == nil || s.checkoutGateway == nil {
		return fmt.Errorf("payments reconciliation unavailable")
	}
	return s.repo.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		configuration, apiKey, configurationErr := s.stripeAccessConfiguration(lockedContext)
		now := s.now().UTC()
		claims, err := s.repo.ClaimDuePaymentOrderReconciliations(lockedContext, now)
		if err != nil {
			return err
		}
		var retryErr error
		for _, claim := range claims {
			outcome, processErr := s.reconcilePaymentOrderClaim(lockedContext, configuration, apiKey, configurationErr, claim, now)
			completed, completeErr := s.repo.CompletePaymentOrderReconciliationClaim(lockedContext, claim, outcome)
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
		refundErr := s.reconcileDuePaymentRefunds(lockedContext, apiKey, configurationErr, now)
		if refundErr != nil {
			retryErr = errors.Join(retryErr, refundErr)
		}
		disputeErr := s.reconcileDuePaymentDisputes(lockedContext, apiKey, configurationErr, now)
		if disputeErr != nil {
			retryErr = errors.Join(retryErr, disputeErr)
		}
		return retryErr
	})
}

func (s *Service) reconcileDuePaymentDisputes(ctx context.Context, apiKey string, configurationErr error, now time.Time) error {
	if s == nil || s.repo == nil || s.disputeGateway == nil {
		return nil
	}
	disputes, err := s.repo.ListDuePaymentDisputes(ctx, now, 100)
	if err != nil {
		return err
	}
	var retryErr error
	for _, dispute := range disputes {
		if configurationErr != nil {
			retryErr = errors.Join(retryErr, configurationErr)
			continue
		}
		provider, retrieveErr := s.disputeGateway.RetrieveDispute(ctx, apiKey, dispute.StripeDisputeID)
		if retrieveErr != nil {
			retryErr = errors.Join(retryErr, retrieveErr)
			continue
		}
		updated, changed, convergeErr := s.repo.ConvergePaymentDispute(ctx, provider, now)
		if convergeErr != nil {
			retryErr = errors.Join(retryErr, convergeErr)
			continue
		}
		if changed {
			s.publishDisputeUpdate(updated.UserID)
		}
		if expireErr := s.expireDisputeCheckoutSessions(ctx, apiKey, updated); expireErr != nil {
			retryErr = errors.Join(retryErr, expireErr)
		}
	}
	return retryErr
}

func (s *Service) reconcileDuePaymentRefunds(ctx context.Context, apiKey string, configurationErr error, now time.Time) error {
	if s == nil || s.repo == nil || s.refundGateway == nil {
		return nil
	}
	refunds, err := s.repo.ListPendingPaymentRefunds(ctx, now.Add(-2*time.Minute), 100)
	if err != nil {
		return err
	}
	var retryErr error
	for _, refund := range refunds {
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
				if _, markErr := s.repo.MarkPaymentRefundFailed(ctx, refund.ID, operationalErrorSummary(err, "Stripe refund request failed"), now); markErr != nil {
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

func (s *Service) reconcilePaymentOrderClaim(ctx context.Context, configuration StoredStripeConfiguration, apiKey string, configurationErr error, claim PaymentReconciliationClaim, now time.Time) (PaymentReconciliationOutcome, error) {
	order := claim.Order
	if configurationErr != nil || configuration.AccountID != order.StripeAccountID {
		err := ErrPurchasesUnavailable
		if configurationErr != nil {
			err = configurationErr
		}
		return paymentReconciliationFailureOutcome(now, order, err, false), err
	}
	if order.Status == PaymentOrderCreating && !configuration.PurchasesEnabled {
		expired, changed, err := s.repo.MarkPaymentOrderExpired(ctx, order.ID)
		if err != nil {
			return paymentReconciliationFailureOutcome(now, order, err, true), err
		}
		if changed {
			s.events.Publish(expired.UserID, PaymentEvent{PublicToken: expired.PublicToken, Status: "failed"})
		}
		return paymentReconciliationTerminal(now), nil
	}
	if order.Status == PaymentOrderPaidPendingCredit {
		result, err := s.repo.CreditPaidPaymentOrder(ctx, order.ID, now)
		if err != nil {
			return paymentReconciliationFailureOutcome(now, order, err, true), err
		}
		if !result.Replayed {
			s.events.Publish(result.Order.UserID, PaymentEvent{PublicToken: result.Order.PublicToken, Status: "credited"})
		}
		return paymentReconciliationTerminal(now), nil
	}
	if order.Status == PaymentOrderCreating {
		// A local creation deadline is not proof that Stripe did not accept the
		// initial request. Replay the public order idempotency key first.
		if _, err := s.createCheckoutSessionForOrder(ctx, apiKey, order); err != nil {
			return paymentReconciliationFailureOutcome(now, order, err, reconciliationFailureIsTransient(err)), err
		}
		return paymentReconciliationCheckpoint(now, order), nil
	}
	if order.StripeCheckoutSessionID == "" {
		err := ErrPurchasesUnavailable
		return paymentReconciliationFailureOutcome(now, order, err, false), err
	}
	resolved, err := s.retrieveAndConvergeCheckout(ctx, apiKey, order)
	if err != nil {
		return paymentReconciliationFailureOutcome(now, order, err, reconciliationFailureIsTransient(err)), err
	}
	if resolved.Status == PaymentOrderCheckoutOpen && !resolved.ExpiresAt.After(now) {
		expired, changed, err := s.repo.MarkPaymentOrderExpired(ctx, resolved.ID)
		if err != nil {
			return paymentReconciliationFailureOutcome(now, order, err, true), err
		}
		if changed {
			s.events.Publish(expired.UserID, PaymentEvent{PublicToken: expired.PublicToken, Status: "failed"})
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
func (s *Service) ListPaymentReconciliationAnomalies(ctx context.Context, limit int32) ([]PaymentReconciliationAnomaly, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("payments reconciliation unavailable")
	}
	return s.repo.ListPaymentReconciliationAnomalies(ctx, limit)
}
