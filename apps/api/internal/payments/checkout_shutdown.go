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
	if s == nil || s.configuration == nil || s.checkoutGateway == nil {
		return fmt.Errorf("payment Checkout shutdown unavailable")
	}
	return s.configuration.WithStripeConfigurationSharedLock(ctx, func(lockedContext context.Context) error {
		shutdown, err := s.configuration.GetPaymentCheckoutShutdown(lockedContext, shutdownID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil || shutdown.CompletedAt != nil {
			return err
		}
		var apiKey string
		for {
			orders, err := s.configuration.ListPaymentOrdersForCheckoutShutdown(lockedContext, shutdown, checkoutShutdownBatchSize)
			if err != nil {
				return err
			}
			if len(orders) == 0 {
				return s.configuration.CompletePaymentCheckoutShutdown(lockedContext, shutdown.ID, s.now().UTC())
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
	if s == nil || s.configuration == nil {
		return fmt.Errorf("payment Checkout shutdown unavailable")
	}
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
			_, err := s.orderLifecycle.expireOrder(userLockedContext, order)
			return err
		case PaymentOrderCheckoutOpen:
			expiration, err := s.checkoutGateway.ExpireCheckoutSession(userLockedContext, ExpireCheckoutSessionInput{
				APIKey: apiKey, SessionID: order.StripeCheckoutSessionID,
			})
			if err != nil {
				return checkoutOperationError(err)
			}
			_, err = s.orderLifecycle.convergeCheckoutExpiration(userLockedContext, order, expiration)
			return err
		default:
			return nil
		}
	})
}
