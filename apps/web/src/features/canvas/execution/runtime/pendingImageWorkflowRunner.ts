import { isTaskResultPendingError, TaskCancelledError } from '../../../../shared/api/taskEvents'
import type { SubmitTaskData } from '../../../../shared/api/task'
import type { ImageExecutionInput, ImageExecutionResultOptions } from '../planning/ImageExecutionPlan'
import type { ResolvedTaskImageResult } from '../result-nodes/resolvedTaskImageResult'
import type { PendingImageResultExecution } from '../result-nodes/usePendingImageResultNodes'
import {
  type PendingImageAttachment,
  type PendingImageExecutionCore,
  type PendingImageExecutionLease,
} from '../pending-execution'
import {
  createCanvasTaskClientActionId,
  type ClientActionIdFactory,
} from '../task-runner/taskClientActionIds'
import { isCanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'

type PendingImageResultOptions = ImageExecutionResultOptions

export type PendingImageWorkflowTaskTransport = Readonly<{
  assertReady: (request: SubmitTaskData, fallbackMessage: string) => Promise<void>
  submit: (
    request: SubmitTaskData,
    clientActionId: string,
    fallbackMessage: string,
  ) => Promise<{ taskId: string }>
  resolve: (
    taskId: string,
    resultIndex: number,
    fallbackRatio: number,
  ) => Promise<ResolvedTaskImageResult>
}>

export type PendingImageWorkflowIntent = {
  source: ImageExecutionInput
  sourceNodeId: string
  attachment: PendingImageAttachment
  outputCount: number
  resultOptions?: PendingImageResultOptions
  taskRequest: SubmitTaskData
  readinessRequest?: SubmitTaskData
  fallbackRatio: number
  successMessage?: string
  fallbackErrorMessage: string
  formatErrorMessage?: (error: Error) => string
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
  afterTaskIdAttached?: () => Promise<void>
  createClientActionId?: ClientActionIdFactory
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
}

function errorMessage(error: unknown, intent: PendingImageWorkflowIntent): string {
  if (!(error instanceof Error)) return intent.fallbackErrorMessage
  return intent.formatErrorMessage?.(error) ?? error.message
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
          intent.fallbackErrorMessage,
        )
        const applied = leases.filter(
          (lease) => options.executionCore.markSubmitted(lease, submission.taskId, actionId).kind === 'applied',
        )
        if (applied.length !== leases.length) {
          const release = options.executionCore.release(applied)
          options.cancelTasks([submission.taskId, ...release.cancellableTaskIds])
          const releasedNodeIds = releasedResultNodeIds(release)
          if (releasedNodeIds.length > 0) options.pendingResults.removePendingResults(releasedNodeIds)
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
    throw new Error('网络错误，请重试')
  }

  async function execute(intent: PendingImageWorkflowIntent): Promise<void> {
    let leases: PendingImageExecutionLease[] = []
    try {
      await options.taskTransport.assertReady(
        intent.readinessRequest ?? intent.taskRequest,
        intent.fallbackErrorMessage,
      )
      if (!options.executionCore.isAttachmentCurrent(intent.attachment)) return

      const resultNodeIds = options.pendingResults.reservePendingResults({
        source: intent.source,
        sourceNodeId: intent.sourceNodeId,
        outputCount: intent.outputCount,
        resultOptions: intent.resultOptions,
      }).resultNodeIds
      leases = resultNodeIds.flatMap((resultNodeId) => {
        const started = options.executionCore.begin(intent.attachment, resultNodeId as `image-${number}`)
        return started.kind === 'started' ? [started.lease] : []
      })
      if (leases.length !== resultNodeIds.length) {
        const release = options.executionCore.release(leases)
        const releasedNodeIds = releasedResultNodeIds(release)
        if (releasedNodeIds.length > 0) options.pendingResults.removePendingResults(releasedNodeIds)
        return
      }

      const taskId = await submitSharedTask(intent, leases, resultNodeIds)
      const attempts = await Promise.allSettled(
        leases.map(async (lease, resultIndex) => ({
          lease,
          result: await options.taskTransport.resolve(taskId, resultIndex, intent.fallbackRatio),
        })),
      )
      if (!options.executionCore.isAttachmentCurrent(intent.attachment)) return

      const resolved = attempts.flatMap((attempt) =>
        attempt.status === 'fulfilled' && options.executionCore.isCurrent(attempt.value.lease, taskId)
          ? [attempt.value]
          : [],
      )
      const pending = attempts.filter(
        (attempt) => attempt.status === 'rejected' && isTaskResultPendingError(attempt.reason),
      )
      const failures = attempts.filter(
        (attempt) => attempt.status === 'rejected' && !isTaskResultPendingError(attempt.reason),
      )
      const failedLeases = attempts.flatMap((attempt, index) => {
        if (attempt.status !== 'rejected' || isTaskResultPendingError(attempt.reason)) return []
        const lease = leases[index]!
        return options.executionCore.fail(lease).kind === 'applied' ? [lease] : []
      })
      if (failedLeases.length > 0) {
        options.pendingResults.removePendingResults(failedLeases.map((lease) => lease.resultNodeId))
      }
      const materialized = resolved.length > 0 ? options.executionCore.materializeBatch(resolved) : null
      const successCount = materialized?.changedNodeIds.length ?? 0
      if (successCount > 0) {
        options.onSuccess(
          intent.successMessage ?? (successCount > 1 ? `生成完成（${successCount} 张）` : '生成完成'),
        )
      }
      if (pending.length > 0) options.onWarning('任务进行中，稍后可在画布上查看结果')
      if (failures.length > 0) {
        const firstFailure = failures[0]
        const handled = firstFailure?.status === 'rejected' && options.onError?.(firstFailure.reason)
        if (!handled) options.onWarning(errorMessage(firstFailure?.status === 'rejected' ? firstFailure.reason : null, intent))
      }
    } catch (error) {
      const release = error instanceof TaskCancelledError ? options.executionCore.cancel(leases) : options.executionCore.release(leases)
      options.cancelTasks(release.cancellableTaskIds)
      const releasedNodeIds = releasedResultNodeIds(release)
      if (releasedNodeIds.length > 0) options.pendingResults.removePendingResults(releasedNodeIds)
      const handled = options.executionCore.isAttachmentCurrent(intent.attachment) && options.onError?.(error)
      if (!handled && options.executionCore.isAttachmentCurrent(intent.attachment)) {
        options.onWarning(errorMessage(error, intent))
      }
    }
  }

  return { execute }
}
