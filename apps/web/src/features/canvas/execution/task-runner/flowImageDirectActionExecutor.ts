import {
  taskApi,
  type SubmitTaskData,
  type SubmitTaskReadinessRequestData,
  type Task,
} from '../../../../shared/api/task'
import {
  createTaskResultFailureError,
  createTaskCancelledError,
  waitTaskImageResultsWithSse,
} from '../../../../shared/api/taskEvents'
import {
  extractImageResultsFromTaskPayload,
  type TaskImageResult,
} from '../../../../shared/query/taskResult'
import { assertTaskSubmitReady } from './taskReadiness'
import { notifyUserCreditsInvalidation } from '../../../../shared/query/userCreditsInvalidation'
import { mapCanvasBillingRequestError } from './canvasBillingPreflight'

export async function assertTaskRequestReady(
  request: SubmitTaskReadinessRequestData,
  fallbackMessage = '提交任务失败',
): Promise<void> {
  await assertTaskSubmitReady(request, fallbackMessage)
}

export async function submitTaskRequest(
  request: SubmitTaskData,
  fallbackMessage = '提交任务失败',
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
  let task:
    | Pick<
        Task,
        'status' | 'outputs' | 'result' | 'error_msg' | 'cancel_reason' | 'execution_metrics'
      >
    | undefined
  try {
    const taskData = await taskApi.get(taskId)
    task = taskData.task
  } catch (error) {
    const status = Number((error as { status?: unknown; code?: unknown })?.status)
    const code = Number((error as { status?: unknown; code?: unknown })?.code)
    if (status === 404 || code === 404) throw error
    return waitTaskImageResultsWithSse(taskId)
  }
  if (task?.status === 'completed') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'completed' })
    const results = extractImageResultsFromTaskPayload(task)
    if (results.length > 0) return results
    throw new Error('任务完成但未返回图片')
  }
  if (task?.status === 'failed') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'failed' })
    throw createTaskResultFailureError(task)
  }
  if (task?.status === 'cancelled') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
    throw createTaskCancelledError(task)
  }
  return waitTaskImageResultsWithSse(taskId)
}
