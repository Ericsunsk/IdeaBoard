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
	ErrStripeConfigurationBusy  = errors.New("仍有未完成的充值订单，请稍后再更换 Stripe API Key")
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

func (s *Service) ListPendingOrders(ctx context.Context, userID int64) (PendingPaymentOrderList, error) {
	result := PendingPaymentOrderList{Items: make([]PendingPaymentOrder, 0)}
	if s == nil || s.configuration == nil || s.repo == nil || userID <= 0 {
		return result, ErrPaymentOrderNotFound
	}
	orders, err := s.repo.ListUserPendingPaymentOrders(ctx, userID)
	if err != nil {
		return result, err
	}
	for _, order := range orders {
		result.Items = append(result.Items, pendingPaymentOrder(order))
	}
	return result, nil
}

func (s *Service) ContinuePendingOrder(ctx context.Context, userID int64, publicToken string) (PendingPaymentOrder, error) {
	if s == nil || s.repo == nil || userID <= 0 {
		return PendingPaymentOrder{}, ErrPaymentOrderNotFound
	}
	var result PendingPaymentOrder
	err := s.configuration.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		return s.repo.WithUserCheckoutLock(lockedContext, userID, func(userLockedContext context.Context) error {
			order, err := s.repo.GetPaymentOrderByPublicToken(userLockedContext, userID, publicToken)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentOrderNotFound
			}
			if err != nil {
				return fmt.Errorf("get pending payment order to continue: %w", err)
			}
			if order.Status == PaymentOrderPaidPendingCredit {
				converged, err := s.orderLifecycle.creditPaidOrder(userLockedContext, order, s.now().UTC())
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
			configuration, apiKey, err := s.checkoutConfiguration(userLockedContext)
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
			if order.Status == PaymentOrderCheckoutOpen && !order.ExpiresAt.After(s.now().UTC()) {
				order, err = s.orderLifecycle.expireOrder(userLockedContext, order)
				if err != nil {
					return err
				}
			}
			result = pendingPaymentOrder(order)
			return nil
		})
	})
	return result, err
}

func (s *Service) AbandonPendingOrder(ctx context.Context, userID int64, publicToken string) (PendingPaymentOrder, error) {
	if s == nil || s.configuration == nil || s.repo == nil || userID <= 0 {
		return PendingPaymentOrder{}, ErrPaymentOrderNotFound
	}
	var result PendingPaymentOrder
	err := s.configuration.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		return s.repo.WithUserCheckoutLock(lockedContext, userID, func(userLockedContext context.Context) error {
			order, err := s.repo.GetPaymentOrderByPublicToken(userLockedContext, userID, publicToken)
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
			configuration, apiKey, err := s.stripeAccessConfiguration(userLockedContext)
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

func (s *Service) GetPurchaseOptions(ctx context.Context, userID int64) (PurchaseOptions, error) {
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
	if !configuration.PurchasesEnabled || !configuration.ChargesEnabled || configuration.StripeProductID == "" {
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

func (s *Service) CreateCheckout(ctx context.Context, command CreateCheckoutCommand) (CheckoutResult, error) {
	if s == nil || s.configuration == nil || s.repo == nil {
		return CheckoutResult{}, ErrPurchasesUnavailable
	}
	var result CheckoutResult
	err := s.configuration.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		return s.repo.WithUserCheckoutLock(lockedContext, command.UserID, func(userLockedContext context.Context) error {
			var err error
			result, err = s.createCheckoutLocked(userLockedContext, command)
			return err
		})
	})
	return result, err
}

func (s *Service) createCheckoutLocked(ctx context.Context, command CreateCheckoutCommand) (CheckoutResult, error) {
	if s == nil || s.repo == nil || s.checkoutGateway == nil || s.wallets == nil {
		return CheckoutResult{}, ErrPurchasesUnavailable
	}
	requestHash, err := CheckoutRequestFingerprint(command)
	if err != nil {
		return CheckoutResult{}, err
	}
	quote, err := NewPurchaseQuote(command.IdeaQuantity)
	if err != nil {
		return CheckoutResult{}, err
	}
	configuration, apiKey, err := s.checkoutConfiguration(ctx)
	if err != nil {
		return CheckoutResult{}, err
	}
	if err := s.requireRechargeableWallet(ctx, command.UserID); err != nil {
		return CheckoutResult{}, err
	}
	returnBaseURL, err := normalizeCheckoutReturnBaseURL(command.ReturnBaseURL)
	if err != nil {
		return CheckoutResult{}, err
	}

	now := s.now().UTC()
	activeOrders, err := s.refreshActivePaymentOrders(ctx, apiKey, command.UserID, now)
	if err != nil {
		return CheckoutResult{}, err
	}
	var order PaymentOrder
	reused := false
	activeOrderCount := len(activeOrders)
	for _, activeOrder := range activeOrders {
		if activeOrder.IdeaQuantity != quote.IdeaQuantity {
			continue
		}
		switch activeOrder.Status {
		case PaymentOrderCheckoutOpen:
			resolved, err := s.retrieveAndConvergeCheckout(ctx, apiKey, activeOrder)
			if err != nil {
				return CheckoutResult{}, err
			}
			switch resolved.Status {
			case PaymentOrderCheckoutOpen:
				return checkoutResult(resolved, true), nil
			case PaymentOrderAsyncPaymentPending:
				return CheckoutResult{}, ErrPaymentAlreadyInProgress
			default:
				activeOrderCount--
			}
		case PaymentOrderAsyncPaymentPending:
			return CheckoutResult{}, ErrPaymentAlreadyInProgress
		case PaymentOrderCreating:
			order, reused = activeOrder, true
		}
		break
	}
	if order.ID == "" {
		if activeOrderCount >= 3 {
			return CheckoutResult{}, ErrPendingCheckoutLimit
		}
		orderID, err := uuid.NewV7()
		if err != nil {
			return CheckoutResult{}, fmt.Errorf("generate payment order ID: %w", err)
		}
		publicOrderNumber, err := newPublicOrderNumber(now)
		if err != nil {
			return CheckoutResult{}, err
		}
		order, reused, err = s.repo.CreatePaymentOrderIntent(ctx, CreatePaymentOrderIntentInput{
			ID: orderID.String(), PublicOrderNumber: publicOrderNumber, PublicToken: uuid.NewString(),
			UserID: command.UserID, ClientIdempotencyKey: strings.TrimSpace(command.ClientIdempotencyKey), RequestHash: requestHash,
			StripeAccountID: configuration.AccountID, StripeProductID: configuration.StripeProductID,
			IdeaQuantity: quote.IdeaQuantity,
			TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
			ReturnBaseURL: returnBaseURL,
			// Stripe accepts expiry with second precision. Persist the exact normalized
			// request value before the first call so every idempotent replay matches.
			ExpiresAt: now.UTC().Truncate(time.Second).Add(checkoutSessionLifetime + checkoutSessionTransportMargin),
		})
		if err != nil {
			return CheckoutResult{}, err
		}
	}
	if order.Status != PaymentOrderCreating {
		if order.Status != PaymentOrderCheckoutOpen && order.Status != PaymentOrderAsyncPaymentPending {
			return CheckoutResult{}, ErrPurchasesUnavailable
		}
		if order.Status == PaymentOrderAsyncPaymentPending {
			return CheckoutResult{}, ErrPaymentAlreadyInProgress
		}
		if order.CheckoutURL == "" {
			return CheckoutResult{}, ErrPurchasesUnavailable
		}
		return checkoutResult(order, true), nil
	}

	order, err = s.createCheckoutSessionForOrder(ctx, apiKey, order)
	if err != nil {
		return CheckoutResult{}, err
	}
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

func (s *Service) refreshActivePaymentOrders(ctx context.Context, apiKey string, userID int64, now time.Time) ([]PaymentOrder, error) {
	orders, err := s.repo.ListActivePaymentOrders(ctx, userID)
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
		resolved, err := s.retrieveAndConvergeCheckout(ctx, apiKey, order)
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

func (s *Service) GetPaymentOrderReturnStatus(ctx context.Context, userID int64, publicToken string) (PaymentOrderReturnStatus, error) {
	if s == nil || s.repo == nil {
		return PaymentOrderReturnStatus{}, ErrPaymentOrderNotFound
	}
	var result PaymentOrderReturnStatus
	err := s.configuration.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		var err error
		result, err = s.getPaymentOrderReturnStatusLocked(lockedContext, userID, publicToken)
		return err
	})
	return result, err
}

func (s *Service) getPaymentOrderReturnStatusLocked(ctx context.Context, userID int64, publicToken string) (PaymentOrderReturnStatus, error) {
	if s == nil || s.repo == nil || userID <= 0 {
		return PaymentOrderReturnStatus{}, ErrPaymentOrderNotFound
	}
	order, err := s.repo.GetPaymentOrderByPublicToken(ctx, userID, publicToken)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentOrderReturnStatus{}, ErrPaymentOrderNotFound
	}
	if err != nil {
		return PaymentOrderReturnStatus{}, fmt.Errorf("get payment order return status: %w", err)
	}
	if order.Status != PaymentOrderCredited && order.StripeCheckoutSessionID != "" && s.checkoutGateway != nil {
		configuration, apiKey, configErr := s.stripeAccessConfiguration(ctx)
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

func (s *Service) checkoutConfiguration(ctx context.Context) (StoredStripeConfiguration, string, error) {
	configuration, apiKey, err := s.stripeAccessConfiguration(ctx)
	if err != nil {
		return StoredStripeConfiguration{}, "", err
	}
	if !configuration.PurchasesEnabled || !configuration.ChargesEnabled || configuration.StripeProductID == "" {
		return StoredStripeConfiguration{}, "", ErrPurchasesUnavailable
	}
	return configuration, apiKey, nil
}

func (s *Service) stripeAccessConfiguration(ctx context.Context) (StoredStripeConfiguration, string, error) {
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

func (s *Service) requireRechargeableWallet(ctx context.Context, userID int64) error {
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

func (s *Service) ensurePaymentCustomer(ctx context.Context, apiKey string, order PaymentOrder) (PaymentCustomer, error) {
	stored, err := s.repo.GetPaymentCustomer(ctx, order.UserID)
	if err == nil && stored.StripeAccountID == order.StripeAccountID && stored.StripeCustomerID != "" {
		return stored, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PaymentCustomer{}, fmt.Errorf("get payment customer: %w", err)
	}
	created, err := s.checkoutGateway.CreateCustomer(ctx, CreateCheckoutCustomerInput{
		APIKey: apiKey, UserID: order.UserID,
		IdempotencyKey: "ideaboard-customer-" + order.StripeAccountID + "-" + strconv.FormatInt(order.UserID, 10),
	})
	if err != nil {
		return PaymentCustomer{}, err
	}
	if strings.TrimSpace(created.ID) == "" {
		return PaymentCustomer{}, ErrPurchasesUnavailable
	}
	return s.repo.ReplacePaymentCustomer(ctx, PaymentCustomer{
		UserID: order.UserID, StripeAccountID: order.StripeAccountID, StripeCustomerID: created.ID,
	})
}

// createCheckoutSessionForOrder replays the immutable Checkout intent. Stripe's
// public order number is the idempotency key, so a process failure between the
// provider call and local bind can recover without creating a second Session.
func (s *Service) createCheckoutSessionForOrder(ctx context.Context, apiKey string, order PaymentOrder) (PaymentOrder, error) {
	if order.Status != PaymentOrderCreating || strings.TrimSpace(order.ReturnBaseURL) == "" {
		return PaymentOrder{}, ErrPurchasesUnavailable
	}
	customer, err := s.ensurePaymentCustomer(ctx, apiKey, order)
	if err != nil {
		return PaymentOrder{}, s.finishRejectedCheckoutCreation(ctx, order, err)
	}
	session, err := s.checkoutGateway.CreateCheckoutSession(ctx, CreateCheckoutSessionInput{
		APIKey: apiKey, CustomerID: customer.StripeCustomerID, ProductID: order.StripeProductID,
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
	bound, err := s.repo.BindPaymentOrderCheckout(ctx, BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: customer.StripeCustomerID,
		StripeSessionID: session.ID, CheckoutURL: session.URL, ExpiresAt: session.ExpiresAt,
	})
	if err != nil {
		return PaymentOrder{}, err
	}
	return s.orderLifecycle.convergeCheckoutSession(ctx, bound, session, checkoutSignalObserved)
}

func (s *Service) finishRejectedCheckoutCreation(ctx context.Context, order PaymentOrder, err error) error {
	operationErr := checkoutOperationError(err)
	var gatewayErr *GatewayError
	if !errors.As(err, &gatewayErr) || gatewayErr.Code() != GatewayErrorRequestRejected {
		return operationErr
	}
	failed, markErr := s.repo.MarkPaymentOrderFailed(ctx, order.ID)
	if markErr != nil {
		return errors.Join(operationErr, fmt.Errorf("mark rejected Checkout order failed: %w", markErr))
	}
	if failed.Status != PaymentOrderFailed {
		return errors.Join(operationErr, fmt.Errorf("rejected Checkout order remained %s", failed.Status))
	}
	return errors.Join(ErrCheckoutRequestRejected, operationErr)
}

func (s *Service) retrieveAndConvergeCheckout(ctx context.Context, apiKey string, order PaymentOrder) (PaymentOrder, error) {
	session, err := s.checkoutGateway.RetrieveCheckoutSession(ctx, RetrieveCheckoutSessionInput{
		APIKey: apiKey, SessionID: order.StripeCheckoutSessionID,
	})
	if err != nil {
		return order, checkoutOperationError(err)
	}
	return s.orderLifecycle.convergeCheckoutSession(ctx, order, session, checkoutSignalObserved)
}

// settleOrdersBeforeStripeKeyChange preserves the single-active-account model:
// every older Checkout is resolved with the current key before that key can be replaced.
func (s *Service) settleOrdersBeforeStripeKeyChange(ctx context.Context, current StoredStripeConfiguration) error {
	orders, err := s.repo.ListUnsettledPaymentOrders(ctx)
	if err != nil {
		return err
	}
	if len(orders) == 0 {
		return nil
	}
	if len(orders) > 1000 || s.checkoutGateway == nil {
		return ErrStripeConfigurationBusy
	}
	currentAPIKey, err := secrets.DecryptMaterialString(current.APIKeyCiphertext, current.SecretMaterialKind)
	if err != nil {
		return ErrStripeConfigurationInvalid
	}
	now := s.now().UTC()
	hasActiveOrder := false
	for _, order := range orders {
		if order.StripeCheckoutSessionID == "" {
			if order.ExpiresAt.After(now) {
				hasActiveOrder = true
				continue
			}
			if _, err := s.orderLifecycle.expireOrder(ctx, order); err != nil {
				return err
			}
			continue
		}
		session, err := s.checkoutGateway.RetrieveCheckoutSession(ctx, RetrieveCheckoutSessionInput{
			APIKey: currentAPIKey, SessionID: order.StripeCheckoutSessionID,
		})
		if err != nil {
			return checkoutOperationError(err)
		}
		resolved, err := s.orderLifecycle.convergeCheckoutSession(ctx, order, session, checkoutSignalObserved)
		if err != nil {
			return err
		}
		if resolved.Status == PaymentOrderCheckoutOpen || resolved.Status == PaymentOrderAsyncPaymentPending {
			hasActiveOrder = true
		}
	}
	if hasActiveOrder {
		return ErrStripeConfigurationBusy
	}
	return nil
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
