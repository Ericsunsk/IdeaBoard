package payments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/secrets"
)

const refundEligibilityWindow = 14 * 24 * time.Hour

var (
	ErrRefundUnavailable      = errors.New("退款服务暂不可用")
	ErrRefundOrderNotEligible = errors.New("支付订单当前不满足退款条件")
	ErrRefundExpired          = errors.New("支付订单已超过 14 天退款期限")
	ErrRefundAmountInvalid    = errors.New("退款 Idea 数量无效")
	ErrRefundAmountExceeds    = errors.New("退款 Idea 数量超过可退余额")
	ErrRefundIdempotency      = errors.New("退款幂等键无效")
	ErrRefundProviderConflict = errors.New("检测到 Stripe Refund identity 冲突")
)

type PaymentRefundStatus string

const (
	PaymentRefundPending   PaymentRefundStatus = "pending"
	PaymentRefundSucceeded PaymentRefundStatus = "succeeded"
	PaymentRefundFailed    PaymentRefundStatus = "failed"
	PaymentRefundAnomaly   PaymentRefundStatus = "anomaly"
)

type CreatePaymentRefundCommand struct {
	ActorID        int64
	OrderNumber    string
	IdeaQuantity   int64
	IdempotencyKey string
	Reason         string
}

type PaymentRefund struct {
	ID                      string              `json:"id"`
	PaymentOrderID          string              `json:"payment_order_id"`
	PublicOrderNumber       string              `json:"public_order_number"`
	UserID                  int64               `json:"user_id"`
	IdeaQuantity            int64               `json:"idea_quantity"`
	AmountMinor             int64               `json:"amount_minor"`
	Currency                string              `json:"currency"`
	Status                  PaymentRefundStatus `json:"status"`
	StripeStatus            string              `json:"stripe_status,omitempty"`
	StripeChargeID          string              `json:"-"`
	StripeRefundID          string              `json:"stripe_refund_id,omitempty"`
	CreditHoldTransactionID string              `json:"credit_hold_transaction_id,omitempty"`
	CreditTransactionID     string              `json:"credit_transaction_id,omitempty"`
	ErrorSummary            string              `json:"error_summary,omitempty"`
	CreatedAt               time.Time           `json:"created_at"`
	UpdatedAt               time.Time           `json:"updated_at"`
}

type AdminPaymentRefundResult struct {
	PublicOrderNumber string              `json:"public_order_number"`
	IdeaQuantity      int64               `json:"idea_quantity"`
	AmountMinor       int64               `json:"amount_minor"`
	Currency          string              `json:"currency"`
	Status            PaymentRefundStatus `json:"status"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

func newAdminPaymentRefundResult(refund PaymentRefund) AdminPaymentRefundResult {
	return AdminPaymentRefundResult{
		PublicOrderNumber: refund.PublicOrderNumber, IdeaQuantity: refund.IdeaQuantity,
		AmountMinor: refund.AmountMinor, Currency: refund.Currency, Status: refund.Status,
		CreatedAt: refund.CreatedAt, UpdatedAt: refund.UpdatedAt,
	}
}

type CreateStripeRefundInput struct {
	APIKey         string
	ChargeID       string
	AmountMinor    int64
	Currency       string
	IdempotencyKey string
}

type StripeRefund struct {
	ID          string
	Status      string
	ChargeID    string
	AmountMinor int64
	Currency    string
}

// RefundGateway is intentionally separate from StripeGateway so existing
// configuration fakes do not need to implement payment mutations.
type RefundGateway interface {
	CreateRefund(context.Context, CreateStripeRefundInput) (StripeRefund, error)
	RetrieveRefund(context.Context, string, string) (StripeRefund, error)
}

func (s *StripeFactsWorkflow) CreatePaymentRefund(ctx context.Context, command CreatePaymentRefundCommand) (PaymentRefund, error) {
	if s == nil || s.convergenceStore == nil || s.refundGateway == nil {
		return PaymentRefund{}, ErrRefundUnavailable
	}
	if command.ActorID <= 0 || strings.TrimSpace(command.OrderNumber) == "" || command.IdeaQuantity <= 0 {
		return PaymentRefund{}, ErrRefundAmountInvalid
	}
	if len(strings.TrimSpace(command.IdempotencyKey)) < 8 || len(strings.TrimSpace(command.IdempotencyKey)) > 160 {
		return PaymentRefund{}, ErrRefundIdempotency
	}
	credential, err := s.configuration.GetStripeCredentialForOrderNumber(ctx, strings.TrimSpace(command.OrderNumber))
	if err != nil {
		return PaymentRefund{}, ErrRefundOrderNotEligible
	}
	apiKey, err := secrets.DecryptMaterialString(credential.APIKeyCiphertext, credential.SecretMaterialKind)
	if err != nil {
		return PaymentRefund{}, ErrStripeConfigurationInvalid
	}
	refund, _, err := s.convergenceStore.CreatePendingPaymentRefund(ctx, CreatePendingPaymentRefundInput{
		ActorID: command.ActorID, OrderNumber: strings.TrimSpace(command.OrderNumber), IdeaQuantity: command.IdeaQuantity,
		IdempotencyKey: strings.TrimSpace(command.IdempotencyKey), Reason: strings.TrimSpace(command.Reason), Now: s.runtime.now().UTC(),
	})
	if err != nil {
		return PaymentRefund{}, err
	}
	s.publishRefundUpdate(refund.UserID)
	if refund.StripeRefundID != "" {
		return refund, nil
	}
	providerRefund, err := s.refundGateway.CreateRefund(ctx, CreateStripeRefundInput{
		APIKey: apiKey, ChargeID: refund.StripeChargeID,
		AmountMinor: refund.AmountMinor, Currency: refund.Currency, IdempotencyKey: "ideaboard-refund-" + refund.ID,
	})
	if err != nil {
		// A timeout remains pending for reconciliation; explicit provider errors
		// release the local hold so the user can consume the Ideas again.
		if gatewayCode := new(GatewayError); errors.As(err, &gatewayCode) && gatewayCode.Code() == GatewayErrorProviderUnavailable {
			return refund, err
		}
		failed, markErr := s.convergenceStore.MarkPaymentRefundFailed(ctx, refund.ID, operationalErrorSummary(err, "Stripe refund request failed"), s.runtime.now().UTC())
		if markErr != nil {
			return PaymentRefund{}, errors.Join(err, markErr)
		}
		s.publishRefundUpdate(failed.UserID)
		return failed, err
	}
	if strings.TrimSpace(providerRefund.ID) == "" {
		return refund, fmt.Errorf("stripe refund response missing ID")
	}
	result, err := s.convergePaymentRefund(ctx, refund, providerRefund)
	result.PublicOrderNumber = refund.PublicOrderNumber
	return result, err
}

func (s *StripeFactsWorkflow) convergePaymentRefund(ctx context.Context, refund PaymentRefund, providerRefund StripeRefund) (PaymentRefund, error) {
	if s == nil || s.convergenceStore == nil {
		return PaymentRefund{}, ErrRefundUnavailable
	}
	if refund.Status == PaymentRefundAnomaly {
		return refund, nil
	}
	providerID := strings.TrimSpace(providerRefund.ID)
	if providerID == "" || (refund.StripeRefundID != "" && refund.StripeRefundID != providerID) {
		return PaymentRefund{}, ErrRefundProviderConflict
	}
	if (providerRefund.ChargeID != "" && providerRefund.ChargeID != refund.StripeChargeID) ||
		(providerRefund.AmountMinor > 0 && providerRefund.AmountMinor != refund.AmountMinor) ||
		(providerRefund.Currency != "" && !strings.EqualFold(providerRefund.Currency, refund.Currency)) {
		return PaymentRefund{}, ErrRefundProviderConflict
	}
	updated, err := s.convergenceStore.MarkPaymentRefundProviderResult(ctx, refund.ID, providerID, providerRefund.Status, s.runtime.now().UTC())
	if err != nil {
		return PaymentRefund{}, err
	}
	switch strings.ToLower(strings.TrimSpace(providerRefund.Status)) {
	case "succeeded":
		settled, settleErr := s.convergenceStore.MarkPaymentRefundSucceeded(ctx, updated.ID, s.runtime.now().UTC())
		if settleErr == nil && updated.Status == PaymentRefundPending {
			s.publishRefundUpdate(settled.UserID)
		}
		return settled, settleErr
	case "failed", "canceled":
		failed, failErr := s.convergenceStore.MarkPaymentRefundFailed(ctx, updated.ID, "Stripe refund failed", s.runtime.now().UTC())
		if failErr == nil && updated.Status == PaymentRefundPending {
			s.publishRefundUpdate(failed.UserID)
		}
		return failed, failErr
	default:
		return updated, nil
	}
}

func (s *StripeFactsWorkflow) publishRefundUpdate(userID int64) {
	if s != nil && s.runtime.events != nil {
		s.runtime.events.Publish(userID, PaymentEvent{Status: "refund_updated"})
	}
}
