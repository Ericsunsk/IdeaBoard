package readmodel

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strings"
	"time"

	"github.com/google/uuid"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

type ConfigurationStore interface {
	GetStripeConfiguration(context.Context) (paymentconfiguration.StoredStripeConfiguration, error)
}

type CheckoutStore interface {
	ListPaymentRechargeHistory(context.Context, int64, time.Time, *RechargeHistoryCursor, int) ([]dbsqlc.ListPaymentRechargeHistoryRow, error)
	ListAdminPaymentOrders(context.Context, AdminPaymentOrderFilters) (paymentcontracts.AdminPaymentOrderPage, error)
	GetAdminPaymentOrder(context.Context, string) (AdminPaymentOrderDetailRecord, []dbsqlc.ListAdminPaymentOrderWebhookEventsRow, error)
}

type ConvergenceStore interface {
	ListPaymentRefundsForOrder(context.Context, string) ([]paymentsdomain.PaymentRefund, error)
	ListPaymentDisputesForOrder(context.Context, string) ([]paymentsdomain.PaymentDispute, error)
}

type Service struct {
	configuration ConfigurationStore
	checkoutStore CheckoutStore
	convergence   ConvergenceStore
	wallets       ports.WalletReader
	now           func() time.Time
}

func NewService(configuration ConfigurationStore, checkoutStore CheckoutStore, convergence ConvergenceStore, wallets ports.WalletReader, now func() time.Time) (*Service, error) {
	if configuration == nil || checkoutStore == nil || convergence == nil || wallets == nil {
		return nil, fmt.Errorf("read model service requires configuration, checkout store, convergence store, and wallet reader")
	}
	if now == nil {
		now = time.Now
	}
	return &Service{configuration: configuration, checkoutStore: checkoutStore, convergence: convergence, wallets: wallets, now: now}, nil
}

func (s *Service) ListAdminPaymentOrders(ctx context.Context, input paymentcontracts.AdminPaymentOrderListInput) (paymentcontracts.AdminPaymentOrderPage, error) {
	if s == nil || s.checkoutStore == nil {
		return paymentcontracts.AdminPaymentOrderPage{}, fmt.Errorf("payments admin read service unavailable")
	}
	filters, err := NormalizeAdminPaymentOrderFilters(input)
	if err != nil {
		return paymentcontracts.AdminPaymentOrderPage{}, err
	}
	return s.checkoutStore.ListAdminPaymentOrders(ctx, filters)
}

func (s *Service) GetAdminPaymentOrder(ctx context.Context, orderNumber string) (paymentcontracts.AdminPaymentOrderDetail, error) {
	if s == nil || s.configuration == nil || s.checkoutStore == nil || s.convergence == nil || !IsValidAdminPaymentOrderNumber(orderNumber) {
		return paymentcontracts.AdminPaymentOrderDetail{}, paymentcontracts.ErrAdminPaymentOrderFilter
	}
	orderNumber = strings.TrimSpace(orderNumber)
	detail, events, err := s.checkoutStore.GetAdminPaymentOrder(ctx, orderNumber)
	if err != nil {
		return paymentcontracts.AdminPaymentOrderDetail{}, err
	}
	facts, err := s.loadAdminPaymentOrderFacts(ctx, orderNumber)
	if err != nil {
		return paymentcontracts.AdminPaymentOrderDetail{}, err
	}
	facts.appendTo(&detail)
	if err := s.setRefundableIdeaQuantity(ctx, &detail, facts); err != nil {
		return paymentcontracts.AdminPaymentOrderDetail{}, err
	}
	appendAdminPaymentOrderWebhookFacts(&detail, events, facts.refundIDs, facts.disputeIDs)
	return detail.AdminPaymentOrderDetail, nil
}

type adminPaymentOrderFacts struct {
	refunds     []paymentsdomain.PaymentRefund
	disputes    []paymentsdomain.PaymentDispute
	refundIDs   map[string]struct{}
	disputeIDs  map[string]struct{}
	disputeMode paymentports.StripeMode
}

func (s *Service) loadAdminPaymentOrderFacts(ctx context.Context, orderNumber string) (adminPaymentOrderFacts, error) {
	refunds, err := s.convergence.ListPaymentRefundsForOrder(ctx, orderNumber)
	if err != nil {
		return adminPaymentOrderFacts{}, err
	}
	disputes, err := s.convergence.ListPaymentDisputesForOrder(ctx, orderNumber)
	if err != nil {
		return adminPaymentOrderFacts{}, err
	}
	mode := paymentports.StripeMode("")
	if len(disputes) > 0 {
		configuration, configErr := s.configuration.GetStripeConfiguration(ctx)
		if configErr == nil {
			mode = configuration.Mode
		} else if !paymentconfiguration.IsNotFound(configErr) {
			return adminPaymentOrderFacts{}, configErr
		}
	}
	return adminPaymentOrderFacts{refunds: refunds, disputes: disputes, disputeMode: mode, refundIDs: paymentRefundIDs(refunds), disputeIDs: paymentDisputeIDs(disputes)}, nil
}

func (s *Service) setRefundableIdeaQuantity(ctx context.Context, detail *AdminPaymentOrderDetailRecord, facts adminPaymentOrderFacts) error {
	if detail.Status != paymentsdomain.PaymentOrderCredited || detail.PaidAt == nil || !s.now().UTC().Before(detail.PaidAt.Add(paymentsdomain.RefundEligibilityWindow)) {
		return nil
	}
	remaining := detail.IdeaQuantity
	for _, refund := range facts.refunds {
		if refund.Status == paymentsdomain.PaymentRefundPending || refund.Status == paymentsdomain.PaymentRefundSucceeded || refund.Status == paymentsdomain.PaymentRefundAnomaly {
			remaining -= refund.IdeaQuantity
		}
	}
	for _, dispute := range facts.disputes {
		if dispute.Status == paymentsdomain.PaymentDisputeOpen || dispute.Status == paymentsdomain.PaymentDisputeLost {
			remaining -= dispute.TargetIdeaQuantity
		}
	}
	if remaining <= 0 {
		return nil
	}
	wallet, err := s.wallets.GetWallet(ctx, detail.InternalUserID)
	if err != nil && !errors.Is(err, domain.ErrWalletNotFound) {
		return err
	}
	if err == nil {
		detail.RefundableIdeaQuantity = min(remaining, wallet.Available)
	}
	return nil
}

func (s *Service) ListRecharges(ctx context.Context, userID int64, cursor string, limit int) (paymentcontracts.RechargeHistoryPage, error) {
	if s == nil || s.checkoutStore == nil || userID <= 0 {
		return paymentcontracts.RechargeHistoryPage{}, paymentcontracts.ErrPaymentOrderNotFound
	}
	if limit == 0 {
		limit = DefaultRechargeHistoryLimit
	}
	if limit < 1 || limit > MaxRechargeHistoryLimit {
		return paymentcontracts.RechargeHistoryPage{}, fmt.Errorf(msgListRecharges, paymentcontracts.ErrRechargeHistoryLimit, MaxRechargeHistoryLimit)
	}
	parsedCursor, err := DecodeRechargeHistoryCursor(cursor)
	if err != nil {
		return paymentcontracts.RechargeHistoryPage{}, err
	}
	rows, err := s.checkoutStore.ListPaymentRechargeHistory(ctx, userID, s.now().UTC().AddDate(0, -12, 0), parsedCursor, limit+1)
	if err != nil {
		return paymentcontracts.RechargeHistoryPage{}, fmt.Errorf("list recharge history: %w", err)
	}
	page := paymentcontracts.RechargeHistoryPage{Items: make([]paymentcontracts.RechargeHistoryItem, 0, min(limit, len(rows)))}
	if len(rows) > limit {
		page.NextCursor, err = EncodeRechargeHistoryCursor(dbutil.TimeValue(rows[limit-1].OccurredAt), uuid.UUID(rows[limit-1].ID.Bytes))
		if err != nil {
			return paymentcontracts.RechargeHistoryPage{}, fmt.Errorf("encode recharge history cursor: %w", err)
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		item, err := rechargeHistoryItem(row)
		if err != nil {
			return paymentcontracts.RechargeHistoryPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

func rechargeHistoryItem(row dbsqlc.ListPaymentRechargeHistoryRow) (paymentcontracts.RechargeHistoryItem, error) {
	// The history status is a read-model derivation, not a durable
	// lifecycle: credited/cancelled mirror the order row, refunded
	// overlays full settlement. The wire words stay identical; the
	// contract DTO keeps its string field (converted below).
	status, paidAmountMinor, paidCurrency := RechargeHistoryStatus(row.Status), int64(0), ""
	switch row.Status {
	case string(paymentsdomain.PaymentOrderCredited):
		if !row.PaidTotalMinor.Valid || row.PaidCurrency == "" {
			return paymentcontracts.RechargeHistoryItem{}, fmt.Errorf("invalid credited payment order history row")
		}
		paidAmountMinor, paidCurrency = row.PaidTotalMinor.Int64, strings.ToLower(row.PaidCurrency)
		if row.RefundedIdeaQuantity >= row.IdeaQuantity {
			status = RechargeHistoryRefunded
		}
	case string(paymentsdomain.PaymentOrderCancelled):
		if row.PaidTotalMinor.Valid || row.PaidCurrency != "" || row.ReceiptUrl != "" {
			return paymentcontracts.RechargeHistoryItem{}, fmt.Errorf("invalid cancelled payment order history row")
		}
	default:
		return paymentcontracts.RechargeHistoryItem{}, fmt.Errorf("invalid payment order history status %q", row.Status)
	}
	return paymentcontracts.RechargeHistoryItem{OccurredAt: dbutil.TimeValue(row.OccurredAt), IdeaQuantity: row.IdeaQuantity, PaidAmountMinor: paidAmountMinor, PaidCurrency: paidCurrency, Status: string(status), RefundedIdeaQuantity: row.RefundedIdeaQuantity, RefundedAmountMinor: row.RefundedAmountMinor, ReceiptURL: row.ReceiptUrl}, nil
}
