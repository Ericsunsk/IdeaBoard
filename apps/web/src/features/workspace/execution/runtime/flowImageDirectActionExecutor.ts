import { taskApi, type SubmitTaskData } from '../../../../shared/api/task'
import {
  createTaskCancelledError,
  waitTaskImageResultsWithSse,
} from '../../../../shared/api/taskEvents'
import {
  extractImageResultsFromTaskPayload,
  type TaskImageResult,
} from '../../../../shared/query/taskResult'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import { assertTaskSubmitReady } from './taskReadiness'

export async function assertTaskRequestReady(
  request: SubmitTaskData,
  fallbackMessage = '提交任务失败',
): Promise<void> {
  await assertTaskSubmitReady(request, fallbackMessage)
}

export async function submitTaskRequest(
  request: SubmitTaskData,
  fallbackMessage = '提交任务失败',
): Promise<{ taskId: string }> {
  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, fallbackMessage))
  }
  return { taskId: submitRes.data.task_id }
}

export async function waitExistingOrLiveTaskImageResults(
  taskId: string,
): Promise<TaskImageResult[]> {
  let task:
    | {
        status?: string
        outputs?: Array<{
          role?: string
          url?: string
          width?: number
          height?: number
          mime_type?: string
        }>
        result?: string | null
        error_msg?: string
        cancel_reason?: string
      }
    | undefined
  try {
    const taskRes = await taskApi.get(taskId)
    task = taskRes.data?.task
  } catch (error) {
    const status = Number((error as { status?: unknown; code?: unknown })?.status)
    const code = Number((error as { status?: unknown; code?: unknown })?.code)
    if (status === 404 || code === 404) throw error
    return waitTaskImageResultsWithSse(taskId)
  }
  if (task?.status === 'completed') {
    const results = extractImageResultsFromTaskPayload(task)
    if (results.length > 0) return results
    throw new Error('任务完成但未返回图片')
  }
  if (task?.status === 'failed') {
    throw new Error(sanitizeApiErrorMessage(task.error_msg, '生成失败'))
  }
  if (task?.status === 'cancelled') {
    throw createTaskCancelledError(task)
  }
  return waitTaskImageResultsWithSse(taskId)
}
