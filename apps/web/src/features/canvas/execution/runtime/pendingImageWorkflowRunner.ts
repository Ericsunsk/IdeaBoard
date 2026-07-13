import { isTaskResultPendingError } from '../../../../shared/api/taskEvents'
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
  createPendingTaskClientActionIds,
  type ClientActionIdFactory,
} from '../task-runner/taskClientActionIds'
import { isCanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'

type PendingImageResultOptions = ImageExecutionResultOptions

export type PendingImageWorkflowPrepared = {
  source?: ImageExecutionInput
  sourceNodeId?: string
  resultOptions?: PendingImageResultOptions
}

export type PendingImageWorkflowBatchParams = {
  source: ImageExecutionInput
  sourceNodeId: string
  boardId: string
  outputCount: number
  clientActionKey?: string
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
  isBoardCurrent: (boardId: string) => boolean
  setNodeRunning: (nodeId: string, running: boolean) => void
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
  const clientActionIds = createPendingTaskClientActionIds(options.createClientActionId)

  function warnIfBoardCurrent(boardId: string, message: string) {
    if (options.isBoardCurrent(boardId)) options.onWarning(message)
  }

  async function run(params: PendingImageWorkflowBatchParams): Promise<void> {
    let resultNodeIds: string[] = []
    try {
      await params.preflight?.()
      const prepared = await params.prepare?.()
      await params.assertReady?.()
      const summary = await options.pendingResults.runPendingResults({
        source: prepared?.source ?? params.source,
        sourceNodeId: prepared?.sourceNodeId ?? params.sourceNodeId,
        outputCount: params.outputCount,
        clientActionKey: params.clientActionKey,
        resultOptions: prepared?.resultOptions ?? params.resultOptions,
        submitTask: (resultNodeId) =>
          params.submitTask(resultNodeId, clientActionIds.getForResultNode(resultNodeId)),
        resolveResult: params.resolveResult,
        persistPendingTaskIds: params.persistPendingTaskIds,
        isCurrent: () => options.isBoardCurrent(params.boardId),
        isPendingError: isTaskResultPendingError,
        afterTaskIdAttached: options.afterTaskIdAttached,
        onReserved: (ids) => {
          resultNodeIds = ids
          ids.forEach((resultNodeId) => options.setNodeRunning(resultNodeId, true))
        },
      })
      if (!options.isBoardCurrent(params.boardId)) return
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
      const handled = options.isBoardCurrent(params.boardId) && options.onError?.(error) === true
      if (!handled) {
        warnIfBoardCurrent(params.boardId, resolvePendingRunErrorMessage(error, params))
      }
    } finally {
      await params.cleanup?.()
      for (const resultNodeId of resultNodeIds) options.setNodeRunning(resultNodeId, false)
    }
  }

  return { run }
}
