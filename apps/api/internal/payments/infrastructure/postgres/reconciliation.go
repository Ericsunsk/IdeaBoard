package postgres

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	paymentsdomain "ideaboard/api/internal/payments/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	paymentReconciliationBatchSize = int32(100)
)

func (r *Repository) ClaimDuePaymentOrderReconciliations(ctx context.Context, now time.Time) ([]paymentreconciliation.Claim, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	now = now.UTC()
	token := uuid.New()
	rows, err := r.DBWithContext(ctx).Queries().ClaimDuePaymentOrderReconciliations(ctx, dbsqlc.ClaimDuePaymentOrderReconciliationsParams{
		StaleBefore: dbutil.TimestamptzFromTime(now.Add(-2 * time.Minute)), NowAt: dbutil.TimestamptzFromTime(now), RowLimit: paymentReconciliationBatchSize,
		ClaimToken: pgtype.UUID{Bytes: token, Valid: true}, ClaimUntil: dbutil.TimestamptzFromTime(now.Add(paymentreconciliation.Lease)),
	})
	if err != nil {
		return nil, fmt.Errorf("claim due payment order reconciliations: %w", err)
	}
	claims := make([]paymentreconciliation.Claim, 0, len(rows))
	for _, row := range rows {
		claims = append(claims, paymentreconciliation.Claim{
			Order: PaymentOrder(row.PaymentOrder), Token: token.String(), Generation: row.ClaimGeneration,
		})
	}
	return claims, nil
}

func (r *Repository) CompletePaymentOrderReconciliationClaim(
	ctx context.Context,
	claim paymentreconciliation.Claim,
	outcome paymentreconciliation.Outcome,
) (bool, error) {
	if r == nil || r.db == nil || claim.Order.ID == "" || claim.Generation <= 0 {
		return false, fmt.Errorf("payments repository: invalid reconciliation claim")
	}
	orderID, err := uuid.Parse(claim.Order.ID)
	if err != nil {
		return false, fmt.Errorf("parse reconciliation payment order ID: %w", err)
	}
	token, err := uuid.Parse(claim.Token)
	if err != nil {
		return false, fmt.Errorf("parse reconciliation claim token: %w", err)
	}
	checkedAt := outcome.CheckedAt.UTC()
	if checkedAt.IsZero() {
		return false, fmt.Errorf("payments repository: reconciliation check time is required")
	}
	rows, err := r.DBWithContext(ctx).Queries().CompletePaymentOrderReconciliationClaim(ctx, dbsqlc.CompletePaymentOrderReconciliationClaimParams{
		CheckedAt: dbutil.TimestamptzFromTime(checkedAt), NextAttemptAt: dbutil.TimestamptzFromTime(outcome.NextAttemptAt.UTC()),
		ErrorCode: BoundedText(outcome.ErrorCode, 64), ErrorSummary: BoundedText(outcome.ErrorSummary, 500),
		ClearAnomaly: outcome.ClearAnomaly, MarkAnomalous: outcome.Anomalous,
		PaymentOrderID: pgUUIDToPgType(orderID), ClaimToken: pgUUIDToPgType(token), ClaimGeneration: claim.Generation,
	})
	if err != nil {
		return false, fmt.Errorf("complete payment order reconciliation claim: %w", err)
	}
	return rows == 1, nil
}

func (r *Repository) ListPaymentReconciliationAnomalies(ctx context.Context, limit int32) ([]paymentreconciliation.Anomaly, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	if limit <= 0 || limit > paymentReconciliationBatchSize {
		limit = paymentReconciliationBatchSize
	}
	rows, err := r.DBWithContext(ctx).Queries().ListPaymentReconciliationAnomalies(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list payment reconciliation anomalies: %w", err)
	}
	items := make([]paymentreconciliation.Anomaly, 0, len(rows))
	for _, row := range rows {
		if !row.AnomalousAt.Valid {
			continue
		}
		items = append(items, paymentreconciliation.Anomaly{
			PaymentOrderID: pgUUIDString(row.ID), PublicOrderNumber: row.PublicOrderNumber, UserID: row.UserID,
			Status: paymentsdomain.PaymentOrderStatus(row.Status), LastErrorCode: row.LastErrorCode, LastErrorSummary: row.LastErrorSummary,
			LastCheckedAt: dbutil.TimePtr(row.LastCheckedAt), AnomalousAt: dbutil.TimeValue(row.AnomalousAt), CreatedAt: dbutil.TimeValue(row.CreatedAt),
		})
	}
	return items, nil
}
