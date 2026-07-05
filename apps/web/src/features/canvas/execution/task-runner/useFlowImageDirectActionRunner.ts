import { ref, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { waitExistingOrLiveTaskImageResults } from './flowImageDirectActionExecutor'
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
import type {
  PendingImageResultExecution,
  PendingImageResultRunSummary,
} from '../result-nodes/usePendingImageResultNodes'
import {
  collectPendingImageTasks,
  isPendingImageTaskSnapshot,
  readPendingImageSnapshot,
} from '../result-nodes/flowPendingImageTaskSnapshot'

type PendingImageResultOptions = ImageExecutionResultOptions

type UseFlowImageDirectActionRunnerOptions = {
  nodes: Ref<Node[]>
  pendingImageResults: PendingImageResultExecution
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  getCurrentBoardId?: () => string
}

type PendingImageBatchWorkflowParams = {
  source: ImageExecutionInput
  sourceNodeId: string
  boardId: string
  outputCount: number
  resultOptions?: PendingImageResultOptions
  preflight?: () => Promise<void>
  assertReady?: () => Promise<void>
  prepare?: () => Promise<PendingImageBatchWorkflowPrepared | void>
  submitTask: (resultNodeId: string) => Promise<{ taskId: string }>
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  persistPendingTaskIds?: boolean
  cleanup?: () => Promise<void>
  successMessage?: string
  fallbackErrorMessage: string
  formatErrorMessage?: (error: Error) => string
}

type PendingImageBatchWorkflowPrepared = {
  source?: ImageExecutionInput
  sourceNodeId?: string
  resultOptions?: PendingImageResultOptions
}

export function useFlowImageDirectActionRunner(options: UseFlowImageDirectActionRunnerOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())
  const activeReattachKeys = new Set<string>()
  const pendingResults = options.pendingImageResults

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

  function reportPendingImageRunSummary(
    summary: PendingImageResultRunSummary,
    params: Pick<
      PendingImageBatchWorkflowParams,
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

  async function runPendingImageBatchWorkflow(
    params: PendingImageBatchWorkflowParams,
  ): Promise<void> {
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
        afterTaskIdsAttached: flushPendingTaskSnapshot,
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
    const tasksToResolve = pendingTasks
      .map(({ nodeId, taskId, fallbackRatio }) => {
        const reattachKey = `${boardId}:${nodeId}:${taskId}`
        if (activeReattachKeys.has(reattachKey) || runningNodeIds.value.has(nodeId)) return null
        activeReattachKeys.add(reattachKey)
        setNodeRunning(nodeId, true)
        return { nodeId, taskId, fallbackRatio, reattachKey }
      })
      .filter((task): task is NonNullable<typeof task> => !!task)
    if (tasksToResolve.length === 0) return

    let successCount: number
    let failureCount: number
    try {
      const taskByNodeId = new Map(tasksToResolve.map((task) => [task.nodeId, task]))
      const summary = await pendingResults.resolveAttachedPendingResults({
        tasks: tasksToResolve.map(({ nodeId, taskId }) => ({
          resultNodeId: nodeId,
          taskId,
        })),
        resolveResult: (pendingTaskId, resultNodeId) => {
          const task = taskByNodeId.get(resultNodeId)
          return resolveFirstTaskImageResult(pendingTaskId, task?.fallbackRatio ?? 1)
        },
        isCurrent: () => isBoardStillCurrent(boardId),
        isAttached: ({ resultNodeId, taskId }) =>
          isPendingImageTaskStillCurrent(resultNodeId, taskId),
        onResolveError: (task, error) => {
          if (
            !isBoardStillCurrent(boardId) ||
            !isPendingImageTaskStillCurrent(task.resultNodeId, task.taskId)
          ) {
            return
          }
          console.warn('[canvas] reattach pending image task failed', error)
        },
      })
      successCount = summary.successCount
      failureCount = summary.failureCount
    } finally {
      tasksToResolve.forEach(({ nodeId, reattachKey }) => {
        activeReattachKeys.delete(reattachKey)
        setNodeRunning(nodeId, false)
      })
    }
    if (!isBoardStillCurrent(boardId)) return
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
    runPendingImageBatchWorkflow,
  }
}
