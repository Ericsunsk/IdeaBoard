import {
  isTaskResultPendingError,
  TaskCancelledError,
  TaskResultPendingError,
} from '../task-runner/taskResultErrors'
import { i18n } from '../../../../../core/i18n'
import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'
import type {
  ImageExecutionInput,
  ImageExecutionResultOptions,
} from '../planning/imageExecutionPlan'
import type { ResolvedTaskImageResult } from '../result-nodes/resolvedTaskImageResult'
import type { PendingImageResultExecution } from '../result-nodes/usePendingImageResultExecution.ts'
import {
  type PendingImageExecutionCore,
  type PendingImageExecutionLease,
} from '../pending-execution/core'
import type { BoardAttachment } from '../board-attachment'
import {
  createCanvasTaskClientActionId,
  type ClientActionIdFactory,
} from '../task-runner/taskClientActionIds'
import { isCanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'
import {
  isCanvasWorkspaceTaskUnmappableError,
  isCanvasWorkspaceUnavailableError,
} from '../workspace/canvasWorkspaceTaskAdapter'

type PendingImageResultOptions = ImageExecutionResultOptions

export type PendingImageWorkflowTaskTransport = Readonly<{
  assertReady: (request: CanvasTaskRequestData, fallbackMessage: string) => Promise<void>
  submit: (
    request: CanvasTaskRequestData,
    clientActionId: string,
    fallbackMessage: string,
  ) => Promise<{ taskId: string }>
  resolve: (
    taskId: string,
    resultIndex: number,
    fallbackRatio: number,
  ) => Promise<ResolvedTaskImageResult>
}>

type PendingImageWorkflowIntent = {
  source: ImageExecutionInput
  sourceNodeId: string
  attachment: BoardAttachment
  outputCount: number
  resultOptions?: PendingImageResultOptions
  taskRequest: CanvasTaskRequestData
  readinessRequest?: CanvasTaskRequestData
  fallbackRatio: number
}

export type PendingImageWorkflowRunner = {
  execute: (intent: PendingImageWorkflowIntent) => Promise<void>
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
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: ((error: unknown) => boolean) | undefined
}

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
  async function submitSharedTask(
    intent: PendingImageWorkflowIntent,
    leases: readonly PendingImageExecutionLease[],
    resultNodeIds: readonly string[],
  ): Promise<string> {
    const actionId = (options.createClientActionId ?? createCanvasTaskClientActionId)()
    for (let attempt = 0; attempt < 2; attempt += 1) {
      if (!options.executionCore.isAttachmentCurrent(intent.attachment)) {
        throw new TaskCancelledError('Canvas execution is no longer attached')
      }
      try {
        const submission = await options.taskTransport.submit(
          intent.taskRequest,
          actionId,
          imageGenerationFailureMessage(),
        )
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
        if (attempt === 0 && isCanvasBillingPreflightError(error, 'network')) continue
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
    result: ResolvedTaskImageResult
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

  function failRejectedLeases(
    leases: readonly PendingImageExecutionLease[],
    attempts: readonly SettledExecutionAttempt[],
  ): void {
    const failedLeases = attempts.flatMap((attempt, index) => {
      if (attempt.status !== 'rejected' || isTaskResultPendingError(attempt.reason)) return []
      const lease = leases[index]
      if (!lease) return []
      return options.executionCore.fail(lease).kind === 'applied' ? [lease] : []
    })
    if (failedLeases.length > 0) {
      options.pendingResults.removePendingResults(failedLeases.map((lease) => lease.resultNodeId))
    }
  }

  function reportSettlementOutcome(
    resolvedCount: number,
    pendingCount: number,
    firstFailure: PromiseRejectedResult | undefined,
  ): void {
    if (firstFailure) {
      const isHandled = options.onError?.(firstFailure.reason)
      if (!isHandled) options.onWarning(taskExecutionWarning(firstFailure.reason))
      return
    }
    if (pendingCount > 0) {
      options.onWarning(i18n.global.t('canvas.execution.tasksInProgress'))
      return
    }
    if (resolvedCount > 0) {
      options.onSuccess(imageGenerationSuccessMessage())
    }
  }

  function settleResolvedAttempts(
    taskId: string,
    leases: readonly PendingImageExecutionLease[],
    attempts: readonly SettledExecutionAttempt[],
  ): void {
    const resolved = attempts.flatMap((attempt) =>
      attempt.status === 'fulfilled' && options.executionCore.isCurrent(attempt.value.lease, taskId)
        ? [attempt.value]
        : [],
    )
    const pending = attempts.filter(
      (attempt) => attempt.status === 'rejected' && isTaskResultPendingError(attempt.reason),
    )
    const firstFailure = attempts.find(
      (attempt): attempt is PromiseRejectedResult =>
        attempt.status === 'rejected' && !isTaskResultPendingError(attempt.reason),
    )
    failRejectedLeases(leases, attempts)
    const materialized =
      resolved.length > 0 ? options.executionCore.materializeBatch(resolved) : null
    reportSettlementOutcome(materialized?.changedNodeIds.length ?? 0, pending.length, firstFailure)
  }

  function compensateExecutionFailure(
    error: unknown,
    intent: PendingImageWorkflowIntent,
    leases: readonly PendingImageExecutionLease[],
  ): void {
    const release =
      error instanceof TaskCancelledError
        ? options.executionCore.cancel(leases)
        : options.executionCore.release(leases)
    options.cancelTasks(release.cancellableTaskIds)
    const releasedNodeIds = releasedResultNodeIds(release)
    if (releasedNodeIds.length > 0) options.pendingResults.removePendingResults(releasedNodeIds)
    const isCurrent = options.executionCore.isAttachmentCurrent(intent.attachment)
    const isHandled = isCurrent && options.onError?.(error)
    if (!isHandled && isCurrent) {
      options.onWarning(taskExecutionWarning(error))
    }
  }

  async function execute(intent: PendingImageWorkflowIntent): Promise<void> {
    let leases: PendingImageExecutionLease[] = []
    try {
      await options.taskTransport.assertReady(
        intent.readinessRequest ?? intent.taskRequest,
        imageGenerationFailureMessage(),
      )
      if (!options.executionCore.isAttachmentCurrent(intent.attachment)) return

      const acquired = acquireExecutionLeases(intent)
      if (!acquired) return
      leases = acquired.leases
      const resultNodeIds = acquired.resultNodeIds

      const taskId = await submitSharedTask(intent, leases, resultNodeIds)
      const attempts = await Promise.allSettled(
        leases.map(async (lease, resultIndex) => ({
          lease,
          result: await options.taskTransport.resolve(taskId, resultIndex, intent.fallbackRatio),
        })),
      )
      if (!options.executionCore.isAttachmentCurrent(intent.attachment)) return
      settleResolvedAttempts(taskId, leases, attempts)
    } catch (error) {
      compensateExecutionFailure(error, intent, leases)
    }
  }

  return { execute }
}
