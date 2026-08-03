package payments

import (
	"context"
	"fmt"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/google/uuid"
)

const (
	paymentReconciliationBatchSize = int32(100)
)

// PaymentReconciliationClaim fences a short-lived worker lease. External Stripe
// calls happen after this claim transaction commits; completion must present the
// same token and generation so a stale worker cannot overwrite a later outcome.
type PaymentReconciliationClaim struct {
	Order      PaymentOrder
	Token      string
	Generation int64
}

type PaymentReconciliationOutcome struct {
	CheckedAt     time.Time
	NextAttemptAt time.Time
	ErrorCode     string
	ErrorSummary  string
	Anomalous     bool
	ClearAnomaly  bool
}

type PaymentReconciliationAnomaly struct {
	PaymentOrderID    string
	PublicOrderNumber string
	UserID            int64
	Status            PaymentOrderStatus
	LastErrorCode     string
	LastErrorSummary  string
	LastCheckedAt     *time.Time
	AnomalousAt       time.Time
	CreatedAt         time.Time
}

func (r *Repository) ClaimDuePaymentOrderReconciliations(ctx context.Context, now time.Time) ([]PaymentReconciliationClaim, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	now = now.UTC()
	token := uuid.New()
	rows, err := r.dbWithContext(ctx).queries().ClaimDuePaymentOrderReconciliations(ctx, dbsqlc.ClaimDuePaymentOrderReconciliationsParams{
		StaleBefore: now.Add(-2 * time.Minute), NowAt: now, RowLimit: paymentReconciliationBatchSize,
		ClaimToken: token, ClaimUntil: now.Add(paymentReconciliationLease),
	})
	if err != nil {
		return nil, fmt.Errorf("claim due payment order reconciliations: %w", err)
	}
	claims := make([]PaymentReconciliationClaim, 0, len(rows))
	for _, row := range rows {
		claims = append(claims, PaymentReconciliationClaim{
			Order: paymentOrder(row.PaymentOrder), Token: token.String(), Generation: row.ClaimGeneration,
		})
	}
	return claims, nil
}

func (r *Repository) CompletePaymentOrderReconciliationClaim(
	ctx context.Context,
	claim PaymentReconciliationClaim,
	outcome PaymentReconciliationOutcome,
) (bool, error) {
	if r == nil || r.db == nil || claim.Order.ID == "" || claim.Generation <= 0 {
		return false, fmt.Errorf("payments repository: invalid reconciliation claim")
	}
	orderID, err := parseOrderUUID(claim.Order.ID)
	if err != nil {
		return false, fmt.Errorf("parse reconciliation payment order ID: %w", err)
	}
	token, err := parseOrderUUID(claim.Token)
	if err != nil {
		return false, fmt.Errorf("parse reconciliation claim token: %w", err)
	}
	checkedAt := outcome.CheckedAt.UTC()
	if checkedAt.IsZero() {
		return false, fmt.Errorf("payments repository: reconciliation check time is required")
	}
	rows, err := r.dbWithContext(ctx).queries().CompletePaymentOrderReconciliationClaim(ctx, dbsqlc.CompletePaymentOrderReconciliationClaimParams{
		CheckedAt: checkedAt, NextAttemptAt: outcome.NextAttemptAt.UTC(),
		ErrorCode: boundedText(outcome.ErrorCode, 64), ErrorSummary: boundedText(outcome.ErrorSummary, 500),
		ClearAnomaly: outcome.ClearAnomaly, MarkAnomalous: outcome.Anomalous,
		PaymentOrderID: orderID, ClaimToken: token, ClaimGeneration: claim.Generation,
	})
	if err != nil {
		return false, fmt.Errorf("complete payment order reconciliation claim: %w", err)
	}
	return rows == 1, nil
}

func (r *Repository) ListPaymentReconciliationAnomalies(ctx context.Context, limit int32) ([]PaymentReconciliationAnomaly, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	if limit <= 0 || limit > paymentReconciliationBatchSize {
		limit = paymentReconciliationBatchSize
	}
	rows, err := r.dbWithContext(ctx).queries().ListPaymentReconciliationAnomalies(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list payment reconciliation anomalies: %w", err)
	}
	items := make([]PaymentReconciliationAnomaly, 0, len(rows))
	for _, row := range rows {
		if !row.AnomalousAt.Valid {
			continue
		}
		items = append(items, PaymentReconciliationAnomaly{
			PaymentOrderID: row.ID.String(), PublicOrderNumber: row.PublicOrderNumber, UserID: row.UserID,
			Status: PaymentOrderStatus(row.Status), LastErrorCode: row.LastErrorCode, LastErrorSummary: row.LastErrorSummary,
			LastCheckedAt: nullTimePointer(row.LastCheckedAt), AnomalousAt: row.AnomalousAt.Time, CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}
