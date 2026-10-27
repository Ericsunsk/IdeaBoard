import { isTaskResultPendingError, TaskCancelledError } from '../task-runner/taskResultErrors'
import { i18n } from '../../../../../core/i18n'
import type { BoardSnapshot } from '../../../../board/ports/canvas'
import type { TaskImageObserver } from '../task-runner/taskImageOutcome'
import { runWithDeadline } from '../../../../../core/utils/request'
import { CANVAS_FOREGROUND_WAIT_MS } from '../canvasBudgets'
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

  async function recoverForAttachment(
    attachment: NonNullable<ReturnType<PendingImageExecutionCore['currentAttachment']>>,
  ): Promise<CanvasExecutionRecoveryResult> {
    const pending = collectPendingImageTasks(options.readBoardSnapshot())
    const hydratedCount = hydratePendingLeases()
    const recoveredNodeIds: string[] = []
    const pendingNodeIds: string[] = []
    const failedNodeIds: string[] = []
    const rejectedNodeIds: string[] = []
    const recoveryErrors: unknown[] = []
    let shouldFlushAutosave = false
    let cancelledCount = 0

    const groups = claimPendingTaskGroups(attachment, pending)

    function recordSlotError({ task, lease }: RecoverySlot, error: unknown): void {
      if (isDisposed || !options.executionCore.isAttachmentCurrent(attachment)) return
      if (
        !options.executionCore.isCurrent(lease, task.taskId) &&
        !(error instanceof PendingImageMaterializationError)
      )
        return
      if (isRecoveryPending(error)) {
        pendingNodeIds.push(task.resultNodeId)
      } else if (error instanceof TaskCancelledError) {
        const cancelled = options.executionCore.cancel([lease])
        options.cancelTasks(releasedTaskIds(cancelled))
      } else {
        failedNodeIds.push(task.resultNodeId)
        if (!(error instanceof PendingImageMaterializationError)) options.executionCore.fail(lease)
        recoveryErrors.push(error)
      }
    }

    await Promise.all(
      [...groups].map(async ([taskId, slots]) => {
        const controller = new AbortController()
        activeReads.add(controller)
        const stop = options.executionCore.subscribe(() => {
          if (!slots.some(({ lease }) => options.executionCore.isCurrent(lease, taskId)))
            controller.abort()
        })
        const deadlineAt = Date.now() + CANVAS_FOREGROUND_WAIT_MS
        try {
          const observation = await runWithDeadline(deadlineAt, controller.signal, (signal) =>
            options.observeTask(taskId, { deadlineAt, signal }),
          )
          await Promise.all(
            slots.map(async (slot) => {
              const { task, lease } = slot
              if (!options.executionCore.isCurrent(lease, taskId)) return
              try {
                const outcome = await runWithDeadline(deadlineAt, controller.signal, (signal) =>
                  observation.resolve(task.resultIndex, task.fallbackRatio, { deadlineAt, signal }),
                )
                if (isDisposed || !options.executionCore.isCurrent(lease, taskId)) return
                if (outcome.kind === 'cancelled') cancelledCount += 1
                if (!outcome.result) {
                  options.executionCore.cancel([lease])
                  return
                }
                const materialized = options.executionCore.materializeBatch([
                  { lease, result: outcome.result },
                ])
                const materializedIds = materializedNodeIds(materialized)
                recoveredNodeIds.push(...materializedIds)
                rejectedNodeIds.push(...readRejectedNodeIds(materialized))
                if (materializedIds.length > 0) shouldFlushAutosave = true
              } catch (error) {
                recordSlotError(slot, error)
              }
            }),
          )
        } catch (error) {
          slots.forEach((slot) => recordSlotError(slot, error))
        } finally {
          stop()
          controller.abort()
          activeReads.delete(controller)
        }
      }),
    )

    if (
      !isDisposed &&
      shouldFlushAutosave &&
      options.executionCore.isAttachmentCurrent(attachment)
    ) {
      try {
        await options.flushAutosave()
      } catch (error) {
        recoveryErrors.push(error)
      }
    }
    if (
      !isDisposed &&
      recoveryErrors.length > 0 &&
      options.executionCore.isAttachmentCurrent(attachment)
    ) {
      reportError(
        recoveryErrors.length === 1
          ? recoveryErrors[0]
          : new AggregateError(recoveryErrors, 'Canvas execution recovery failed'),
      )
    }
    if (
      !isDisposed &&
      cancelledCount > 0 &&
      options.executionCore.isAttachmentCurrent(attachment)
    ) {
      options.onWarning(
        i18n.global.t(
          recoveredNodeIds.length > 0
            ? 'canvas.task.cancelledWithPartialResults'
            : 'canvas.task.cancelled',
        ),
      )
    }
    return Object.freeze({
      attachment: Object.freeze({ boardId: attachment.boardId, epoch: attachment.epoch }),
      hydratedCount,
      recoveredNodeIds: Object.freeze([...new Set(recoveredNodeIds)]),
      pendingNodeIds: Object.freeze([...new Set(pendingNodeIds)]),
      failedNodeIds: Object.freeze([...new Set(failedNodeIds)]),
      rejectedNodeIds: Object.freeze([...new Set(rejectedNodeIds)]),
    })
  }

  function recover(): Promise<CanvasExecutionRecoveryResult | null> {
    const attachment = options.executionCore.currentAttachment()
    if (isDisposed || !attachment) return Promise.resolve(null)
    const key = attachmentKey(attachment)
    const current = activeRecoveries.get(key)
    if (current) return current
    const recovery = recoverForAttachment(attachment).finally(() => {
      if (activeRecoveries.get(key) === recovery) activeRecoveries.delete(key)
    })
    activeRecoveries.set(key, recovery)
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
