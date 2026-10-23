-- Payment reconciliation sweep (split from payments.sql).

-- name: ListUnsettledPaymentOrders :many
SELECT *
FROM payment_orders
WHERE status IN ('creating', 'checkout_open', 'async_payment_pending', 'paid_pending_credit')
ORDER BY created_at, id
LIMIT 1001;


-- name: ClaimDuePaymentOrderReconciliations :many
WITH candidates AS (
  SELECT orders.id
  FROM payment_orders AS orders
  LEFT JOIN payment_order_reconciliations AS reconciliation
    ON reconciliation.payment_order_id = orders.id
  WHERE orders.status IN ('creating', 'checkout_open', 'async_payment_pending', 'paid_pending_credit')
    AND orders.updated_at <= sqlc.arg(stale_before)::TIMESTAMPTZ
    AND COALESCE(reconciliation.next_attempt_at, orders.created_at) <= sqlc.arg(now_at)::TIMESTAMPTZ
    AND (reconciliation.claim_token IS NULL OR reconciliation.claimed_until <= sqlc.arg(now_at)::TIMESTAMPTZ)
  ORDER BY orders.updated_at, orders.id
  FOR UPDATE OF orders SKIP LOCKED
  LIMIT sqlc.arg(row_limit)::INTEGER
), claimed AS (
  INSERT INTO payment_order_reconciliations (
    payment_order_id, claim_token, claim_generation, claimed_at, claimed_until, next_attempt_at
  )
  SELECT id, sqlc.arg(claim_token)::UUID, 1, sqlc.arg(now_at)::TIMESTAMPTZ,
         sqlc.arg(claim_until)::TIMESTAMPTZ, sqlc.arg(now_at)::TIMESTAMPTZ
  FROM candidates
  ON CONFLICT (payment_order_id) DO UPDATE
  SET claim_token = EXCLUDED.claim_token,
      claim_generation = payment_order_reconciliations.claim_generation + 1,
      claimed_at = EXCLUDED.claimed_at,
      claimed_until = EXCLUDED.claimed_until
  RETURNING payment_order_id, claim_token, claim_generation
)
SELECT sqlc.embed(orders), claimed.claim_token, claimed.claim_generation
FROM claimed
JOIN payment_orders AS orders ON orders.id = claimed.payment_order_id
ORDER BY orders.id;


-- name: CompletePaymentOrderReconciliationClaim :execrows
UPDATE payment_order_reconciliations
SET claim_token = NULL,
    claimed_at = NULL,
    claimed_until = NULL,
    last_checked_at = sqlc.arg(checked_at)::TIMESTAMPTZ,
    next_attempt_at = sqlc.arg(next_attempt_at)::TIMESTAMPTZ,
    last_error_code = sqlc.arg(error_code)::TEXT,
    last_error_summary = sqlc.arg(error_summary)::TEXT,
    anomalous_at = CASE
      WHEN sqlc.arg(clear_anomaly)::BOOLEAN THEN NULL
      WHEN sqlc.arg(mark_anomalous)::BOOLEAN THEN COALESCE(anomalous_at, sqlc.arg(checked_at)::TIMESTAMPTZ)
      ELSE anomalous_at
    END
WHERE payment_order_id = sqlc.arg(payment_order_id)::UUID
  AND claim_token = sqlc.arg(claim_token)::UUID
  AND claim_generation = sqlc.arg(claim_generation)::BIGINT;


-- name: ListPaymentReconciliationAnomalies :many
SELECT orders.id,
       orders.public_order_number,
       orders.user_id,
       orders.status,
       reconciliation.last_error_code,
       reconciliation.last_error_summary,
       reconciliation.last_checked_at,
       reconciliation.anomalous_at,
       orders.created_at
FROM payment_order_reconciliations AS reconciliation
JOIN payment_orders AS orders ON orders.id = reconciliation.payment_order_id
WHERE reconciliation.anomalous_at IS NOT NULL
ORDER BY reconciliation.anomalous_at DESC, orders.id DESC
LIMIT sqlc.arg(row_limit)::INTEGER;
