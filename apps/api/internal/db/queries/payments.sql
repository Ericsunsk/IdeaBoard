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

-- name: InsertPaymentWebhookEvent :execrows
INSERT INTO payment_webhook_events (
  stripe_event_id,
  event_type,
  stripe_object_id,
  livemode,
  stripe_created_at,
  processing_result,
  processed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
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
  expires_at
)
VALUES (
  $1, $2, $3, $4, $5, $6, 'creating', $7, $8, $9, $9, $10, $11, $12
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
SET status = 'async_payment_pending'
WHERE id = $1 AND status = 'checkout_open'
RETURNING *;

-- name: MarkPaymentOrderFailed :one
UPDATE payment_orders
SET status = 'payment_failed'
WHERE id = $1 AND status IN ('creating', 'checkout_open', 'async_payment_pending')
RETURNING *;

-- name: MarkPaymentOrderCancelled :one
UPDATE payment_orders
SET status = 'cancelled'
WHERE id = $1 AND user_id = $2 AND status = 'checkout_open'
RETURNING *;

-- name: MarkPaymentOrderExpired :one
UPDATE payment_orders
SET status = 'expired'
WHERE id = $1 AND status IN ('creating', 'checkout_open', 'async_payment_pending')
RETURNING *;

-- name: ListUnsettledPaymentOrders :many
SELECT *
FROM payment_orders
WHERE status IN ('creating', 'checkout_open', 'async_payment_pending', 'paid_pending_credit')
ORDER BY created_at, id
LIMIT 1001;

-- name: RecordPaymentOrderPaid :one
UPDATE payment_orders
SET status = 'paid_pending_credit',
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
SELECT id,
       paid_at AS occurred_at,
       idea_quantity,
       paid_total_minor,
       paid_currency,
       receipt_url
FROM payment_orders
WHERE user_id = sqlc.arg(user_id)::BIGINT
  AND status = 'credited'
  AND paid_at >= sqlc.arg(cutoff_at)::TIMESTAMPTZ
  AND (
    NOT sqlc.arg(cursor_set)::BOOLEAN
    OR (paid_at, id) < (sqlc.narg(cursor_paid_at)::TIMESTAMPTZ, sqlc.narg(cursor_id)::UUID)
  )
ORDER BY paid_at DESC, id DESC
LIMIT sqlc.arg(page_limit)::INTEGER;

-- name: MarkPaymentWebhookEventProcessed :execrows
UPDATE payment_webhook_events
SET processing_result = $2,
    error_summary = $3,
    processed_at = $4
WHERE stripe_event_id = $1;
