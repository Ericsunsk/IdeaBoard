import { queryClient } from '../query/queryClient'
import { getGetUserTaskQueryKey } from './generated-query/tasks/tasks'
import type { ApiResponseTask } from './generated-query/model'
import {
  extractImageResultsFromTaskPayload,
  type TaskImageResult,
} from '../query/taskResult'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'
import { taskApi } from './task'
import type { Task } from './task'
import { resolveApiUrl } from '../config/runtimeUrls'

type TaskEventPayload = Pick<
  Task,
  | 'task_id'
  | 'status'
  | 'result'
  | 'error_msg'
  | 'outputs'
  | 'cancel_reason'
  | 'execution_metrics'
>

const SSE_TIMEOUT_MS = 6 * 60 * 1000
const POLL_INITIAL_DELAY_MS = 1200
const POLL_MAX_DELAY_MS = 8000
const POLL_BACKOFF_FACTOR = 1.6
const RESULT_FINALIZATION_PENDING_MESSAGE = '图片已生成，但结果保存暂时失败，请稍后重试'

type WaitTaskResultOptions = {
  timeoutMs?: number
  pollInitialDelayMs?: number
  pollMaxDelayMs?: number
}

export class TaskResultPendingError extends Error {
  constructor(message = '任务仍在处理中，请稍后查看结果') {
    super(message)
    this.name = 'TaskResultPendingError'
  }
}

export function isTaskResultPendingError(error: unknown): error is TaskResultPendingError {
  return error instanceof TaskResultPendingError
}

type TaskFailurePayload = Pick<TaskEventPayload, 'error_msg' | 'execution_metrics'>

function normalizeTaskFailureValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function isResultFinalizationFailure(task: TaskFailurePayload): boolean {
  const metrics = task.execution_metrics
  const failureStage = normalizeTaskFailureValue(metrics?.failure_stage)
  const failureCategory = normalizeTaskFailureValue(metrics?.failure_category)
  if (
    failureStage === 'finalizing' &&
    (failureCategory === 'storage_error' || failureCategory === 'finalize_error')
  ) {
    return true
  }

  const message = normalizeTaskFailureValue(task.error_msg)
  return message.includes('图片结果保存失败') || message.includes('任务结果保存失败')
}

export function createTaskResultFailureError(
  task: TaskFailurePayload,
  fallbackMessage = '生成失败',
): Error {
  if (isResultFinalizationFailure(task)) {
    return new TaskResultPendingError(RESULT_FINALIZATION_PENDING_MESSAGE)
  }
  const msg = task.error_msg?.trim() || fallbackMessage
  return new Error(sanitizeApiErrorMessage(msg, fallbackMessage))
}

export function waitTaskImageResultsWithSse(
  taskId: string,
  options?: WaitTaskResultOptions,
): Promise<TaskImageResult[]> {
  const timeoutMs = options?.timeoutMs ?? SSE_TIMEOUT_MS
  const deadlineAt = Date.now() + timeoutMs
  const pollOptions = {
    initialDelayMs: options?.pollInitialDelayMs ?? POLL_INITIAL_DELAY_MS,
    maxDelayMs: options?.pollMaxDelayMs ?? POLL_MAX_DELAY_MS,
  }
  if (typeof EventSource === 'undefined') {
    return waitWithPolling(taskId, deadlineAt, pollOptions)
  }
  return waitWithEventSource(taskId, deadlineAt, pollOptions)
}

function waitWithEventSource(
  taskId: string,
  deadlineAt: number,
  pollOptions: { initialDelayMs: number; maxDelayMs: number },
): Promise<TaskImageResult[]> {
  return new Promise((resolve, reject) => {
    const es = new EventSource(resolveApiUrl(`/user/tasks/${encodeURIComponent(taskId)}/events`), {
      withCredentials: true,
    })
    let settled = false
    let timeout = 0

    const cleanup = () => {
      clearTimeout(timeout)
      es.close()
    }

    const settleResolve = (value: TaskImageResult[]) => {
      if (settled) return
      settled = true
      cleanup()
      resolve(value)
    }

    const settleReject = (error: Error) => {
      if (settled) return
      settled = true
      cleanup()
      reject(error)
    }

    const fallbackToPolling = () => {
      if (settled) return
      settled = true
      cleanup()
      waitWithPolling(taskId, deadlineAt, pollOptions).then(resolve, reject)
    }

    timeout = window.setTimeout(
      () => {
        settleReject(new TaskResultPendingError('任务进度推送超时，请稍后查看结果'))
      },
      Math.max(0, deadlineAt - Date.now()),
    )

    const handleTaskEvent = (task: TaskEventPayload) => {
      if (!task || typeof task !== 'object' || typeof task.status !== 'string') {
        settleReject(new Error('任务进度数据异常，请稍后重试'))
        return
      }

      queryClient.setQueryData(
        getGetUserTaskQueryKey(taskId),
        (old: ApiResponseTask | undefined) => {
          if (!old?.data?.task) return old
          return {
            ...old,
            data: {
              ...old.data,
              task: { ...old.data.task, ...task },
            },
          }
        },
      )

      if (task.status === 'completed') {
        settleResolve(extractImageResultsFromTaskEvent(task))
        return
      }
      if (task.status === 'failed') {
        settleReject(createTaskResultFailureError(task))
        return
      }
      if (task.status === 'cancelled') {
        settleReject(createTaskCancelledError(task))
      }
    }

    es.addEventListener('task', (event: MessageEvent) => {
      try {
        handleTaskEvent(JSON.parse(event.data) as TaskEventPayload)
      } catch {
        settleReject(new Error('任务进度数据异常，请稍后重试'))
      }
    })

    es.addEventListener('close', () => {
      fallbackToPolling()
    })

    es.onerror = () => {
      fallbackToPolling()
    }
  })
}

async function waitWithPolling(
  taskId: string,
  deadlineAt: number,
  options: { initialDelayMs: number; maxDelayMs: number },
): Promise<TaskImageResult[]> {
  let nextDelayMs = options.initialDelayMs
  let lastError: unknown

  while (Date.now() <= deadlineAt) {
    let task: TaskEventPayload | undefined
    try {
      const taskRes = await taskApi.get(taskId)
      if (taskRes.data?.task) {
        queryClient.setQueryData(getGetUserTaskQueryKey(taskId), taskRes)
      }
      task = taskRes.data?.task
      lastError = undefined
    } catch (error) {
      if (isTaskNotFoundError(error)) throw error
      lastError = error
    }

    if (task?.status === 'completed') {
      return extractImageResultsFromTaskEvent(task)
    }
    if (task?.status === 'failed') {
      throw createTaskResultFailureError(task)
    }
    if (task?.status === 'cancelled') {
      throw createTaskCancelledError(task)
    }

    const remainingMs = deadlineAt - Date.now()
    if (remainingMs <= 0) break
    await sleep(Math.min(nextDelayMs, remainingMs))
    nextDelayMs = Math.min(
      options.maxDelayMs,
      Math.max(nextDelayMs + 1, Math.ceil(nextDelayMs * POLL_BACKOFF_FACTOR)),
    )
  }

  if (lastError) {
    throw new TaskResultPendingError('任务状态查询超时，请稍后查看结果')
  }
  throw new TaskResultPendingError('任务进度推送超时，请稍后查看结果')
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

function isTaskNotFoundError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  return code === 404
}

export function createTaskCancelledError(task?: Pick<Task, 'error_msg' | 'cancel_reason'>): Error {
  const msg = task?.cancel_reason?.trim() || task?.error_msg?.trim() || '任务已取消'
  return new Error(sanitizeApiErrorMessage(msg, '任务已取消'))
}

function extractImageResultsFromTaskEvent(task: TaskEventPayload): TaskImageResult[] {
  return extractImageResultsFromTaskPayload(task)
}
