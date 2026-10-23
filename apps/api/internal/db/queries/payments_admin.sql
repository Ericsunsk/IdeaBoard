-- Payment admin reads (split from payments.sql).

-- name: CountAdminPaymentOrders :one
SELECT COUNT(*)
FROM payment_orders AS orders
JOIN users ON users.id = orders.user_id
LEFT JOIN payment_order_reconciliations AS reconciliation
  ON reconciliation.payment_order_id = orders.id
WHERE (sqlc.arg(order_number)::TEXT = '' OR orders.public_order_number = sqlc.arg(order_number)::TEXT)
  AND (NOT sqlc.arg(has_user_public_id)::BOOLEAN OR users.public_id = sqlc.arg(user_public_id)::UUID)
  AND (sqlc.arg(display_name)::TEXT = '' OR STRPOS(LOWER(users.display_name), LOWER(sqlc.arg(display_name)::TEXT)) > 0)
  AND (sqlc.arg(status)::TEXT = '' OR orders.status = sqlc.arg(status)::TEXT)
  AND (NOT sqlc.arg(anomaly_only)::BOOLEAN OR reconciliation.anomalous_at IS NOT NULL);


-- name: ListAdminPaymentOrders :many
SELECT orders.id,
       orders.public_order_number,
       users.public_id AS user_public_id,
       users.display_name,
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
  AND (NOT sqlc.arg(has_user_public_id)::BOOLEAN OR users.public_id = sqlc.arg(user_public_id)::UUID)
  AND (sqlc.arg(display_name)::TEXT = '' OR STRPOS(LOWER(users.display_name), LOWER(sqlc.arg(display_name)::TEXT)) > 0)
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
       orders.billing_account_id,
       users.public_id AS user_public_id,
       users.display_name,
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
