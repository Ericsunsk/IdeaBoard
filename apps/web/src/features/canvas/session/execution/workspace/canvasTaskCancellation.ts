// Canvas task cancel/binding seam (issue #859).
//
// Task -> workspace binding plus workspace-scoped cancel, split out of the
// workspace task port. Cancel needs the workspace the task was submitted
// under, which may differ from the current selection after a workspace
// switch: submissions register the binding, cancel resolves it per task.
// No SSE, no readiness, no Vue: the retry policy stays with the
// `task-lifecycle` cancel owner, which injects this seam.

import { sanitizeApiErrorMessage } from '../../../../../core/utils/errorMessages'
import { requireWorkspaceId as requireSharedWorkspaceId } from '../../../../../core/utils/workspaceId'
import {
  cancelWorkspaceTask,
  isWorkspaceTaskGoneError,
} from '../../../../../api/workspaceTaskTransport'
import { taskCancellationIdempotencyKey } from '../../../../../api/idempotencyKey'
import { i18n } from '../../../../../core/i18n'
import { getWorkspaceSelectionStore as getDefaultWorkspaceSelectionStore } from '../../../../workspace'
import { errorCode, errorStatus } from './canvasTaskContractMapping'

export const CANVAS_WORKSPACE_REQUIRED_MESSAGE = 'Select a workspace before running image tasks.'

export type CanvasWorkspaceIdResolver = () => string | null

function defaultCanvasWorkspaceIdResolver(): string | null {
  try {
    return getDefaultWorkspaceSelectionStore().getSelectedId()
  } catch {
    return null
  }
}

export class CanvasWorkspaceUnavailableError extends Error {
  readonly kind = 'workspace-unavailable' as const

  constructor(message: string = CANVAS_WORKSPACE_REQUIRED_MESSAGE) {
    super(message)
    this.name = 'CanvasWorkspaceUnavailableError'
  }
}

export function isCanvasWorkspaceUnavailableError(
  error: unknown,
): error is CanvasWorkspaceUnavailableError {
  return error instanceof CanvasWorkspaceUnavailableError
}

/** Single source of truth: the `features/workspace` selection store (via injected port). */
export function requireCanvasWorkspaceId(resolve?: CanvasWorkspaceIdResolver): string {
  return requireSharedWorkspaceId(
    (resolve ?? defaultCanvasWorkspaceIdResolver)(),
    () => new CanvasWorkspaceUnavailableError(),
  )
}

// Task -> workspace registry: cancel needs the workspace the task was
// submitted under, which may differ from the current selection after a
// workspace switch. Populated at submit; cancel falls back to the current
// selection and fails closed (warn + skip) when neither resolves.
const taskWorkspaceIds = new Map<string, string>()

export function registerCanvasWorkspaceTask(taskId: string, workspaceId: string): void {
  const task = taskId.trim()
  const workspace = workspaceId.trim()
  if (task && workspace) taskWorkspaceIds.set(task, workspace)
}

function resolveCanvasTaskWorkspaceId(taskId: string): string | null {
  return taskWorkspaceIds.get(taskId.trim()) ?? null
}

/** Test seam: drop task -> workspace bindings (specs only). */
export function resetCanvasWorkspaceTaskRegistryForTests(): void {
  taskWorkspaceIds.clear()
}

/**
 * Binding lookup for non-cancel consumers (result waiting): the task may
 * settle under a workspace that differs from the current selection, so
 * waiters resolve the bound workspace instead of the selection.
 */
export function resolveBoundCanvasTaskWorkspaceId(taskId: string): string | null {
  return resolveCanvasTaskWorkspaceId(taskId)
}

function isTaskStateConflict(error: unknown): boolean {
  if (errorStatus(error) === 409) return true
  return errorCode(error) === 'task_state_conflict'
}

export async function cancelCanvasWorkspaceTask(taskId: string, reason?: string): Promise<void> {
  const trimmed = taskId.trim()
  if (!trimmed) return
  const workspaceId = resolveCanvasTaskWorkspaceId(trimmed) ?? defaultCanvasWorkspaceIdResolver()
  if (!workspaceId) {
    // Fail closed: never cancel into an unknown workspace. The lifecycle
    // reports this via its final-failure path.
    throw new CanvasWorkspaceUnavailableError(
      'Cannot cancel the task without a workspace selection.',
    )
  }
  try {
    await cancelWorkspaceTask(
      workspaceId,
      trimmed,
      reason?.trim() ? { reason: reason.trim() } : undefined,
      { idempotencyKey: taskCancellationIdempotencyKey(trimmed) },
    )
  } catch (error) {
    // Idempotent cancel: gone (404 converged hidden) and already-terminal
    // (409 state conflict) settle silently instead of retry-looping.
    if (isWorkspaceTaskGoneError(error) || isTaskStateConflict(error)) return
    const message = error instanceof Error ? error.message : ''
    throw new Error(
      sanitizeApiErrorMessage(message, i18n.global.t('common.errors.taskCancelFailed')),
      { cause: error },
    )
  }
}
