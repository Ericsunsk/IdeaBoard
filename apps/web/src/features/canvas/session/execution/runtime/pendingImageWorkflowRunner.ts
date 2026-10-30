import {
  isTaskResultPendingError,
  TaskCancelledError,
  TaskResultPendingError,
} from '../task-runner/taskResultErrors'
import { i18n } from '../../../../../core/i18n'
import { readinessErrorCopy, userErrorCode } from '../../../../../core/errors/errorCatalog'
import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'
import type {
  ImageExecutionInput,
  ImageExecutionResultOptions,
} from '../planning/imageExecutionPlan'
import type {
  ResolvedTaskImageOutcome,
  TaskImageObservation,
  TaskImageObserver,
} from '../task-runner/taskImageOutcome'
import { runWithDeadline } from '../../../../../core/utils/request'
import type { PendingImageResultExecution } from '../result-nodes/usePendingImageResultExecution.ts'
import {
  PendingImageMaterializationError,
  type PendingImageExecutionCore,
  type PendingImageExecutionLease,
} from '../pending-execution/core'
import type { BoardAttachment } from '../board-attachment'
import {
  createCanvasTaskClientActionId,
  type ClientActionIdFactory,
} from '../task-runner/taskClientActionIds'
import {
  isCanvasBillingPreflightError,
  isRetryableCanvasTaskRequestError,
} from '../task-runner/canvasBillingPreflight'
import {
  isCanvasWorkspaceTaskUnmappableError,
  isCanvasWorkspaceUnavailableError,
} from '../workspace/canvasWorkspaceTaskAdapter'
import {
  BACKGROUND_CONTINUATION_MAX_ATTEMPTS,
  backgroundContinuationDelayMs,
  SHARED_TASK_REQUEST_ATTEMPTS,
  CANVAS_FOREGROUND_WAIT_MS,
  CANVAS_TOTAL_WAIT_MS,
} from '../canvasBudgets'

type PendingImageResultOptions = ImageExecutionResultOptions

export type PendingImageWorkflowTaskTransport = Readonly<{
  submit: (request: CanvasTaskRequestData, clientActionId: string) => Promise<{ taskId: string }>
  observe: TaskImageObserver
}>

type PendingImageWorkflowIntent = {
  source: ImageExecutionInput
  sourceNodeId: string
  attachment: BoardAttachment
  outputCount: number
  resultOptions?: PendingImageResultOptions
  taskRequest: CanvasTaskRequestData
  fallbackRatio: number
}

export type PendingImageWorkflowRunner = {
  execute: (intent: PendingImageWorkflowIntent) => Promise<void>
  dispose: () => void
}

export type PendingImageWorkflowRunnerOptions = {
  pendingResults: Pick<
    PendingImageResultExecution,
    'reservePendingResults' | 'attachPendingTaskIds' | 'removePendingResults'
  >
  taskTransport: PendingImageWorkflowTaskTransport
  executionCore: PendingImageExecutionCore
  cancelTasks: (taskIds: readonly string[]) => void
  afterTaskIdAttached?: (() => Promise<void>) | undefined
  createClientActionId?: ClientActionIdFactory | undefined
  /**
   * 后台续轮询调度器 (#P1-2): 默认 `setTimeout`；spec 注入同步/即时
   * 调度以确定性覆盖后台续跑路径。
   */
  scheduleContinuation?: ((callback: () => void, delayMs: number) => () => void) | undefined
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: ((error: unknown) => boolean) | undefined
}

/**
 * 提交重试预算 (#P1-4): submit 最多 2 次尝试（初始 + 单次重试），
 * 仅可重试错误（network / 503 定价瞬态 /
 * 原始 503-429）进入重试；确定性失败（空池、余额、校验 422）不重试。
 * 用户手动重提不受影响（无状态锁）。
 *
 * 数值见 `../canvasBudgets`（冻结预算表，零变更收敛）。
 *
 * Board 落子有界重试见 `../canvasBudgets` 的
 * `PENDING_IMAGE_MATERIALIZATION_ATTEMPTS`（图写原地重试）。
 */

function imageGenerationSuccessMessage(): string {
  return i18n.global.t('canvas.feedback.imageGenerated')
}

function imageGenerationFailureMessage(): string {
  return i18n.global.t('canvas.feedback.imageGenerationFailed')
}

/** Presentation boundary: transport errors carry a kind; copy is resolved here. */
function taskExecutionWarning(error: unknown): string {
  // Fail-closed guidance is user-safe by construction: surface it verbatim
  // instead of collapsing to the generic failure copy. Billing preflight
  // errors carry resolved user copy for the same reason (including the
  // `resolution-unavailable` toast for an empty Service Model Pool).
  if (isCanvasBillingPreflightError(error)) return error.message
  if (isCanvasWorkspaceUnavailableError(error) || isCanvasWorkspaceTaskUnmappableError(error)) {
    return error.message
  }
  if (error instanceof TaskResultPendingError) {
    return i18n.global.t(
      error.kind === 'timeout' ? 'canvas.task.progressTimeout' : 'canvas.task.stillProcessing',
    )
  }
  if (error instanceof TaskCancelledError) return i18n.global.t('canvas.task.cancelled')
  // Submit/quote 侧 board 错误码精确文案 (#P1-7): task_forbidden /
  // board_forbidden / board_archived / board_not_found 走共享错误目录，
  // 不再坍缩为通用失败 copy。
  const code = userErrorCode(error)
  if (
    code === 'task_forbidden' ||
    code === 'board_forbidden' ||
    code === 'board_archived' ||
    code === 'board_not_found'
  ) {
    return readinessErrorCopy(code)
  }
  return imageGenerationFailureMessage()
}

function releasedResultNodeIds(
  release: ReturnType<PendingImageExecutionCore['release']>,
): string[] {
  return release.outcomes.flatMap((outcome) =>
    outcome.kind === 'released' || outcome.kind === 'cancelled' ? [outcome.resultNodeId] : [],
  )
}

export function createPendingImageWorkflowRunner(
  options: PendingImageWorkflowRunnerOptions,
): PendingImageWorkflowRunner {
  function isExecutionCurrent(intent: Pick<PendingImageWorkflowIntent, 'attachment'>): boolean {
    return !isDisposed && options.executionCore.isAttachmentCurrent(intent.attachment)
  }

  async function submitSharedTask(
    intent: PendingImageWorkflowIntent,
    leases: readonly PendingImageExecutionLease[],
    resultNodeIds: readonly string[],
  ): Promise<string> {
    const actionId = (options.createClientActionId ?? createCanvasTaskClientActionId)()
    for (let attempt = 0; attempt < SHARED_TASK_REQUEST_ATTEMPTS; attempt += 1) {
      if (!isExecutionCurrent(intent)) {
        throw new TaskCancelledError('Canvas execution is no longer active')
      }
      try {
        const submission = await options.taskTransport.submit(intent.taskRequest, actionId)
        const applied = leases.filter(
          (lease) =>
            options.executionCore.markSubmitted(lease, submission.taskId, actionId).kind ===
            'applied',
        )
        if (applied.length !== leases.length) {
          const release = options.executionCore.release(applied)
          options.cancelTasks([submission.taskId, ...release.cancellableTaskIds])
          const releasedNodeIds = releasedResultNodeIds(release)
          if (releasedNodeIds.length > 0)
            options.pendingResults.removePendingResults(releasedNodeIds)
          throw new TaskCancelledError('Canvas execution lease is no longer current')
        }
        options.pendingResults.attachPendingTaskIds(
          resultNodeIds.map((resultNodeId, resultIndex) => ({
            resultNodeId,
            resultIndex,
            taskId: submission.taskId,
            clientActionId: actionId,
          })),
        )
        await options.afterTaskIdAttached?.()
        return submission.taskId
      } catch (error) {
        // 共用重试预算 (#P1-4): 可重试错误（network / 503 定价瞬态 /
        // 原始 503-429）单次重试；空池、余额、校验类确定性失败与
        // 手动重提路径不受影响。
        if (
          isExecutionCurrent(intent) &&
          attempt + 1 < SHARED_TASK_REQUEST_ATTEMPTS &&
          isRetryableCanvasTaskRequestError(error)
        )
          continue
        throw error
      }
    }
    throw new Error(i18n.global.t('canvas.execution.networkRetry'))
  }

  type AcquiredExecutionLeases = {
    leases: PendingImageExecutionLease[]
    resultNodeIds: string[]
  }

  type SettledExecutionAttempt = PromiseSettledResult<{
    lease: PendingImageExecutionLease
    outcome: ResolvedTaskImageOutcome
  }>

  function acquireExecutionLeases(
    intent: PendingImageWorkflowIntent,
  ): AcquiredExecutionLeases | null {
    const resultNodeIds = options.pendingResults.reservePendingResults({
      source: intent.source,
      sourceNodeId: intent.sourceNodeId,
      outputCount: intent.outputCount,
      resultOptions: intent.resultOptions,
    }).resultNodeIds
    const leases = resultNodeIds.flatMap((resultNodeId) => {
      const started = options.executionCore.begin(
        intent.attachment,
        resultNodeId as `image-${number}`,
      )
      return started.kind === 'started' ? [started.lease] : []
    })
    if (leases.length === resultNodeIds.length) return { leases, resultNodeIds }
    const release = options.executionCore.release(leases)
    const releasedNodeIds = releasedResultNodeIds(release)
    if (releasedNodeIds.length > 0) options.pendingResults.removePendingResults(releasedNodeIds)
    return null
  }

  type ExecutionSlot = { lease: PendingImageExecutionLease; resultIndex: number }
  type ResultWait = {
    taskId: string
    attachment: BoardAttachment
    fallbackRatio: number
    slots: ExecutionSlot[]
    observation?: TaskImageObservation
    deadlineAt: number
    controller: AbortController
    stop: () => void
    cancelTimer: (() => void) | undefined
    isApplying: boolean
    resolvedCount: number
    cancelledCount: number
    firstFailure: unknown
  }
  const activeWaits = new Set<ResultWait>()
  let isDisposed = false

  function finishWait(wait: ResultWait): void {
    wait.cancelTimer?.()
    wait.stop()
    wait.controller.abort()
    activeWaits.delete(wait)
  }

  function createWait(
    intent: PendingImageWorkflowIntent,
    taskId: string,
    leases: readonly PendingImageExecutionLease[],
  ): ResultWait {
    const wait: ResultWait = {
      taskId,
      attachment: intent.attachment,
      fallbackRatio: intent.fallbackRatio,
      slots: leases.map((lease, resultIndex) => ({ lease, resultIndex })),
      deadlineAt: Date.now() + CANVAS_TOTAL_WAIT_MS,
      controller: new AbortController(),
      stop: () => undefined,
      cancelTimer: undefined,
      isApplying: false,
      resolvedCount: 0,
      cancelledCount: 0,
      firstFailure: undefined,
    }
    wait.stop = options.executionCore.subscribe(() => {
      if (wait.isApplying) return
      if (
        !options.executionCore.isAttachmentCurrent(wait.attachment) ||
        !wait.slots.some((slot) => options.executionCore.isCurrent(slot.lease, taskId))
      ) {
        finishWait(wait)
      }
    })
    activeWaits.add(wait)
    return wait
  }

  function isWaitCurrent(wait: ResultWait): boolean {
    return (
      !isDisposed &&
      !wait.controller.signal.aborted &&
      options.executionCore.isAttachmentCurrent(wait.attachment)
    )
  }

  async function resolveSlots(
    wait: ResultWait,
    deadlineAt: number,
  ): Promise<SettledExecutionAttempt[]> {
    try {
      wait.observation ??= await runWithDeadline(deadlineAt, wait.controller.signal, (signal) =>
        options.taskTransport.observe(wait.taskId, { deadlineAt, signal }),
      )
    } catch (error) {
      const reason =
        error instanceof DOMException && error.name === 'TimeoutError'
          ? new TaskResultPendingError('timeout')
          : error
      return wait.slots.map(() => ({ status: 'rejected', reason }))
    }
    const observation = wait.observation
    wait.slots = wait.slots.filter((slot) =>
      options.executionCore.isCurrent(slot.lease, wait.taskId),
    )
    return Promise.allSettled(
      wait.slots.map(async ({ lease, resultIndex }) => {
        try {
          const outcome = await runWithDeadline(deadlineAt, wait.controller.signal, (signal) =>
            observation.resolve(resultIndex, wait.fallbackRatio, { deadlineAt, signal }),
          )
          return { lease, outcome }
        } catch (error) {
          if (error instanceof DOMException && error.name === 'TimeoutError') {
            throw new TaskResultPendingError('timeout')
          }
          throw error
        }
      }),
    )
  }

  /** The same slot settlement runs in isForeground and every continuation. */
  function settleAttempts(wait: ResultWait, attempts: readonly SettledExecutionAttempt[]): void {
    const pending: ExecutionSlot[] = []
    const materializable: Array<{
      lease: PendingImageExecutionLease
      result: NonNullable<ResolvedTaskImageOutcome['result']>
    }> = []
    const removeNodeIds: string[] = []
    wait.isApplying = true
    try {
      attempts.forEach((attempt, index) => {
        const slot = wait.slots[index]
        if (!slot || !options.executionCore.isCurrent(slot.lease, wait.taskId)) return
        if (attempt.status === 'rejected') {
          if (isTaskResultPendingError(attempt.reason)) {
            pending.push(slot)
          } else if (attempt.reason instanceof TaskCancelledError) {
            wait.cancelledCount += 1
            options.executionCore.cancel([slot.lease])
            removeNodeIds.push(slot.lease.resultNodeId)
          } else {
            wait.firstFailure ??= attempt.reason
            options.executionCore.fail(slot.lease)
            removeNodeIds.push(slot.lease.resultNodeId)
          }
          return
        }
        const { outcome } = attempt.value
        if (outcome.kind === 'cancelled') wait.cancelledCount += 1
        if (outcome.result) materializable.push({ lease: slot.lease, result: outcome.result })
        else {
          options.executionCore.cancel([slot.lease])
          removeNodeIds.push(slot.lease.resultNodeId)
        }
      })
      if (materializable.length > 0) {
        wait.resolvedCount +=
          options.executionCore.materializeBatch(materializable).changedNodeIds.length
      }
      if (removeNodeIds.length > 0) options.pendingResults.removePendingResults(removeNodeIds)
      wait.slots = pending.filter((slot) =>
        options.executionCore.isCurrent(slot.lease, wait.taskId),
      )
    } finally {
      wait.isApplying = false
    }
  }

  function reportOutcome(wait: ResultWait, isForeground: boolean): void {
    if (!isWaitCurrent(wait)) return
    if (wait.firstFailure !== undefined) {
      if (!options.onError?.(wait.firstFailure))
        options.onWarning(taskExecutionWarning(wait.firstFailure))
    } else if (wait.cancelledCount > 0) {
      options.onWarning(
        i18n.global.t(
          wait.resolvedCount > 0
            ? 'canvas.task.cancelledWithPartialResults'
            : 'canvas.task.cancelled',
        ),
      )
    } else if (wait.slots.length > 0) {
      if (isForeground) options.onWarning(i18n.global.t('canvas.execution.tasksInProgress'))
    } else if (wait.resolvedCount > 0) {
      options.onSuccess(imageGenerationSuccessMessage())
    }
  }

  function scheduleBackgroundContinuation(wait: ResultWait, attempt: number): void {
    if (!isWaitCurrent(wait)) return
    const remainingMs = wait.deadlineAt - Date.now()
    if (attempt >= BACKGROUND_CONTINUATION_MAX_ATTEMPTS || remainingMs <= 0) {
      options.onWarning(i18n.global.t('canvas.task.progressTimeout'))
      finishWait(wait)
      return
    }
    const schedule =
      options.scheduleContinuation ??
      ((callback, delayMs) => {
        const timer = globalThis.setTimeout(callback, delayMs)
        return () => globalThis.clearTimeout(timer)
      })
    const delayMs = Math.min(backgroundContinuationDelayMs(attempt), remainingMs)
    wait.cancelTimer = schedule(() => {
      wait.cancelTimer = undefined
      void continueInBackground(wait, attempt)
    }, delayMs)
  }

  async function continueInBackground(wait: ResultWait, attempt: number): Promise<void> {
    if (!isWaitCurrent(wait)) return
    if (Date.now() >= wait.deadlineAt) {
      scheduleBackgroundContinuation(wait, BACKGROUND_CONTINUATION_MAX_ATTEMPTS)
      return
    }
    const attempts = await resolveSlots(wait, wait.deadlineAt)
    if (!isWaitCurrent(wait)) return
    try {
      settleAttempts(wait, attempts)
      if (wait.slots.length > 0) scheduleBackgroundContinuation(wait, attempt + 1)
      reportOutcome(wait, false)
    } catch (error) {
      compensateExecutionFailure(
        error,
        wait,
        wait.slots.map((slot) => slot.lease),
      )
      finishWait(wait)
      return
    }
    if (wait.slots.length === 0) finishWait(wait)
  }

  function compensateExecutionFailure(
    error: unknown,
    intent: Pick<PendingImageWorkflowIntent, 'attachment'>,
    leases: readonly PendingImageExecutionLease[],
  ): void {
    const release =
      error instanceof TaskCancelledError
        ? options.executionCore.cancel(leases)
        : options.executionCore.release(leases)
    options.cancelTasks(release.cancellableTaskIds)
    const releasedNodeIds = releasedResultNodeIds(release)
    if (releasedNodeIds.length > 0) options.pendingResults.removePendingResults(releasedNodeIds)
    const isCurrent = isExecutionCurrent(intent)
    const isHandled = isCurrent && options.onError?.(error)
    if (!isHandled && isCurrent) {
      // Board 落子失败（任务已扣费、媒体已落盘、board 无图）(#P1-5):
      // 失败 facts 已置 failed、节点保留待重 attach 回收；此处以明确
      // 的恢复失败 copy 提示“可重试落子”，不丢结果。
      if (error instanceof PendingImageMaterializationError) {
        options.onWarning(i18n.global.t('canvas.execution.recoveryFailed'))
        return
      }
      options.onWarning(taskExecutionWarning(error))
    }
  }

  async function execute(intent: PendingImageWorkflowIntent): Promise<void> {
    if (!isExecutionCurrent(intent)) return
    let leases: PendingImageExecutionLease[] = []
    try {
      const acquired = acquireExecutionLeases(intent)
      if (!acquired) return
      leases = acquired.leases
      const resultNodeIds = acquired.resultNodeIds

      const taskId = await submitSharedTask(intent, leases, resultNodeIds)
      if (!isExecutionCurrent(intent)) return
      const wait = createWait(intent, taskId, leases)
      try {
        const attempts = await resolveSlots(
          wait,
          Math.min(wait.deadlineAt, Date.now() + CANVAS_FOREGROUND_WAIT_MS),
        )
        if (!isWaitCurrent(wait)) return
        settleAttempts(wait, attempts)
        // Schedule remaining slots before presenting individual outcomes.
        if (wait.slots.length > 0) scheduleBackgroundContinuation(wait, 0)
        reportOutcome(wait, true)
        if (wait.slots.length === 0) finishWait(wait)
      } catch (error) {
        finishWait(wait)
        throw error
      }
    } catch (error) {
      // Disposal stops this runner; an in-flight submission may still have
      // committed, so preserve its pending nodes instead of cancelling or
      // deleting them on an unknown response. A confirmed task id is still
      // attached and flushed by submitSharedTask for the next recovery.
      if (isDisposed) return
      compensateExecutionFailure(error, intent, leases)
    }
  }

  return {
    execute,
    dispose() {
      isDisposed = true
      for (const wait of activeWaits) finishWait(wait)
    },
  }
}
