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
import type { CanvasExecutionAttachment, CanvasExecutionRuntime } from './CanvasExecutionRuntime'
import { createCanvasPendingResultLifecycle } from './CanvasExecutionRuntime'

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
    const createClientActionId = options.createClientActionId ?? createCanvasTaskClientActionId
    const isCurrent = () => options.executionRuntime.isAttachmentCurrent(params.attachment)
    const lifecycle = createCanvasPendingResultLifecycle(
      options.executionRuntime,
      params.attachment,
    )
    const isResultExecutionCurrent = lifecycle.isCurrent
    const transitionOutcome = (outcome: {
      resultNodeId: string
      taskId?: string
      status: 'pending' | 'succeeded' | 'failed' | 'cancelled'
    }) =>
      outcome.status === 'pending'
        ? false
        : lifecycle.transition(outcome.resultNodeId, outcome.status, outcome.taskId)
    const transitionActiveResults = (status: 'failed' | 'cancelled') =>
      lifecycle.transitionAll(resultNodeIds, status)

    async function submitSharedTaskWithTransportRetry(resultNodeIds: readonly string[]) {
      const clientActionId = createClientActionId()
      for (let attempt = 0; attempt < 2; attempt += 1) {
        if (!isCurrent()) throw new TaskCancelledError('Canvas execution is no longer attached')
        try {
          const submission = await params.submitTask(resultNodeIds[0] ?? '', clientActionId, 0)
          let attachedCount = 0
          for (const resultNodeId of resultNodeIds) {
            if (
              lifecycle.isCurrent(resultNodeId) &&
              lifecycle.transition(resultNodeId, 'running', submission.taskId)
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
        const lease = lifecycle.begin(resultNodeId)
        if (!lease) {
          transitionActiveResults('cancelled')
          options.pendingResults.cancelPendingResults(resultNodeIds)
          return
        }
      }
      const sharedSubmissionPromise = submitSharedTaskWithTransportRetry(resultNodeIds)
      const resultIndexByNodeId = new Map(
        resultNodeIds.map((resultNodeId, resultIndex) => [resultNodeId, resultIndex]),
      )
      let hasCurrentOutcome = false
      const summary = await options.pendingResults.runReservedPendingResults({
        resultNodeIds,
        submitTask: () => sharedSubmissionPromise,
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
