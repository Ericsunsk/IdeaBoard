package payments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const checkoutShutdownBatchSize int32 = 100

// RunPaymentCheckoutShutdown expires Checkout Sessions that existed when an
// administrator disabled purchases. The run cutoff keeps a later re-enable
// from affecting newly created Sessions.
func (s *Service) RunPaymentCheckoutShutdown(ctx context.Context, shutdownID string) error {
	if s == nil || s.repo == nil || s.checkoutGateway == nil {
		return fmt.Errorf("payment Checkout shutdown unavailable")
	}
	return s.repo.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		shutdown, err := s.repo.GetPaymentCheckoutShutdown(lockedContext, shutdownID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil || shutdown.CompletedAt != nil {
			return err
		}
		var apiKey string
		for {
			orders, err := s.repo.ListPaymentOrdersForCheckoutShutdown(lockedContext, shutdown, checkoutShutdownBatchSize)
			if err != nil {
				return err
			}
			if len(orders) == 0 {
				return s.repo.CompletePaymentCheckoutShutdown(lockedContext, shutdown.ID, s.now().UTC())
			}
			if apiKey == "" {
				configuration, resolvedAPIKey, err := s.stripeAccessConfiguration(lockedContext)
				if err != nil {
					return err
				}
				if configuration.AccountID != shutdown.StripeAccountID {
					return ErrPurchasesUnavailable
				}
				apiKey = resolvedAPIKey
			}
			for _, order := range orders {
				if err := s.expireCheckoutForShutdown(lockedContext, apiKey, shutdown, order); err != nil {
					return err
				}
			}
		}
	})
}

func (s *Service) RunActivePaymentCheckoutShutdowns(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("payment Checkout shutdown unavailable")
	}
	shutdowns, err := s.repo.ListActivePaymentCheckoutShutdowns(ctx)
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

func (s *Service) expireCheckoutForShutdown(ctx context.Context, apiKey string, shutdown PaymentCheckoutShutdown, candidate PaymentOrder) error {
	return s.repo.WithUserCheckoutLock(ctx, candidate.UserID, func(userLockedContext context.Context) error {
		order, err := s.repo.GetPaymentOrderByID(userLockedContext, candidate.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if order.StripeAccountID != shutdown.StripeAccountID || order.CreatedAt.After(shutdown.CutoffAt) {
			return nil
		}
		switch order.Status {
		case PaymentOrderCreating:
			expired, changed, err := s.repo.MarkPaymentOrderExpired(userLockedContext, order.ID)
			if err == nil && changed {
				s.events.Publish(expired.UserID, PaymentEvent{PublicToken: expired.PublicToken, Status: "failed"})
			}
			return err
		case PaymentOrderCheckoutOpen:
			expiration, err := s.checkoutGateway.ExpireCheckoutSession(userLockedContext, ExpireCheckoutSessionInput{
				APIKey: apiKey, SessionID: order.StripeCheckoutSessionID,
			})
			if err != nil {
				return checkoutOperationError(err)
			}
			if err := validateCheckoutSessionIdentity(order, expiration.Session); err != nil {
				return err
			}
			return s.convergeCheckoutExpiration(userLockedContext, order, expiration)
		default:
			return nil
		}
	})
}
