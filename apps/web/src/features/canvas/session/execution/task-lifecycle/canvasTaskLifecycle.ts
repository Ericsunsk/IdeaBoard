export const CANVAS_TASK_CANCEL_RETRY_DELAYS_MS = [1000, 3000, 10_000, 30_000] as const

type CanvasTaskCancelAdapter = {
  cancel: (taskId: string) => Promise<void>
}

type CanvasTaskRetryScheduler = (callback: () => void, delayMs: number) => void

type CanvasTaskFinalFailure = {
  taskId: string
  error: unknown
}

type CanvasTaskLifecycleOptions = {
  cancel: CanvasTaskCancelAdapter['cancel']
  scheduleRetry?: CanvasTaskRetryScheduler
  onFinalFailure?: (failure: CanvasTaskFinalFailure) => void
}

type CanvasTaskLifecycle = {
  cancelTasks: (taskIds: readonly string[]) => void
  /**
   * 持久化可重试取消 (#P1-3): 最终失败的 taskId 留档（远端可能仍在跑），
   * 下次 cancelTasks 捎带重提；UI 对账入口经 `retryPendingCancels` 手动
   * 重试。成功即清档。
   */
  pendingCancelTaskIds: () => string[]
  retryPendingCancels: () => void
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
  // 持久留档：最终失败的取消不止于 console.warn，taskId 留在内存注册
  // 表中供捎带重试与手动重试（远端可能仍在跑，见调用方 toast 对账）。
  const pendingFailures = new Map<string, unknown>()

  function cancelWithRetry(taskId: string, retryAttempt = 0) {
    void options.cancel(taskId).then(
      () => {
        pendingFailures.delete(taskId)
      },
      (error: unknown) => {
        const retryDelay = CANVAS_TASK_CANCEL_RETRY_DELAYS_MS[retryAttempt]
        if (retryDelay !== undefined) {
          scheduleRetry(() => cancelWithRetry(taskId, retryAttempt + 1), retryDelay)
          return
        }
        pendingFailures.set(taskId, error)
        onFinalFailure({ taskId, error })
      },
    )
  }

  function cancelTasks(taskIds: readonly string[]) {
    const uniqueTaskIds = Array.from(
      new Set(taskIds.map((taskId) => taskId.trim()).filter(Boolean)),
    )
    // 捎带重试：此前最终失败的留档取消随本次一起重提（幂等取消，
    // gone/terminal 由 cancel 缝收敛为静默成功并清档）。
    for (const pendingTaskId of pendingFailures.keys()) {
      if (!uniqueTaskIds.includes(pendingTaskId)) uniqueTaskIds.push(pendingTaskId)
    }
    uniqueTaskIds.forEach((taskId) => cancelWithRetry(taskId))
  }

  function pendingCancelTaskIds(): string[] {
    return [...pendingFailures.keys()]
  }

  function retryPendingCancels(): void {
    const pending = pendingCancelTaskIds()
    pending.forEach((taskId) => cancelWithRetry(taskId))
  }

  return { cancelTasks, pendingCancelTaskIds, retryPendingCancels }
}
