import { ref, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/flowImageDirectActionExecutor'
import { loadImageSizeFromUrl } from '../media/imageSize'
import { readCurrentBoardId } from '../../utils/currentBoardId'
import { isTaskResultPendingError } from '../../../../shared/api/taskEvents'
import {
  resolveFirstTaskImageResultDisplay,
  type ResolvedTaskImageResult,
} from '../result-nodes/flowImageDirectActionTaskResult'
import type {
  ImageExecutionInput,
  ImageExecutionResultOptions,
} from '../planning/ImageExecutionPlan'
import {
  usePendingImageResultExecution,
  type PendingImageResultRunSummary,
} from '../result-nodes/usePendingImageResultNodes'
import type { ResultMaterializerImageResult } from '../result-nodes/resultMaterializer'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasGraphNodeId, CanvasGraphStore } from '../../graph-runtime'
import {
  collectPendingImageTasks,
  isPendingImageTaskSnapshot,
  readPendingImageSnapshot,
} from '../result-nodes/flowPendingImageTaskSnapshot'

type PendingImageResultOptions = ImageExecutionResultOptions

type UseImageExecutionLifecycleOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore?: () => CanvasGraphStore
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  getCurrentBoardId?: () => string
}

export type ImageExecutionBatchParams = {
  source: ImageExecutionInput
  sourceNodeId: string
  boardId: string
  outputCount: number
  resultOptions?: PendingImageResultOptions
  preflight?: () => Promise<void>
  assertReady?: () => Promise<void>
  prepare?: () => Promise<ImageExecutionBatchPrepared | void>
  submitTask: (resultNodeId: string) => Promise<{ taskId: string }>
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  persistPendingTaskIds?: boolean
  cleanup?: () => Promise<void>
  successMessage?: string
  fallbackErrorMessage: string
  formatErrorMessage?: (error: Error) => string
}

type ImageExecutionBatchPrepared = {
  source?: ImageExecutionInput
  sourceNodeId?: string
  resultOptions?: PendingImageResultOptions
}

export type ImageExecutionLifecycle = {
  isNodeDirectActionRunning: (nodeId: string) => boolean
  reattachPendingImageResults: () => Promise<void>
  resolveFirstTaskImageResult: (
    taskId: string,
    fallbackRatio: number,
  ) => Promise<ResolvedTaskImageResult>
  runImageExecutionBatch: (params: ImageExecutionBatchParams) => Promise<void>
}

export function useImageExecutionLifecycle(
  options: UseImageExecutionLifecycleOptions,
): ImageExecutionLifecycle {
  const runningNodeIds = ref<Set<string>>(new Set())
  const activeReattachKeys = new Set<string>()
  const pendingResults = usePendingImageResultExecution({
    nodes: options.nodes,
    edges: options.edges,
    graphAdapter: options.graphAdapter,
    createCurrentCanvasGraphStore: options.createCurrentCanvasGraphStore,
    boardBaseSnapshot: options.boardBaseSnapshot,
    refreshNodeInternals: options.refreshNodeInternals,
  })

  function setNodeRunning(nodeId: string, running: boolean) {
    const next = new Set(runningNodeIds.value)
    if (running) {
      next.add(nodeId)
    } else {
      next.delete(nodeId)
    }
    runningNodeIds.value = next
  }

  function isBoardStillCurrent(boardId: string): boolean {
    return !boardId || readCurrentBoardId(options.getCurrentBoardId) === boardId
  }

  function warnIfBoardCurrent(boardId: string, message: string) {
    if (isBoardStillCurrent(boardId)) options.onWarning(message)
  }

  async function flushPendingTaskSnapshot() {
    try {
      await options.flushAutosave?.({ silent: true })
    } catch (error) {
      console.warn('[canvas] flush pending task snapshot failed', error)
      options.onWarning('画布保存失败，刷新页面后任务进度可能丢失')
    }
  }

  async function resolveFirstTaskImageResult(taskId: string, fallbackRatio: number) {
    return await resolveFirstTaskImageResultDisplay(
      await waitExistingOrLiveTaskImageResults(taskId),
      fallbackRatio,
      loadImageSizeFromUrl,
    )
  }

  function isPendingImageTaskStillCurrent(nodeId: string, taskId: string): boolean {
    const snapshot = readPendingImageSnapshot(options.nodes.value, nodeId)
    return !!snapshot && isPendingImageTaskSnapshot(snapshot, taskId)
  }

  function isCanvasGraphNodeId(nodeId: string): nodeId is CanvasGraphNodeId {
    return /^(?:image|prompt|fusion)-\d+(?:\.\d+)?$/.test(nodeId)
  }

  function reportPendingImageRunSummary(
    summary: PendingImageResultRunSummary,
    params: Pick<
      ImageExecutionBatchParams,
      'persistPendingTaskIds' | 'successMessage' | 'fallbackErrorMessage'
    >,
  ) {
    const persistPendingTaskIds = params.persistPendingTaskIds ?? true
    const totalCount = summary.totalCount
    if (summary.successCount === 0 && summary.pendingCount > 0) {
      if (!persistPendingTaskIds) {
        throw summary.firstFailure instanceof Error
          ? summary.firstFailure
          : new Error('任务仍在处理中，请稍后重试')
      }
      options.onWarning(
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
      options.onWarning(
        persistPendingTaskIds
          ? `生成完成 ${summary.successCount}/${totalCount} 张，其余任务稍后可查看`
          : `生成完成 ${summary.successCount}/${totalCount} 张，部分任务仍在处理中，请稍后重试`,
      )
      return
    }
    if (summary.failureCount > 0) {
      options.onWarning(`生成完成 ${summary.successCount}/${totalCount} 张，部分任务失败`)
      return
    }
    options.onSuccess(
      params.successMessage ??
        (summary.successCount > 1 ? `生成完成（${summary.successCount} 张）` : '生成完成'),
    )
  }

  async function runImageExecutionBatch(params: ImageExecutionBatchParams): Promise<void> {
    let resultNodeIds: string[] = []
    try {
      await params.preflight?.()
      const prepared = await params.prepare?.()
      await params.assertReady?.()
      const summary = await pendingResults.runPendingResults({
        source: prepared?.source ?? params.source,
        sourceNodeId: prepared?.sourceNodeId ?? params.sourceNodeId,
        outputCount: params.outputCount,
        resultOptions: prepared?.resultOptions ?? params.resultOptions,
        submitTask: params.submitTask,
        resolveResult: params.resolveResult,
        persistPendingTaskIds: params.persistPendingTaskIds,
        isCurrent: () => isBoardStillCurrent(params.boardId),
        isPendingError: isTaskResultPendingError,
        afterTaskIdAttached: flushPendingTaskSnapshot,
        onReserved: (ids) => {
          resultNodeIds = ids
          ids.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
        },
      })
      if (!isBoardStillCurrent(params.boardId)) return
      reportPendingImageRunSummary(summary, params)
    } catch (error) {
      const message =
        error instanceof Error
          ? (params.formatErrorMessage?.(error) ?? error.message)
          : params.fallbackErrorMessage
      warnIfBoardCurrent(params.boardId, message)
    } finally {
      await params.cleanup?.()
      for (const resultNodeId of resultNodeIds) setNodeRunning(resultNodeId, false)
    }
  }

  async function reattachPendingImageResults(): Promise<void> {
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    const pendingTasks = collectPendingImageTasks(options.nodes.value)
    if (pendingTasks.length === 0) return
    let failureCount = 0
    const resolvedResults: ResultMaterializerImageResult[] = []
    await Promise.allSettled(
      pendingTasks.map(async ({ nodeId, taskId, fallbackRatio }) => {
        const reattachKey = `${boardId}:${nodeId}:${taskId}`
        if (activeReattachKeys.has(reattachKey) || runningNodeIds.value.has(nodeId)) return
        activeReattachKeys.add(reattachKey)
        setNodeRunning(nodeId, true)
        try {
          const resolved = await resolveFirstTaskImageResult(taskId, fallbackRatio)
          if (!isBoardStillCurrent(boardId) || !isPendingImageTaskStillCurrent(nodeId, taskId)) {
            return
          }
          if (!isCanvasGraphNodeId(nodeId)) return
          resolvedResults.push({
            ...resolved,
            resultNodeId: nodeId,
            taskId,
          })
        } catch (error) {
          if (!isBoardStillCurrent(boardId) || !isPendingImageTaskStillCurrent(nodeId, taskId)) {
            return
          }
          console.warn('[canvas] reattach pending image task failed', error)
          failureCount += 1
        } finally {
          activeReattachKeys.delete(reattachKey)
          setNodeRunning(nodeId, false)
        }
      }),
    )
    if (!isBoardStillCurrent(boardId)) return
    const materialized =
      resolvedResults.length > 0
        ? pendingResults.materializeResolvedAttachedPendingResults({
            results: resolvedResults,
            isAttached: (result) =>
              !!result.taskId && isPendingImageTaskStillCurrent(result.resultNodeId, result.taskId),
          })
        : null
    const successCount = materialized?.materializedNodeIds.length ?? 0
    if (successCount > 0) {
      options.onSuccess(successCount > 1 ? `已恢复 ${successCount} 个生成结果` : '已恢复生成结果')
      await flushPendingTaskSnapshot()
    }
    if (failureCount > 0) {
      options.onWarning(
        failureCount > 1 ? `${failureCount} 个生成任务恢复失败` : '生成任务恢复失败',
      )
    }
  }

  function isNodeDirectActionRunning(nodeId: string): boolean {
    return runningNodeIds.value.has(nodeId)
  }

  return {
    isNodeDirectActionRunning,
    reattachPendingImageResults,
    resolveFirstTaskImageResult,
    runImageExecutionBatch,
  }
}
