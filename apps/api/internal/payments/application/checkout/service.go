package checkout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/secrets"
)

type ConfigurationStore interface {
	GetStripeConfiguration(context.Context) (paymentconfiguration.StoredStripeConfiguration, error)
	GetStripeCredential(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error)
	GetPaymentCheckoutShutdown(context.Context, string) (paymentconfiguration.PaymentCheckoutShutdown, error)
	ListActivePaymentCheckoutShutdowns(context.Context) ([]paymentconfiguration.PaymentCheckoutShutdown, error)
	ListPaymentOrdersForCheckoutShutdown(context.Context, paymentconfiguration.PaymentCheckoutShutdown, int32) ([]paymentsdomain.PaymentOrder, error)
	CompletePaymentCheckoutShutdown(context.Context, string, time.Time) error
}

const (
	SessionLifetime = 30 * time.Minute
	TransportMargin = time.Minute
)

type Request struct {
	Quote                 paymentsdomain.PurchaseQuote
	RequestHash           string
	Configuration         paymentconfiguration.StoredStripeConfiguration
	APIKey, ReturnBaseURL string
	Now                   time.Time
}
type OrderResolution struct {
	Order  paymentsdomain.PaymentOrder
	Reused bool
}

func (s *Service) PrepareRequest(ctx context.Context, command paymentsdomain.CreateCheckoutCommand) (Request, error) {
	hash, err := paymentsdomain.CheckoutRequestFingerprint(command)
	if err != nil {
		return Request{}, err
	}
	quote, err := paymentsdomain.NewPurchaseQuote(command.IdeaQuantity)
	if err != nil {
		return Request{}, err
	}
	configuration, err := s.configuration.GetStripeConfiguration(ctx)
	if err != nil {
		return Request{}, paymentsdomain.ErrPurchasesUnavailable
	}
	if !configuration.PurchasesEnabled || !configuration.ChargesEnabled || configuration.StripeProductID == "" || configuration.PaymentMethodConfigurationID == "" {
		return Request{}, paymentsdomain.ErrPurchasesUnavailable
	}
	if err := s.requireWallet(ctx, command.UserID); err != nil {
		return Request{}, err
	}
	apiKey, err := secrets.DecryptMaterialString(configuration.APIKeyCiphertext, configuration.SecretMaterialKind)
	if err != nil {
		return Request{}, paymentsdomain.ErrPurchasesUnavailable
	}
	base, err := NormalizeReturnBaseURL(command.ReturnBaseURL)
	if err != nil {
		return Request{}, err
	}
	return Request{Quote: quote, RequestHash: hash, Configuration: configuration, APIKey: apiKey, ReturnBaseURL: base, Now: s.now().UTC()}, nil
}

func (s *Service) requireWallet(ctx context.Context, userID int64) error {
	wallet, err := s.wallets.GetWallet(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrWalletNotFound) {
			return paymentsdomain.ErrWalletNotRechargeable
		}
		return fmt.Errorf("get Wallet recharge eligibility: %w", err)
	}
	if wallet.Status != domain.WalletStatusActive || len(wallet.ActiveRestrictions) != 0 {
		return paymentsdomain.ErrWalletNotRechargeable
	}
	return nil
}

func (s *Service) ResolveOrder(ctx context.Context, command paymentsdomain.CreateCheckoutCommand, request Request, resolve func(context.Context, paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error)) (OrderResolution, error) {
	active, err := s.RefreshActivePaymentOrders(ctx, command.UserID, request.Now, resolve)
	if err != nil {
		return OrderResolution{}, err
	}
	count := len(active)
	for _, order := range active {
		if order.IdeaQuantity != request.Quote.IdeaQuantity {
			continue
		}
		switch order.Status {
		case paymentsdomain.PaymentOrderCheckoutOpen:
			resolved, err := s.RetrieveAndConverge(ctx, request.APIKey, order)
			if err != nil {
				return OrderResolution{}, err
			}
			if resolved.Status == paymentsdomain.PaymentOrderCheckoutOpen {
				return OrderResolution{resolved, true}, nil
			}
			if resolved.Status == paymentsdomain.PaymentOrderAsyncPaymentPending {
				return OrderResolution{}, paymentcontracts.ErrPaymentAlreadyInProgress
			}
			count--
		case paymentsdomain.PaymentOrderAsyncPaymentPending:
			return OrderResolution{}, paymentcontracts.ErrPaymentAlreadyInProgress
		case paymentsdomain.PaymentOrderCreating:
			return OrderResolution{order, true}, nil
		}
		break
	}
	if count >= 3 {
		return OrderResolution{}, paymentcontracts.ErrPendingCheckoutLimit
	}
	id, err := uuid.NewV7()
	if err != nil {
		return OrderResolution{}, fmt.Errorf("generate payment order ID: %w", err)
	}
	number, err := NewPublicOrderNumber(request.Now)
	if err != nil {
		return OrderResolution{}, err
	}
	order, reused, err := s.store.CreatePaymentOrderIntent(ctx, paymentsdomain.CreatePaymentOrderIntentInput{ID: id.String(), PublicOrderNumber: number, PublicToken: uuid.NewString(), UserID: command.UserID, ClientIdempotencyKey: strings.TrimSpace(command.ClientIdempotencyKey), RequestHash: request.RequestHash, StripeCredentialID: request.Configuration.CredentialID, StripeAccountID: request.Configuration.AccountID, StripeProductID: request.Configuration.StripeProductID, IdeaQuantity: request.Quote.IdeaQuantity, TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion, ReturnBaseURL: request.ReturnBaseURL, ExpiresAt: request.Now.Truncate(time.Second).Add(SessionLifetime + TransportMargin)})
	if err != nil {
		return OrderResolution{}, err
	}
	return OrderResolution{order, reused}, nil
}

type Store interface {
	WithUserCheckoutLock(context.Context, int64, func(context.Context) error) error
	CreatePaymentOrderIntent(context.Context, paymentsdomain.CreatePaymentOrderIntentInput) (paymentsdomain.PaymentOrder, bool, error)
	GetPaymentCustomer(context.Context, int64, string, paymentports.StripeMode) (PaymentCustomer, error)
	UpsertPaymentCustomer(context.Context, PaymentCustomer) (PaymentCustomer, error)
	BindPaymentOrderCheckout(context.Context, paymentsdomain.BindPaymentOrderCheckoutInput) (paymentsdomain.PaymentOrder, error)
	GetPaymentOrderByID(context.Context, string) (paymentsdomain.PaymentOrder, error)
	GetPaymentOrderByPublicToken(context.Context, int64, string) (paymentsdomain.PaymentOrder, error)
	ListActivePaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error)
	ListUserPendingPaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error)
}

type Service struct {
	configuration ConfigurationStore
	store         Store
	gateway       paymentports.CheckoutGateway
	wallets       ports.WalletReader
	lifecycle     *paymentorder.PaymentOrderLifecycle
	now           func() time.Time
}

func NewService(configuration ConfigurationStore, store Store, gateway paymentports.CheckoutGateway, wallets ports.WalletReader, lifecycle *paymentorder.PaymentOrderLifecycle, now func() time.Time) (*Service, error) {
	if configuration == nil || store == nil || gateway == nil || wallets == nil || lifecycle == nil {
		return nil, fmt.Errorf("checkout service requires configuration, store, gateway, wallet reader, and lifecycle")
	}
	if now == nil {
		now = time.Now
	}
	return &Service{configuration: configuration, store: store, gateway: gateway, wallets: wallets, lifecycle: lifecycle, now: now}, nil
}

func (s *Service) RefreshActivePaymentOrders(ctx context.Context, userID int64, now time.Time, resolve func(context.Context, paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error)) ([]paymentsdomain.PaymentOrder, error) {
	orders, err := s.store.ListActivePaymentOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	active := make([]paymentsdomain.PaymentOrder, 0, len(orders))
	for _, order := range orders {
		if order.Status == paymentsdomain.PaymentOrderAsyncPaymentPending || order.ExpiresAt.After(now) {
			active = append(active, order)
			continue
		}
		if order.Status == paymentsdomain.PaymentOrderCreating {
			if _, err := s.lifecycle.ExpireOrder(ctx, order); err != nil {
				return nil, err
			}
			continue
		}
		resolved, err := resolve(ctx, order)
		if err != nil {
			return nil, err
		}
		if resolved.Status == paymentsdomain.PaymentOrderCheckoutOpen && !resolved.ExpiresAt.After(now) {
			resolved, err = s.lifecycle.ExpireOrder(ctx, resolved)
			if err != nil {
				return nil, err
			}
		}
		if resolved.Status == paymentsdomain.PaymentOrderCheckoutOpen || resolved.Status == paymentsdomain.PaymentOrderAsyncPaymentPending {
			active = append(active, resolved)
		}
	}
	return active, nil
}

func (s *Service) EnsureCustomer(ctx context.Context, configuration paymentconfiguration.StoredStripeConfiguration, apiKey string, order paymentsdomain.PaymentOrder) (PaymentCustomer, error) {
	if configuration.AccountID != order.StripeAccountID || configuration.Mode == "" {
		return PaymentCustomer{}, paymentsdomain.ErrPurchasesUnavailable
	}
	stored, err := s.store.GetPaymentCustomer(ctx, order.UserID, order.StripeAccountID, configuration.Mode)
	if err == nil && stored.StripeCustomerID != "" {
		return stored, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PaymentCustomer{}, fmt.Errorf("get payment customer: %w", err)
	}
	created, err := s.gateway.CreateCustomer(ctx, paymentports.CreateCheckoutCustomerInput{APIKey: apiKey, UserID: order.UserID, IdempotencyKey: "ideaboard-customer-" + order.StripeAccountID + "-" + string(configuration.Mode) + "-" + strconv.FormatInt(order.UserID, 10)})
	if err != nil {
		return PaymentCustomer{}, err
	}
	if strings.TrimSpace(created.ID) == "" {
		return PaymentCustomer{}, paymentsdomain.ErrPurchasesUnavailable
	}
	return s.store.UpsertPaymentCustomer(ctx, PaymentCustomer{UserID: order.UserID, StripeAccountID: order.StripeAccountID, StripeMode: configuration.Mode, StripeCustomerID: created.ID})
}

func (s *Service) CreateSessionForOrder(ctx context.Context, configuration paymentconfiguration.StoredStripeConfiguration, apiKey string, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error) {
	if order.Status != paymentsdomain.PaymentOrderCreating || strings.TrimSpace(order.ReturnBaseURL) == "" {
		return paymentsdomain.PaymentOrder{}, paymentsdomain.ErrPurchasesUnavailable
	}
	customer, err := s.EnsureCustomer(ctx, configuration, apiKey, order)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, s.finishRejected(ctx, order, err)
	}
	session, err := s.gateway.CreateCheckoutSession(ctx, paymentports.CreateCheckoutSessionInput{APIKey: apiKey, PaymentMethodConfigurationID: configuration.PaymentMethodConfigurationID, CustomerID: customer.StripeCustomerID, ProductID: order.StripeProductID, PublicOrderNumber: order.PublicOrderNumber, PaymentOrderID: order.ID, RequestHash: order.RequestHash, IdeaQuantity: order.IdeaQuantity, AmountMinor: order.BaseAmountMinor, ExpiresAt: order.ExpiresAt, SuccessURL: ReturnURL(order.ReturnBaseURL, order.PublicToken, "success"), CancelURL: ReturnURL(order.ReturnBaseURL, order.PublicToken, "cancel"), IdempotencyKey: order.PublicOrderNumber})
	if err != nil {
		return paymentsdomain.PaymentOrder{}, s.finishRejected(ctx, order, err)
	}
	if err := validateCreatedCheckoutSession(order, customer, session); err != nil {
		return paymentsdomain.PaymentOrder{}, err
	}
	if session.IsPaid() || session.Status == paymentports.CheckoutSessionStatusExpired {
		return s.lifecycle.ConvergeCheckoutSession(ctx, order, session, paymentorder.SignalObserved)
	}
	bound, err := s.store.BindPaymentOrderCheckout(ctx, paymentsdomain.BindPaymentOrderCheckoutInput{OrderID: order.ID, StripeCustomerID: customer.StripeCustomerID, StripeSessionID: session.ID, CheckoutURL: session.URL, ExpiresAt: session.ExpiresAt})
	if err != nil {
		return paymentsdomain.PaymentOrder{}, err
	}
	return s.lifecycle.ConvergeCheckoutSession(ctx, bound, session, paymentorder.SignalObserved)
}

func (s *Service) RetrieveAndConverge(ctx context.Context, apiKey string, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error) {
	session, err := s.gateway.RetrieveCheckoutSession(ctx, paymentports.RetrieveCheckoutSessionInput{APIKey: apiKey, SessionID: order.StripeCheckoutSessionID})
	if err != nil {
		return order, OperationError(err)
	}
	return s.lifecycle.ConvergeCheckoutSession(ctx, order, session, paymentorder.SignalObserved)
}

func (s *Service) finishRejected(ctx context.Context, order paymentsdomain.PaymentOrder, err error) error {
	var gatewayErr *paymentports.GatewayError
	if !errors.As(err, &gatewayErr) || gatewayErr.Code() != paymentports.GatewayErrorRequestRejected {
		return OperationError(err)
	}
	failed, markErr := s.lifecycle.MarkFailed(ctx, order)
	if markErr != nil {
		return errors.Join(OperationError(err), fmt.Errorf("mark rejected Checkout order failed: %w", markErr))
	}
	if failed.Status != paymentsdomain.PaymentOrderFailed {
		return errors.Join(OperationError(err), fmt.Errorf("rejected Checkout order remained %s", failed.Status))
	}
	return errors.Join(paymentsdomain.ErrCheckoutRequestRejected, OperationError(err))
}

// OperationError maps provider failures to stable checkout boundary errors.
func OperationError(err error) error {
	var gatewayErr *paymentports.GatewayError
	if errors.As(err, &gatewayErr) && gatewayErr.Code() == paymentports.GatewayErrorProviderUnavailable {
		return errors.Join(paymentcontracts.ErrStripeProviderUnavailable, err)
	}
	return errors.Join(paymentsdomain.ErrPurchasesUnavailable, err)
}

func validateCreatedCheckoutSession(order paymentsdomain.PaymentOrder, customer PaymentCustomer, session paymentports.CheckoutSession) error {
	if (!session.IsPaid() && session.Status != paymentports.CheckoutSessionStatusExpired && strings.TrimSpace(session.URL) == "") || session.CustomerID != customer.StripeCustomerID || !session.ExpiresAt.Equal(order.ExpiresAt) {
		return paymentsdomain.ErrPurchasesUnavailable
	}
	return paymentorder.ValidateCheckoutSessionIdentity(order, session)
}

func (s *Service) RunPaymentCheckoutShutdown(ctx context.Context, shutdownID string) error {
	shutdown, err := s.configuration.GetPaymentCheckoutShutdown(ctx, shutdownID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil || shutdown.CompletedAt != nil {
		return err
	}
	var apiKey string
	for {
		orders, err := s.configuration.ListPaymentOrdersForCheckoutShutdown(ctx, shutdown, 100)
		if err != nil {
			return err
		}
		if len(orders) == 0 {
			return s.configuration.CompletePaymentCheckoutShutdown(ctx, shutdown.ID, s.now().UTC())
		}
		if apiKey == "" {
			apiKey, err = s.shutdownAPIKey(ctx, shutdown)
			if err != nil {
				return err
			}
		}
		for _, order := range orders {
			if err := s.processCheckoutShutdownOrder(ctx, shutdown, apiKey, order); err != nil {
				return err
			}
		}
	}
}

func (s *Service) shutdownAPIKey(ctx context.Context, shutdown paymentconfiguration.PaymentCheckoutShutdown) (string, error) {
	configuration, err := s.configuration.GetStripeCredential(ctx, shutdown.StripeCredentialID)
	if err != nil {
		return "", err
	}
	if configuration.AccountID != shutdown.StripeAccountID {
		return "", paymentsdomain.ErrPurchasesUnavailable
	}
	apiKey, err := secrets.DecryptMaterialString(configuration.APIKeyCiphertext, configuration.SecretMaterialKind)
	if err != nil {
		return "", paymentcontracts.ErrStripeConfigurationInvalid
	}
	return apiKey, nil
}

func (s *Service) processCheckoutShutdownOrder(ctx context.Context, shutdown paymentconfiguration.PaymentCheckoutShutdown, apiKey string, order paymentsdomain.PaymentOrder) error {
	return s.store.WithUserCheckoutLock(ctx, order.UserID, func(c context.Context) error {
		current, err := s.store.GetPaymentOrderByID(c, order.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.StripeCredentialID != shutdown.StripeCredentialID || current.StripeAccountID != shutdown.StripeAccountID || current.CreatedAt.After(shutdown.CutoffAt) {
			return nil
		}
		switch current.Status {
		case paymentsdomain.PaymentOrderCreating:
			_, err = s.lifecycle.ExpireOrder(c, current)
		case paymentsdomain.PaymentOrderCheckoutOpen:
			expiration, gatewayErr := s.gateway.ExpireCheckoutSession(c, paymentports.ExpireCheckoutSessionInput{APIKey: apiKey, SessionID: current.StripeCheckoutSessionID})
			if gatewayErr != nil {
				return gatewayErr
			}
			_, err = s.lifecycle.ConvergeCheckoutExpiration(c, current, expiration)
		}
		return err
	})
}

func (s *Service) RunActivePaymentCheckoutShutdowns(ctx context.Context) error {
	shutdowns, err := s.configuration.ListActivePaymentCheckoutShutdowns(ctx)
	if err != nil {
		return err
	}
	for _, shutdown := range shutdowns {
		if err := s.RunPaymentCheckoutShutdown(ctx, shutdown.ID); err != nil {
			return err
		}
	}
	return nil
}
