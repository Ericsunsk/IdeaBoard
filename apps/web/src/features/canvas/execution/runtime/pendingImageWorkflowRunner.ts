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
  CanvasNodeExecutionChange,
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
  activationId: string
  resultOptions?: PendingImageResultOptions
  preflight?: () => Promise<void>
  assertReady?: () => Promise<void>
  prepare?: () => Promise<PendingImageWorkflowPrepared | void>
  submitTask: (resultNodeId: string, clientActionId: string) => Promise<{ taskId: string }>
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
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
  pendingResults: Pick<PendingImageResultExecution, 'runPendingResults'>
  executionRuntime: CanvasExecutionRuntime
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
    const transitionCurrentExecution = (change: CanvasNodeExecutionChange) => {
      const lease = leasesByResultNodeId.get(change.nodeId)
      if (lease) options.executionRuntime.transitionNode(lease, change.state)
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

    async function submitWithTransportRetry(resultNodeId: string) {
      const clientActionId =
        clientActionIdsByResultNodeId.get(resultNodeId) ?? createClientActionId()
      clientActionIdsByResultNodeId.set(resultNodeId, clientActionId)
      for (let attempt = 0; attempt < 2; attempt += 1) {
        try {
          return await params.submitTask(resultNodeId, clientActionId)
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
      const summary = await options.pendingResults.runPendingResults({
        source: prepared?.source ?? params.source,
        sourceNodeId: prepared?.sourceNodeId ?? params.sourceNodeId,
        outputCount: params.outputCount,
        clientActionKey: params.activationId,
        resultOptions: prepared?.resultOptions ?? params.resultOptions,
        submitTask: submitWithTransportRetry,
        resolveResult: params.resolveResult,
        persistPendingTaskIds: params.persistPendingTaskIds,
        isCurrent,
        isPendingError: isTaskResultPendingError,
        afterTaskIdAttached: options.afterTaskIdAttached,
        onExecutionStateChange: transitionCurrentExecution,
        onReserved: (ids) => {
          resultNodeIds = ids
          ids.forEach((resultNodeId) => {
            const lease = options.executionRuntime.startNode(params.attachment, resultNodeId, {
              status: 'submitting',
            })
            if (lease) leasesByResultNodeId.set(resultNodeId, lease)
          })
        },
      })
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
