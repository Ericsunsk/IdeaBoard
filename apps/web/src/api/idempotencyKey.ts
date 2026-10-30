import { createUniqueId } from '../core/utils/ids'

// Retries of the same command attempt reuse its key; a new intent mints a new key.
function createIdempotencyKey(scope: string): string {
  return `${scope}:${createUniqueId()}`
}

// Task cancellation is an idempotent command keyed by its target task.
export function taskCancellationIdempotencyKey(taskID: string): string {
  return `task-cancel:${taskID}`
}

// Workspace creation key: caller-generated per command attempt.
export function workspaceCreationIdempotencyKey(): string {
  return createIdempotencyKey('workspace-create')
}

// Organization creation atomically provisions its own financial account.
export function organizationCreationIdempotencyKey(): string {
  return createIdempotencyKey('organization-create')
}

// Organization closure key (issue #955, `organization:close` backend scope).
export function organizationClosureIdempotencyKey(): string {
  return createIdempotencyKey('organization:close')
}

// Workspace deletion key: caller-generated per delete attempt.
export function workspaceDeletionIdempotencyKey(): string {
  return createIdempotencyKey('workspace-delete')
}

// Workspace board write key (issue #802): per board operation attempt.
export function boardOperationIdempotencyKey(operation: string): string {
  const normalized = operation.trim() || 'write'
  return createIdempotencyKey(`board-${normalized}`)
}

// Workspace upload session key per operation (issue #803).
export function workspaceUploadIdempotencyKey(operation: 'create' | 'bytes' | 'complete'): string {
  return createIdempotencyKey(`upload:${operation}`)
}

// Workspace media key per operation (issue #803, `media:<operation>` backend scope).
export function workspaceMediaIdempotencyKey(operation: 'archive'): string {
  return createIdempotencyKey(`media:${operation}`)
}

// Workspace billing checkout key per attempt (issue #804).
export function workspaceBillingCheckoutIdempotencyKey(): string {
  return createIdempotencyKey('billing:checkout')
}

// Workspace pending-order command key per attempt (issue #804).
export function workspaceBillingPendingOrderIdempotencyKey(
  operation: 'continue' | 'abandon',
): string {
  return createIdempotencyKey(`billing:pending-${operation}`)
}

// Workspace library write key per operation (issue #806).
export function workspaceLibraryIdempotencyKey(operation: string): string {
  const normalized = operation.trim() || 'write'
  return createIdempotencyKey(`library:${normalized}`)
}

// Workspace vision-query key per attempt (issue #806).
export function workspaceVisionQueryIdempotencyKey(): string {
  return createIdempotencyKey('search:vision')
}

// Workspace reward-redeem key per attempt (issue #804).
export function workspaceRewardsRedeemIdempotencyKey(): string {
  return createIdempotencyKey('rewards:redeem')
}

// Workspace task key per operation (issue #805).
export function workspaceTaskIdempotencyKey(operation: 'create' | 'cancel'): string {
  return createIdempotencyKey(`task:${operation}`)
}

// Workspace member key per operation (workspace members slice).
export function workspaceMemberIdempotencyKey(operation: 'invite' | 'remove'): string {
  return createIdempotencyKey(`workspace:members.${operation}`)
}

// Workspace billing handover key per operation (#948).
export function workspaceBillingTransferIdempotencyKey(
  operation: 'invite' | 'accept' | 'reject' | 'cancel',
): string {
  return createIdempotencyKey(`billing:transfer-${operation}`)
}

// Control Plane user administration key per operation (issue #808).
export function controlUserIdempotencyKey(
  operation: 'ban' | 'unban' | 'revoke-sessions' | 'role' | 'delete' | 'impersonate',
): string {
  return createIdempotencyKey(`control:users.${operation}`)
}

// Control Plane credit command key per operation (issue #808).
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
