-- Control Plane durable idempotency facts (#891). The guard claims the
-- in-progress row INSIDE its guard-owned transaction before running the
-- handler: concurrent same-key claimants block on the primary key until
-- the winner commits or rolls back, so exactly one winner executes the
-- business side effect. In-progress rows are therefore never committed
-- (a crash aborts them with the transaction); only 2xx outcomes flip
-- the row to completed with the response for replay. Business failures
-- delete the claim so the retry re-executes.

-- name: ClaimControlIdempotency :one
INSERT INTO control_idempotency_records (operator_identity, operation, target_type, target_id, idempotency_key, request_hash)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (operator_identity, operation, target_type, target_id, idempotency_key) DO NOTHING
RETURNING request_hash, completed, expires_at;

-- name: GetControlIdempotency :one
SELECT request_hash, completed, response_status, response_content_type, response_body, expires_at
FROM control_idempotency_records
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5;

-- name: CompleteControlIdempotency :exec
UPDATE control_idempotency_records
SET completed = TRUE,
    response_status = $6,
    response_content_type = $7,
    response_body = $8,
    completed_at = now(),
    expires_at = $9
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5 AND NOT completed;

-- name: AbandonControlIdempotency :exec
DELETE FROM control_idempotency_records
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5 AND NOT completed;

-- name: DeleteControlIdempotencyRecord :exec
DELETE FROM control_idempotency_records
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5;

-- name: PurgeExpiredControlIdempotency :execrows
DELETE FROM control_idempotency_records
WHERE completed AND expires_at <= now();
