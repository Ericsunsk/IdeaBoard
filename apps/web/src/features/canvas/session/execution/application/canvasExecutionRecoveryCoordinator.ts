import { isTaskResultPendingError, TaskCancelledError } from '../task-runner/taskResultErrors'
import { i18n } from '../../../../../core/i18n'
import type { BoardSnapshot } from '../../../../board/ports/canvas'
import type { TaskImageObservation, TaskImageObserver } from '../task-runner/taskImageOutcome'
import { runWithDeadline } from '../../../../../core/utils/request'
import { sleep } from '../../../../../core/utils/polling'
import {
  BACKGROUND_CONTINUATION_MAX_ATTEMPTS,
  backgroundContinuationDelayMs,
  CANVAS_FOREGROUND_WAIT_MS,
  CANVAS_TOTAL_WAIT_MS,
} from '../canvasBudgets'
import {
  PendingImageMaterializationError,
  type PendingImageExecutionCore,
  type PendingImageMaterializationResult,
  type PendingImageExecutionLease,
} from '../pending-execution/core'
import {
  collectPendingImageTasks,
  type PendingImageTaskSnapshot,
} from '../pending-execution/taskSnapshot'

export type CanvasExecutionRecoveryResult = Readonly<{
  attachment: Readonly<{ boardId: string; epoch: number }>
  hydratedCount: number
  recoveredNodeIds: readonly string[]
  pendingNodeIds: readonly string[]
  failedNodeIds: readonly string[]
  rejectedNodeIds: readonly string[]
}>

export type CanvasExecutionRecoveryCoordinatorOptions = Readonly<{
  executionCore: PendingImageExecutionCore
  readBoardSnapshot: () => BoardSnapshot
  observeTask: TaskImageObserver
  flushAutosave: () => Promise<void>
  cancelTasks: (taskIds: readonly string[]) => void
  onError?: ((error: unknown) => boolean) | undefined
  onWarning: (message: string) => void
}>

export type CanvasExecutionRecoveryCoordinator = Readonly<{
  hydratePendingLeases: (snapshot?: BoardSnapshot) => number
  recover: () => Promise<CanvasExecutionRecoveryResult | null>
  dispose: () => void
}>

function attachmentKey(attachment: { boardId: string; epoch: number }): string {
  return `${attachment.boardId}:${attachment.epoch}`
}

function releasedTaskIds(
  result: ReturnType<PendingImageExecutionCore['release']>,
): readonly string[] {
  return result.cancellableTaskIds
}

function materializedNodeIds(result: PendingImageMaterializationResult): readonly string[] {
  return result.outcomes.flatMap((outcome) =>
    outcome.kind === 'materialized' ? [outcome.resultNodeId] : [],
  )
}

function readRejectedNodeIds(result: PendingImageMaterializationResult): readonly string[] {
  return result.outcomes.flatMap((outcome) =>
    outcome.kind === 'rejected' ? [outcome.resultNodeId] : [],
  )
}

function isRecoveryPending(error: unknown): boolean {
  return (
    isTaskResultPendingError(error) ||
    (error instanceof DOMException && error.name === 'TimeoutError')
  )
}

export function createCanvasExecutionRecoveryCoordinator(
  options: CanvasExecutionRecoveryCoordinatorOptions,
): CanvasExecutionRecoveryCoordinator {
  const activeReads = new Set<AbortController>()
  let isDisposed = false
  const hydratedAttachments = new Set<string>()
  const activeRecoveries = new Map<string, Promise<CanvasExecutionRecoveryResult | null>>()

  function reportError(error: unknown): void {
    if (options.onError?.(error)) return
    options.onWarning(i18n.global.t('canvas.execution.recoveryFailed'))
  }

  function hydratePendingLeases(snapshot?: BoardSnapshot): number {
    const attachment = options.executionCore.currentAttachment()
    if (isDisposed || !attachment) return 0
    const pending = collectPendingImageTasks(snapshot ?? options.readBoardSnapshot())
    const key = attachmentKey(attachment)
    if (hydratedAttachments.has(key)) return 0
    hydratedAttachments.clear()
    hydratedAttachments.add(key)
    let hydratedCount = 0
    for (const task of pending) {
      const result = options.executionCore.hydratePendingLease(attachment, task)
      if (result.kind !== 'started') continue
      hydratedCount += 1
      if (result.cancellableTaskIds.length > 0) {
        options.cancelTasks(result.cancellableTaskIds)
      }
    }
    return hydratedCount
  }

  type RecoverySlot = { task: PendingImageTaskSnapshot; lease: PendingImageExecutionLease }

  function claimPendingTaskGroups(
    attachment: NonNullable<ReturnType<PendingImageExecutionCore['currentAttachment']>>,
    pending: readonly PendingImageTaskSnapshot[],
  ): Map<string, RecoverySlot[]> {
    const groups = new Map<string, RecoverySlot[]>()
    for (const task of pending) {
      const lease = options.executionCore.claimHydratedLease(attachment, task)
      if (!lease) continue
      const slots = groups.get(task.taskId) ?? []
      slots.push({ task, lease })
      groups.set(task.taskId, slots)
    }
    return groups
  }

  type RecoveryTaskGroup = {
    taskId: string
    controller: AbortController
    observation?: TaskImageObservation
  }

  type RecoveryProgress = {
    recoveredNodeIds: string[]
    pendingSlots: RecoverySlot[]
    failedNodeIds: string[]
    rejectedNodeIds: string[]
    errors: unknown[]
    cancelledCount: number
  }

  async function observeSlots(
    group: RecoveryTaskGroup,
    slots: readonly RecoverySlot[],
    deadlineAt: number,
  ): Promise<RecoveryProgress> {
    const { taskId, controller } = group
    const progress: RecoveryProgress = {
      recoveredNodeIds: [],
      pendingSlots: [],
      failedNodeIds: [],
      rejectedNodeIds: [],
      errors: [],
      cancelledCount: 0,
    }
    function recordSlotError(slot: RecoverySlot, error: unknown): void {
      const { task, lease } = slot
      if (isDisposed || !options.executionCore.isAttachmentCurrent(lease.attachment)) return
      if (
        !options.executionCore.isCurrent(lease, task.taskId) &&
        !(error instanceof PendingImageMaterializationError)
      )
        return
      if (isRecoveryPending(error)) {
        progress.pendingSlots.push(slot)
      } else if (error instanceof TaskCancelledError) {
        const cancelled = options.executionCore.cancel([lease])
        options.cancelTasks(releasedTaskIds(cancelled))
      } else {
        progress.failedNodeIds.push(task.resultNodeId)
        if (!(error instanceof PendingImageMaterializationError)) options.executionCore.fail(lease)
        progress.errors.push(error)
      }
    }
    try {
      group.observation ??= await runWithDeadline(deadlineAt, controller.signal, (signal) =>
        options.observeTask(taskId, { deadlineAt, signal }),
      )
      const observation = group.observation
      await Promise.all(
        slots.map(async (slot) => {
          const { task, lease } = slot
          if (!options.executionCore.isCurrent(lease, taskId)) return
          try {
            const outcome = await runWithDeadline(deadlineAt, controller.signal, (signal) =>
              observation.resolve(task.resultIndex, task.fallbackRatio, { deadlineAt, signal }),
            )
            if (isDisposed || !options.executionCore.isCurrent(lease, taskId)) return
            if (outcome.kind === 'cancelled') progress.cancelledCount += 1
            if (!outcome.result) {
              options.executionCore.cancel([lease])
              return
            }
            const materialized = options.executionCore.materializeBatch([
              { lease, result: outcome.result },
            ])
            progress.recoveredNodeIds.push(...materializedNodeIds(materialized))
            progress.rejectedNodeIds.push(...readRejectedNodeIds(materialized))
          } catch (error) {
            recordSlotError(slot, error)
          }
        }),
      )
    } catch (error) {
      slots.forEach((slot) => recordSlotError(slot, error))
    }
    return progress
  }

  async function publishProgress(
    attachment: NonNullable<ReturnType<PendingImageExecutionCore['currentAttachment']>>,
    progress: readonly RecoveryProgress[],
  ): Promise<void> {
    if (isDisposed || !options.executionCore.isAttachmentCurrent(attachment)) return
    const errors = progress.flatMap((item) => item.errors)
    const recoveredCount = progress.reduce((total, item) => total + item.recoveredNodeIds.length, 0)
    if (recoveredCount > 0) {
      try {
        await options.flushAutosave()
      } catch (error) {
        errors.push(error)
      }
    }
    if (isDisposed || !options.executionCore.isAttachmentCurrent(attachment)) return
    if (errors.length > 0) {
      reportError(
        errors.length === 1
          ? errors[0]
          : new AggregateError(errors, 'Canvas execution recovery failed'),
      )
    }
    if (progress.some((item) => item.cancelledCount > 0)) {
      options.onWarning(
        i18n.global.t(
          recoveredCount > 0 ? 'canvas.task.cancelledWithPartialResults' : 'canvas.task.cancelled',
        ),
      )
    }
  }

  async function continueInBackground(
    attachment: NonNullable<ReturnType<PendingImageExecutionCore['currentAttachment']>>,
    group: RecoveryTaskGroup,
    initialSlots: readonly RecoverySlot[],
    deadlineAt: number,
  ): Promise<void> {
    const { taskId, controller } = group
    let slots = initialSlots
    try {
      for (let attempt = 0; attempt < BACKGROUND_CONTINUATION_MAX_ATTEMPTS; attempt += 1) {
        const remainingMs = deadlineAt - Date.now()
        if (remainingMs <= 0) break
        await sleep(
          Math.min(backgroundContinuationDelayMs(attempt), remainingMs),
          controller.signal,
        )
        slots = slots.filter(({ lease }) => options.executionCore.isCurrent(lease, taskId))
        if (slots.length === 0 || Date.now() >= deadlineAt) break
        const progress = await observeSlots(group, slots, deadlineAt)
        await publishProgress(attachment, [progress])
        slots = progress.pendingSlots.filter(({ lease }) =>
          options.executionCore.isCurrent(lease, taskId),
        )
        if (slots.length === 0) return
      }
      if (
        !isDisposed &&
        !controller.signal.aborted &&
        slots.some(({ lease }) => options.executionCore.isCurrent(lease, taskId))
      ) {
        options.onWarning(i18n.global.t('canvas.task.progressTimeout'))
      }
    } catch (error) {
      if (
        !isDisposed &&
        !controller.signal.aborted &&
        options.executionCore.isAttachmentCurrent(attachment)
      )
        reportError(error)
    }
  }

  async function recoverForAttachment(
    attachment: NonNullable<ReturnType<PendingImageExecutionCore['currentAttachment']>>,
    backgroundContinuations: Promise<void>[],
  ): Promise<CanvasExecutionRecoveryResult> {
    const pending = collectPendingImageTasks(options.readBoardSnapshot())
    const hydratedCount = hydratePendingLeases()
    const groups = claimPendingTaskGroups(attachment, pending)
    const progress = await Promise.all(
      [...groups].map(async ([taskId, slots]) => {
        const controller = new AbortController()
        const group: RecoveryTaskGroup = { taskId, controller }
        activeReads.add(controller)
        const stop = options.executionCore.subscribe(() => {
          if (!slots.some(({ lease }) => options.executionCore.isCurrent(lease, taskId)))
            controller.abort()
        })
        const deadlineAt = Date.now() + CANVAS_TOTAL_WAIT_MS
        function finish(): void {
          stop()
          controller.abort()
          activeReads.delete(controller)
          slots.forEach(({ lease }) => options.executionCore.releaseHydratedLease(lease))
        }
        try {
          const foreground = await observeSlots(
            group,
            slots,
            Date.now() + CANVAS_FOREGROUND_WAIT_MS,
          )
          if (foreground.pendingSlots.length > 0 && !isDisposed && !controller.signal.aborted) {
            backgroundContinuations.push(
              continueInBackground(attachment, group, foreground.pendingSlots, deadlineAt).finally(
                finish,
              ),
            )
          } else finish()
          return foreground
        } catch (error) {
          finish()
          throw error
        }
      }),
    )
    await publishProgress(attachment, progress)
    return Object.freeze({
      attachment: Object.freeze({ boardId: attachment.boardId, epoch: attachment.epoch }),
      hydratedCount,
      recoveredNodeIds: Object.freeze([
        ...new Set(progress.flatMap((item) => item.recoveredNodeIds)),
      ]),
      pendingNodeIds: Object.freeze([
        ...new Set(
          progress.flatMap((item) => item.pendingSlots.map(({ task }) => task.resultNodeId)),
        ),
      ]),
      failedNodeIds: Object.freeze([...new Set(progress.flatMap((item) => item.failedNodeIds))]),
      rejectedNodeIds: Object.freeze([
        ...new Set(progress.flatMap((item) => item.rejectedNodeIds)),
      ]),
    })
  }

  function recover(): Promise<CanvasExecutionRecoveryResult | null> {
    const attachment = options.executionCore.currentAttachment()
    if (isDisposed || !attachment) return Promise.resolve(null)
    const key = attachmentKey(attachment)
    const current = activeRecoveries.get(key)
    if (current) return current
    const backgroundContinuations: Promise<void>[] = []
    const recovery = recoverForAttachment(attachment, backgroundContinuations)
    activeRecoveries.set(key, recovery)
    // Keep concurrent drivers on the foreground result while its Task groups
    // continue observing. Completion releases the claims for a later recovery.
    const finish = async () => {
      await Promise.allSettled(backgroundContinuations)
      if (activeRecoveries.get(key) === recovery) activeRecoveries.delete(key)
    }
    void recovery.then(finish, finish)
    return recovery
  }

  return {
    hydratePendingLeases,
    recover,
    dispose() {
      isDisposed = true
      for (const controller of activeReads) controller.abort()
    },
  }
}
