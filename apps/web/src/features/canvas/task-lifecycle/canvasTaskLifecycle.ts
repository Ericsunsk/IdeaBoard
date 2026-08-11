export const CANVAS_TASK_CANCEL_RETRY_DELAYS_MS = [1000, 3000] as const

type CanvasTaskCancelAdapter = {
  cancel: (taskId: string) => Promise<void>
}

type CanvasTaskRetryScheduler = (callback: () => void, delayMs: number) => void

type CanvasTaskFinalFailure = {
  taskId: string
  error: unknown
}

export type CanvasTaskLifecycleOptions = {
  cancel: CanvasTaskCancelAdapter['cancel']
  scheduleRetry?: CanvasTaskRetryScheduler
  onFinalFailure?: (failure: CanvasTaskFinalFailure) => void
}

export type CanvasTaskLifecycle = {
  cancelTasks: (taskIds: readonly string[]) => void
}

function defaultScheduleRetry(callback: () => void, delayMs: number) {
  window.setTimeout(callback, delayMs)
}

function defaultOnFinalFailure(failure: CanvasTaskFinalFailure) {
  console.warn('[canvas] failed to cancel task', failure)
}

export function createCanvasTaskLifecycle(
  options: CanvasTaskLifecycleOptions,
): CanvasTaskLifecycle {
  const scheduleRetry = options.scheduleRetry ?? defaultScheduleRetry
  const onFinalFailure = options.onFinalFailure ?? defaultOnFinalFailure

  function cancelWithRetry(taskId: string, retryAttempt = 0) {
    void options.cancel(taskId).catch((error: unknown) => {
      const retryDelay = CANVAS_TASK_CANCEL_RETRY_DELAYS_MS[retryAttempt]
      if (retryDelay !== undefined) {
        scheduleRetry(() => cancelWithRetry(taskId, retryAttempt + 1), retryDelay)
        return
      }
      onFinalFailure({ taskId, error })
    })
  }

  function cancelTasks(taskIds: readonly string[]) {
    const uniqueTaskIds = Array.from(
      new Set(taskIds.map((taskId) => taskId.trim()).filter(Boolean)),
    )
    uniqueTaskIds.forEach((taskId) => cancelWithRetry(taskId))
  }

  return { cancelTasks }
}
