-- Payment webhook inbox (split from payments.sql).

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


-- name: GetPaymentWebhookEvent :one
SELECT *
FROM payment_webhook_events
WHERE stripe_event_id = $1;


-- name: ClaimPaymentWebhookEvent :execrows
UPDATE payment_webhook_events
SET processing_result = 'processing',
    processed_at = clock_timestamp(),
    error_summary = ''
WHERE stripe_event_id = $1
  AND (
    processing_result IN ('received', 'failed')
    OR (processing_result = 'processing' AND processed_at < clock_timestamp() - INTERVAL '5 minutes')
  );


-- name: MarkPaymentWebhookEventProcessed :execrows
UPDATE payment_webhook_events
SET processing_result = $2,
    error_summary = $3,
    processed_at = $4
WHERE stripe_event_id = $1;
