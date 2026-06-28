export type ImageProcessingSchedulerTaskContext = {
  taskId: string
  revision: number
  signal: AbortSignal
  isCurrent: () => boolean
}

export type ImageProcessingSchedulerRunner<TValue> = (
  context: ImageProcessingSchedulerTaskContext,
) => Promise<TValue> | TValue

export type ImageProcessingSchedulerResult<TValue> =
  | {
      status: 'completed'
      taskId: string
      revision: number
      value: TValue
    }
  | {
      status: 'cancelled' | 'stale'
      taskId: string
      revision: number
      reason: string
    }
  | {
      status: 'failed'
      taskId: string
      revision: number
      error: unknown
    }

export type ImageProcessingScheduleOptions = {
  taskId?: string
  debounceMs?: number
  latestOnly?: boolean
}

export type ImageProcessingSchedulerOptions = {
  debounceMs?: number
  latestOnly?: boolean
  createTaskId?: (revision: number) => string
}

type TimerHandle = ReturnType<typeof setTimeout>

type ScheduledTask<TValue> = {
  key: string
  taskId: string
  revision: number
  latestOnly: boolean
  controller: AbortController
  runner: ImageProcessingSchedulerRunner<TValue>
  resolve: (result: ImageProcessingSchedulerResult<TValue>) => void
  settled: boolean
  timer: TimerHandle | null
}

const DEFAULT_LATEST_ONLY = true

export class ImageProcessingScheduler {
  private revision = 0
  private disposed = false
  private readonly pendingTasks = new Map<string, ScheduledTask<unknown>>()
  private readonly activeTasks = new Map<string, ScheduledTask<unknown>>()
  private readonly defaultDebounceMs: number
  private readonly defaultLatestOnly: boolean
  private readonly createTaskId: (revision: number) => string

  constructor(options: ImageProcessingSchedulerOptions = {}) {
    this.defaultDebounceMs = Math.max(0, options.debounceMs ?? 0)
    this.defaultLatestOnly = options.latestOnly ?? DEFAULT_LATEST_ONLY
    this.createTaskId = options.createTaskId ?? ((revision) => `image-processing-${revision}`)
  }

  get currentRevision() {
    return this.revision
  }

  schedule<TValue>(
    runner: ImageProcessingSchedulerRunner<TValue>,
    options: ImageProcessingScheduleOptions = {},
  ): Promise<ImageProcessingSchedulerResult<TValue>> {
    if (this.disposed) {
      const revision = this.revision
      const taskId = options.taskId ?? this.createTaskId(revision)
      return Promise.resolve({
        status: 'cancelled',
        taskId,
        revision,
        reason: 'scheduler disposed',
      })
    }

    const revision = this.revision + 1
    this.revision = revision
    const taskId = options.taskId ?? this.createTaskId(revision)
    const latestOnly = options.latestOnly ?? this.defaultLatestOnly
    const debounceMs = Math.max(0, options.debounceMs ?? this.defaultDebounceMs)

    if (latestOnly) {
      this.settleAll('stale', 'superseded by newer revision')
    }

    return new Promise<ImageProcessingSchedulerResult<TValue>>((resolve) => {
      const task: ScheduledTask<TValue> = {
        key: `${revision}:${taskId}`,
        taskId,
        revision,
        latestOnly,
        controller: new AbortController(),
        runner,
        resolve,
        settled: false,
        timer: null,
      }

      if (debounceMs > 0) {
        task.timer = setTimeout(() => {
          this.pendingTasks.delete(task.key)
          this.startTask(task)
        }, debounceMs)
        this.pendingTasks.set(task.key, task as ScheduledTask<unknown>)
        return
      }

      this.startTask(task)
    })
  }

  cancel(taskId?: string): number {
    if (!taskId) {
      return this.settleAll('cancelled', 'cancelled')
    }

    let cancelled = 0
    for (const task of this.pendingTasks.values()) {
      if (task.taskId !== taskId) continue
      this.pendingTasks.delete(task.key)
      this.settleTask(task, 'cancelled', 'cancelled')
      cancelled += 1
    }

    for (const task of this.activeTasks.values()) {
      if (task.taskId !== taskId) continue
      this.activeTasks.delete(task.key)
      this.settleTask(task, 'cancelled', 'cancelled')
      cancelled += 1
    }

    return cancelled
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    this.cancel()
  }

  private startTask<TValue>(task: ScheduledTask<TValue>) {
    if (task.settled) return
    this.activeTasks.set(task.key, task as ScheduledTask<unknown>)

    const context: ImageProcessingSchedulerTaskContext = {
      taskId: task.taskId,
      revision: task.revision,
      signal: task.controller.signal,
      isCurrent: () => this.isTaskCurrent(task),
    }

    Promise.resolve()
      .then(() => task.runner(context))
      .then((value) => {
        if (task.settled) return
        const isCurrent = this.isTaskCurrent(task)
        this.activeTasks.delete(task.key)
        if (!isCurrent) {
          this.settleTask(task, 'stale', 'superseded by newer revision')
          return
        }
        this.settleCompletedTask(task, value)
      })
      .catch((error: unknown) => {
        this.activeTasks.delete(task.key)
        if (task.settled) return
        if (task.controller.signal.aborted) {
          this.settleTask(task, 'cancelled', 'cancelled')
          return
        }
        task.settled = true
        task.resolve({
          status: 'failed',
          taskId: task.taskId,
          revision: task.revision,
          error,
        })
      })
  }

  private isTaskCurrent<TValue>(task: ScheduledTask<TValue>) {
    if (task.controller.signal.aborted) return false
    if (!task.latestOnly) return this.activeTasks.get(task.key) === task
    return this.revision === task.revision && this.activeTasks.get(task.key) === task
  }

  private settleAll(status: 'cancelled' | 'stale', reason: string) {
    let settled = 0
    for (const task of this.pendingTasks.values()) {
      this.settleTask(task, status, reason)
      settled += 1
    }
    this.pendingTasks.clear()

    for (const task of this.activeTasks.values()) {
      this.settleTask(task, status, reason)
      settled += 1
    }
    this.activeTasks.clear()
    return settled
  }

  private settleTask<TValue>(
    task: ScheduledTask<TValue>,
    status: 'cancelled' | 'stale',
    reason: string,
  ) {
    if (task.settled) return
    task.settled = true
    if (task.timer) {
      clearTimeout(task.timer)
      task.timer = null
    }
    if (!task.controller.signal.aborted) {
      task.controller.abort(reason)
    }
    task.resolve({
      status,
      taskId: task.taskId,
      revision: task.revision,
      reason,
    })
  }

  private settleCompletedTask<TValue>(task: ScheduledTask<TValue>, value: TValue) {
    if (task.settled) return
    task.settled = true
    task.resolve({
      status: 'completed',
      taskId: task.taskId,
      revision: task.revision,
      value,
    })
  }
}
