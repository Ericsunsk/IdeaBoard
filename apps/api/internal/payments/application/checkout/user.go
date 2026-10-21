package checkout

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/database"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/payments/ports"
)

// paymentOrderReturnStatusWords are the user-facing return-status words
// for PaymentOrderReturnStatus.Status. The wire words are unchanged
// ("processing","credited","failed"); the constants only kill the raw
// literals. The DTO field stays string with conversion at this boundary.
const (
	paymentOrderReturnProcessing = "processing"
	paymentOrderReturnCredited   = "credited"
	paymentOrderReturnFailed     = "failed"
)

// ListPendingOrders returns the payer account's unsettled checkout intents.
func (s *Service) ListPendingOrders(ctx context.Context, billingAccountID int64) (paymentcontracts.PendingPaymentOrderList, error) {
	result := paymentcontracts.PendingPaymentOrderList{Items: make([]paymentcontracts.PendingPaymentOrder, 0)}
	if s == nil || s.store == nil || billingAccountID <= 0 {
		return result, paymentcontracts.ErrPaymentOrderNotFound
	}
	orders, err := s.store.ListAccountPendingPaymentOrders(ctx, billingAccountID)
	if err != nil {
		return result, err
	}
	for _, order := range orders {
		result.Items = append(result.Items, pendingPaymentOrder(order))
	}
	return result, nil
}

func (s *Service) ContinuePendingOrder(ctx context.Context, billingAccountID int64, publicToken string) (paymentcontracts.PendingPaymentOrder, error) {
	if s == nil || s.configuration == nil || s.store == nil || billingAccountID <= 0 {
		return paymentcontracts.PendingPaymentOrder{}, paymentcontracts.ErrPaymentOrderNotFound
	}
	var result paymentcontracts.PendingPaymentOrder
	err := s.store.WithBillingAccountCheckoutLock(ctx, billingAccountID, func(locked context.Context) error {
		var err error
		result, err = s.continuePendingOrderLocked(locked, billingAccountID, publicToken)
		return err
	})
	return result, err
}

func (s *Service) continuePendingOrderLocked(ctx context.Context, billingAccountID int64, publicToken string) (paymentcontracts.PendingPaymentOrder, error) {
	var result paymentcontracts.PendingPaymentOrder
	order, err := s.store.GetPaymentOrderByAccountToken(ctx, billingAccountID, publicToken)
	if database.IsNotFound(err) {
		return result, paymentcontracts.ErrPaymentOrderNotFound
	}
	if err != nil {
		return result, fmt.Errorf("get pending payment order to continue: %w", err)
	}
	if order.Status == paymentsdomain.PaymentOrderPaidPendingCredit {
		order, err = s.lifecycle.CreditPaidOrder(ctx, order, s.now().UTC())
		if err != nil {
			return result, err
		}
		result = pendingPaymentOrder(order)
		return result, nil
	}
	if order.Status != paymentsdomain.PaymentOrderCheckoutOpen {
		result = pendingPaymentOrder(order)
		return result, nil
	}
	configuration, apiKey, err := s.CredentialForOrder(ctx, order)
	if err != nil || configuration.AccountID != order.StripeAccountID {
		return result, paymentsdomain.ErrPurchasesUnavailable
	}
	if err := s.requireWallet(ctx, billingAccountID); err != nil {
		return result, err
	}
	session, err := s.gateway.RetrieveCheckoutSession(ctx, ports.RetrieveCheckoutSessionInput{APIKey: apiKey, SessionID: order.StripeCheckoutSessionID})
	if err != nil {
		return result, OperationError(err)
	}
	order, err = s.lifecycle.ConvergeCheckoutSession(ctx, order, session, paymentorder.SignalObserved)
	if err != nil {
		return result, err
	}
	if order.Status == paymentsdomain.PaymentOrderCheckoutOpen && !order.ExpiresAt.After(s.now().UTC()) {
		order, err = s.lifecycle.ExpireOrder(ctx, order)
		if err != nil {
			return result, err
		}
	}
	result = pendingPaymentOrder(order)
	return result, nil
}

func (s *Service) AbandonPendingOrder(ctx context.Context, billingAccountID int64, publicToken string) (paymentcontracts.PendingPaymentOrder, error) {
	if s == nil || s.configuration == nil || s.store == nil || billingAccountID <= 0 {
		return paymentcontracts.PendingPaymentOrder{}, paymentcontracts.ErrPaymentOrderNotFound
	}
	var result paymentcontracts.PendingPaymentOrder
	err := s.store.WithBillingAccountCheckoutLock(ctx, billingAccountID, func(locked context.Context) error {
		order, err := s.store.GetPaymentOrderByAccountToken(locked, billingAccountID, publicToken)
		if database.IsNotFound(err) {
			return paymentcontracts.ErrPaymentOrderNotFound
		}
		if err != nil {
			return fmt.Errorf("get pending payment order to abandon: %w", err)
		}
		if order.Status != paymentsdomain.PaymentOrderCheckoutOpen {
			result = pendingPaymentOrder(order)
			return nil
		}
		configuration, apiKey, err := s.CredentialForOrder(locked, order)
		if err != nil || configuration.AccountID != order.StripeAccountID {
			return paymentsdomain.ErrPurchasesUnavailable
		}
		expiration, err := s.gateway.ExpireCheckoutSession(locked, ports.ExpireCheckoutSessionInput{APIKey: apiKey, SessionID: order.StripeCheckoutSessionID})
		if err != nil {
			return OperationError(err)
		}
		order, err = s.lifecycle.ConvergeCheckoutExpiration(locked, order, expiration)
		if err != nil {
			return err
		}
		result = pendingPaymentOrder(order)
		return nil
	})
	return result, err
}

func pendingPaymentOrder(order paymentsdomain.PaymentOrder) paymentcontracts.PendingPaymentOrder {
	checkoutURL := ""
	if order.Status == paymentsdomain.PaymentOrderCheckoutOpen {
		checkoutURL = order.CheckoutURL
	}
	return paymentcontracts.PendingPaymentOrder{PublicToken: order.PublicToken, IdeaQuantity: order.IdeaQuantity, CheckoutURL: checkoutURL, Status: string(order.Status), ExpiresAt: order.ExpiresAt, UpdatedAt: order.UpdatedAt}
}

func (s *Service) GetPurchaseOptions(ctx context.Context, billingAccountID int64) (paymentcontracts.PurchaseOptions, error) {
	options := paymentcontracts.PurchaseOptions{
		QuickIdeaQuantities: append([]int64(nil), paymentsdomain.QuickPurchaseIdeaQuantities[:]...),
		MinIdeaQuantity:     paymentsdomain.MinimumPurchaseIdeas, MaxIdeaQuantity: paymentsdomain.MaximumPurchaseIdeas, IdeasPerUSD: paymentsdomain.IdeasPerUSD,
		Terms:        paymentcontracts.PaymentLegalDocument{Version: paymentsdomain.CurrentTermsVersion, URL: "/terms"},
		RefundPolicy: paymentcontracts.PaymentLegalDocument{Version: paymentsdomain.CurrentRefundPolicyVersion, URL: "/refund-policy"},
	}
	if s == nil || s.configuration == nil || s.wallets == nil || billingAccountID <= 0 {
		options.UnavailableReason = "payments_not_configured"
		return options, nil
	}
	configuration, err := s.configuration.GetStripeConfiguration(ctx)
	if paymentconfiguration.IsNotFound(err) {
		options.UnavailableReason = "payments_not_configured"
		return options, nil
	}
	if err != nil {
		return paymentcontracts.PurchaseOptions{}, fmt.Errorf("get purchase configuration: %w", err)
	}
	if !configuration.PurchasesEnabled || !configuration.ChargesEnabled || configuration.StripeProductID == "" || configuration.PaymentMethodConfigurationID == "" {
		options.UnavailableReason = "purchases_disabled"
		return options, nil
	}
	wallet, err := s.wallets.GetWallet(ctx, billingAccountID)
	if err != nil {
		if errors.Is(err, domain.ErrWalletNotFound) {
			options.UnavailableReason = "wallet_restricted"
			return options, nil
		}
		return paymentcontracts.PurchaseOptions{}, fmt.Errorf("get Wallet purchase eligibility: %w", err)
	}
	if wallet.Status != domain.WalletStatusActive || len(wallet.ActiveRestrictions) > 0 {
		options.UnavailableReason = "wallet_restricted"
		for _, restriction := range wallet.ActiveRestrictions {
			if restriction.Type == "payment_dispute" {
				options.RestrictionType = restriction.Type
				break
			}
		}
		return options, nil
	}
	options.PurchasesAvailable = true
	return options, nil
}

func (s *Service) CreateCheckout(ctx context.Context, command paymentsdomain.CreateCheckoutCommand) (paymentcontracts.CheckoutResult, error) {
	if s == nil || s.configuration == nil || s.store == nil {
		return paymentcontracts.CheckoutResult{}, paymentsdomain.ErrPurchasesUnavailable
	}
	var result paymentcontracts.CheckoutResult
	err := s.store.WithBillingAccountCheckoutLock(ctx, command.BillingAccountID, func(locked context.Context) error {
		request, err := s.PrepareRequest(locked, command)
		if err != nil {
			return err
		}
		resolution, err := s.ResolveOrder(locked, command, request, func(c context.Context, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error) {
			_, key, err := s.CredentialForOrder(c, order)
			if err != nil {
				return order, err
			}
			return s.RetrieveAndConverge(c, key, order)
		})
		if err != nil {
			return err
		}
		order, reused := resolution.Order, resolution.Reused
		if order.Status != paymentsdomain.PaymentOrderCreating {
			result, err = finalizeExistingCheckoutResult(order)
			return err
		}
		order, err = s.CreateSessionForOrder(locked, request.Configuration, request.APIKey, order)
		if err != nil {
			return err
		}
		result, err = finalizeCreatedCheckoutResult(order, reused)
		return err
	})
	return result, err
}

func finalizeExistingCheckoutResult(order paymentsdomain.PaymentOrder) (paymentcontracts.CheckoutResult, error) {
	if order.Status == paymentsdomain.PaymentOrderAsyncPaymentPending {
		return paymentcontracts.CheckoutResult{}, paymentcontracts.ErrPaymentAlreadyInProgress
	}
	if order.Status != paymentsdomain.PaymentOrderCheckoutOpen || order.CheckoutURL == "" {
		return paymentcontracts.CheckoutResult{}, paymentsdomain.ErrPurchasesUnavailable
	}
	return checkoutResult(order, true), nil
}

func finalizeCreatedCheckoutResult(order paymentsdomain.PaymentOrder, reused bool) (paymentcontracts.CheckoutResult, error) {
	switch order.Status {
	case paymentsdomain.PaymentOrderCheckoutOpen:
		return checkoutResult(order, reused), nil
	case paymentsdomain.PaymentOrderAsyncPaymentPending:
		return paymentcontracts.CheckoutResult{}, paymentcontracts.ErrPaymentAlreadyInProgress
	case paymentsdomain.PaymentOrderPaidPendingCredit, paymentsdomain.PaymentOrderCredited:
		return paymentcontracts.CheckoutResult{}, paymentcontracts.ErrPaymentAlreadyCompleted
	case paymentsdomain.PaymentOrderExpired:
		return paymentcontracts.CheckoutResult{}, paymentcontracts.ErrCheckoutSessionExpired
	default:
		return paymentcontracts.CheckoutResult{}, paymentsdomain.ErrPurchasesUnavailable
	}
}

func checkoutResult(order paymentsdomain.PaymentOrder, reused bool) paymentcontracts.CheckoutResult {
	return paymentcontracts.CheckoutResult{CheckoutURL: order.CheckoutURL, PublicToken: order.PublicToken, ExpiresAt: order.ExpiresAt, Reused: reused}
}

func (s *Service) GetPaymentOrderReturnStatus(ctx context.Context, billingAccountID int64, publicToken string) (paymentcontracts.PaymentOrderReturnStatus, error) {
	if s == nil || s.configuration == nil || s.store == nil {
		return paymentcontracts.PaymentOrderReturnStatus{}, paymentcontracts.ErrPaymentOrderNotFound
	}
	var result paymentcontracts.PaymentOrderReturnStatus
	err := s.store.WithBillingAccountCheckoutLock(ctx, billingAccountID, func(locked context.Context) error {
		order, err := s.store.GetPaymentOrderByAccountToken(locked, billingAccountID, publicToken)
		if database.IsNotFound(err) {
			return paymentcontracts.ErrPaymentOrderNotFound
		}
		if err != nil {
			return fmt.Errorf("get payment order return status: %w", err)
		}
		if order.Status != paymentsdomain.PaymentOrderCredited && order.StripeCheckoutSessionID != "" {
			configuration, apiKey, configErr := s.CredentialForOrder(locked, order)
			if configErr != nil {
				return configErr
			}
			if configuration.AccountID == order.StripeAccountID {
				session, retrieveErr := s.gateway.RetrieveCheckoutSession(locked, ports.RetrieveCheckoutSessionInput{APIKey: apiKey, SessionID: order.StripeCheckoutSessionID})
				if retrieveErr != nil {
					return OperationError(retrieveErr)
				}
				order, err = s.lifecycle.ConvergeCheckoutSession(locked, order, session, paymentorder.SignalObserved)
				if err != nil {
					return err
				}
			}
		}
		result = paymentOrderReturnStatus(order)
		return nil
	})
	return result, err
}

func paymentOrderReturnStatus(order paymentsdomain.PaymentOrder) paymentcontracts.PaymentOrderReturnStatus {
	status := paymentOrderReturnProcessing
	switch order.Status {
	case paymentsdomain.PaymentOrderCredited:
		status = paymentOrderReturnCredited
	case paymentsdomain.PaymentOrderCancelled, paymentsdomain.PaymentOrderExpired, paymentsdomain.PaymentOrderFailed:
		status = paymentOrderReturnFailed
	}
	return paymentcontracts.PaymentOrderReturnStatus{IdeaQuantity: order.IdeaQuantity, Status: status, UpdatedAt: order.UpdatedAt}
}

func (s *Service) CredentialForOrder(ctx context.Context, order paymentsdomain.PaymentOrder) (paymentconfiguration.StoredStripeConfiguration, string, error) {
	configuration, apiKey, err := s.resolver.ForCredentialID(ctx, order.StripeCredentialID)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, "", paymentsdomain.ErrPurchasesUnavailable
	}
	return configuration, apiKey, nil
}
