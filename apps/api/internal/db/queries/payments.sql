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
