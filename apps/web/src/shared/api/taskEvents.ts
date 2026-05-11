import { queryClient } from '../query/queryClient'
import { queryKeys } from '../query/keys'
import { extractImageUrlsFromTaskResult } from '../query/taskResult'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'
import type { Task } from './task'

type TaskEventPayload = Pick<Task, 'task_id' | 'status' | 'result' | 'error_msg'>

const SSE_TIMEOUT_MS = 6 * 60 * 1000 // 6 minutes

/**
 * Wait for a task to complete via SSE.
 * Polling fallback was intentionally removed; task progress now depends on the SSE endpoint.
 */
export function waitTaskResultWithSse(
  taskId: string,
  options?: { timeoutMs?: number },
): Promise<string[]> {
  const timeoutMs = options?.timeoutMs ?? SSE_TIMEOUT_MS
  if (typeof EventSource === 'undefined') {
    return Promise.reject(new Error('当前浏览器不支持任务进度推送'))
  }
  return waitWithEventSource(taskId, timeoutMs)
}

function waitWithEventSource(taskId: string, timeoutMs: number): Promise<string[]> {
  return new Promise((resolve, reject) => {
    const url = `/api/user/drawing/${encodeURIComponent(taskId)}/events`
    const es = new EventSource(url, { withCredentials: true })
    let settled = false

    function cleanup() {
      clearTimeout(timeout)
      es.close()
    }

    function settle<T>(fn: (value: T) => void, value: T) {
      if (settled) return
      settled = true
      cleanup()
      fn(value)
    }

    const timeout = window.setTimeout(() => {
      if (!settled) {
        settled = true
        es.close()
        reject(new Error('任务进度推送超时，请稍后查看结果'))
      }
    }, timeoutMs)

    const handleTaskEvent = (task: TaskEventPayload) => {
      queryClient.setQueryData(queryKeys.tasks.detail(taskId), (old: Task | undefined) => ({ ...old, ...task }))

      if (task.status === 'completed') {
        settle(resolve, extractImageUrlsFromTaskResult(task.result))
        return
      }
      if (task.status === 'failed') {
        const msg = task.error_msg?.trim() || '生成失败'
        settle(reject, new Error(sanitizeApiErrorMessage(msg, '生成失败')))
      }
    }

    es.addEventListener('task', (event: MessageEvent) => {
      try {
        const task = JSON.parse(event.data) as TaskEventPayload
        handleTaskEvent(task)
      } catch {
        // Ignore malformed events
      }
    })

    es.onerror = () => {
      if (!settled) {
        settle(reject, new Error('任务进度连接中断，请稍后重试'))
      }
    }
  })
}

/**
 * Check if SSE is supported by the browser.
 */
export function isSseSupported(): boolean {
  return typeof EventSource !== 'undefined'
}
