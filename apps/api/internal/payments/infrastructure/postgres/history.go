package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) ListPaymentRechargeHistory(
	ctx context.Context,
	userID int64,
	cutoff time.Time,
	cursor *paymentreadmodel.RechargeHistoryCursor,
	limit int,
) ([]paymentreadmodel.RechargeHistoryRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	params := dbsqlc.ListPaymentRechargeHistoryParams{UserID: userID, CutoffAt: dbutil.TimestamptzFromTime(cutoff), PageLimit: int32(limit)}
	if cursor != nil {
		occurredAt, err := time.Parse(time.RFC3339Nano, cursor.OccurredAt)
		if err != nil {
			return nil, paymentcontracts.ErrRechargeHistoryCursor
		}
		id, err := uuid.Parse(cursor.OrderID)
		if err != nil {
			return nil, paymentcontracts.ErrRechargeHistoryCursor
		}
		params.CursorSet = true
		params.CursorPaidAt = pgtype.Timestamptz{Time: occurredAt, Valid: true}
		params.CursorID = pgtype.UUID{Bytes: id, Valid: true}
	}
	rows, err := r.DBWithContext(ctx).Queries().ListPaymentRechargeHistory(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list payment recharge history: %w", err)
	}
	records := make([]paymentreadmodel.RechargeHistoryRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, decodeRechargeHistoryRecord(row))
	}
	return records, nil
}

func decodeRechargeHistoryRecord(row dbsqlc.ListPaymentRechargeHistoryRow) paymentreadmodel.RechargeHistoryRecord {
	return paymentreadmodel.RechargeHistoryRecord{
		OrderID:              uuid.UUID(row.ID.Bytes),
		OccurredAt:           dbutil.TimeValue(row.OccurredAt),
		IdeaQuantity:         row.IdeaQuantity,
		Status:               row.Status,
		PaidTotalMinor:       NullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:         strings.ToLower(row.PaidCurrency),
		ReceiptURL:           row.ReceiptUrl,
		RefundedIdeaQuantity: row.RefundedIdeaQuantity,
		RefundedAmountMinor:  row.RefundedAmountMinor,
	}
}
