import { queryClient } from '../query/queryClient'
import { getGetUserTaskQueryKey } from './generated-query/tasks/tasks'
import type { ApiResponseTask } from './generated-query/model'
import { extractImageUrlsFromTaskResult } from '../query/taskResult'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'
import type { Task } from './task'

type TaskEventPayload = Pick<Task, 'task_id' | 'status' | 'result' | 'error_msg'>

const SSE_TIMEOUT_MS = 6 * 60 * 1000

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
    const es = new EventSource(`/api/user/drawing/${encodeURIComponent(taskId)}/events`, { withCredentials: true })
    let settled = false

    const cleanup = () => {
      clearTimeout(timeout)
      es.close()
    }

    const settleResolve = (value: string[]) => {
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

    const timeout = window.setTimeout(() => {
      settleReject(new Error('任务进度推送超时，请稍后查看结果'))
    }, timeoutMs)

    const handleTaskEvent = (task: TaskEventPayload) => {
      if (!task || typeof task !== 'object' || typeof task.status !== 'string') {
        settleReject(new Error('任务进度数据异常，请稍后重试'))
        return
      }

      queryClient.setQueryData(getGetUserTaskQueryKey(taskId), (old: ApiResponseTask | undefined) => {
        if (!old?.data?.task) return old
        return {
          ...old,
          data: {
            ...old.data,
            task: { ...old.data.task, ...task },
          },
        }
      })

      if (task.status === 'completed') {
        settleResolve(extractImageUrlsFromTaskResult(task.result))
        return
      }
      if (task.status === 'failed') {
        const msg = task.error_msg?.trim() || '生成失败'
        settleReject(new Error(sanitizeApiErrorMessage(msg, '生成失败')))
      }
    }

    es.addEventListener('task', (event: MessageEvent) => {
      try {
        handleTaskEvent(JSON.parse(event.data) as TaskEventPayload)
      } catch {
        settleReject(new Error('任务进度数据异常，请稍后重试'))
      }
    })

    es.onerror = () => {
      settleReject(new Error('任务进度连接中断，请稍后重试'))
    }
  })
}
