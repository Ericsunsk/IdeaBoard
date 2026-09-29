package refund

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/payments/application/credentials"
	"ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/payments/ports"
)

type Store interface {
	CreatePendingPaymentRefund(context.Context, CreatePendingPaymentRefundInput) (domain.PaymentRefund, bool, error)
	MarkPaymentRefundFailed(context.Context, string, string, time.Time) (domain.PaymentRefund, error)
	MarkPaymentRefundProviderResult(context.Context, string, string, string, time.Time) (domain.PaymentRefund, error)
	MarkPaymentRefundSucceeded(context.Context, string, time.Time) (domain.PaymentRefund, error)
	GetPaymentRefundByStripeID(context.Context, string) (domain.PaymentRefund, error)
	CreateExternalPaymentRefund(context.Context, ports.StripeRefund, time.Time) (domain.PaymentRefund, error)
	RecordPaymentRefundAnomaly(context.Context, ports.StripeRefund, string, time.Time) (int64, error)
}

type Service struct {
	store    Store
	resolver *credentials.Resolver
	gateway  ports.RefundGateway
	events   ports.PaymentEventPublisher
	now      func() time.Time
}

func NewService(store Store, gateway ports.RefundGateway, events ports.PaymentEventPublisher, now func() time.Time, resolver *credentials.Resolver) (*Service, error) {
	if store == nil || resolver == nil || gateway == nil {
		return nil, fmt.Errorf("refund service requires store, credential resolver, and gateway")
	}
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, resolver: resolver, gateway: gateway, events: events, now: now}, nil
}

func (s *Service) CreatePaymentRefund(ctx context.Context, command contracts.CreatePaymentRefundCommand) (domain.PaymentRefund, error) {
	if s == nil {
		return domain.PaymentRefund{}, contracts.ErrRefundUnavailable
	}
	command, err := NormalizePaymentRefundCommand(command)
	if err != nil {
		return domain.PaymentRefund{}, err
	}
	apiKey, err := s.refundAPIKey(ctx, command.OrderNumber)
	if err != nil {
		return domain.PaymentRefund{}, MapCredentialError(err)
	}
	refund, _, err := s.store.CreatePendingPaymentRefund(ctx, CreatePendingPaymentRefundInput{ActorID: command.ActorID, OrderNumber: command.OrderNumber, IdeaQuantity: command.IdeaQuantity, IdempotencyKey: command.IdempotencyKey, Reason: command.Reason, Now: s.now().UTC()})
	if err != nil {
		return domain.PaymentRefund{}, err
	}
	s.publish(refund.UserID)
	if refund.StripeRefundID != "" {
		return refund, nil
	}
	providerRefund, err := s.gateway.CreateRefund(ctx, ports.CreateStripeRefundInput{APIKey: apiKey, ChargeID: refund.StripeChargeID, AmountMinor: refund.AmountMinor, Currency: refund.Currency, IdempotencyKey: "ideaboard-refund-" + refund.ID})
	if err != nil {
		var gatewayErr *ports.GatewayError
		if errors.As(err, &gatewayErr) && gatewayErr.Code() == ports.GatewayErrorProviderUnavailable {
			return refund, err
		}
		failed, markErr := s.store.MarkPaymentRefundFailed(ctx, refund.ID, operationalErrorSummary(err), s.now().UTC())
		if markErr != nil {
			return domain.PaymentRefund{}, errors.Join(err, markErr)
		}
		s.publish(failed.UserID)
		return failed, err
	}
	if strings.TrimSpace(providerRefund.ID) == "" {
		return refund, fmt.Errorf("stripe refund response missing ID")
	}
	result, err := s.Converge(ctx, refund, providerRefund)
	result.PublicOrderNumber = refund.PublicOrderNumber
	return result, err
}

func operationalErrorSummary(err error) string {
	var gatewayErr *ports.GatewayError
	if errors.As(err, &gatewayErr) {
		return gatewayErr.Error()
	}
	return "Stripe refund request failed"
}

func (s *Service) Converge(ctx context.Context, refund domain.PaymentRefund, providerRefund ports.StripeRefund) (domain.PaymentRefund, error) {
	if s == nil || s.store == nil {
		return domain.PaymentRefund{}, contracts.ErrRefundUnavailable
	}
	if refund.Status == domain.PaymentRefundAnomaly {
		return refund, nil
	}
	if err := ValidateProviderRefund(refund, providerRefund); err != nil {
		return domain.PaymentRefund{}, err
	}
	updated, err := s.store.MarkPaymentRefundProviderResult(ctx, refund.ID, strings.TrimSpace(providerRefund.ID), providerRefund.Status, s.now().UTC())
	if err != nil {
		return domain.PaymentRefund{}, err
	}
	switch ports.StripeRefundStatus(strings.ToLower(strings.TrimSpace(providerRefund.Status))) {
	case ports.StripeRefundSucceeded:
		settled, settleErr := s.store.MarkPaymentRefundSucceeded(ctx, updated.ID, s.now().UTC())
		if settleErr == nil && updated.Status == domain.PaymentRefundPending {
			s.publish(settled.UserID)
		}
		return settled, settleErr
	case ports.StripeRefundFailed, ports.StripeRefundCanceled:
		failed, failErr := s.store.MarkPaymentRefundFailed(ctx, updated.ID, "Stripe refund failed", s.now().UTC())
		if failErr == nil && updated.Status == domain.PaymentRefundPending {
			s.publish(failed.UserID)
		}
		return failed, failErr
	default:
		return updated, nil
	}
}

func (s *Service) CreateProviderRefund(ctx context.Context, apiKey string, refund domain.PaymentRefund) (ports.StripeRefund, error) {
	return s.gateway.CreateRefund(ctx, ports.CreateStripeRefundInput{APIKey: apiKey, ChargeID: refund.StripeChargeID, AmountMinor: refund.AmountMinor, Currency: refund.Currency, IdempotencyKey: "ideaboard-refund-" + refund.ID})
}

func (s *Service) RetrieveProviderRefund(ctx context.Context, apiKey, id string) (ports.StripeRefund, error) {
	return s.gateway.RetrieveRefund(ctx, apiKey, id)
}

func (s *Service) RefundAPIKey(ctx context.Context, orderNumber string) (string, error) {
	return s.refundAPIKey(ctx, orderNumber)
}

func (s *Service) RefundAPIKeyForOrder(ctx context.Context, orderID string) (string, error) {
	_, apiKey, err := s.resolver.ForOrder(ctx, orderID)
	if err != nil {
		return "", contracts.ErrStripeConfigurationInvalid
	}
	return apiKey, nil
}

func (s *Service) publish(userID int64) {
	if s != nil && s.events != nil {
		s.events.Publish(userID, ports.PaymentEvent{Status: ports.PaymentEventRefundUpdated})
	}
}

func (s *Service) refundAPIKey(ctx context.Context, orderNumber string) (string, error) {
	_, apiKey, err := s.resolver.ForOrderNumber(ctx, orderNumber)
	if err != nil {
		if errors.Is(err, contracts.ErrRefundOrderNotEligible) {
			return "", err
		}
		return "", contracts.ErrStripeConfigurationInvalid
	}
	return apiKey, nil
}
