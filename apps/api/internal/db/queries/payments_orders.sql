-- Payment order lifecycle (split from payments.sql).

-- name: GetPaymentOrderByAccountIdempotency :one
-- Account idempotency scope (#919): the durable replay scope is the payer
-- Billing Account, never the operator. Same key + same hash replays, same
-- key + different hash conflicts; cross-account key reuse never replays.
SELECT *
FROM payment_orders
WHERE billing_account_id = $1 AND client_idempotency_key = $2;


-- name: GetPaymentOrderByIDForUpdate :one
-- Unified ID lookup (merged from GetPaymentOrderByID + GetPaymentOrderByIDForUpdate):
-- the lock version is kept. Write paths (RecordPaid/Credit) run inside withTx
-- and need the row lock for serialization; single-statement reads outside an
-- explicit tx release the lock immediately, so they behave like the plain
-- read they replaced.
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


-- name: ListAccountActivePaymentOrders :many
-- Account unpaid-slot accounting (#919): Checkout create/continue serialize
-- per payer account under the account advisory lock.
SELECT *
FROM payment_orders
WHERE billing_account_id = $1
  AND status IN ('creating', 'checkout_open', 'async_payment_pending')
ORDER BY created_at, id;


-- name: ListPaymentOrdersForCheckoutShutdown :many
SELECT *
FROM payment_orders
WHERE stripe_account_id = $1
  AND stripe_credential_id = $2
  AND created_at <= $3
  AND status IN ('creating', 'checkout_open')
ORDER BY created_at, id
LIMIT $4;


-- name: ListAccountPendingPaymentOrders :many
-- Account pending orders (#919): every Billing Owner/Admin of the payer
-- account manages these, not just the creating operator.
SELECT *
FROM payment_orders
WHERE billing_account_id = $1
  AND status IN ('checkout_open', 'async_payment_pending', 'paid_pending_credit')
ORDER BY created_at DESC, id DESC;


-- name: InsertPaymentOrderIntent :one
INSERT INTO payment_orders (
  id,
  public_order_number,
  public_token,
  user_id,
  billing_account_id,
  client_idempotency_key,
  request_hash,
  status,
  stripe_credential_id,
  stripe_account_id,
  stripe_product_id,
  idea_quantity,
  base_amount_minor,
  terms_version,
  refund_policy_version,
  return_base_url,
  expires_at
)
SELECT $1, $2, $3, $4, $5, $6, $7, 'creating', $8, $9, $10, $11, $11, $12, $13, $14, $15
FROM payment_stripe_configurations configuration
WHERE configuration.singleton_id = 1
  AND configuration.active_credential_id = $8
  AND configuration.purchases_enabled
RETURNING *;


-- name: GetPaymentCustomerByAccount :one
-- Account Stripe Customer (#919): reused per payer Billing Account, never
-- per operator, so test/live stay isolated and key rotation never duplicates.
SELECT *
FROM payment_customers
WHERE billing_account_id = $1
  AND stripe_account_id = $2
  AND stripe_mode = $3;


-- name: UpsertPaymentCustomerByAccount :one
INSERT INTO payment_customers (user_id, billing_account_id, stripe_account_id, stripe_mode, stripe_customer_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (billing_account_id, stripe_account_id, stripe_mode) DO UPDATE
SET stripe_customer_id = EXCLUDED.stripe_customer_id,
    user_id = EXCLUDED.user_id
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


-- name: GetPaymentOrderByCheckoutSession :one
-- Unified Checkout-session lookup (merged from ...ForUpdate + plain): the
-- non-lock version is kept. The only caller is the webhook read path
-- (dispatch, outside a write tx), which needs no row lock.
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


-- name: MarkPaymentOrderCancelledByAccount :one
-- Account abandon (#919): any Billing Owner/Admin of the persisted payer
-- account may abandon its open Checkout.
UPDATE payment_orders
SET status = 'cancelled',
    checkout_url = ''
WHERE id = $1 AND billing_account_id = $2 AND status = 'checkout_open'
RETURNING *;


-- name: MarkPaymentOrderExpired :one
UPDATE payment_orders
SET status = 'expired',
    checkout_url = ''
WHERE id = $1 AND status IN ('creating', 'checkout_open', 'async_payment_pending')
RETURNING *;


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


-- name: GetPaymentOrderByAccountToken :one
-- Account order reads (#919): continue/abandon/Return address the persisted
-- payer account. Callers authorize Billing Owner/Admin first and converge
-- unknown tokens to not-found (anti-enumeration); the token alone is never
-- an ownership oracle.
SELECT *
FROM payment_orders
WHERE public_token = $1 AND billing_account_id = $2;


-- name: ListAccountRechargeHistory :many
-- Account recharge history (#919): credited, refunded-overlay, and
-- user-abandoned facts of one payer Billing Account for its authorized
-- managers. Same 12-month window and opaque-cursor paging as the
-- operator read it replaces; no payer PII or provider secrets.
SELECT orders.id,
       COALESCE(orders.paid_at, orders.updated_at) AS occurred_at,
       orders.idea_quantity,
       orders.status,
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
WHERE orders.billing_account_id = sqlc.arg(billing_account_id)::BIGINT
  AND orders.status IN ('credited', 'cancelled')
  AND COALESCE(orders.paid_at, orders.updated_at) >= sqlc.arg(cutoff_at)::TIMESTAMPTZ
  AND (
    NOT sqlc.arg(cursor_set)::BOOLEAN
    OR (COALESCE(orders.paid_at, orders.updated_at), orders.id) < (sqlc.narg(cursor_paid_at)::TIMESTAMPTZ, sqlc.narg(cursor_id)::UUID)
  )
ORDER BY COALESCE(orders.paid_at, orders.updated_at) DESC, orders.id DESC
LIMIT sqlc.arg(page_limit)::INTEGER;


-- name: ListBillingAccountManagerIDs :many
-- Server-internal SSE fanout audience (#919): the current financial
-- managers of one Billing Account. No auth gate here; callers fan out
-- post-commit hints only to these users, so exited members and
-- non-managers never observe another account's settlement. Financial
-- management grants no content access; this list never leaves the server.
SELECT user_id
FROM billing_account_members
WHERE billing_account_id = $1
ORDER BY user_id;
