package postgres

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"

	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) InsertStripeWebhookEvent(ctx context.Context, event paymentports.StripeWebhookEvent, result string, processedAt *time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.DBWithContext(ctx).Queries().InsertPaymentWebhookEvent(ctx, dbsqlc.InsertPaymentWebhookEventParams{
		StripeEventID: event.ID, EventType: event.Type, StripeObjectID: event.ObjectID,
		PaymentIntentID: event.PaymentIntentID, ChargeID: event.ChargeID,
		Livemode: event.Livemode, StripeCreatedAt: dbutil.TimestamptzFromTime(event.CreatedAt), ProcessingResult: result,
		ProcessedAt: dbutil.NullTime(processedAt),
	})
	if err != nil {
		return false, fmt.Errorf("insert Stripe webhook event: %w", err)
	}
	return rows == 0, nil
}

func (r *Repository) GetStripeWebhookEvent(ctx context.Context, eventID string) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("payments repository: database unavailable")
	}
	event, err := r.DBWithContext(ctx).Queries().GetPaymentWebhookEvent(ctx, eventID)
	if err != nil {
		return "", fmt.Errorf("get Stripe webhook event: %w", err)
	}
	return event.ProcessingResult, nil
}

func (r *Repository) ClaimStripeWebhookEvent(ctx context.Context, eventID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.DBWithContext(ctx).Queries().ClaimPaymentWebhookEvent(ctx, eventID)
	if err != nil {
		return false, fmt.Errorf("claim Stripe webhook event: %w", err)
	}
	return rows == 1, nil
}

func (r *Repository) MarkStripeWebhookEventProcessed(ctx context.Context, eventID, result, errorSummary string, processedAt time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.DBWithContext(ctx).Queries().MarkPaymentWebhookEventProcessed(ctx, dbsqlc.MarkPaymentWebhookEventProcessedParams{
		StripeEventID: eventID, ProcessingResult: result, ErrorSummary: BoundedText(errorSummary, 500),
		ProcessedAt: pgtype.Timestamptz{Time: processedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("mark Stripe webhook event processed: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("mark Stripe webhook event processed: event not found")
	}
	return nil
}
