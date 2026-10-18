package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Queryer is the minimal query surface shared by the refunds and disputes
// aggregate subpackages. Both pgx.Tx and *pgxpool.Pool satisfy it, so the
// shared readmodels below run inside a caller's transaction or on the pool.
type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// paymentOrderExposureSQL aggregates the occupied Ideas/amounts for one
// payment order in a single round trip: refunds in a provisional/terminal
// state plus open/lost disputes. It is the single definition of the
// "occupied Ideas" shape shared by pending-refund validation,
// external-refund intake, and dispute sizing so the three readmodels
// cannot drift on status predicates.
const paymentOrderExposureSQL = `
SELECT
  COALESCE((SELECT SUM(idea_quantity) FROM payment_refunds WHERE payment_order_id = $1::uuid AND status IN ('pending', 'succeeded', 'anomaly')), 0),
  COALESCE((SELECT SUM(amount_minor) FROM payment_refunds WHERE payment_order_id = $1::uuid AND status IN ('pending', 'succeeded', 'anomaly')), 0),
  COALESCE((SELECT SUM(target_idea_quantity) FROM payment_disputes WHERE payment_order_id = $1::uuid AND status IN ('open', 'lost')), 0),
  COALESCE((SELECT SUM(amount_minor) FROM payment_disputes WHERE payment_order_id = $1::uuid AND status IN ('open', 'lost')), 0)`

// PaymentOrderExposure carries the occupied Ideas/amounts shared by the
// refunds and disputes subpackages.
type PaymentOrderExposure struct {
	RefundedIdeas  int64
	RefundedAmount int64
	DisputedIdeas  int64
	DisputedAmount int64
}

// QueryPaymentOrderExposure sums the occupied payment Ideas for one order.
// Callers pass their own transaction so the read participates in the
// surrounding unit of work.
func QueryPaymentOrderExposure(ctx context.Context, q Queryer, orderID string) (PaymentOrderExposure, error) {
	var exposure PaymentOrderExposure
	if err := q.QueryRow(ctx, paymentOrderExposureSQL, orderID).Scan(
		&exposure.RefundedIdeas, &exposure.RefundedAmount, &exposure.DisputedIdeas, &exposure.DisputedAmount,
	); err != nil {
		return PaymentOrderExposure{}, fmt.Errorf("sum occupied payment ideas: %w", err)
	}
	return exposure, nil
}
