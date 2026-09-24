// Package postgres implements workspacetasks.WorkspaceTaskStore against
// PostgreSQL/sqlc (#819 Phase 4-02).
//
// Mapping contract (schema.sql truth):
//   - WorkspaceTask.ID <-> tasks.public_id (task_...)
//   - WorkspaceTask.WorkspaceID <-> workspaces.public_id (resolved to
//     internal_id per call; cross-workspace ids stay invisible/ErrNotFound)
//   - WorkflowKind <-> tasks.workflow_type (canonical AI-image feature slug)
//   - ExecutionToken <-> workflow_input envelope execution_token (durable
//     across states); tasks.lease_token mirrors it ONLY while running
//     (chk_tasks_lease_status) and fences Claim/terminal transitions
//   - MediaAssetIDs + Params + BillingSnapshot <-> workflow_input JSONB
//     envelope (object per chk_tasks_workflow_input)
//   - CancelReason <-> workflow_input envelope (tasks has no cancel column;
//     cancelled_at/cancelled_by_user_id carry the DB facts)
//   - OutputRefs <-> task_outputs rows (output_kind media_asset/remote_url;
//     asset public id + checksum + role ride in metadata)
//
// The transactional invariant (Quote snapshot + Credit Hold via
// credits.Commands + task insert + River enqueue in ONE PG transaction,
// terminal Capture/Release atomic, events only after commit) is
// orchestrated by workspacetasks.Service; this store is the Insert and
// state-flip participant. Multi-statement transitions that must not
// interleave (complete + outputs, retry + token, add-outputs batch) run
// in a caller-owned dbutil.WithTx transaction. Repository methods take
// context.Context first and use dbWithContext(ctx).
package postgres
