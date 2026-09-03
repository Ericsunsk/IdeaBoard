package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentports "ideaboard/api/internal/payments/ports"
)

func (r *Repository) InsertStripeWebhookEvent(ctx context.Context, event paymentports.StripeWebhookEvent, result string, processedAt *time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).queries().InsertPaymentWebhookEvent(ctx, dbsqlc.InsertPaymentWebhookEventParams{
		StripeEventID: event.ID, EventType: event.Type, StripeObjectID: event.ObjectID,
		PaymentIntentID: event.PaymentIntentID, ChargeID: event.ChargeID,
		Livemode: event.Livemode, StripeCreatedAt: event.CreatedAt, ProcessingResult: result,
		ProcessedAt: dbutil.NullTime(processedAt),
	})
	if err != nil {
		return false, fmt.Errorf("insert Stripe webhook event: %w", err)
	}
	return rows == 0, nil
}

func (r *Repository) MarkStripeWebhookEventProcessed(ctx context.Context, eventID, result, errorSummary string, processedAt time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).queries().MarkPaymentWebhookEventProcessed(ctx, dbsqlc.MarkPaymentWebhookEventProcessedParams{
		StripeEventID: eventID, ProcessingResult: result, ErrorSummary: boundedText(errorSummary, 500),
		ProcessedAt: sql.NullTime{Time: processedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("mark Stripe webhook event processed: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("mark Stripe webhook event processed: event not found")
	}
	return nil
}
