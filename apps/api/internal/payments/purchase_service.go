package payments

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/secrets"

	"github.com/google/uuid"
)

const (
	checkoutSessionLifetime        = 30 * time.Minute
	checkoutSessionTransportMargin = time.Minute
)

type checkoutSignal string

const (
	checkoutSignalObserved     checkoutSignal = "observed"
	checkoutSignalCompleted    checkoutSignal = "completed"
	checkoutSignalAsyncSuccess checkoutSignal = "async_succeeded"
	checkoutSignalAsyncFailed  checkoutSignal = "async_failed"
	checkoutSignalExpired      checkoutSignal = "expired"
)

var (
	ErrPaymentOrderNotFound     = errors.New("充值订单不存在")
	ErrPendingCheckoutLimit     = errors.New("最多同时保留 3 笔待支付订单，请先完成或放弃一笔")
	ErrPaymentAlreadyInProgress = errors.New("该数量的支付正在处理中，请等待支付结果")
	ErrPaymentAlreadyCompleted  = errors.New("该充值已完成，请勿重复支付")
	ErrCheckoutSessionExpired   = errors.New("支付页面已过期，请重新发起充值")
)

type PurchaseOptions struct {
	PurchasesAvailable  bool                 `json:"purchases_available"`
	UnavailableReason   string               `json:"unavailable_reason,omitempty"`
	RestrictionType     string               `json:"restriction_type,omitempty"`
	QuickIdeaQuantities []int64              `json:"quick_idea_quantities"`
	MinIdeaQuantity     int64                `json:"min_idea_quantity"`
	MaxIdeaQuantity     int64                `json:"max_idea_quantity"`
	IdeasPerUSD         int64                `json:"ideas_per_usd"`
	Terms               PaymentLegalDocument `json:"terms"`
	RefundPolicy        PaymentLegalDocument `json:"refund_policy"`
}

type PaymentLegalDocument struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type CheckoutResult struct {
	CheckoutURL string    `json:"checkout_url"`
	PublicToken string    `json:"public_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	Reused      bool      `json:"reused"`
}

type PaymentOrderReturnStatus struct {
	IdeaQuantity int64     `json:"idea_quantity"`
	Status       string    `json:"status"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PendingPaymentOrder struct {
	PublicToken  string    `json:"public_token"`
	IdeaQuantity int64     `json:"idea_quantity"`
	CheckoutURL  string    `json:"checkout_url,omitempty"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expires_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PendingPaymentOrderList struct {
	Items []PendingPaymentOrder `json:"items"`
}

func (s *CheckoutWorkflow) ListPendingOrders(ctx context.Context, userID int64) (PendingPaymentOrderList, error) {
	result := PendingPaymentOrderList{Items: make([]PendingPaymentOrder, 0)}
	if s == nil || s.checkoutStore == nil || userID <= 0 {
		return result, ErrPaymentOrderNotFound
	}
	orders, err := s.checkoutStore.ListUserPendingPaymentOrders(ctx, userID)
	if err != nil {
		return result, err
	}
	for _, order := range orders {
		result.Items = append(result.Items, pendingPaymentOrder(order))
	}
	return result, nil
}

func (s *CheckoutWorkflow) ContinuePendingOrder(ctx context.Context, userID int64, publicToken string) (PendingPaymentOrder, error) {
	if s == nil || s.configuration == nil || s.checkoutStore == nil || userID <= 0 {
		return PendingPaymentOrder{}, ErrPaymentOrderNotFound
	}
	var result PendingPaymentOrder
	err := s.checkoutStore.WithUserCheckoutLock(ctx, userID, func(userLockedContext context.Context) error {
		order, err := s.checkoutStore.GetPaymentOrderByPublicToken(userLockedContext, userID, publicToken)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentOrderNotFound
		}
		if err != nil {
			return fmt.Errorf("get pending payment order to continue: %w", err)
		}
		if order.Status == PaymentOrderPaidPendingCredit {
			converged, err := s.orderLifecycle.creditPaidOrder(userLockedContext, order, s.runtime.now().UTC())
			if err != nil {
				return err
			}
			result = pendingPaymentOrder(converged)
			return nil
		}
		if order.Status != PaymentOrderCheckoutOpen {
			result = pendingPaymentOrder(order)
			return nil
		}
		if s.checkoutGateway == nil || s.wallets == nil {
			return ErrPurchasesUnavailable
		}
		configuration, apiKey, err := s.stripeOrderConfiguration(userLockedContext, order)
		if err != nil {
			return err
		}
		if configuration.AccountID != order.StripeAccountID {
			return ErrPurchasesUnavailable
		}
		if err := s.requireRechargeableWallet(userLockedContext, userID); err != nil {
			return err
		}
		session, err := s.checkoutGateway.RetrieveCheckoutSession(userLockedContext, RetrieveCheckoutSessionInput{
			APIKey: apiKey, SessionID: order.StripeCheckoutSessionID,
		})
		if err != nil {
			return checkoutOperationError(err)
		}
		order, err = s.orderLifecycle.convergeCheckoutSession(userLockedContext, order, session, checkoutSignalObserved)
		if err != nil {
			return err
		}
		if order.Status == PaymentOrderCheckoutOpen && !order.ExpiresAt.After(s.runtime.now().UTC()) {
			order, err = s.orderLifecycle.expireOrder(userLockedContext, order)
			if err != nil {
				return err
			}
		}
		result = pendingPaymentOrder(order)
		return nil
	})
	return result, err
}

func (s *CheckoutWorkflow) AbandonPendingOrder(ctx context.Context, userID int64, publicToken string) (PendingPaymentOrder, error) {
	if s == nil || s.configuration == nil || s.checkoutStore == nil || userID <= 0 {
		return PendingPaymentOrder{}, ErrPaymentOrderNotFound
	}
	var result PendingPaymentOrder
	err := s.checkoutStore.WithUserCheckoutLock(ctx, userID, func(userLockedContext context.Context) error {
		order, err := s.checkoutStore.GetPaymentOrderByPublicToken(userLockedContext, userID, publicToken)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentOrderNotFound
		}
		if err != nil {
			return fmt.Errorf("get pending payment order to abandon: %w", err)
		}
		if order.Status != PaymentOrderCheckoutOpen {
			result = pendingPaymentOrder(order)
			return nil
		}
		if s.checkoutGateway == nil {
			return ErrPurchasesUnavailable
		}
		configuration, apiKey, err := s.stripeOrderConfiguration(userLockedContext, order)
		if err != nil || configuration.AccountID != order.StripeAccountID {
			return ErrPurchasesUnavailable
		}
		expiration, err := s.checkoutGateway.ExpireCheckoutSession(userLockedContext, ExpireCheckoutSessionInput{
			APIKey: apiKey, SessionID: order.StripeCheckoutSessionID,
		})
		if err != nil {
			return checkoutOperationError(err)
		}
		order, err = s.orderLifecycle.convergeCheckoutExpiration(userLockedContext, order, expiration)
		if err != nil {
			return err
		}
		result = pendingPaymentOrder(order)
		return nil
	})
	return result, err
}

func pendingPaymentOrder(order PaymentOrder) PendingPaymentOrder {
	checkoutURL := ""
	if order.Status == PaymentOrderCheckoutOpen {
		checkoutURL = order.CheckoutURL
	}
	return PendingPaymentOrder{
		PublicToken: order.PublicToken, IdeaQuantity: order.IdeaQuantity, CheckoutURL: checkoutURL,
		Status: string(order.Status), ExpiresAt: order.ExpiresAt, UpdatedAt: order.UpdatedAt,
	}
}

func (s *CheckoutWorkflow) GetPurchaseOptions(ctx context.Context, userID int64) (PurchaseOptions, error) {
	options := PurchaseOptions{
		QuickIdeaQuantities: append([]int64(nil), QuickPurchaseIdeaQuantities[:]...),
		MinIdeaQuantity:     MinimumPurchaseIdeas, MaxIdeaQuantity: MaximumPurchaseIdeas, IdeasPerUSD: IdeasPerUSD,
		Terms:        PaymentLegalDocument{Version: CurrentTermsVersion, URL: "/terms"},
		RefundPolicy: PaymentLegalDocument{Version: CurrentRefundPolicyVersion, URL: "/refund-policy"},
	}
	if s == nil || s.configuration == nil || s.wallets == nil || userID <= 0 {
		options.UnavailableReason = "payments_not_configured"
		return options, nil
	}
	configuration, err := s.configuration.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(err) {
		options.UnavailableReason = "payments_not_configured"
		return options, nil
	}
	if err != nil {
		return PurchaseOptions{}, fmt.Errorf("get purchase configuration: %w", err)
	}
	if !configuration.PurchasesEnabled || !configuration.ChargesEnabled || configuration.StripeProductID == "" ||
		configuration.PaymentMethodConfigurationID == "" {
		options.UnavailableReason = "purchases_disabled"
		return options, nil
	}
	wallet, err := s.wallets.GetWallet(ctx, userID)
	if err != nil {
		if errors.Is(err, credits.ErrWalletNotFound) {
			options.UnavailableReason = "wallet_restricted"
			return options, nil
		}
		return PurchaseOptions{}, fmt.Errorf("get Wallet purchase eligibility: %w", err)
	}
	if wallet.Status != credits.WalletStatusActive || len(wallet.ActiveRestrictions) > 0 {
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

func (s *CheckoutWorkflow) CreateCheckout(ctx context.Context, command CreateCheckoutCommand) (CheckoutResult, error) {
	if s == nil || s.configuration == nil || s.checkoutStore == nil {
		return CheckoutResult{}, ErrPurchasesUnavailable
	}
	var result CheckoutResult
	err := s.checkoutStore.WithUserCheckoutLock(ctx, command.UserID, func(userLockedContext context.Context) error {
		var err error
		result, err = s.createCheckoutLocked(userLockedContext, command)
		return err
	})
	return result, err
}

func (s *CheckoutWorkflow) createCheckoutLocked(ctx context.Context, command CreateCheckoutCommand) (CheckoutResult, error) {
	if s == nil || s.checkoutStore == nil || s.checkoutGateway == nil || s.wallets == nil {
		return CheckoutResult{}, ErrPurchasesUnavailable
	}
	request, err := s.prepareCheckoutRequest(ctx, command)
	if err != nil {
		return CheckoutResult{}, err
	}
	resolution, err := s.resolveCheckoutOrder(ctx, command, request)
	if err != nil {
		return CheckoutResult{}, err
	}
	order, reused := resolution.order, resolution.reused
	if order.Status != PaymentOrderCreating {
		return finalizeExistingCheckoutResult(order)
	}
	order, err = s.createCheckoutSessionForOrder(ctx, request.configuration, request.apiKey, order)
	if err != nil {
		return CheckoutResult{}, err
	}
	return finalizeCreatedCheckoutResult(order, reused)
}

type checkoutRequest struct {
	quote         PurchaseQuote
	requestHash   string
	configuration StoredStripeConfiguration
	apiKey        string
	returnBaseURL string
	now           time.Time
}

type checkoutOrderResolution struct {
	order  PaymentOrder
	reused bool
}

func (s *CheckoutWorkflow) prepareCheckoutRequest(ctx context.Context, command CreateCheckoutCommand) (checkoutRequest, error) {
	requestHash, err := CheckoutRequestFingerprint(command)
	if err != nil {
		return checkoutRequest{}, err
	}
	quote, err := NewPurchaseQuote(command.IdeaQuantity)
	if err != nil {
		return checkoutRequest{}, err
	}
	configuration, apiKey, err := s.checkoutConfiguration(ctx)
	if err != nil {
		return checkoutRequest{}, err
	}
	if err := s.requireRechargeableWallet(ctx, command.UserID); err != nil {
		return checkoutRequest{}, err
	}
	returnBaseURL, err := normalizeCheckoutReturnBaseURL(command.ReturnBaseURL)
	if err != nil {
		return checkoutRequest{}, err
	}
	return checkoutRequest{
		quote: quote, requestHash: requestHash, configuration: configuration,
		apiKey: apiKey, returnBaseURL: returnBaseURL, now: s.runtime.now().UTC(),
	}, nil
}

func (s *CheckoutWorkflow) resolveCheckoutOrder(ctx context.Context, command CreateCheckoutCommand, request checkoutRequest) (checkoutOrderResolution, error) {
	activeOrders, err := s.refreshActivePaymentOrders(ctx, command.UserID, request.now)
	if err != nil {
		return checkoutOrderResolution{}, err
	}
	activeOrderCount := len(activeOrders)
	for _, activeOrder := range activeOrders {
		if activeOrder.IdeaQuantity != request.quote.IdeaQuantity {
			continue
		}
		switch activeOrder.Status {
		case PaymentOrderCheckoutOpen:
			resolved, err := s.retrieveAndConvergeCheckout(ctx, request.apiKey, activeOrder)
			if err != nil {
				return checkoutOrderResolution{}, err
			}
			switch resolved.Status {
			case PaymentOrderCheckoutOpen:
				return checkoutOrderResolution{order: resolved, reused: true}, nil
			case PaymentOrderAsyncPaymentPending:
				return checkoutOrderResolution{}, ErrPaymentAlreadyInProgress
			default:
				activeOrderCount--
			}
		case PaymentOrderAsyncPaymentPending:
			return checkoutOrderResolution{}, ErrPaymentAlreadyInProgress
		case PaymentOrderCreating:
			return checkoutOrderResolution{order: activeOrder, reused: true}, nil
		}
		break
	}
	if activeOrderCount >= 3 {
		return checkoutOrderResolution{}, ErrPendingCheckoutLimit
	}
	orderID, err := uuid.NewV7()
	if err != nil {
		return checkoutOrderResolution{}, fmt.Errorf("generate payment order ID: %w", err)
	}
	publicOrderNumber, err := newPublicOrderNumber(request.now)
	if err != nil {
		return checkoutOrderResolution{}, err
	}
	order, reused, err := s.checkoutStore.CreatePaymentOrderIntent(ctx, CreatePaymentOrderIntentInput{
		ID: orderID.String(), PublicOrderNumber: publicOrderNumber, PublicToken: uuid.NewString(),
		UserID: command.UserID, ClientIdempotencyKey: strings.TrimSpace(command.ClientIdempotencyKey), RequestHash: request.requestHash,
		StripeCredentialID: request.configuration.CredentialID, StripeAccountID: request.configuration.AccountID, StripeProductID: request.configuration.StripeProductID,
		IdeaQuantity: request.quote.IdeaQuantity, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: request.returnBaseURL,
		// Stripe accepts expiry with second precision. Persist the exact normalized
		// request value before the first call so every idempotent replay matches.
		ExpiresAt: request.now.Truncate(time.Second).Add(checkoutSessionLifetime + checkoutSessionTransportMargin),
	})
	if err != nil {
		return checkoutOrderResolution{}, err
	}
	return checkoutOrderResolution{order: order, reused: reused}, nil
}

func finalizeExistingCheckoutResult(order PaymentOrder) (CheckoutResult, error) {
	if order.Status == PaymentOrderAsyncPaymentPending {
		return CheckoutResult{}, ErrPaymentAlreadyInProgress
	}
	if order.Status != PaymentOrderCheckoutOpen || order.CheckoutURL == "" {
		return CheckoutResult{}, ErrPurchasesUnavailable
	}
	return checkoutResult(order, true), nil
}

func finalizeCreatedCheckoutResult(order PaymentOrder, reused bool) (CheckoutResult, error) {
	switch order.Status {
	case PaymentOrderCheckoutOpen:
		return checkoutResult(order, reused), nil
	case PaymentOrderAsyncPaymentPending:
		return CheckoutResult{}, ErrPaymentAlreadyInProgress
	case PaymentOrderPaidPendingCredit, PaymentOrderCredited:
		return CheckoutResult{}, ErrPaymentAlreadyCompleted
	case PaymentOrderExpired:
		return CheckoutResult{}, ErrCheckoutSessionExpired
	default:
		return CheckoutResult{}, ErrPurchasesUnavailable
	}
}

func (s *CheckoutWorkflow) refreshActivePaymentOrders(ctx context.Context, userID int64, now time.Time) ([]PaymentOrder, error) {
	orders, err := s.checkoutStore.ListActivePaymentOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	active := make([]PaymentOrder, 0, len(orders))
	for _, order := range orders {
		if order.Status == PaymentOrderAsyncPaymentPending || order.ExpiresAt.After(now) {
			active = append(active, order)
			continue
		}
		if order.Status == PaymentOrderCreating {
			if _, err := s.orderLifecycle.expireOrder(ctx, order); err != nil {
				return nil, err
			}
			continue
		}
		_, orderAPIKey, accessErr := s.stripeOrderConfiguration(ctx, order)
		if accessErr != nil {
			return nil, accessErr
		}
		resolved, err := s.retrieveAndConvergeCheckout(ctx, orderAPIKey, order)
		if err != nil {
			return nil, err
		}
		if resolved.Status == PaymentOrderCheckoutOpen && !resolved.ExpiresAt.After(now) {
			resolved, err = s.orderLifecycle.expireOrder(ctx, resolved)
			if err != nil {
				return nil, err
			}
		}
		if resolved.Status == PaymentOrderCheckoutOpen || resolved.Status == PaymentOrderAsyncPaymentPending {
			active = append(active, resolved)
		}
	}
	return active, nil
}

func (s *CheckoutWorkflow) GetPaymentOrderReturnStatus(ctx context.Context, userID int64, publicToken string) (PaymentOrderReturnStatus, error) {
	if s == nil || s.configuration == nil || s.checkoutStore == nil {
		return PaymentOrderReturnStatus{}, ErrPaymentOrderNotFound
	}
	var result PaymentOrderReturnStatus
	err := s.checkoutStore.WithUserCheckoutLock(ctx, userID, func(lockedContext context.Context) error {
		var err error
		result, err = s.getPaymentOrderReturnStatusLocked(lockedContext, userID, publicToken)
		return err
	})
	return result, err
}

func (s *CheckoutWorkflow) getPaymentOrderReturnStatusLocked(ctx context.Context, userID int64, publicToken string) (PaymentOrderReturnStatus, error) {
	if s == nil || s.checkoutStore == nil || userID <= 0 {
		return PaymentOrderReturnStatus{}, ErrPaymentOrderNotFound
	}
	order, err := s.checkoutStore.GetPaymentOrderByPublicToken(ctx, userID, publicToken)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentOrderReturnStatus{}, ErrPaymentOrderNotFound
	}
	if err != nil {
		return PaymentOrderReturnStatus{}, fmt.Errorf("get payment order return status: %w", err)
	}
	if order.Status != PaymentOrderCredited && order.StripeCheckoutSessionID != "" && s.checkoutGateway != nil {
		configuration, apiKey, configErr := s.stripeOrderConfiguration(ctx, order)
		if configErr != nil {
			return PaymentOrderReturnStatus{}, configErr
		}
		if configuration.AccountID == order.StripeAccountID {
			session, retrieveErr := s.checkoutGateway.RetrieveCheckoutSession(ctx, RetrieveCheckoutSessionInput{
				APIKey: apiKey, SessionID: order.StripeCheckoutSessionID,
			})
			if retrieveErr != nil {
				return PaymentOrderReturnStatus{}, checkoutOperationError(retrieveErr)
			}
			order, err = s.orderLifecycle.convergeCheckoutSession(ctx, order, session, checkoutSignalObserved)
			if err != nil {
				return PaymentOrderReturnStatus{}, err
			}
		}
	}
	return paymentOrderReturnStatus(order), nil
}

func (s *CheckoutWorkflow) checkoutConfiguration(ctx context.Context) (StoredStripeConfiguration, string, error) {
	configuration, apiKey, err := s.stripeAccessConfiguration(ctx)
	if err != nil {
		return StoredStripeConfiguration{}, "", err
	}
	if !configuration.PurchasesEnabled || !configuration.ChargesEnabled || configuration.StripeProductID == "" ||
		configuration.PaymentMethodConfigurationID == "" {
		return StoredStripeConfiguration{}, "", ErrPurchasesUnavailable
	}
	return configuration, apiKey, nil
}

func (s *CheckoutWorkflow) stripeAccessConfiguration(ctx context.Context) (StoredStripeConfiguration, string, error) {
	configuration, err := s.configuration.GetStripeConfiguration(ctx)
	if isConfigurationNotFound(err) {
		return StoredStripeConfiguration{}, "", ErrPurchasesUnavailable
	}
	if err != nil {
		return StoredStripeConfiguration{}, "", fmt.Errorf("load Stripe checkout configuration: %w", err)
	}
	apiKey, err := secrets.DecryptMaterialString(configuration.APIKeyCiphertext, configuration.SecretMaterialKind)
	if err != nil {
		return StoredStripeConfiguration{}, "", ErrPurchasesUnavailable
	}
	return configuration, apiKey, nil
}

func (s *CheckoutWorkflow) stripeOrderConfiguration(ctx context.Context, order PaymentOrder) (StoredStripeConfiguration, string, error) {
	configuration, err := s.configuration.GetStripeCredential(ctx, order.StripeCredentialID)
	if err != nil {
		return StoredStripeConfiguration{}, "", ErrPurchasesUnavailable
	}
	apiKey, err := secrets.DecryptMaterialString(configuration.APIKeyCiphertext, configuration.SecretMaterialKind)
	if err != nil {
		return StoredStripeConfiguration{}, "", ErrPurchasesUnavailable
	}
	return configuration, apiKey, nil
}

func (s *CheckoutWorkflow) requireRechargeableWallet(ctx context.Context, userID int64) error {
	wallet, err := s.wallets.GetWallet(ctx, userID)
	if err != nil {
		if errors.Is(err, credits.ErrWalletNotFound) {
			return ErrWalletNotRechargeable
		}
		return fmt.Errorf("get Wallet recharge eligibility: %w", err)
	}
	if wallet.Status != credits.WalletStatusActive || len(wallet.ActiveRestrictions) != 0 {
		return ErrWalletNotRechargeable
	}
	return nil
}

func (s *CheckoutWorkflow) ensurePaymentCustomer(
	ctx context.Context,
	configuration StoredStripeConfiguration,
	apiKey string,
	order PaymentOrder,
) (PaymentCustomer, error) {
	if configuration.AccountID != order.StripeAccountID || configuration.Mode == "" {
		return PaymentCustomer{}, ErrPurchasesUnavailable
	}
	stored, err := s.checkoutStore.GetPaymentCustomer(ctx, order.UserID, order.StripeAccountID, configuration.Mode)
	if err == nil && stored.StripeCustomerID != "" {
		return stored, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PaymentCustomer{}, fmt.Errorf("get payment customer: %w", err)
	}
	created, err := s.checkoutGateway.CreateCustomer(ctx, CreateCheckoutCustomerInput{
		APIKey: apiKey, UserID: order.UserID,
		IdempotencyKey: "ideaboard-customer-" + order.StripeAccountID + "-" + string(configuration.Mode) + "-" + strconv.FormatInt(order.UserID, 10),
	})
	if err != nil {
		return PaymentCustomer{}, err
	}
	if strings.TrimSpace(created.ID) == "" {
		return PaymentCustomer{}, ErrPurchasesUnavailable
	}
	return s.checkoutStore.UpsertPaymentCustomer(ctx, PaymentCustomer{
		UserID: order.UserID, StripeAccountID: order.StripeAccountID,
		StripeMode: configuration.Mode, StripeCustomerID: created.ID,
	})
}

// createCheckoutSessionForOrder replays the immutable Checkout intent. Stripe's
// public order number is the idempotency key, so a process failure between the
// provider call and local bind can recover without creating a second Session.
func (s *CheckoutWorkflow) createCheckoutSessionForOrder(
	ctx context.Context,
	configuration StoredStripeConfiguration,
	apiKey string,
	order PaymentOrder,
) (PaymentOrder, error) {
	if order.Status != PaymentOrderCreating || strings.TrimSpace(order.ReturnBaseURL) == "" {
		return PaymentOrder{}, ErrPurchasesUnavailable
	}
	customer, err := s.ensurePaymentCustomer(ctx, configuration, apiKey, order)
	if err != nil {
		return PaymentOrder{}, s.finishRejectedCheckoutCreation(ctx, order, err)
	}
	session, err := s.checkoutGateway.CreateCheckoutSession(ctx, CreateCheckoutSessionInput{
		APIKey: apiKey, PaymentMethodConfigurationID: configuration.PaymentMethodConfigurationID,
		CustomerID: customer.StripeCustomerID, ProductID: order.StripeProductID,
		PublicOrderNumber: order.PublicOrderNumber, PaymentOrderID: order.ID, RequestHash: order.RequestHash,
		IdeaQuantity: order.IdeaQuantity, AmountMinor: order.BaseAmountMinor, ExpiresAt: order.ExpiresAt,
		SuccessURL:     checkoutReturnURL(order.ReturnBaseURL, order.PublicToken, "success"),
		CancelURL:      checkoutReturnURL(order.ReturnBaseURL, order.PublicToken, "cancel"),
		IdempotencyKey: order.PublicOrderNumber,
	})
	if err != nil {
		return PaymentOrder{}, s.finishRejectedCheckoutCreation(ctx, order, err)
	}
	if err := validateCreatedCheckoutSession(order, customer, session); err != nil {
		return PaymentOrder{}, err
	}
	if session.IsPaid() || session.Status == CheckoutSessionStatusExpired {
		return s.orderLifecycle.convergeCheckoutSession(ctx, order, session, checkoutSignalObserved)
	}
	bound, err := s.checkoutStore.BindPaymentOrderCheckout(ctx, BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: customer.StripeCustomerID,
		StripeSessionID: session.ID, CheckoutURL: session.URL, ExpiresAt: session.ExpiresAt,
	})
	if err != nil {
		return PaymentOrder{}, err
	}
	return s.orderLifecycle.convergeCheckoutSession(ctx, bound, session, checkoutSignalObserved)
}

func (s *CheckoutWorkflow) finishRejectedCheckoutCreation(ctx context.Context, order PaymentOrder, err error) error {
	operationErr := checkoutOperationError(err)
	var gatewayErr *GatewayError
	if !errors.As(err, &gatewayErr) || gatewayErr.Code() != GatewayErrorRequestRejected {
		return operationErr
	}
	failed, markErr := s.orderLifecycle.markFailed(ctx, order)
	if markErr != nil {
		return errors.Join(operationErr, fmt.Errorf("mark rejected Checkout order failed: %w", markErr))
	}
	if failed.Status != PaymentOrderFailed {
		return errors.Join(operationErr, fmt.Errorf("rejected Checkout order remained %s", failed.Status))
	}
	return errors.Join(ErrCheckoutRequestRejected, operationErr)
}

func (s *CheckoutWorkflow) retrieveAndConvergeCheckout(ctx context.Context, apiKey string, order PaymentOrder) (PaymentOrder, error) {
	session, err := s.checkoutGateway.RetrieveCheckoutSession(ctx, RetrieveCheckoutSessionInput{
		APIKey: apiKey, SessionID: order.StripeCheckoutSessionID,
	})
	if err != nil {
		return order, checkoutOperationError(err)
	}
	return s.orderLifecycle.convergeCheckoutSession(ctx, order, session, checkoutSignalObserved)
}

func validateCreatedCheckoutSession(order PaymentOrder, customer PaymentCustomer, session CheckoutSession) error {
	requiresCheckoutURL := !session.IsPaid() && session.Status != CheckoutSessionStatusExpired
	if (requiresCheckoutURL && strings.TrimSpace(session.URL) == "") ||
		session.CustomerID != customer.StripeCustomerID || !session.ExpiresAt.Equal(order.ExpiresAt) {
		return ErrPurchasesUnavailable
	}
	return validateCheckoutSessionIdentity(order, session)
}

func validateCheckoutSessionIdentity(order PaymentOrder, session CheckoutSession) error {
	if strings.TrimSpace(session.ID) == "" || session.ID != order.StripeCheckoutSessionID && order.StripeCheckoutSessionID != "" ||
		session.PublicOrderNumber != order.PublicOrderNumber || session.PaymentOrderID != order.ID ||
		session.RequestHash != order.RequestHash || session.IdeaQuantity != order.IdeaQuantity {
		return ErrCheckoutIdempotencyConflict
	}
	return nil
}

func paidCheckoutFactsFromSession(accountID string, session CheckoutSession, paidAt time.Time) PaidCheckoutFacts {
	return PaidCheckoutFacts{
		StripeAccountID: accountID, StripeSessionID: session.ID, PublicOrderNumber: session.PublicOrderNumber,
		ExpiresAt:      session.ExpiresAt,
		PaymentOrderID: session.PaymentOrderID, RequestHash: session.RequestHash, StripeCustomerID: session.CustomerID,
		StripePaymentIntentID: session.PaymentIntentID, StripeChargeID: session.ChargeID, IdeaQuantity: session.IdeaQuantity,
		PaidSubtotalMinor: session.AmountSubtotalMinor, PaidTaxMinor: session.AmountTaxMinor,
		PaidTotalMinor: session.AmountTotalMinor, PaidCurrency: session.Currency, ReceiptURL: session.ReceiptURL, PaidAt: paidAt,
	}
}

func checkoutResult(order PaymentOrder, reused bool) CheckoutResult {
	return CheckoutResult{CheckoutURL: order.CheckoutURL, PublicToken: order.PublicToken, ExpiresAt: order.ExpiresAt, Reused: reused}
}

func paymentOrderReturnStatus(order PaymentOrder) PaymentOrderReturnStatus {
	status := "processing"
	switch order.Status {
	case PaymentOrderCredited:
		status = "credited"
	case PaymentOrderCancelled, PaymentOrderExpired, PaymentOrderFailed:
		status = "failed"
	}
	return PaymentOrderReturnStatus{IdeaQuantity: order.IdeaQuantity, Status: status, UpdatedAt: order.UpdatedAt}
}

func normalizeCheckoutReturnBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrPurchasesUnavailable
	}
	hostname := parsed.Hostname()
	isLoopback := hostname == "localhost" || hostname == "127.0.0.1"
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopback) {
		return "", ErrPurchasesUnavailable
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func checkoutReturnURL(baseURL, publicToken, source string) string {
	return baseURL + "?account=recharge&payment_return=" + url.QueryEscape(publicToken) + "&checkout_return=" + source
}

func newPublicOrderNumber(now time.Time) (string, error) {
	randomBytes := make([]byte, 6)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate public payment order number: %w", err)
	}
	return "IBP-" + now.Format("20060102") + "-" + strings.ToUpper(hex.EncodeToString(randomBytes)), nil
}

func checkoutOperationError(err error) error {
	var gatewayErr *GatewayError
	if errors.As(err, &gatewayErr) && gatewayErr.Code() == GatewayErrorProviderUnavailable {
		return errors.Join(ErrStripeProviderUnavailable, err)
	}
	// Keep the safe, typed gateway cause for durable reconciliation while the
	// browser-facing boundary still receives the stable product error.
	return errors.Join(ErrPurchasesUnavailable, err)
}
