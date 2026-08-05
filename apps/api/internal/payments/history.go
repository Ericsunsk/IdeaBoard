package payments

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/google/uuid"
)

const (
	defaultRechargeHistoryLimit = 20
	maxRechargeHistoryLimit     = 20
	maxRechargeHistoryCursorLen = 512
)

var (
	ErrRechargeHistoryCursor = errors.New("充值记录游标无效")
	ErrRechargeHistoryLimit  = errors.New("充值记录 limit 无效")
)

// RechargeHistoryItem is the minimal user-visible payment fact. Stripe remains
// the receipt source, so no payer data or provider internals cross this boundary.
type RechargeHistoryItem struct {
	OccurredAt           time.Time `json:"occurred_at"`
	IdeaQuantity         int64     `json:"idea_quantity"`
	PaidAmountMinor      int64     `json:"paid_amount_minor"`
	PaidCurrency         string    `json:"paid_currency"`
	Status               string    `json:"status"`
	RefundedIdeaQuantity int64     `json:"refunded_idea_quantity,omitempty"`
	RefundedAmountMinor  int64     `json:"refunded_amount_minor,omitempty"`
	ReceiptURL           string    `json:"receipt_url,omitempty"`
}

type RechargeHistoryPage struct {
	Items      []RechargeHistoryItem `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

type rechargeHistoryCursor struct {
	OccurredAt string `json:"occurred_at"`
	OrderID    string `json:"order_id"`
}

func (s *Service) ListRecharges(ctx context.Context, userID int64, cursor string, limit int) (RechargeHistoryPage, error) {
	if s == nil || s.repo == nil || userID <= 0 {
		return RechargeHistoryPage{}, ErrPaymentOrderNotFound
	}
	if limit == 0 {
		limit = defaultRechargeHistoryLimit
	}
	if limit < 1 || limit > maxRechargeHistoryLimit {
		return RechargeHistoryPage{}, fmt.Errorf("%w：必须在 1 到 %d 之间", ErrRechargeHistoryLimit, maxRechargeHistoryLimit)
	}

	parsedCursor, err := decodeRechargeHistoryCursor(cursor)
	if err != nil {
		return RechargeHistoryPage{}, err
	}
	cutoff := s.now().UTC().AddDate(0, -12, 0)
	rows, err := s.repo.listPaymentRechargeHistory(ctx, userID, cutoff, parsedCursor, limit+1)
	if err != nil {
		return RechargeHistoryPage{}, fmt.Errorf("list recharge history: %w", err)
	}
	page := RechargeHistoryPage{Items: make([]RechargeHistoryItem, 0, min(limit, len(rows)))}
	if len(rows) > limit {
		last := rows[limit-1]
		page.NextCursor, err = encodeRechargeHistoryCursor(last.OccurredAt.Time, last.ID)
		if err != nil {
			return RechargeHistoryPage{}, fmt.Errorf("encode recharge history cursor: %w", err)
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		if !row.OccurredAt.Valid || !row.PaidTotalMinor.Valid {
			return RechargeHistoryPage{}, fmt.Errorf("invalid credited payment order history row")
		}
		status := "credited"
		if row.RefundedIdeaQuantity >= row.IdeaQuantity {
			status = "refunded"
		}
		page.Items = append(page.Items, RechargeHistoryItem{
			OccurredAt: row.OccurredAt.Time, IdeaQuantity: row.IdeaQuantity,
			PaidAmountMinor: row.PaidTotalMinor.Int64, PaidCurrency: strings.ToLower(row.PaidCurrency),
			Status: status, RefundedIdeaQuantity: row.RefundedIdeaQuantity,
			RefundedAmountMinor: row.RefundedAmountMinor, ReceiptURL: row.ReceiptUrl,
		})
	}
	return page, nil
}

func (r *Repository) listPaymentRechargeHistory(
	ctx context.Context,
	userID int64,
	cutoff time.Time,
	cursor *rechargeHistoryCursor,
	limit int,
) ([]dbsqlc.ListPaymentRechargeHistoryRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	params := dbsqlc.ListPaymentRechargeHistoryParams{
		UserID: userID, CutoffAt: cutoff, PageLimit: int32(limit),
	}
	if cursor != nil {
		occurredAt, err := time.Parse(time.RFC3339Nano, cursor.OccurredAt)
		if err != nil {
			return nil, ErrRechargeHistoryCursor
		}
		id, err := uuid.Parse(cursor.OrderID)
		if err != nil {
			return nil, ErrRechargeHistoryCursor
		}
		params.CursorSet = true
		params.CursorPaidAt = sql.NullTime{Time: occurredAt, Valid: true}
		params.CursorID = uuid.NullUUID{UUID: id, Valid: true}
	}
	rows, err := r.dbWithContext(ctx).queries().ListPaymentRechargeHistory(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list payment recharge history: %w", err)
	}
	return rows, nil
}

func decodeRechargeHistoryCursor(value string) (*rechargeHistoryCursor, error) {
	if len(value) > maxRechargeHistoryCursorLen {
		return nil, ErrRechargeHistoryCursor
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, ErrRechargeHistoryCursor
	}
	var cursor rechargeHistoryCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.OccurredAt == "" || cursor.OrderID == "" {
		return nil, ErrRechargeHistoryCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.OccurredAt); err != nil {
		return nil, ErrRechargeHistoryCursor
	}
	if _, err := uuid.Parse(cursor.OrderID); err != nil {
		return nil, ErrRechargeHistoryCursor
	}
	return &cursor, nil
}

func encodeRechargeHistoryCursor(occurredAt time.Time, orderID uuid.UUID) (string, error) {
	raw, err := json.Marshal(rechargeHistoryCursor{OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), OrderID: orderID.String()})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
