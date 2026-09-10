import {
  taskApi,
  type SubmitTaskData,
  type SubmitTaskReadinessRequestData,
  type Task,
} from '../../../../../shared/api/task'
import {
  createTaskResultFailureError,
  createTaskCancelledError,
  resolveCancelledWithResults,
  waitTaskImageResultsWithSse,
} from '../../../../../shared/api/taskEvents'
import {
  extractImageResultsFromTaskPayload,
  type TaskImageResult,
} from '../../../../../shared/query/taskResult'
import { assertTaskSubmitReady } from './taskReadiness'
import { notifyUserCreditsInvalidation } from '../../../../../shared/query/userCreditsInvalidation'
import { mapCanvasBillingRequestError } from './canvasBillingPreflight'
import { i18n } from '../../../../../shared/i18n'

export async function assertTaskRequestReady(
  request: SubmitTaskReadinessRequestData,
  fallbackMessage: string = i18n.global.t('common.errors.taskSubmitFailed'),
): Promise<void> {
  await assertTaskSubmitReady(request, fallbackMessage)
}

export async function submitTaskRequest(
  request: SubmitTaskData,
  fallbackMessage: string = i18n.global.t('common.errors.taskSubmitFailed'),
): Promise<{ taskId: string }> {
  let submission: Awaited<ReturnType<typeof taskApi.submit>>
  try {
    submission = await taskApi.submit(request)
  } catch (error) {
    throw mapCanvasBillingRequestError(error)
  }
  if (!submission.task_id) throw new Error(fallbackMessage)
  notifyUserCreditsInvalidation({ source: 'task-hold', taskId: submission.task_id })
  return { taskId: submission.task_id }
}

export async function waitExistingOrLiveTaskImageResults(
  taskId: string,
): Promise<TaskImageResult[]> {
  let task: FastPathTerminalTask
  try {
    const taskData = await taskApi.get(taskId)
    task = taskData.task
  } catch (error) {
    const status = Number((error as { status?: unknown; code?: unknown })?.status)
    const code = Number((error as { status?: unknown; code?: unknown })?.code)
    if (status === 404 || code === 404) throw error
    return waitTaskImageResultsWithSse(taskId)
  }
  return settleFastPathTerminal(taskId, task) ?? waitTaskImageResultsWithSse(taskId)
}

type FastPathTerminalTask = Pick<
  Task,
  'status' | 'outputs' | 'result' | 'error_msg' | 'cancel_reason' | 'execution_metrics'
> | null
| undefined

/**
 * Single terminal-status settlement for the task fast path (issue #676).
 * Returns the materialized results, throws the mapped terminal error, or
 * returns null when the task is still live (caller continues to SSE wait).
 */
function settleFastPathTerminal(taskId: string, task: FastPathTerminalTask): TaskImageResult[] | null {
  if (!task) return null
  if (task.status === 'completed') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'completed' })
    const results = extractImageResultsFromTaskPayload(task)
    if (results.length > 0) return results
    throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImage'))
  }
  if (task.status === 'failed') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'failed' })
    throw createTaskResultFailureError(task)
  }
  if (task.status === 'cancelled') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
    throw createTaskCancelledError(task)
  }
  const cancelledResults = resolveCancelledWithResults(task)
  if (cancelledResults) {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
    return cancelledResults
  }
  if (task.status === 'cancelled_with_results') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
    throw createTaskCancelledError(task)
  }
  return null
}
