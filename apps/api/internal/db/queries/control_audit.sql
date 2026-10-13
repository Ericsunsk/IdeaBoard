-- Control Plane audit record writes (#809). Best-effort durable sink
-- behind the control guard's structured slog record; audit failures
-- never fail the guarded request.

-- name: InsertControlAuditRecord :one
INSERT INTO control_audit_records (
  public_id, operator_user_id, operation_type, capability,
  target_type, target_id, outcome, error_message,
  request_correlation_id, idempotency_key, metadata
)
VALUES (
  sqlc.arg(public_id), sqlc.arg(operator_user_id), sqlc.arg(operation_type),
  sqlc.arg(capability), sqlc.arg(target_type), sqlc.arg(target_id),
  sqlc.arg(outcome), sqlc.arg(error_message),
  sqlc.arg(request_correlation_id), sqlc.arg(idempotency_key),
  sqlc.arg(metadata)
)
RETURNING internal_id, public_id, created_at;
