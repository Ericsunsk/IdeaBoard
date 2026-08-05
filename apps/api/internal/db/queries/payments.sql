-- name: GetPaymentStripeConfiguration :one
SELECT singleton_id,
       account_id,
       mode,
       charges_enabled,
       product_tax_code,
       purchases_enabled,
       stripe_product_id,
       stripe_webhook_endpoint_id,
       api_key_ciphertext,
       webhook_secret_ciphertext,
       secret_material_kind,
       created_by,
       updated_by,
       created_at,
       updated_at
FROM payment_stripe_configurations
WHERE singleton_id = 1;

-- name: ReplacePaymentStripeConfiguration :one
INSERT INTO payment_stripe_configurations (
  singleton_id,
  account_id,
  mode,
  charges_enabled,
  product_tax_code,
  purchases_enabled,
  stripe_product_id,
  stripe_webhook_endpoint_id,
  api_key_ciphertext,
  webhook_secret_ciphertext,
  secret_material_kind,
  created_by,
  updated_by
)
VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
ON CONFLICT (singleton_id) DO UPDATE
SET account_id = EXCLUDED.account_id,
    mode = EXCLUDED.mode,
    charges_enabled = EXCLUDED.charges_enabled,
    product_tax_code = EXCLUDED.product_tax_code,
    purchases_enabled = EXCLUDED.purchases_enabled,
    stripe_product_id = EXCLUDED.stripe_product_id,
    stripe_webhook_endpoint_id = EXCLUDED.stripe_webhook_endpoint_id,
    api_key_ciphertext = EXCLUDED.api_key_ciphertext,
    webhook_secret_ciphertext = EXCLUDED.webhook_secret_ciphertext,
    secret_material_kind = EXCLUDED.secret_material_kind,
    updated_by = EXCLUDED.updated_by
RETURNING singleton_id,
          account_id,
          mode,
          charges_enabled,
          product_tax_code,
          purchases_enabled,
          stripe_product_id,
          stripe_webhook_endpoint_id,
          api_key_ciphertext,
          webhook_secret_ciphertext,
          secret_material_kind,
          created_by,
          updated_by,
          created_at,
          updated_at;

-- name: InsertPaymentAdminAction :exec
INSERT INTO payment_admin_actions (
  action_type,
  actor_id,
  target_type,
  target_id,
  outcome,
  error_code,
  error_summary,
  metadata
)
VALUES ($1, $2, 'stripe_configuration', 'current', $3, $4, $5, sqlc.arg(metadata)::JSONB);

-- name: InsertPaymentCheckoutShutdown :one
INSERT INTO payment_checkout_shutdowns (stripe_account_id, cutoff_at)
VALUES ($1, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetPaymentCheckoutShutdown :one
SELECT *
FROM payment_checkout_shutdowns
WHERE id = $1;

-- name: ListActivePaymentCheckoutShutdowns :many
SELECT *
FROM payment_checkout_shutdowns
WHERE completed_at IS NULL
ORDER BY created_at, id;

-- name: CompletePaymentCheckoutShutdown :execrows
UPDATE payment_checkout_shutdowns
SET completed_at = $2
WHERE id = $1 AND completed_at IS NULL;

-- name: InsertPaymentWebhookEvent :execrows
INSERT INTO payment_webhook_events (
  stripe_event_id,
  event_type,
  stripe_object_id,
  payment_order_id,
  livemode,
  stripe_created_at,
  processing_result,
  processed_at
)
VALUES (
  sqlc.arg(stripe_event_id),
  sqlc.arg(event_type),
  sqlc.arg(stripe_object_id),
  (
    SELECT orders.id
    FROM payment_orders AS orders
    WHERE (sqlc.arg(payment_intent_id)::TEXT <> '' AND orders.stripe_payment_intent_id = sqlc.arg(payment_intent_id)::TEXT)
       OR (sqlc.arg(charge_id)::TEXT <> '' AND orders.stripe_charge_id = sqlc.arg(charge_id)::TEXT)
  ),
  sqlc.arg(livemode),
  sqlc.arg(stripe_created_at),
  sqlc.arg(processing_result),
  sqlc.narg(processed_at)
)
ON CONFLICT (stripe_event_id) DO NOTHING;

-- name: GetPaymentOrderByIdempotency :one
SELECT *
FROM payment_orders
WHERE user_id = $1 AND client_idempotency_key = $2;

-- name: GetPaymentOrderByID :one
SELECT *
FROM payment_orders
WHERE id = $1;

-- name: GetPaymentOrderByIDForUpdate :one
SELECT *
FROM payment_orders
WHERE id = $1
FOR UPDATE;

-- name: ListActivePaymentOrders :many
SELECT *
FROM payment_orders
WHERE user_id = $1
  AND status IN ('creating', 'checkout_open', 'async_payment_pending')
ORDER BY created_at, id;

-- name: ListPaymentOrdersForCheckoutShutdown :many
SELECT *
FROM payment_orders
WHERE stripe_account_id = $1
  AND created_at <= $2
  AND status IN ('creating', 'checkout_open')
ORDER BY created_at, id
LIMIT $3;

-- name: ListUserPendingPaymentOrders :many
SELECT *
FROM payment_orders
WHERE user_id = $1
  AND status IN ('checkout_open', 'async_payment_pending', 'paid_pending_credit')
ORDER BY created_at DESC, id DESC;

-- name: InsertPaymentOrderIntent :one
INSERT INTO payment_orders (
  id,
  public_order_number,
  public_token,
  user_id,
  client_idempotency_key,
  request_hash,
  status,
  stripe_account_id,
  stripe_product_id,
  idea_quantity,
  base_amount_minor,
  terms_version,
  refund_policy_version,
  return_base_url,
  expires_at
)
VALUES (
  $1, $2, $3, $4, $5, $6, 'creating', $7, $8, $9, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: GetPaymentCustomer :one
SELECT *
FROM payment_customers
WHERE user_id = $1;

-- name: ReplacePaymentCustomer :one
INSERT INTO payment_customers (user_id, stripe_account_id, stripe_customer_id)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET stripe_account_id = EXCLUDED.stripe_account_id,
    stripe_customer_id = EXCLUDED.stripe_customer_id
RETURNING *;

-- name: BindPaymentOrderCheckout :one
UPDATE payment_orders
SET status = 'checkout_open',
    stripe_customer_id = $2,
    stripe_checkout_session_id = $3,
    checkout_url = $4,
    expires_at = $5
WHERE id = $1 AND status = 'creating'
RETURNING *;

-- name: GetPaymentOrderByCheckoutSessionForUpdate :one
SELECT *
FROM payment_orders
WHERE stripe_account_id = $1 AND stripe_checkout_session_id = $2
FOR UPDATE;

-- name: GetPaymentOrderByCheckoutSession :one
SELECT *
FROM payment_orders
WHERE stripe_account_id = $1 AND stripe_checkout_session_id = $2;

-- name: MarkPaymentOrderAsyncPending :one
UPDATE payment_orders
SET status = 'async_payment_pending',
    checkout_url = ''
WHERE id = $1 AND status = 'checkout_open'
RETURNING *;

-- name: MarkPaymentOrderFailed :one
UPDATE payment_orders
SET status = 'payment_failed',
    checkout_url = ''
WHERE id = $1 AND status IN ('creating', 'checkout_open', 'async_payment_pending')
RETURNING *;

-- name: MarkPaymentOrderCancelled :one
UPDATE payment_orders
SET status = 'cancelled',
    checkout_url = ''
WHERE id = $1 AND user_id = $2 AND status = 'checkout_open'
RETURNING *;

-- name: MarkPaymentOrderExpired :one
UPDATE payment_orders
SET status = 'expired',
    checkout_url = ''
WHERE id = $1 AND status IN ('creating', 'checkout_open', 'async_payment_pending')
RETURNING *;

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

-- name: CountAdminPaymentOrders :one
SELECT COUNT(*)
FROM payment_orders AS orders
JOIN users ON users.id = orders.user_id
LEFT JOIN payment_order_reconciliations AS reconciliation
  ON reconciliation.payment_order_id = orders.id
WHERE (sqlc.arg(order_number)::TEXT = '' OR orders.public_order_number = sqlc.arg(order_number)::TEXT)
  AND (sqlc.arg(user_id)::BIGINT <= 0 OR orders.user_id = sqlc.arg(user_id)::BIGINT)
  AND (sqlc.arg(username)::TEXT = '' OR STRPOS(LOWER(users.username), LOWER(sqlc.arg(username)::TEXT)) > 0)
  AND (sqlc.arg(status)::TEXT = '' OR orders.status = sqlc.arg(status)::TEXT)
  AND (NOT sqlc.arg(anomaly_only)::BOOLEAN OR reconciliation.anomalous_at IS NOT NULL);

-- name: ListAdminPaymentOrders :many
SELECT orders.id,
       orders.public_order_number,
       orders.user_id,
       users.username,
       orders.status,
       orders.idea_quantity,
       orders.base_currency,
       orders.base_amount_minor,
       orders.paid_total_minor,
       orders.paid_currency,
       orders.paid_at,
       orders.credited_at,
       orders.created_at,
       orders.updated_at,
       reconciliation.anomalous_at,
       reconciliation.last_checked_at
FROM payment_orders AS orders
JOIN users ON users.id = orders.user_id
LEFT JOIN payment_order_reconciliations AS reconciliation
  ON reconciliation.payment_order_id = orders.id
WHERE (sqlc.arg(order_number)::TEXT = '' OR orders.public_order_number = sqlc.arg(order_number)::TEXT)
  AND (sqlc.arg(user_id)::BIGINT <= 0 OR orders.user_id = sqlc.arg(user_id)::BIGINT)
  AND (sqlc.arg(username)::TEXT = '' OR STRPOS(LOWER(users.username), LOWER(sqlc.arg(username)::TEXT)) > 0)
  AND (sqlc.arg(status)::TEXT = '' OR orders.status = sqlc.arg(status)::TEXT)
  AND (NOT sqlc.arg(anomaly_only)::BOOLEAN OR reconciliation.anomalous_at IS NOT NULL)
  AND (
    NOT sqlc.arg(cursor_set)::BOOLEAN
    OR (orders.created_at, orders.id) < (sqlc.narg(cursor_created_at)::TIMESTAMPTZ, sqlc.narg(cursor_id)::UUID)
  )
ORDER BY orders.created_at DESC, orders.id DESC
LIMIT sqlc.arg(page_limit)::INTEGER;

-- name: GetAdminPaymentOrder :one
SELECT orders.id,
       orders.public_order_number,
       orders.user_id,
       users.username,
       orders.status,
       orders.idea_quantity,
       orders.base_currency,
       orders.base_amount_minor,
       orders.paid_subtotal_minor,
       orders.paid_tax_minor,
       orders.paid_total_minor,
       orders.paid_currency,
       orders.stripe_account_id,
       orders.stripe_customer_id,
       orders.stripe_product_id,
       orders.stripe_checkout_session_id,
       orders.stripe_payment_intent_id,
       orders.stripe_charge_id,
       orders.terms_version,
       orders.refund_policy_version,
       orders.receipt_url,
       orders.expires_at,
       orders.paid_at,
       orders.credited_at,
       orders.created_at,
       orders.updated_at,
       reconciliation.last_error_code,
       reconciliation.last_error_summary,
       reconciliation.last_checked_at,
       reconciliation.next_attempt_at,
       reconciliation.anomalous_at
FROM payment_orders AS orders
JOIN users ON users.id = orders.user_id
LEFT JOIN payment_order_reconciliations AS reconciliation
  ON reconciliation.payment_order_id = orders.id
WHERE orders.public_order_number = sqlc.arg(order_number)::TEXT;

-- name: ListAdminPaymentOrderWebhookEvents :many
SELECT event_type,
       stripe_object_id,
       processing_result,
       error_summary,
       stripe_created_at,
       received_at,
       processed_at
FROM payment_webhook_events
WHERE payment_order_id = sqlc.arg(payment_order_id)::UUID
  AND event_type IN ('refund.updated', 'refund.failed', 'charge.dispute.created', 'charge.dispute.closed')
ORDER BY stripe_created_at DESC, stripe_event_id DESC
LIMIT sqlc.arg(row_limit)::INTEGER;

-- name: RecordPaymentOrderPaid :one
UPDATE payment_orders
SET status = 'paid_pending_credit',
    checkout_url = '',
    stripe_customer_id = $2,
    stripe_checkout_session_id = $3,
    expires_at = CASE WHEN stripe_checkout_session_id IS NULL THEN $4 ELSE expires_at END,
    stripe_payment_intent_id = $5,
    stripe_charge_id = $6,
    paid_subtotal_minor = $7,
    paid_tax_minor = $8,
    paid_total_minor = $9,
    paid_currency = $10,
    receipt_url = $11,
    paid_at = $12
WHERE id = $1
  AND status IN ('creating', 'checkout_open', 'async_payment_pending', 'cancelled', 'expired', 'payment_failed')
RETURNING *;

-- name: MarkPaymentOrderCredited :one
UPDATE payment_orders
SET status = 'credited',
    credit_transaction_id = $2,
    credited_at = $3
WHERE id = $1 AND status = 'paid_pending_credit'
RETURNING *;

-- name: GetPaymentOrderByPublicToken :one
SELECT *
FROM payment_orders
WHERE public_token = $1 AND user_id = $2;

-- name: ListPaymentRechargeHistory :many
SELECT orders.id,
       orders.paid_at AS occurred_at,
       orders.idea_quantity,
       orders.paid_total_minor,
       orders.paid_currency,
       orders.receipt_url,
       COALESCE(refunds.idea_quantity, 0)::BIGINT AS refunded_idea_quantity,
       COALESCE(refunds.amount_minor, 0)::BIGINT AS refunded_amount_minor
FROM payment_orders orders
LEFT JOIN LATERAL (
  SELECT SUM(payment_refunds.idea_quantity)::BIGINT AS idea_quantity,
         SUM(payment_refunds.amount_minor)::BIGINT AS amount_minor
  FROM payment_refunds
  WHERE payment_refunds.payment_order_id = orders.id
    AND payment_refunds.status = 'succeeded'
) refunds ON TRUE
WHERE orders.user_id = sqlc.arg(user_id)::BIGINT
  AND orders.status = 'credited'
  AND orders.paid_at >= sqlc.arg(cutoff_at)::TIMESTAMPTZ
  AND (
    NOT sqlc.arg(cursor_set)::BOOLEAN
    OR (orders.paid_at, orders.id) < (sqlc.narg(cursor_paid_at)::TIMESTAMPTZ, sqlc.narg(cursor_id)::UUID)
  )
ORDER BY orders.paid_at DESC, orders.id DESC
LIMIT sqlc.arg(page_limit)::INTEGER;

-- name: MarkPaymentWebhookEventProcessed :execrows
UPDATE payment_webhook_events
SET processing_result = $2,
    error_summary = $3,
    processed_at = $4
WHERE stripe_event_id = $1;
