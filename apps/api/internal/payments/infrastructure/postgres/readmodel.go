package postgres

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcheckout "ideaboard/api/internal/payments/application/checkout"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) GetStripeConfiguration(ctx context.Context) (paymentconfiguration.StoredStripeConfiguration, error) {
	if r == nil || r.db == nil {
		return paymentconfiguration.StoredStripeConfiguration{}, fmt.Errorf("payments repository: database unavailable")
	}
	q := r.dbWithContext(ctx).queries()
	settings, err := q.GetPaymentStripeConfiguration(ctx)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, err
	}
	credential, err := q.GetPaymentStripeCredential(ctx, settings.ActiveCredentialID)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, err
	}
	return storedStripeConfiguration(settings, credential), nil
}

func (r *Repository) GetStripeCredential(ctx context.Context, credentialID string) (paymentconfiguration.StoredStripeConfiguration, error) {
	id, err := uuid.Parse(credentialID)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, database.ErrNotFound
	}
	credential, err := r.dbWithContext(ctx).queries().GetPaymentStripeCredential(ctx, pgUUIDToPgType(id))
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, err
	}
	return storedStripeCredential(credential), nil
}

func (r *Repository) GetStripeCredentialForOrder(ctx context.Context, orderID string) (paymentconfiguration.StoredStripeConfiguration, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, database.ErrNotFound
	}
	credential, err := r.dbWithContext(ctx).queries().GetPaymentStripeCredentialForOrder(ctx, pgUUIDToPgType(id))
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, err
	}
	return storedStripeCredential(credential), nil
}

func (r *Repository) GetStripeCredentialForOrderNumber(ctx context.Context, orderNumber string) (paymentconfiguration.StoredStripeConfiguration, error) {
	credential, err := r.dbWithContext(ctx).queries().GetPaymentStripeCredentialForOrderNumber(ctx, orderNumber)
	if err != nil {
		return paymentconfiguration.StoredStripeConfiguration{}, err
	}
	return storedStripeCredential(credential), nil
}

func (r *Repository) ListStripeCredentials(ctx context.Context) ([]paymentconfiguration.StoredStripeConfiguration, error) {
	rows, err := r.dbWithContext(ctx).queries().ListPaymentStripeCredentials(ctx)
	if err != nil {
		return nil, err
	}
	credentials := make([]paymentconfiguration.StoredStripeConfiguration, 0, len(rows))
	for _, row := range rows {
		credentials = append(credentials, storedStripeCredential(row))
	}
	return credentials, nil
}

func (r *Repository) GetPaymentCheckoutShutdown(ctx context.Context, id string) (paymentconfiguration.PaymentCheckoutShutdown, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return paymentconfiguration.PaymentCheckoutShutdown{}, pgx.ErrNoRows
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentCheckoutShutdown(ctx, pgUUIDToPgType(parsed))
	if err != nil {
		return paymentconfiguration.PaymentCheckoutShutdown{}, err
	}
	return paymentCheckoutShutdown(row), nil
}

func (r *Repository) ListActivePaymentCheckoutShutdowns(ctx context.Context) ([]paymentconfiguration.PaymentCheckoutShutdown, error) {
	rows, err := r.dbWithContext(ctx).queries().ListActivePaymentCheckoutShutdowns(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]paymentconfiguration.PaymentCheckoutShutdown, 0, len(rows))
	for _, row := range rows {
		items = append(items, paymentCheckoutShutdown(row))
	}
	return items, nil
}

func (r *Repository) ListPaymentOrdersForCheckoutShutdown(ctx context.Context, shutdown paymentconfiguration.PaymentCheckoutShutdown, limit int32) ([]paymentsdomain.PaymentOrder, error) {
	if limit < 1 {
		return nil, fmt.Errorf("invalid Checkout shutdown batch limit")
	}
	rows, err := r.dbWithContext(ctx).queries().ListPaymentOrdersForCheckoutShutdown(ctx, dbsqlc.ListPaymentOrdersForCheckoutShutdownParams{
		StripeAccountID: shutdown.StripeAccountID, StripeCredentialID: pgtype.UUID{Bytes: uuid.MustParse(shutdown.StripeCredentialID), Valid: true},
		CreatedAt: dbutil.TimestamptzFromTime(shutdown.CutoffAt), Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list Checkout shutdown orders: %w", err)
	}
	items := make([]paymentsdomain.PaymentOrder, 0, len(rows))
	for _, row := range rows {
		items = append(items, PaymentOrder(row))
	}
	return items, nil
}

func (r *Repository) GetPaymentCustomer(ctx context.Context, userID int64, accountID string, mode paymentports.StripeMode) (paymentcheckout.PaymentCustomer, error) {
	if r == nil || r.db == nil {
		return paymentcheckout.PaymentCustomer{}, fmt.Errorf("payments repository: database unavailable")
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentCustomer(ctx, dbsqlc.GetPaymentCustomerParams{
		UserID: userID, StripeAccountID: accountID, StripeMode: string(mode),
	})
	if err != nil {
		return paymentcheckout.PaymentCustomer{}, err
	}
	return paymentcheckout.PaymentCustomer{
		UserID: row.UserID, StripeAccountID: row.StripeAccountID, StripeMode: paymentports.StripeMode(row.StripeMode), StripeCustomerID: row.StripeCustomerID,
		CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}, nil
}

func (r *Repository) GetPaymentOrderByPublicToken(ctx context.Context, userID int64, token string) (paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	publicToken, err := ParseUUID(token)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, database.ErrNotFound
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentOrderByPublicToken(ctx, dbsqlc.GetPaymentOrderByPublicTokenParams{
		PublicToken: pgUUIDToPgType(publicToken), UserID: userID,
	})
	if err != nil {
		return paymentsdomain.PaymentOrder{}, err
	}
	return PaymentOrder(row), nil
}

func (r *Repository) GetPaymentOrderByID(ctx context.Context, orderID string) (paymentsdomain.PaymentOrder, error) {
	id, err := ParseUUID(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, database.ErrNotFound
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentOrderByID(ctx, pgUUIDToPgType(id))
	if err != nil {
		return paymentsdomain.PaymentOrder{}, err
	}
	return PaymentOrder(row), nil
}

func (r *Repository) GetPaymentOrderByCheckoutSession(ctx context.Context, accountID, sessionID string) (paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil || strings.TrimSpace(accountID) == "" || strings.TrimSpace(sessionID) == "" {
		return paymentsdomain.PaymentOrder{}, database.ErrNotFound
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentOrderByCheckoutSession(ctx, dbsqlc.GetPaymentOrderByCheckoutSessionParams{
		StripeAccountID:         accountID,
		StripeCheckoutSessionID: pgtype.Text{String: sessionID, Valid: true},
	})
	if err != nil {
		return paymentsdomain.PaymentOrder{}, err
	}
	return PaymentOrder(row), nil
}

func (r *Repository) ListActivePaymentOrders(ctx context.Context, userID int64) ([]paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return nil, fmt.Errorf("payments repository: invalid active order query")
	}
	rows, err := r.dbWithContext(ctx).queries().ListActivePaymentOrders(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active payment orders: %w", err)
	}
	orders := make([]paymentsdomain.PaymentOrder, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, PaymentOrder(row))
	}
	return orders, nil
}

func (r *Repository) ListUserPendingPaymentOrders(ctx context.Context, userID int64) ([]paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return nil, fmt.Errorf("payments repository: invalid pending order query")
	}
	rows, err := r.dbWithContext(ctx).queries().ListUserPendingPaymentOrders(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user pending payment orders: %w", err)
	}
	orders := make([]paymentsdomain.PaymentOrder, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, PaymentOrder(row))
	}
	return orders, nil
}

func (r *Repository) ListUnsettledPaymentOrders(ctx context.Context) ([]paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).queries().ListUnsettledPaymentOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsettled payment orders: %w", err)
	}
	orders := make([]paymentsdomain.PaymentOrder, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, PaymentOrder(row))
	}
	return orders, nil
}
