// Task cancellation is an idempotent command keyed by its target task.
export function taskCancellationIdempotencyKey(taskID: string): string {
  return `task-cancel:${taskID}`
}

function randomKeySuffix(): string {
  const randomUUID = (globalThis.crypto as Crypto | undefined)?.randomUUID
  if (typeof randomUUID === 'function') return randomUUID.call(globalThis.crypto)
  return `${Date.now().toString(36)}-${Math.floor(Math.random() * 0xffffffff).toString(36)}`
}

// Single mint behind every caller-generated key: `scope` carries the
// backend idempotency scope for debuggability. Retries of the SAME
// attempt must reuse the returned value; a new user intent needs a
// fresh call.
function createIdempotencyKey(scope: string): string {
  return `${scope}:${randomKeySuffix()}`
}

// Workspace creation key: caller-generated per command attempt. Concurrent
// retries of the SAME attempt must reuse the returned value; a new user
// intent needs a fresh call.
export function workspaceCreationIdempotencyKey(): string {
  return createIdempotencyKey('workspace-create')
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
// key carries the same operation prefix for debuggability. Retries of the
// SAME attempt must reuse the returned value.
export function workspaceUploadIdempotencyKey(operation: 'create' | 'complete'): string {
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
