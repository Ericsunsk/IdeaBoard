import { createUniqueId } from '../core/utils/ids'

// Task cancellation is an idempotent command keyed by its target task.
export function taskCancellationIdempotencyKey(taskID: string): string {
  return `task-cancel:${taskID}`
}

// Single mint behind every caller-generated key: `scope` carries the
// backend idempotency scope for debuggability. Retries of the SAME
// attempt must reuse the returned value; a new user intent needs a
// fresh call.
function createIdempotencyKey(scope: string): string {
  return `${scope}:${createUniqueId()}`
}

// Workspace creation key: caller-generated per command attempt. Concurrent
// retries of the SAME attempt must reuse the returned value; a new user
// intent needs a fresh call.
export function workspaceCreationIdempotencyKey(): string {
  return createIdempotencyKey('workspace-create')
}

// Organization creation atomically provisions its own financial account.
export function organizationCreationIdempotencyKey(): string {
  return createIdempotencyKey('organization-create')
}

// Organization closure key (issue #955,
// `organization:close` backend scope bound to operator +
// organization). Retries of the SAME attempt must reuse the returned
// value; a new operator intent needs a fresh call.
export function organizationClosureIdempotencyKey(): string {
  return createIdempotencyKey('organization:close')
}

// Workspace deletion key: caller-generated per delete attempt (the backend
// `workspace:delete` scope fails closed without an `Idempotency-Key`).
// Retries of the SAME attempt must reuse the returned value; a new user
// intent needs a fresh call.
export function workspaceDeletionIdempotencyKey(): string {
  return createIdempotencyKey('workspace-delete')
}

// Workspace board write key (issue #802): per board operation attempt.
// Retries of the same attempt (e.g. after a 409 revision conflict refresh)
// must reuse the returned value so the server replays instead of duplicating.
export function boardOperationIdempotencyKey(operation: string): string {
  const normalized = operation.trim() || 'write'
  return createIdempotencyKey(`board-${normalized}`)
}

// Workspace upload session key per operation (issue #803). The backend
// idempotency scope is `upload:<operation>` bound to the principal; the
// key carries the same operation prefix for debuggability. `bytes` covers
// the raw-byte staging PUT (retryable); `complete` materializes the asset.
// Retries of the SAME attempt must reuse the returned value.
export function workspaceUploadIdempotencyKey(operation: 'create' | 'bytes' | 'complete'): string {
  return createIdempotencyKey(`upload:${operation}`)
}

// Workspace media key per operation (issue #803, `media:<operation>`
// backend scope). Same reuse rule as above.
export function workspaceMediaIdempotencyKey(operation: 'archive'): string {
  return createIdempotencyKey(`media:${operation}`)
}

// Workspace billing checkout key per attempt (issue #804,
// `billing.checkout.create` backend scope bound to principal + workspace).
// Retries of the SAME attempt must reuse the returned value; a new
// checkout intent needs a fresh call.
export function workspaceBillingCheckoutIdempotencyKey(): string {
  return createIdempotencyKey('billing:checkout')
}

// Workspace pending-order command key per attempt (issue #804,
// `billing.pending.continue|abandon` backend scopes bound to principal +
// workspace). Retries of the SAME attempt must reuse the returned value;
// a new operator intent needs a fresh call.
export function workspaceBillingPendingOrderIdempotencyKey(
  operation: 'continue' | 'abandon',
): string {
  return createIdempotencyKey(`billing:pending-${operation}`)
}

// Workspace library write key per operation (issue #806, `library:<operation>`
// backend scope bound to principal + workspace). Retries of the SAME
// attempt must reuse the returned value so the server replays instead
// of duplicating folders or assets.
export function workspaceLibraryIdempotencyKey(operation: string): string {
  const normalized = operation.trim() || 'write'
  return createIdempotencyKey(`library:${normalized}`)
}

// Workspace vision-query key per attempt (issue #806, `search:vision`
// backend scope bound to principal + workspace). Retries of the SAME
// attempt must reuse the returned value; a new vision intent needs a
// fresh call.
export function workspaceVisionQueryIdempotencyKey(): string {
  return createIdempotencyKey('search:vision')
}

// Workspace reward-redeem key per attempt (issue #804, `rewards:redeem`
// backend scope bound to principal + workspace). Retries of the SAME
// attempt must reuse the returned value; a new redemption intent needs a
// fresh call.
export function workspaceRewardsRedeemIdempotencyKey(): string {
  return createIdempotencyKey('rewards:redeem')
}

// Workspace task key per operation (issue #805, `task:<operation>`
// backend scope bound to principal + workspace). Retries of the SAME
// attempt must reuse the returned value; a new task intent needs a
// fresh call.
export function workspaceTaskIdempotencyKey(operation: 'create' | 'cancel'): string {
  return createIdempotencyKey(`task:${operation}`)
}

// Workspace member key per operation (workspace members slice,
// `workspace:*` backend idempotency scope bound to operator + workspace).
// Invite and remove carry `Idempotency-Key` per attempt: retries of the SAME
// attempt must reuse the returned value; a new operator intent needs a
// fresh call. Role changes are idempotent by value and carry no key.
export function workspaceMemberIdempotencyKey(operation: 'invite' | 'remove'): string {
  return createIdempotencyKey(`workspace:members.${operation}`)
}

// Workspace billing handover key per operation (#948,
// `billing.account.transfer-invite|accept|reject|cancel` backend scopes
// bound to operator + workspace). Retries of the SAME attempt must reuse
// the returned value; a new handover intent needs a fresh call.
export function workspaceBillingTransferIdempotencyKey(
  operation: 'invite' | 'accept' | 'reject' | 'cancel',
): string {
  return createIdempotencyKey(`billing:transfer-${operation}`)
}

// Control Plane user administration key per operation (issue #808,
// `control:<operation>` backend scope bound to the operator). Retries of
// the SAME attempt must reuse the returned value; a new operator intent
// needs a fresh call.
export function controlUserIdempotencyKey(
  operation: 'ban' | 'unban' | 'revoke-sessions' | 'role' | 'delete' | 'impersonate',
): string {
  return createIdempotencyKey(`control:users.${operation}`)
}

// Control Plane credit command key per operation (issue #808,
// `control:credits.<operation>` backend scope bound to the operator).
export function controlCreditIdempotencyKey(
  operation:
    | 'grant'
    | 'deduct'
    | 'refund'
    | 'correct'
    | 'reward-create'
    | 'reward-update'
    | 'reward-enabled'
    | 'reward-delete',
): string {
  return createIdempotencyKey(`control:credits.${operation}`)
}
