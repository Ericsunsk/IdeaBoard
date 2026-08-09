package payments

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/credits"
	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/google/uuid"
)

const (
	defaultAdminPaymentOrderLimit = 20
	maxAdminPaymentOrderLimit     = 100
	maxAdminPaymentOrderCursorLen = 512
	maxAdminPaymentOrderUserLen   = 50
)

var (
	ErrAdminPaymentOrderFilter   = errors.New("支付订单筛选条件无效")
	ErrAdminPaymentOrderCursor   = errors.New("支付订单游标无效")
	ErrAdminPaymentOrderNotFound = errors.New("支付订单不存在")
	adminPaymentOrderNumber      = regexp.MustCompile(`^IBP-[0-9]{8}-[A-Z0-9]{12}$`)
	stripeDisputeIDPattern       = regexp.MustCompile(`^dp_[A-Za-z0-9]+$`)
)

type AdminPaymentOrderListInput struct {
	OrderNumber string
	User        string
	Status      PaymentOrderStatus
	AnomalyOnly bool
	Cursor      string
	Limit       int
}

type AdminPaymentOrderPage struct {
	Orders     []AdminPaymentOrderSummary `json:"orders"`
	Total      int64                      `json:"total"`
	NextCursor string                     `json:"next_cursor,omitempty"`
}

type AdminPaymentOrderUser struct {
	PublicID    uuid.UUID `json:"public_id"`
	DisplayName string    `json:"display_name"`
}

type AdminPaymentOrderSummary struct {
	PublicOrderNumber string                `json:"public_order_number"`
	User              AdminPaymentOrderUser `json:"user"`
	IdeaQuantity      int64                 `json:"idea_quantity"`
	BaseAmountMinor   int64                 `json:"base_amount_minor"`
	BaseCurrency      string                `json:"base_currency"`
	PaidAmountMinor   *int64                `json:"paid_amount_minor,omitempty"`
	PaidCurrency      string                `json:"paid_currency,omitempty"`
	Status            PaymentOrderStatus    `json:"status"`
	StripeStatus      string                `json:"stripe_status"`
	CreditStatus      string                `json:"credit_status"`
	CreatedAt         time.Time             `json:"created_at"`
	PaidAt            *time.Time            `json:"paid_at,omitempty"`
	CreditedAt        *time.Time            `json:"credited_at,omitempty"`
	LastCheckedAt     *time.Time            `json:"last_checked_at,omitempty"`
	AnomalousAt       *time.Time            `json:"anomalous_at,omitempty"`
}

type AdminPaymentOrderDetail struct {
	internalUserID         int64
	PublicOrderNumber      string                       `json:"public_order_number"`
	User                   AdminPaymentOrderUser        `json:"user"`
	IdeaQuantity           int64                        `json:"idea_quantity"`
	BaseAmountMinor        int64                        `json:"base_amount_minor"`
	BaseCurrency           string                       `json:"base_currency"`
	PaidSubtotalMinor      *int64                       `json:"paid_subtotal_minor,omitempty"`
	PaidTaxMinor           *int64                       `json:"paid_tax_minor,omitempty"`
	PaidTotalMinor         *int64                       `json:"paid_total_minor,omitempty"`
	PaidCurrency           string                       `json:"paid_currency,omitempty"`
	Status                 PaymentOrderStatus           `json:"status"`
	StripeStatus           string                       `json:"stripe_status"`
	CreditStatus           string                       `json:"credit_status"`
	Stripe                 AdminPaymentStripeSummary    `json:"stripe"`
	TermsVersion           string                       `json:"terms_version"`
	RefundPolicyVersion    string                       `json:"refund_policy_version"`
	ReceiptURL             string                       `json:"receipt_url,omitempty"`
	ExpiresAt              time.Time                    `json:"expires_at"`
	CreatedAt              time.Time                    `json:"created_at"`
	UpdatedAt              time.Time                    `json:"updated_at"`
	PaidAt                 *time.Time                   `json:"paid_at,omitempty"`
	CreditedAt             *time.Time                   `json:"credited_at,omitempty"`
	Reconciliation         AdminPaymentReconciliation   `json:"reconciliation"`
	Refunds                []AdminPaymentRefundSummary  `json:"refunds"`
	Disputes               []AdminPaymentDisputeSummary `json:"disputes"`
	RefundableIdeaQuantity int64                        `json:"refundable_idea_quantity"`
}

type AdminPaymentStripeSummary struct {
	AccountID         string `json:"account_id,omitempty"`
	CustomerID        string `json:"customer_id,omitempty"`
	CheckoutSessionID string `json:"checkout_session_id,omitempty"`
	PaymentIntentID   string `json:"payment_intent_id,omitempty"`
	ChargeID          string `json:"charge_id,omitempty"`
}

type AdminPaymentReconciliation struct {
	Anomalous        bool       `json:"anomalous"`
	AnomalousAt      *time.Time `json:"anomalous_at,omitempty"`
	LastCheckedAt    *time.Time `json:"last_checked_at,omitempty"`
	NextAttemptAt    *time.Time `json:"next_attempt_at,omitempty"`
	LastErrorCode    string     `json:"last_error_code,omitempty"`
	LastErrorSummary string     `json:"last_error_summary,omitempty"`
}

// AdminPaymentRefundSummary exposes durable Refund state without provider
// payloads, internal ledger identities, or payer PII.
type AdminPaymentRefundSummary struct {
	ID           string     `json:"id,omitempty"`
	Status       string     `json:"status"`
	IdeaQuantity int64      `json:"idea_quantity,omitempty"`
	AmountMinor  int64      `json:"amount_minor,omitempty"`
	Currency     string     `json:"currency,omitempty"`
	StripeStatus string     `json:"stripe_status,omitempty"`
	ErrorSummary string     `json:"error_summary,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

// AdminPaymentDisputeSummary follows the same safe event-fact boundary.
type AdminPaymentDisputeSummary struct {
	ID                  string     `json:"id,omitempty"`
	Status              string     `json:"status"`
	ProviderStatus      string     `json:"provider_status,omitempty"`
	AmountMinor         int64      `json:"amount_minor,omitempty"`
	Currency            string     `json:"currency,omitempty"`
	RatioBasisPoints    int64      `json:"ratio_basis_points,omitempty"`
	TargetIdeaQuantity  int64      `json:"target_idea_quantity,omitempty"`
	DebitedIdeaQuantity int64      `json:"debited_idea_quantity,omitempty"`
	UnrecoveredCredits  int64      `json:"unrecovered_credits,omitempty"`
	StripeDashboardURL  string     `json:"stripe_dashboard_url,omitempty"`
	OpenedAt            *time.Time `json:"opened_at,omitempty"`
	ClosedAt            *time.Time `json:"closed_at,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

type adminPaymentOrderCursor struct {
	CreatedAt string `json:"created_at"`
	OrderID   string `json:"order_id"`
}

type adminPaymentOrderFilters struct {
	orderNumber  string
	userPublicID uuid.UUID
	displayName  string
	status       string
	anomalyOnly  bool
	cursor       *adminPaymentOrderCursor
	limit        int
}

func (s *Service) ListAdminPaymentOrders(ctx context.Context, input AdminPaymentOrderListInput) (AdminPaymentOrderPage, error) {
	if s == nil || s.repo == nil {
		return AdminPaymentOrderPage{}, fmt.Errorf("payments admin read service unavailable")
	}
	filters, err := normalizeAdminPaymentOrderFilters(input)
	if err != nil {
		return AdminPaymentOrderPage{}, err
	}
	return s.repo.listAdminPaymentOrders(ctx, filters)
}

func (s *Service) GetAdminPaymentOrder(ctx context.Context, orderNumber string) (AdminPaymentOrderDetail, error) {
	if s == nil || s.repo == nil {
		return AdminPaymentOrderDetail{}, fmt.Errorf("payments admin read service unavailable")
	}
	orderNumber = strings.TrimSpace(orderNumber)
	if !adminPaymentOrderNumber.MatchString(orderNumber) {
		return AdminPaymentOrderDetail{}, ErrAdminPaymentOrderFilter
	}
	detail, events, err := s.repo.getAdminPaymentOrder(ctx, orderNumber)
	if err != nil {
		return AdminPaymentOrderDetail{}, err
	}
	durableRefunds, err := s.repo.ListPaymentRefundsForOrder(ctx, orderNumber)
	if err != nil {
		return AdminPaymentOrderDetail{}, err
	}
	durableStripeRefundIDs := make(map[string]struct{}, len(durableRefunds))
	for _, refund := range durableRefunds {
		updatedAt := refund.UpdatedAt
		detail.Refunds = append(detail.Refunds, AdminPaymentRefundSummary{ID: maskPaymentIdentity(refund.ID), Status: string(refund.Status), IdeaQuantity: refund.IdeaQuantity, AmountMinor: refund.AmountMinor, Currency: refund.Currency, StripeStatus: refund.StripeStatus, ErrorSummary: refund.ErrorSummary, UpdatedAt: &updatedAt})
		if refund.StripeRefundID != "" {
			durableStripeRefundIDs[refund.StripeRefundID] = struct{}{}
		}
	}
	durableDisputes, err := s.repo.ListPaymentDisputesForOrder(ctx, orderNumber)
	if err != nil {
		return AdminPaymentOrderDetail{}, err
	}
	disputeMode := StripeMode("")
	if len(durableDisputes) > 0 {
		configuration, configurationErr := s.configuration.GetStripeConfiguration(ctx)
		if configurationErr == nil {
			disputeMode = configuration.Mode
		} else if !isConfigurationNotFound(configurationErr) {
			return AdminPaymentOrderDetail{}, configurationErr
		}
	}
	durableStripeDisputeIDs := make(map[string]struct{}, len(durableDisputes))
	for _, dispute := range durableDisputes {
		updatedAt := dispute.UpdatedAt
		openedAt := dispute.OpenedAt
		detail.Disputes = append(detail.Disputes, AdminPaymentDisputeSummary{
			ID: maskPaymentIdentity(dispute.StripeDisputeID), Status: string(dispute.Status),
			ProviderStatus: dispute.ProviderStatus, AmountMinor: dispute.AmountMinor, Currency: dispute.Currency,
			RatioBasisPoints:   paymentDisputeRatioBasisPoints(dispute.AmountMinor, detail.PaidTotalMinor),
			TargetIdeaQuantity: dispute.TargetIdeaQuantity, DebitedIdeaQuantity: dispute.DebitedIdeaQuantity,
			UnrecoveredCredits: dispute.UnrecoveredCredits,
			StripeDashboardURL: stripeDisputeDashboardURL(disputeMode, dispute.StripeDisputeID),
			OpenedAt:           &openedAt, ClosedAt: dispute.ClosedAt, UpdatedAt: &updatedAt,
		})
		durableStripeDisputeIDs[dispute.StripeDisputeID] = struct{}{}
	}
	if detail.Status == PaymentOrderCredited && detail.PaidAt != nil && s.now().UTC().Before(detail.PaidAt.Add(refundEligibilityWindow)) && s.wallets != nil {
		remaining := detail.IdeaQuantity
		for _, refund := range durableRefunds {
			if refund.Status == PaymentRefundPending || refund.Status == PaymentRefundSucceeded || refund.Status == PaymentRefundAnomaly {
				remaining -= refund.IdeaQuantity
			}
		}
		for _, dispute := range durableDisputes {
			if dispute.Status == PaymentDisputeOpen || dispute.Status == PaymentDisputeLost {
				remaining -= dispute.TargetIdeaQuantity
			}
		}
		if remaining > 0 {
			wallet, walletErr := s.wallets.GetWallet(ctx, detail.internalUserID)
			if walletErr != nil && !errors.Is(walletErr, credits.ErrWalletNotFound) {
				return AdminPaymentOrderDetail{}, walletErr
			}
			if walletErr == nil {
				detail.RefundableIdeaQuantity = min(remaining, wallet.Available)
			}
		}
	}
	for _, event := range events {
		updatedAt := nullTimePointer(event.ProcessedAt)
		if updatedAt == nil {
			updatedAt = &event.ReceivedAt
		}
		switch event.EventType {
		case "refund.updated", "refund.failed":
			if _, durable := durableStripeRefundIDs[event.StripeObjectID]; durable {
				continue
			}
			detail.Refunds = append(detail.Refunds, AdminPaymentRefundSummary{
				ID: maskPaymentIdentity(event.StripeObjectID), Status: event.EventType, UpdatedAt: updatedAt,
			})
		case "charge.dispute.created", "charge.dispute.closed":
			if _, durable := durableStripeDisputeIDs[event.StripeObjectID]; durable {
				continue
			}
			detail.Disputes = append(detail.Disputes, AdminPaymentDisputeSummary{
				ID: maskPaymentIdentity(event.StripeObjectID), Status: event.EventType, UpdatedAt: updatedAt,
			})
		}
	}
	return detail, nil
}

func stripeDisputeDashboardURL(mode StripeMode, disputeID string) string {
	disputeID = strings.TrimSpace(disputeID)
	if !stripeDisputeIDPattern.MatchString(disputeID) {
		return ""
	}
	if mode == StripeModeTest {
		return "https://dashboard.stripe.com/test/disputes/" + disputeID
	}
	if mode == StripeModeLive {
		return "https://dashboard.stripe.com/disputes/" + disputeID
	}
	return ""
}

func paymentDisputeRatioBasisPoints(amountMinor int64, paidTotalMinor *int64) int64 {
	if amountMinor <= 0 || paidTotalMinor == nil || *paidTotalMinor <= 0 {
		return 0
	}
	ratio := new(big.Int).Mul(big.NewInt(amountMinor), big.NewInt(10_000))
	ratio.Quo(ratio, big.NewInt(*paidTotalMinor))
	if !ratio.IsInt64() {
		return 0
	}
	return ratio.Int64()
}

func normalizeAdminPaymentOrderFilters(input AdminPaymentOrderListInput) (adminPaymentOrderFilters, error) {
	filters := adminPaymentOrderFilters{
		orderNumber: strings.TrimSpace(input.OrderNumber),
		status:      string(input.Status),
		anomalyOnly: input.AnomalyOnly,
		limit:       input.Limit,
	}
	if filters.orderNumber != "" && !adminPaymentOrderNumber.MatchString(filters.orderNumber) {
		return adminPaymentOrderFilters{}, ErrAdminPaymentOrderFilter
	}
	if filters.status != "" && !validAdminPaymentOrderStatus(PaymentOrderStatus(filters.status)) {
		return adminPaymentOrderFilters{}, ErrAdminPaymentOrderFilter
	}
	if filters.limit == 0 {
		filters.limit = defaultAdminPaymentOrderLimit
	}
	if filters.limit < 1 || filters.limit > maxAdminPaymentOrderLimit {
		return adminPaymentOrderFilters{}, ErrAdminPaymentOrderFilter
	}
	user := strings.TrimSpace(input.User)
	if utf8.RuneCountInString(user) > maxAdminPaymentOrderUserLen {
		return adminPaymentOrderFilters{}, ErrAdminPaymentOrderFilter
	}
	if user != "" {
		if publicID, err := uuid.Parse(user); err == nil {
			filters.userPublicID = publicID
		} else if validAdminPaymentOrderDisplayName(user) {
			filters.displayName = user
		} else {
			return adminPaymentOrderFilters{}, ErrAdminPaymentOrderFilter
		}
	}
	cursor, err := decodeAdminPaymentOrderCursor(input.Cursor)
	if err != nil {
		return adminPaymentOrderFilters{}, err
	}
	filters.cursor = cursor
	return filters, nil
}

func validAdminPaymentOrderStatus(status PaymentOrderStatus) bool {
	switch status {
	case PaymentOrderCreating, PaymentOrderCheckoutOpen, PaymentOrderAsyncPaymentPending,
		PaymentOrderPaidPendingCredit, PaymentOrderCredited, PaymentOrderCancelled,
		PaymentOrderExpired, PaymentOrderFailed:
		return true
	default:
		return false
	}
}

func validAdminPaymentOrderDisplayName(value string) bool {
	return strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\x00\r\n")
}

func (r *Repository) listAdminPaymentOrders(ctx context.Context, filters adminPaymentOrderFilters) (AdminPaymentOrderPage, error) {
	if r == nil || r.db == nil {
		return AdminPaymentOrderPage{}, fmt.Errorf("payments repository: database unavailable")
	}
	db := r.dbWithContext(ctx)
	var page AdminPaymentOrderPage
	err := db.withReadSnapshot(func(q *dbsqlc.Queries) error {
		params := dbsqlc.CountAdminPaymentOrdersParams{
			OrderNumber: filters.orderNumber, HasUserPublicID: filters.userPublicID != uuid.Nil, UserPublicID: filters.userPublicID, DisplayName: filters.displayName,
			Status: filters.status, AnomalyOnly: filters.anomalyOnly,
		}
		total, err := q.CountAdminPaymentOrders(ctx, params)
		if err != nil {
			return fmt.Errorf("count admin payment orders: %w", err)
		}
		listParams := dbsqlc.ListAdminPaymentOrdersParams{
			OrderNumber: filters.orderNumber, HasUserPublicID: filters.userPublicID != uuid.Nil, UserPublicID: filters.userPublicID, DisplayName: filters.displayName,
			Status: filters.status, AnomalyOnly: filters.anomalyOnly, PageLimit: int32(filters.limit + 1),
		}
		if filters.cursor != nil {
			createdAt, err := time.Parse(time.RFC3339Nano, filters.cursor.CreatedAt)
			if err != nil {
				return ErrAdminPaymentOrderCursor
			}
			orderID, err := uuid.Parse(filters.cursor.OrderID)
			if err != nil {
				return ErrAdminPaymentOrderCursor
			}
			listParams.CursorSet = true
			listParams.CursorCreatedAt = sql.NullTime{Time: createdAt, Valid: true}
			listParams.CursorID = uuid.NullUUID{UUID: orderID, Valid: true}
		}
		rows, err := q.ListAdminPaymentOrders(ctx, listParams)
		if err != nil {
			return fmt.Errorf("list admin payment orders: %w", err)
		}
		page = AdminPaymentOrderPage{Orders: make([]AdminPaymentOrderSummary, 0, min(filters.limit, len(rows))), Total: total}
		if len(rows) > filters.limit {
			last := rows[filters.limit-1]
			page.NextCursor, err = encodeAdminPaymentOrderCursor(last.CreatedAt, last.ID)
			if err != nil {
				return fmt.Errorf("encode admin payment order cursor: %w", err)
			}
			rows = rows[:filters.limit]
		}
		for _, row := range rows {
			page.Orders = append(page.Orders, adminPaymentOrderSummary(row))
		}
		return nil
	})
	if err != nil {
		return AdminPaymentOrderPage{}, err
	}
	return page, nil
}

func (db paymentContextDB) withReadSnapshot(run func(*dbsqlc.Queries) error) (resultErr error) {
	if db.transactionStarter == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	tx, err := db.transactionStarter.BeginTx(db.ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin payments read snapshot: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && resultErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("rollback payments read snapshot: %w", rollbackErr))
		}
	}()
	if err := run(dbsqlc.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit payments read snapshot: %w", err)
	}
	return nil
}

func (r *Repository) getAdminPaymentOrder(ctx context.Context, orderNumber string) (AdminPaymentOrderDetail, []dbsqlc.ListAdminPaymentOrderWebhookEventsRow, error) {
	if r == nil || r.db == nil {
		return AdminPaymentOrderDetail{}, nil, fmt.Errorf("payments repository: database unavailable")
	}
	queries := r.dbWithContext(ctx).queries()
	row, err := queries.GetAdminPaymentOrder(ctx, orderNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminPaymentOrderDetail{}, nil, ErrAdminPaymentOrderNotFound
	}
	if err != nil {
		return AdminPaymentOrderDetail{}, nil, fmt.Errorf("get admin payment order: %w", err)
	}
	events, err := queries.ListAdminPaymentOrderWebhookEvents(ctx, dbsqlc.ListAdminPaymentOrderWebhookEventsParams{
		PaymentOrderID: row.ID,
		RowLimit:       50,
	})
	if err != nil {
		return AdminPaymentOrderDetail{}, nil, fmt.Errorf("list admin payment order provider events: %w", err)
	}
	return adminPaymentOrderDetail(row), events, nil
}

func adminPaymentOrderSummary(row dbsqlc.ListAdminPaymentOrdersRow) AdminPaymentOrderSummary {
	return AdminPaymentOrderSummary{
		PublicOrderNumber: row.PublicOrderNumber,
		User:              AdminPaymentOrderUser{PublicID: row.UserPublicID, DisplayName: row.DisplayName},
		IdeaQuantity:      row.IdeaQuantity,
		BaseAmountMinor:   row.BaseAmountMinor,
		BaseCurrency:      strings.ToLower(row.BaseCurrency),
		PaidAmountMinor:   nullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:      strings.ToLower(row.PaidCurrency),
		Status:            PaymentOrderStatus(row.Status),
		StripeStatus:      adminStripeStatus(PaymentOrderStatus(row.Status)),
		CreditStatus:      adminCreditStatus(PaymentOrderStatus(row.Status)),
		CreatedAt:         row.CreatedAt,
		PaidAt:            nullTimePointer(row.PaidAt),
		CreditedAt:        nullTimePointer(row.CreditedAt),
		LastCheckedAt:     nullTimePointer(row.LastCheckedAt),
		AnomalousAt:       nullTimePointer(row.AnomalousAt),
	}
}

func adminPaymentOrderDetail(row dbsqlc.GetAdminPaymentOrderRow) AdminPaymentOrderDetail {
	status := PaymentOrderStatus(row.Status)
	return AdminPaymentOrderDetail{
		internalUserID:    row.UserID,
		PublicOrderNumber: row.PublicOrderNumber,
		User:              AdminPaymentOrderUser{PublicID: row.UserPublicID, DisplayName: row.DisplayName},
		IdeaQuantity:      row.IdeaQuantity,
		BaseAmountMinor:   row.BaseAmountMinor,
		BaseCurrency:      strings.ToLower(row.BaseCurrency),
		PaidSubtotalMinor: nullInt64Pointer(row.PaidSubtotalMinor),
		PaidTaxMinor:      nullInt64Pointer(row.PaidTaxMinor),
		PaidTotalMinor:    nullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:      strings.ToLower(row.PaidCurrency),
		Status:            status,
		StripeStatus:      adminStripeStatus(status),
		CreditStatus:      adminCreditStatus(status),
		Stripe: AdminPaymentStripeSummary{
			AccountID: maskPaymentIdentity(row.StripeAccountID), CustomerID: maskPaymentIdentity(row.StripeCustomerID),
			CheckoutSessionID: maskPaymentIdentity(row.StripeCheckoutSessionID.String),
			PaymentIntentID:   maskPaymentIdentity(row.StripePaymentIntentID), ChargeID: maskPaymentIdentity(row.StripeChargeID),
		},
		TermsVersion: row.TermsVersion, RefundPolicyVersion: row.RefundPolicyVersion,
		ReceiptURL: row.ReceiptUrl, ExpiresAt: row.ExpiresAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		PaidAt: nullTimePointer(row.PaidAt), CreditedAt: nullTimePointer(row.CreditedAt),
		Reconciliation: AdminPaymentReconciliation{
			Anomalous: row.AnomalousAt.Valid, AnomalousAt: nullTimePointer(row.AnomalousAt),
			LastCheckedAt: nullTimePointer(row.LastCheckedAt), NextAttemptAt: nullTimePointer(row.NextAttemptAt),
			LastErrorCode: row.LastErrorCode.String, LastErrorSummary: row.LastErrorSummary.String,
		},
		Refunds:  []AdminPaymentRefundSummary{},
		Disputes: []AdminPaymentDisputeSummary{},
	}
}

func adminStripeStatus(status PaymentOrderStatus) string {
	switch status {
	case PaymentOrderCreating:
		return "creating"
	case PaymentOrderCheckoutOpen:
		return "open"
	case PaymentOrderAsyncPaymentPending:
		return "async_payment_pending"
	case PaymentOrderPaidPendingCredit, PaymentOrderCredited:
		return "paid"
	case PaymentOrderCancelled:
		return "cancelled"
	case PaymentOrderExpired:
		return "expired"
	case PaymentOrderFailed:
		return "payment_failed"
	default:
		return "unknown"
	}
}

func adminCreditStatus(status PaymentOrderStatus) string {
	switch status {
	case PaymentOrderPaidPendingCredit:
		return "pending"
	case PaymentOrderCredited:
		return "credited"
	default:
		return "not_applicable"
	}
}

func maskPaymentIdentity(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return ""
	}
	return value[:4] + "…" + value[len(value)-4:]
}

func decodeAdminPaymentOrderCursor(value string) (*adminPaymentOrderCursor, error) {
	if len(value) > maxAdminPaymentOrderCursorLen {
		return nil, ErrAdminPaymentOrderCursor
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != value {
		return nil, ErrAdminPaymentOrderCursor
	}
	var cursor adminPaymentOrderCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil || cursor.CreatedAt == "" || cursor.OrderID == "" {
		return nil, ErrAdminPaymentOrderCursor
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, ErrAdminPaymentOrderCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil || createdAt.UTC().Format(time.RFC3339Nano) != cursor.CreatedAt {
		return nil, ErrAdminPaymentOrderCursor
	}
	parsedID, err := uuid.Parse(cursor.OrderID)
	if err != nil || parsedID.String() != strings.ToLower(cursor.OrderID) {
		return nil, ErrAdminPaymentOrderCursor
	}
	return &cursor, nil
}

func encodeAdminPaymentOrderCursor(createdAt time.Time, orderID uuid.UUID) (string, error) {
	raw, err := json.Marshal(adminPaymentOrderCursor{CreatedAt: createdAt.UTC().Format(time.RFC3339Nano), OrderID: orderID.String()})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
