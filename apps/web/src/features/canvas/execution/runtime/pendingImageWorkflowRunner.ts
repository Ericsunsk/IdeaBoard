import { isTaskResultPendingError, TaskCancelledError } from '../../../../shared/api/taskEvents'
import type {
  ImageExecutionInput,
  ImageExecutionResultOptions,
} from '../planning/ImageExecutionPlan'
import type { ResolvedTaskImageResult } from '../result-nodes/resolvedTaskImageResult'
import type {
  PendingImageResultExecution,
  PendingImageResultRunSummary,
} from '../result-nodes/usePendingImageResultNodes'
import {
  createCanvasTaskClientActionId,
  type ClientActionIdFactory,
} from '../task-runner/taskClientActionIds'
import { isCanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'
import type {
  CanvasExecutionAttachment,
  CanvasExecutionLease,
  CanvasExecutionRuntime,
} from './CanvasExecutionRuntime'

type PendingImageResultOptions = ImageExecutionResultOptions

export type PendingImageWorkflowPrepared = {
  source?: ImageExecutionInput
  sourceNodeId?: string
  resultOptions?: PendingImageResultOptions
}

export type PendingImageWorkflowBatchParams = {
  source: ImageExecutionInput
  sourceNodeId: string
  attachment: CanvasExecutionAttachment
  outputCount: number
  submissionMode?: 'per-result' | 'shared-task'
  resultOptions?: PendingImageResultOptions
  preflight?: () => Promise<void>
  assertReady?: () => Promise<void>
  prepare?: () => Promise<PendingImageWorkflowPrepared | void>
  submitTask: (
    resultNodeId: string,
    clientActionId: string,
    resultIndex: number,
  ) => Promise<{ taskId: string }>
  resolveResult: (
    taskId: string,
    resultNodeId: string,
    resultIndex: number,
  ) => Promise<ResolvedTaskImageResult>
  persistPendingTaskIds?: boolean
  cleanup?: () => Promise<void>
  successMessage?: string
  fallbackErrorMessage: string
  formatErrorMessage?: (error: Error) => string
}

export type PendingImageWorkflowRunner = {
  run: (params: PendingImageWorkflowBatchParams) => Promise<void>
}

export type PendingImageWorkflowRunnerOptions = {
  pendingResults: Pick<
    PendingImageResultExecution,
    'reservePendingResults' | 'runReservedPendingResults' | 'cancelPendingResults'
  >
  executionRuntime: CanvasExecutionRuntime
  cancelTasks: (taskIds: readonly string[]) => void
  afterTaskIdAttached?: (params: { resultNodeId: string; taskId: string }) => Promise<void>
  createClientActionId?: ClientActionIdFactory
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
}

function resolvePendingRunErrorMessage(
  error: unknown,
  params: Pick<PendingImageWorkflowBatchParams, 'fallbackErrorMessage' | 'formatErrorMessage'>,
): string {
  if (!(error instanceof Error)) return params.fallbackErrorMessage
  return params.formatErrorMessage?.(error) ?? error.message
}

function reportPendingImageRunSummary(
  summary: PendingImageResultRunSummary,
  params: Pick<
    PendingImageWorkflowBatchParams,
    'persistPendingTaskIds' | 'successMessage' | 'fallbackErrorMessage'
  >,
  onSuccess: (message: string) => void,
  onWarning: (message: string) => void,
) {
  const persistPendingTaskIds = params.persistPendingTaskIds ?? true
  const totalCount = summary.totalCount
  if (summary.successCount === 0 && summary.pendingCount > 0) {
    if (!persistPendingTaskIds) {
      throw summary.firstFailure instanceof Error
        ? summary.firstFailure
        : new Error('任务仍在处理中，请稍后重试')
    }
    onWarning(
      totalCount === 1 && summary.firstFailure instanceof Error
        ? summary.firstFailure.message
        : '任务进行中，稍后可在画布上查看结果',
    )
    return
  }
  if (summary.successCount === 0) {
    throw summary.firstFailure instanceof Error
      ? summary.firstFailure
      : new Error(params.fallbackErrorMessage)
  }
  if (summary.pendingCount > 0) {
    onWarning(
      persistPendingTaskIds
        ? `生成完成 ${summary.successCount}/${totalCount} 张，其余任务稍后可查看`
        : `生成完成 ${summary.successCount}/${totalCount} 张，部分任务仍在处理中，请稍后重试`,
    )
    return
  }
  if (summary.failureCount > 0) {
    onWarning(`生成完成 ${summary.successCount}/${totalCount} 张，部分任务失败`)
    return
  }
  onSuccess(
    params.successMessage ??
      (summary.successCount > 1 ? `生成完成（${summary.successCount} 张）` : '生成完成'),
  )
}

export function createPendingImageWorkflowRunner(
  options: PendingImageWorkflowRunnerOptions,
): PendingImageWorkflowRunner {
  function warnIfAttachmentCurrent(attachment: CanvasExecutionAttachment, message: string) {
    if (options.executionRuntime.isAttachmentCurrent(attachment)) options.onWarning(message)
  }

  async function run(params: PendingImageWorkflowBatchParams): Promise<void> {
    let resultNodeIds: string[] = []
    const leasesByResultNodeId = new Map<string, CanvasExecutionLease>()
    const clientActionIdsByResultNodeId = new Map<string, string>()
    const createClientActionId = options.createClientActionId ?? createCanvasTaskClientActionId
    const isCurrent = () => options.executionRuntime.isAttachmentCurrent(params.attachment)
    const isResultExecutionCurrent = (resultNodeId: string, taskId?: string) => {
      const lease = leasesByResultNodeId.get(resultNodeId)
      const state = options.executionRuntime.getNodeState(resultNodeId)
      return (
        !!lease &&
        options.executionRuntime.getNodeLease(resultNodeId) === lease &&
        options.executionRuntime.isNodeActive(resultNodeId) &&
        (!taskId || state?.taskId === taskId)
      )
    }
    const transitionOutcome = (outcome: {
      resultNodeId: string
      taskId?: string
      status: 'pending' | 'succeeded' | 'failed' | 'cancelled'
    }) => {
      if (outcome.status === 'pending') return false
      const lease = leasesByResultNodeId.get(outcome.resultNodeId)
      if (!lease || !options.executionRuntime.isNodeActive(outcome.resultNodeId)) return false
      return options.executionRuntime.transitionNode(lease, {
        status: outcome.status,
        taskId: outcome.taskId,
      })
    }
    const transitionActiveResults = (status: 'failed' | 'cancelled') => {
      resultNodeIds.forEach((nodeId) => {
        const lease = leasesByResultNodeId.get(nodeId)
        const current = options.executionRuntime.getNodeState(nodeId)
        if (!lease || !current || !options.executionRuntime.isNodeActive(nodeId)) return
        options.executionRuntime.transitionNode(lease, {
          status,
          taskId: current.taskId,
        })
      })
    }

    async function submitWithTransportRetry(resultNodeId: string, resultIndex: number) {
      if (!isCurrent()) throw new TaskCancelledError('Canvas execution is no longer attached')
      const clientActionId =
        clientActionIdsByResultNodeId.get(resultNodeId) ?? createClientActionId()
      clientActionIdsByResultNodeId.set(resultNodeId, clientActionId)
      for (let attempt = 0; attempt < 2; attempt += 1) {
        if (!isCurrent()) throw new TaskCancelledError('Canvas execution is no longer attached')
        try {
          const submission = await params.submitTask(resultNodeId, clientActionId, resultIndex)
          const lease = leasesByResultNodeId.get(resultNodeId)
          const didAttachTask =
            !!lease &&
            options.executionRuntime.transitionNode(lease, {
              status: 'running',
              taskId: submission.taskId,
            })
          if (!didAttachTask) {
            options.cancelTasks([submission.taskId])
            throw new TaskCancelledError('Canvas execution is no longer attached')
          }
          return submission
        } catch (error) {
          if (attempt === 0 && isCanvasBillingPreflightError(error, 'network')) continue
          throw error
        }
      }
      throw new Error('网络错误，请重试')
    }

    async function submitSharedTaskWithTransportRetry(resultNodeIds: readonly string[]) {
      const clientActionId = createClientActionId()
      for (let attempt = 0; attempt < 2; attempt += 1) {
        if (!isCurrent()) throw new TaskCancelledError('Canvas execution is no longer attached')
        try {
          const submission = await params.submitTask(resultNodeIds[0] ?? '', clientActionId, 0)
          let attachedCount = 0
          for (const resultNodeId of resultNodeIds) {
            const lease = leasesByResultNodeId.get(resultNodeId)
            if (
              lease &&
              options.executionRuntime.transitionNode(lease, {
                status: 'running',
                taskId: submission.taskId,
              })
            ) {
              attachedCount += 1
            }
          }
          if (attachedCount === 0) {
            options.cancelTasks([submission.taskId])
            throw new TaskCancelledError('Canvas execution is no longer attached')
          }
          return submission
        } catch (error) {
          if (attempt === 0 && isCanvasBillingPreflightError(error, 'network')) continue
          throw error
        }
      }
      throw new Error('网络错误，请重试')
    }

    try {
      await params.preflight?.()
      const prepared = await params.prepare?.()
      await params.assertReady?.()
      if (!isCurrent()) return
      const reservation = options.pendingResults.reservePendingResults({
        source: prepared?.source ?? params.source,
        sourceNodeId: prepared?.sourceNodeId ?? params.sourceNodeId,
        outputCount: params.outputCount,
        resultOptions: prepared?.resultOptions ?? params.resultOptions,
      })
      resultNodeIds = reservation.resultNodeIds
      for (const resultNodeId of resultNodeIds) {
        const lease = options.executionRuntime.beginSubmission(params.attachment, resultNodeId)
        if (!lease) {
          transitionActiveResults('cancelled')
          options.pendingResults.cancelPendingResults(resultNodeIds)
          return
        }
        leasesByResultNodeId.set(resultNodeId, lease)
      }
      const sharedSubmissionPromise =
        params.submissionMode === 'shared-task'
          ? submitSharedTaskWithTransportRetry(resultNodeIds)
          : null
      const resultIndexByNodeId = new Map(
        resultNodeIds.map((resultNodeId, resultIndex) => [resultNodeId, resultIndex]),
      )
      let hasCurrentOutcome = false
      const summary = await options.pendingResults.runReservedPendingResults({
        resultNodeIds,
        submitTask: (resultNodeId) => {
          if (sharedSubmissionPromise) return sharedSubmissionPromise
          return submitWithTransportRetry(resultNodeId, resultIndexByNodeId.get(resultNodeId) ?? -1)
        },
        resolveResult: (taskId, resultNodeId) =>
          params.resolveResult(taskId, resultNodeId, resultIndexByNodeId.get(resultNodeId) ?? -1),
        persistPendingTaskIds: params.persistPendingTaskIds,
        isCurrent,
        isResultExecutionCurrent,
        isPendingError: isTaskResultPendingError,
        afterTaskIdAttached: options.afterTaskIdAttached,
        onOutcome: (outcome) => {
          if (outcome.status === 'pending') {
            hasCurrentOutcome ||=
              !!outcome.taskId && isResultExecutionCurrent(outcome.resultNodeId, outcome.taskId)
            return
          }
          hasCurrentOutcome = transitionOutcome(outcome) || hasCurrentOutcome
        },
      })
      summary.outcomes.forEach((outcome) => {
        if (outcome.status === 'pending') {
          hasCurrentOutcome ||=
            !!outcome.taskId && isResultExecutionCurrent(outcome.resultNodeId, outcome.taskId)
          return
        }
        hasCurrentOutcome = transitionOutcome(outcome) || hasCurrentOutcome
      })
      if (!hasCurrentOutcome) return
      if (!isCurrent()) return
      let typedFailureHandled = false
      if (summary.successCount > 0) {
        const typedFailure = summary.failures.find((error) =>
          isCanvasBillingPreflightError(error, 'insufficient-credits'),
        )
        typedFailureHandled = !!typedFailure && options.onError?.(typedFailure) === true
      }
      if (typedFailureHandled) return
      reportPendingImageRunSummary(summary, params, options.onSuccess, options.onWarning)
    } catch (error) {
      transitionActiveResults(error instanceof TaskCancelledError ? 'cancelled' : 'failed')
      const handled = isCurrent() && options.onError?.(error) === true
      if (!handled) {
        warnIfAttachmentCurrent(params.attachment, resolvePendingRunErrorMessage(error, params))
      }
    } finally {
      await params.cleanup?.()
      if (!isCurrent()) transitionActiveResults('cancelled')
    }
  }

  return { run }
}
