// Canvas task SSE/poll waiting seam (issue #859).
//
// Maps the canonical terminal waiter into typed completed/cancelled results.
// Transport failures remain pending; terminal task failures propagate.
// Asset refs retain explicit workspace binding. Lifecycle and feedback stay
// in the execution workspace assembly.

import { isWorkspaceTaskGoneError } from '@/api/tasks/workspaceTaskTransport'
import { waitWorkspaceTaskTerminal } from '@/api/tasks/workspaceTaskWaiter'
import {
  isWorkspaceTaskTerminal,
  type WorkspaceTaskItem,
} from '@/api/tasks/workspaceTaskContract'
import type { WorkspaceTaskOutputRef } from '@/api/tasks/workspaceTaskContract'
import { i18n } from '../../../../../core/i18n'
import { notifyUserCreditsInvalidation } from '../../../../../core/query/userCreditsInvalidation'
import type { TaskImageResult } from '../../../../../core/query/taskResult'
import type { WaitWorkspaceTaskOptions } from '@/api/tasks/workspaceTaskWaiter'
import type { TaskImageOutcome } from '../task-runner/taskImageOutcome'
import {
  createTaskResultFailureError,
  TaskResultPendingError,
} from '../task-runner/taskResultErrors'
import {
  requireCanvasWorkspaceId,
  resolveBoundCanvasTaskWorkspaceId,
} from './canvasTaskCancellation'

const STORAGE_OUTPUT_MARKERS = ['ws-uploads', 'uploads/ws', 'builtin-local', 'storage_key']

function toAssetImageResult(
  workspaceId: string,
  ref: WorkspaceTaskOutputRef,
): TaskImageResult | null {
  const assetId = `${ref.asset_id ?? ''}`.trim()
  if (!assetId) return null
  return {
    workspaceId,
    assetId,
    ...(Number.isInteger(ref.result_index) && ref.result_index >= 0
      ? { resultIndex: ref.result_index }
      : {}),
    ...(typeof ref.width === 'number' && ref.width > 0 ? { width: ref.width } : {}),
    ...(typeof ref.height === 'number' && ref.height > 0 ? { height: ref.height } : {}),
    ...(ref.mime_type ? { mimeType: ref.mime_type } : {}),
    ...(typeof ref.preview_ready === 'boolean' ? { isPreviewReady: ref.preview_ready } : {}),
  }
}

function toRemoteImageResult(ref: WorkspaceTaskOutputRef): TaskImageResult | null {
  const url = `${ref.url ?? ''}`.trim()
  if (!url || STORAGE_OUTPUT_MARKERS.some((marker) => url.toLowerCase().includes(marker))) {
    return null
  }
  return {
    url,
    ...(Number.isInteger(ref.result_index) && ref.result_index >= 0
      ? { resultIndex: ref.result_index }
      : {}),
  }
}

function toWorkspaceImageResults(
  workspaceId: string,
  outputs: readonly WorkspaceTaskOutputRef[],
): TaskImageResult[] {
  const results: TaskImageResult[] = []
  for (const ref of outputs) {
    if (ref.kind === 'asset') {
      const result = toAssetImageResult(workspaceId, ref)
      if (result) results.push(result)
    } else if (ref.kind === 'remote') {
      const result = toRemoteImageResult(ref)
      if (result) results.push(result)
    }
  }
  return results
}

export function extractWorkspaceTaskImageResults(
  workspaceId: string,
  outputs: readonly WorkspaceTaskOutputRef[] | undefined,
): TaskImageResult[] {
  // Both 'image' and 'result' role facts carry image results (backend returns WorkspaceProviderResultRole="result").
  const imageRefs = (outputs ?? []).filter((ref) => ref.role === 'image' || ref.role === 'result')
  return toWorkspaceImageResults(workspaceId, imageRefs)
}

function settleWorkspaceTaskItem(taskId: string, item: WorkspaceTaskItem): TaskImageOutcome | null {
  // Single terminal source: live tasks return null; every terminal status
  // below notifies exactly once through the shared settlement event.
  if (!isWorkspaceTaskTerminal(item.status)) return null
  notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: item.status })
  const workspaceId = item.workspace_id
  if (item.status === 'completed') {
    const results = extractWorkspaceTaskImageResults(workspaceId, item.output_refs)
    if (results.length > 0) return { kind: 'completed', results }
    throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImage'))
  }
  if (item.status === 'failed') {
    throw createTaskResultFailureError({ error_msg: item.error_msg })
  }
  if (item.status === 'cancelled') {
    return {
      kind: 'cancelled',
      results: extractWorkspaceTaskImageResults(workspaceId, item.output_refs),
    }
  }
  throw new Error(`Unknown workspace task status: ${String(item.status)}`)
}

function resolveWaitWorkspaceId(taskId: string): string {
  return resolveBoundCanvasTaskWorkspaceId(taskId) ?? requireCanvasWorkspaceId()
}

export async function waitCanvasWorkspaceTaskImageOutcome(
  taskId: string,
  options?: WaitWorkspaceTaskOptions,
): Promise<TaskImageOutcome> {
  const workspaceId = resolveWaitWorkspaceId(taskId)
  let item: WorkspaceTaskItem
  try {
    item = await waitWorkspaceTaskTerminal(workspaceId, taskId, options)
  } catch (error) {
    if (isWorkspaceTaskGoneError(error) || options?.signal?.aborted) throw error
    if (error instanceof TaskResultPendingError) throw error
    throw new TaskResultPendingError('timeout', error instanceof Error ? error.message : '')
  }
  const outcome = settleWorkspaceTaskItem(taskId, item)
  if (outcome) return outcome
  throw new TaskResultPendingError('timeout')
}
