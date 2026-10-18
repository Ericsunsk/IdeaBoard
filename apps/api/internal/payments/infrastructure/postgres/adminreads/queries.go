package adminreads

import (
	"context"
	"errors"
	"fmt"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	postgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) ListAdminPaymentOrders(ctx context.Context, filters paymentreadmodel.AdminPaymentOrderFilters) ([]paymentreadmodel.AdminPaymentOrderRecord, int64, error) {
	if s == nil || !s.Available() {
		return nil, 0, fmt.Errorf("payments repository: database unavailable")
	}
	db := s.DBWithContext(ctx)
	var records []paymentreadmodel.AdminPaymentOrderRecord
	var total int64
	err := db.WithReadSnapshot(func(q *dbsqlc.Queries) error {
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

func (s *Store) GetAdminPaymentOrder(ctx context.Context, orderNumber string) (paymentreadmodel.AdminPaymentOrderDetailFacts, []paymentreadmodel.AdminPaymentWebhookEventRecord, error) {
	if s == nil || !s.Available() {
		return paymentreadmodel.AdminPaymentOrderDetailFacts{}, nil, fmt.Errorf("payments repository: database unavailable")
	}
	queries := s.DBWithContext(ctx).Queries()
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

// merged from internal/payments/infrastructure/postgres/adminreads/store.go (structural consolidation; symbols unchanged).
// Store owns the admin order readmodel. It embeds the root *postgres.Repository
// so context-bound execution (pool, advisory-lock connection, guard carrier)
// is shared, while admin-read methods live only here.
type Store struct {
	*postgres.Repository
}

// NewStore wraps a root payments repository with the admin-read aggregate.
func NewStore(repo *postgres.Repository) *Store {
	return &Store{Repository: repo}
}
