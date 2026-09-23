package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) ListAdminPaymentOrders(ctx context.Context, filters paymentreadmodel.AdminPaymentOrderFilters) (paymentcontracts.AdminPaymentOrderPage, error) {
	if r == nil || r.db == nil {
		return paymentcontracts.AdminPaymentOrderPage{}, fmt.Errorf("payments repository: database unavailable")
	}
	db := r.dbWithContext(ctx)
	var page paymentcontracts.AdminPaymentOrderPage
	err := db.withReadSnapshot(func(q *dbsqlc.Queries) error {
		params := dbsqlc.CountAdminPaymentOrdersParams{
			OrderNumber: filters.OrderNumber, HasUserPublicID: filters.UserPublicID != uuid.Nil, UserPublicID: pgtype.UUID{Bytes: filters.UserPublicID, Valid: filters.UserPublicID != uuid.Nil}, DisplayName: filters.DisplayName,
			Status: filters.Status, AnomalyOnly: filters.AnomalyOnly,
		}
		total, err := q.CountAdminPaymentOrders(ctx, params)
		if err != nil {
			return fmt.Errorf("count admin payment orders: %w", err)
		}
		listParams := dbsqlc.ListAdminPaymentOrdersParams{
			OrderNumber: filters.OrderNumber, HasUserPublicID: filters.UserPublicID != uuid.Nil, UserPublicID: pgtype.UUID{Bytes: filters.UserPublicID, Valid: filters.UserPublicID != uuid.Nil}, DisplayName: filters.DisplayName,
			Status: filters.Status, AnomalyOnly: filters.AnomalyOnly, PageLimit: int32(filters.Limit + 1),
		}
		if filters.Cursor != nil {
			createdAt, err := time.Parse(time.RFC3339Nano, filters.Cursor.CreatedAt)
			if err != nil {
				return paymentcontracts.ErrAdminPaymentOrderCursor
			}
			orderID, err := uuid.Parse(filters.Cursor.OrderID)
			if err != nil {
				return paymentcontracts.ErrAdminPaymentOrderCursor
			}
			listParams.CursorSet = true
			listParams.CursorCreatedAt = pgtype.Timestamptz{Time: createdAt, Valid: true}
			listParams.CursorID = pgtype.UUID{Bytes: orderID, Valid: true}
		}
		rows, err := q.ListAdminPaymentOrders(ctx, listParams)
		if err != nil {
			return fmt.Errorf("list admin payment orders: %w", err)
		}
		page = paymentcontracts.AdminPaymentOrderPage{Orders: make([]paymentcontracts.AdminPaymentOrderSummary, 0, min(filters.Limit, len(rows))), Total: total}
		if len(rows) > filters.Limit {
			last := rows[filters.Limit-1]
			page.NextCursor, err = paymentreadmodel.EncodeAdminPaymentOrderCursor(dbutil.TimeValue(last.CreatedAt), uuid.UUID(last.ID.Bytes))
			if err != nil {
				return fmt.Errorf("encode admin payment order cursor: %w", err)
			}
			rows = rows[:filters.Limit]
		}
		for _, row := range rows {
			page.Orders = append(page.Orders, adminPaymentOrderSummary(row))
		}
		return nil
	})
	if err != nil {
		return paymentcontracts.AdminPaymentOrderPage{}, err
	}
	return page, nil
}

func (db paymentContextDB) withReadSnapshot(run func(*dbsqlc.Queries) error) (resultErr error) {
	if db.transactionStarter == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	tx, err := db.transactionStarter.BeginTx(db.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin payments read snapshot: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(db.ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && resultErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("rollback payments read snapshot: %w", rollbackErr))
		}
	}()
	if err := run(dbsqlc.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(db.ctx); err != nil {
		return fmt.Errorf("commit payments read snapshot: %w", err)
	}
	return nil
}

func (r *Repository) GetAdminPaymentOrder(ctx context.Context, orderNumber string) (paymentreadmodel.AdminPaymentOrderDetailRecord, []dbsqlc.ListAdminPaymentOrderWebhookEventsRow, error) {
	if r == nil || r.db == nil {
		return paymentreadmodel.AdminPaymentOrderDetailRecord{}, nil, fmt.Errorf("payments repository: database unavailable")
	}
	queries := r.dbWithContext(ctx).queries()
	row, err := queries.GetAdminPaymentOrder(ctx, orderNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return paymentreadmodel.AdminPaymentOrderDetailRecord{}, nil, paymentcontracts.ErrAdminPaymentOrderNotFound
	}
	if err != nil {
		return paymentreadmodel.AdminPaymentOrderDetailRecord{}, nil, fmt.Errorf("get admin payment order: %w", err)
	}
	events, err := queries.ListAdminPaymentOrderWebhookEvents(ctx, dbsqlc.ListAdminPaymentOrderWebhookEventsParams{
		PaymentOrderID: row.ID,
		RowLimit:       50,
	})
	if err != nil {
		return paymentreadmodel.AdminPaymentOrderDetailRecord{}, nil, fmt.Errorf("list admin payment order provider events: %w", err)
	}
	return adminPaymentOrderDetail(row), events, nil
}
