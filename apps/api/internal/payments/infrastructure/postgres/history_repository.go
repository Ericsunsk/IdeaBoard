package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"

	"github.com/google/uuid"
)

func (r *Repository) ListPaymentRechargeHistory(
	ctx context.Context,
	userID int64,
	cutoff time.Time,
	cursor *paymentreadmodel.RechargeHistoryCursor,
	limit int,
) ([]dbsqlc.ListPaymentRechargeHistoryRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	params := dbsqlc.ListPaymentRechargeHistoryParams{UserID: userID, CutoffAt: cutoff, PageLimit: int32(limit)}
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
		params.CursorPaidAt = sql.NullTime{Time: occurredAt, Valid: true}
		params.CursorID = uuid.NullUUID{UUID: id, Valid: true}
	}
	rows, err := r.dbWithContext(ctx).queries().ListPaymentRechargeHistory(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list payment recharge history: %w", err)
	}
	return rows, nil
}
