import { isTaskResultPendingError, TaskCancelledError } from '../../../../shared/api/taskEvents'
import { i18n } from '../../../../shared/i18n'
import type { BoardSnapshot } from '../../../board/domain'
import {
  PendingImageMaterializationError,
  collectPendingImageTasks,
  type PendingImageExecutionCore,
  type PendingImageMaterializationResult,
  type PendingImageResolvedResult,
  type PendingImageTaskSnapshot,
} from '../pending-execution'

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
  resolveResult: (pending: PendingImageTaskSnapshot) => Promise<PendingImageResolvedResult>
  flushAutosave: () => Promise<void>
  cancelTasks: (taskIds: readonly string[]) => void
  onError?: ((error: unknown) => boolean) | undefined
  onWarning: (message: string) => void
}>

export type CanvasExecutionRecoveryCoordinator = Readonly<{
  hydratePendingLeases: (snapshot?: BoardSnapshot) => number
  recover: () => Promise<CanvasExecutionRecoveryResult | null>
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

export function createCanvasExecutionRecoveryCoordinator(
  options: CanvasExecutionRecoveryCoordinatorOptions,
): CanvasExecutionRecoveryCoordinator {
  const hydratedAttachments = new Set<string>()
  const activeRecoveries = new Map<string, Promise<CanvasExecutionRecoveryResult | null>>()

  function reportError(error: unknown): void {
    if (options.onError?.(error)) return
    options.onWarning(i18n.global.t('canvas.execution.recoveryFailed'))
  }

  function hydratePendingLeases(snapshot?: BoardSnapshot): number {
    const attachment = options.executionCore.currentAttachment()
    if (!attachment) return 0
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

    await Promise.all(
      pending.map(async (task) => {
        const lease = options.executionCore.claimHydratedLease(attachment, task)
        if (!lease) return
        try {
          const result = await options.resolveResult(task)
          if (!options.executionCore.isCurrent(lease, task.taskId)) return
          const materialized = options.executionCore.materializeBatch([{ lease, result }])
          const materializedIds = materializedNodeIds(materialized)
          const rejectedIds = readRejectedNodeIds(materialized)
          recoveredNodeIds.push(...materializedIds)
          rejectedNodeIds.push(...rejectedIds)
          if (materializedIds.length > 0) shouldFlushAutosave = true
        } catch (error) {
          if (isTaskResultPendingError(error)) {
            pendingNodeIds.push(task.resultNodeId)
            return
          }
          if (error instanceof TaskCancelledError) {
            const cancelled = options.executionCore.cancel([lease])
            options.cancelTasks(releasedTaskIds(cancelled))
            return
          }
          failedNodeIds.push(task.resultNodeId)
          if (!(error instanceof PendingImageMaterializationError)) {
            options.executionCore.fail(lease)
          }
          recoveryErrors.push(error)
        }
      }),
    )

    if (shouldFlushAutosave) {
      try {
        await options.flushAutosave()
      } catch (error) {
        recoveryErrors.push(error)
      }
    }
    if (recoveryErrors.length > 0) {
      reportError(
        recoveryErrors.length === 1
          ? recoveryErrors[0]
          : new AggregateError(recoveryErrors, 'Canvas execution recovery failed'),
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
    if (!attachment) return Promise.resolve(null)
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
  }
}
