package payments

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrDisputeUnavailable      = errors.New("Dispute 服务暂不可用")
	ErrDisputeOrderNotEligible = errors.New("Dispute 无法关联有效支付订单")
	ErrDisputeProviderConflict = errors.New("Stripe Dispute identity 冲突")
	ErrDisputeTransition       = errors.New("Dispute 状态迁移无效")
)

type PaymentDisputeStatus string

const (
	PaymentDisputeOpen PaymentDisputeStatus = "open"
	PaymentDisputeWon  PaymentDisputeStatus = "won"
	PaymentDisputeLost PaymentDisputeStatus = "lost"
)

type StripeDispute struct {
	ID              string
	Status          string
	ChargeID        string
	PaymentIntentID string
	AmountMinor     int64
	Currency        string
	CreatedAt       time.Time
}

type DisputeGateway interface {
	RetrieveDispute(context.Context, string, string) (StripeDispute, error)
}

type PaymentDispute struct {
	ID                    string
	PaymentOrderID        string
	PublicOrderNumber     string
	UserID                int64
	StripeDisputeID       string
	StripeChargeID        string
	StripePaymentIntentID string
	ProviderStatus        string
	Status                PaymentDisputeStatus
	AmountMinor           int64
	Currency              string
	TargetIdeaQuantity    int64
	DebitedIdeaQuantity   int64
	UnrecoveredCredits    int64
	RestrictionID         string
	DebitTransactionID    string
	RestoreTransactionID  string
	OpenedAt              time.Time
	ClosedAt              *time.Time
	NextReconciliationAt  time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func canonicalPaymentDisputeStatus(providerStatus string) PaymentDisputeStatus {
	switch strings.ToLower(strings.TrimSpace(providerStatus)) {
	case "won", "warning_closed":
		return PaymentDisputeWon
	case "lost":
		return PaymentDisputeLost
	default:
		return PaymentDisputeOpen
	}
}

// expireDisputeCheckoutSessions runs only after the restriction transaction
// commits. River and webhook retries both re-enter this idempotent convergence
// path, so transient Stripe failures cannot strand an open Checkout forever.
func (s *Service) expireDisputeCheckoutSessions(ctx context.Context, apiKey string, dispute PaymentDispute) error {
	if s == nil || s.repo == nil || s.checkoutGateway == nil ||
		(dispute.Status != PaymentDisputeOpen && dispute.Status != PaymentDisputeLost) {
		return nil
	}
	return s.repo.WithUserCheckoutLock(ctx, dispute.UserID, func(userLockedContext context.Context) error {
		orders, err := s.repo.ListActivePaymentOrders(userLockedContext, dispute.UserID)
		if err != nil {
			return err
		}
		for _, order := range orders {
			switch order.Status {
			case PaymentOrderCreating:
				if _, expireErr := s.orderLifecycle.expireOrder(userLockedContext, order); expireErr != nil {
					return expireErr
				}
			case PaymentOrderCheckoutOpen:
				expiration, expireErr := s.checkoutGateway.ExpireCheckoutSession(userLockedContext, ExpireCheckoutSessionInput{
					APIKey: apiKey, SessionID: order.StripeCheckoutSessionID,
				})
				if expireErr != nil {
					return checkoutOperationError(expireErr)
				}
				if _, err := s.orderLifecycle.convergeCheckoutExpiration(userLockedContext, order, expiration); err != nil {
					return err
				}
			case PaymentOrderAsyncPaymentPending:
				// Payment submission already happened; normal Checkout convergence
				// remains responsible for this terminal provider outcome.
			default:
				return sql.ErrNoRows
			}
		}
		return nil
	})
}
