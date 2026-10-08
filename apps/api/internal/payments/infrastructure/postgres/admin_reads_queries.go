package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) ListAdminPaymentOrders(ctx context.Context, filters paymentreadmodel.AdminPaymentOrderFilters) ([]paymentreadmodel.AdminPaymentOrderRecord, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, fmt.Errorf("payments repository: database unavailable")
	}
	db := r.dbWithContext(ctx)
	var records []paymentreadmodel.AdminPaymentOrderRecord
	var total int64
	err := db.withReadSnapshot(func(q *dbsqlc.Queries) error {
		params := dbsqlc.CountAdminPaymentOrdersParams{
			OrderNumber: filters.OrderNumber, HasUserPublicID: filters.UserPublicID != uuid.Nil, UserPublicID: pgtype.UUID{Bytes: filters.UserPublicID, Valid: filters.UserPublicID != uuid.Nil}, DisplayName: filters.DisplayName,
			Status: filters.Status, AnomalyOnly: filters.AnomalyOnly,
		}
		count, err := q.CountAdminPaymentOrders(ctx, params)
		if err != nil {
			return fmt.Errorf("count admin payment orders: %w", err)
		}
		total = count
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
		records = make([]paymentreadmodel.AdminPaymentOrderRecord, 0, len(rows))
		for _, row := range rows {
			records = append(records, decodeAdminPaymentOrderRecord(row))
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return records, total, nil
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

func (r *Repository) GetAdminPaymentOrder(ctx context.Context, orderNumber string) (paymentreadmodel.AdminPaymentOrderDetailFacts, []paymentreadmodel.AdminPaymentWebhookEventRecord, error) {
	if r == nil || r.db == nil {
		return paymentreadmodel.AdminPaymentOrderDetailFacts{}, nil, fmt.Errorf("payments repository: database unavailable")
	}
	queries := r.dbWithContext(ctx).queries()
	row, err := queries.GetAdminPaymentOrder(ctx, orderNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return paymentreadmodel.AdminPaymentOrderDetailFacts{}, nil, paymentcontracts.ErrAdminPaymentOrderNotFound
	}
	if err != nil {
		return paymentreadmodel.AdminPaymentOrderDetailFacts{}, nil, fmt.Errorf("get admin payment order: %w", err)
	}
	events, err := queries.ListAdminPaymentOrderWebhookEvents(ctx, dbsqlc.ListAdminPaymentOrderWebhookEventsParams{
		PaymentOrderID: row.ID,
		RowLimit:       50,
	})
	if err != nil {
		return paymentreadmodel.AdminPaymentOrderDetailFacts{}, nil, fmt.Errorf("list admin payment order provider events: %w", err)
	}
	records := make([]paymentreadmodel.AdminPaymentWebhookEventRecord, 0, len(events))
	for _, event := range events {
		records = append(records, decodeAdminPaymentWebhookEventRecord(event))
	}
	return decodeAdminPaymentOrderDetailFacts(row), records, nil
}
