-- Control idempotency authority. A unique attempt_token fences all writes.
-- Atomic mutations commit completion with business writes; detached mutations
-- commit settlement_unknown before external I/O, excluding lease takeover.
-- Only completed responses expire; unknown outcomes never automatically retry.

-- name: ClaimControlIdempotency :one
INSERT INTO control_idempotency_records (operator_identity, operation, target_type, target_id, idempotency_key, request_hash, attempt_token)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (operator_identity, operation, target_type, target_id, idempotency_key) DO NOTHING
RETURNING request_hash, completed, expires_at, attempt_token, settlement_unknown;

-- name: GetControlIdempotency :one
SELECT request_hash, completed, response_status, response_content_type, response_body, expires_at, attempt_token, settlement_unknown
FROM control_idempotency_records
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5;

-- name: CompleteControlIdempotency :execrows
UPDATE control_idempotency_records
SET completed = TRUE,
    response_status = $6,
    response_content_type = $7,
    response_body = $8,
    completed_at = now(),
    expires_at = $9,
    settlement_unknown = FALSE
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5 AND attempt_token = $10 AND NOT completed;

-- name: AbandonControlIdempotency :exec
DELETE FROM control_idempotency_records
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5 AND attempt_token = $6 AND NOT completed;

-- name: MarkUnknownControlIdempotency :execrows
UPDATE control_idempotency_records
SET settlement_unknown = TRUE
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5 AND attempt_token = $6 AND NOT completed;

-- name: DeleteControlIdempotencyRecord :exec
DELETE FROM control_idempotency_records
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5 AND attempt_token = $6 AND completed AND expires_at <= now();

-- name: TryAbandonStaleControlIdempotency :execrows
DELETE FROM control_idempotency_records
WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5 AND attempt_token = $6 AND NOT completed AND NOT settlement_unknown AND created_at < $7;

-- name: PurgeExpiredControlIdempotency :execrows
DELETE FROM control_idempotency_records
WHERE completed AND expires_at <= now();
