// Canvas task SSE/poll waiting seam (issue #859).
//
// Wait mapping split out of the workspace task port: workspace task
// terminal -> `TaskImageResult[]`. Fast-path read first (terminal
// settlement errors propagate; only live tasks continue to the shared
// terminal waiter); waiter transport rejections map to the timeout pending
// kind while settlement errors (failure/cancelled/no-image) propagate
// unwrapped. Asset refs resolve through the media browser URL with explicit
// workspace binding; remote refs resolve direct. No readiness, no cancel,
// no Vue: the Vue lifecycle binds through the execution workspace assembly.

import {
  getWorkspaceTask,
  isWorkspaceTaskGoneError,
} from '../../../../../shared/api/workspaceTaskTransport'
import { waitWorkspaceTaskTerminal } from '../../../../../shared/api/workspaceTaskWaiter'
import type { WorkspaceTaskItem } from '../../../../../shared/api/workspaceTaskContract'
import type { WorkspaceTaskOutputRef } from '../../../../../shared/api/workspaceTaskContract'
import { i18n } from '../../../../../shared/i18n'
import { resolveWorkspaceMediaAssetDerivativeURL } from '../../../../../shared/api/mediaAssetDelivery'
import { notifyUserCreditsInvalidation } from '../../../../../shared/query/userCreditsInvalidation'
import type { TaskImageResult } from '../../../../../shared/query/taskResult'
import {
  createTaskCancelledError,
  createTaskResultFailureError,
  TaskResultPendingError,
} from '../task-runner/taskResultErrors'
import {
  requireCanvasWorkspaceId,
  resolveBoundCanvasTaskWorkspaceId,
} from './canvasTaskCancellation'

const STORAGE_OUTPUT_MARKERS = ['ws-uploads', 'builtin-local', 'storage_key']

function toWorkspaceImageResults(
  workspaceId: string,
  outputs: readonly WorkspaceTaskOutputRef[],
): TaskImageResult[] {
  const results: TaskImageResult[] = []
  for (const ref of outputs) {
    if (ref.kind === 'asset') {
      const assetId = `${ref.asset_id ?? ''}`.trim()
      if (!assetId) continue
      // Explicit workspace binding: the task may settle under a workspace
      // that differs from the current selection (registry path), so the
      // delivery URL must not resolve the workspace implicitly.
      results.push({
        url: resolveWorkspaceMediaAssetDerivativeURL(workspaceId, assetId, 'preview'),
        assetId,
        ...(Number.isInteger(ref.result_index) && ref.result_index >= 0
          ? { resultIndex: ref.result_index }
          : {}),
      })
      continue
    }
    if (ref.kind === 'remote') {
      // v1 refs carry no width/height/mime: downstream resolves display
      // size from the URL. Storage internals never become browser URLs.
      const url = `${ref.url ?? ''}`.trim()
      if (!url) continue
      if (STORAGE_OUTPUT_MARKERS.some((marker) => url.toLowerCase().includes(marker))) continue
      results.push({
        url,
        ...(Number.isInteger(ref.result_index) && ref.result_index >= 0
          ? { resultIndex: ref.result_index }
          : {}),
      })
    }
  }
  return results
}

export function extractWorkspaceTaskImageResults(
  workspaceId: string,
  outputs: readonly WorkspaceTaskOutputRef[] | undefined,
): TaskImageResult[] {
  // Role parity with the legacy extractor (`role === 'image'`), relaxed to
  // accept blank roles: v1 output_refs are image outputs by construction of
  // the canvas flows, and several carry no role fact.
  const imageRefs = (outputs ?? []).filter((ref) => !ref.role?.trim() || ref.role === 'image')
  return toWorkspaceImageResults(workspaceId, imageRefs)
}

function settleWorkspaceTaskItem(
  taskId: string,
  item: WorkspaceTaskItem,
): TaskImageResult[] | null {
  const workspaceId = item.workspace_id
  if (item.status === 'completed') {
    const results = extractWorkspaceTaskImageResults(workspaceId, item.output_refs)
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'completed' })
    if (results.length > 0) return results
    throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImage'))
  }
  if (item.status === 'failed') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'failed' })
    // v1 tasks carry no execution_metrics: the legacy finalize-failure
    // pending detection is unavailable, so finalize failures surface as
    // failed immediately (documented residual).
    throw createTaskResultFailureError({ error_msg: item.error_msg })
  }
  if (item.status === 'cancelled') {
    const results = extractWorkspaceTaskImageResults(workspaceId, item.output_refs)
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
    // cancelled_with_results converges to cancelled on the v1 surface with
    // outputs carrying the results (legacy #676 behavior preserved).
    if (results.length > 0) return results
    throw createTaskCancelledError({
      cancel_reason: item.cancel_reason,
      error_msg: item.error_msg,
    })
  }
  return null
}

function resolveWaitWorkspaceId(taskId: string): string {
  return resolveBoundCanvasTaskWorkspaceId(taskId) ?? requireCanvasWorkspaceId()
}

export async function waitCanvasWorkspaceTaskImageResults(
  taskId: string,
  options?: { timeoutMs?: number | undefined },
): Promise<TaskImageResult[]> {
  const workspaceId = resolveWaitWorkspaceId(taskId)
  const readOptions = options?.timeoutMs == null ? {} : { timeoutMs: options.timeoutMs }
  let fast: WorkspaceTaskItem | undefined
  try {
    fast = await getWorkspaceTask(workspaceId, taskId)
  } catch (error) {
    if (isWorkspaceTaskGoneError(error)) throw error
    // Non-404 fast-path read failures fall through to the terminal wait
    // (parity with the legacy fast path, which fell back to SSE wait).
  }
  if (fast) {
    // Terminal settlement errors propagate; only live tasks continue below.
    const settled = settleWorkspaceTaskItem(taskId, fast)
    if (settled) return settled
  }
  let item: WorkspaceTaskItem
  try {
    item = await waitWorkspaceTaskTerminal(workspaceId, taskId, readOptions)
  } catch (error) {
    if (isWorkspaceTaskGoneError(error)) throw error
    if (error instanceof TaskResultPendingError) throw error
    throw new TaskResultPendingError('timeout', error instanceof Error ? error.message : '')
  }
  // Settlement errors (failure/cancelled/no-image) propagate unwrapped: only
  // waiter transport rejections map to the timeout pending kind above.
  const settled = settleWorkspaceTaskItem(taskId, item)
  if (settled) return settled
  throw new TaskResultPendingError('timeout')
}
