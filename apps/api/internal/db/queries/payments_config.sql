-- Payment Stripe configuration (split from payments.sql).

-- name: GetPaymentStripeConfiguration :one
SELECT singleton_id,
       active_credential_id,
       product_tax_code,
       purchases_enabled,
       revision,
       updated_by,
       created_at,
       updated_at
FROM payment_stripe_configurations
WHERE singleton_id = 1;


-- name: GetPaymentStripeCredential :one
SELECT *
FROM payment_stripe_credentials
WHERE id = $1;


-- name: GetPaymentStripeCredentialForOrder :one
SELECT credentials.*
FROM payment_orders orders
JOIN payment_stripe_credentials credentials ON credentials.id = orders.stripe_credential_id
WHERE orders.id = $1;


-- name: GetPaymentStripeCredentialForOrderNumber :one
SELECT credentials.*
FROM payment_orders orders
JOIN payment_stripe_credentials credentials ON credentials.id = orders.stripe_credential_id
WHERE orders.public_order_number = $1;


-- name: ListPaymentStripeCredentials :many
SELECT *
FROM payment_stripe_credentials
ORDER BY created_at DESC, id DESC;


-- name: InsertPaymentStripeCredential :one
INSERT INTO payment_stripe_credentials (
  id,
  account_id,
  mode,
  charges_enabled,
  payment_method_configuration_id,
  stripe_product_id,
  stripe_webhook_endpoint_id,
  api_key_ciphertext,
  webhook_secret_ciphertext,
  secret_material_kind,
  created_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;


-- name: ReplacePaymentStripeConfiguration :one
INSERT INTO payment_stripe_configurations (
  singleton_id,
  active_credential_id,
  product_tax_code,
  purchases_enabled,
  revision,
  updated_by
)
VALUES (1, $1, $2, $3, 1, $4)
ON CONFLICT (singleton_id) DO UPDATE
SET active_credential_id = EXCLUDED.active_credential_id,
    product_tax_code = EXCLUDED.product_tax_code,
    purchases_enabled = EXCLUDED.purchases_enabled,
    revision = payment_stripe_configurations.revision + 1,
    updated_by = EXCLUDED.updated_by
WHERE payment_stripe_configurations.revision = sqlc.arg(expected_revision)::BIGINT
RETURNING singleton_id,
          active_credential_id,
          product_tax_code,
          purchases_enabled,
          revision,
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
INSERT INTO payment_checkout_shutdowns (stripe_credential_id, stripe_account_id, cutoff_at)
VALUES ($1, $2, CURRENT_TIMESTAMP)
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
