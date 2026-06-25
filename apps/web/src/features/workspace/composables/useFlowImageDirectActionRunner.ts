import { ref, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../types/canvasNodes'
import { waitExistingOrLiveTaskImageResults } from '../services/flowImageDirectActionExecutor'
import { loadImageSizeFromUrl } from '../utils/imageSize'
import { readCurrentBoardId } from '../utils/currentBoardId'
import { isTaskResultPendingError } from '../../../shared/api/taskEvents'
import {
  pickFirstValidImageResult,
  resolveTaskImageResultDisplay,
  type ResolvedTaskImageResult,
} from '../utils/flowImageDirectActionTaskResult'
import type { ImageDirectActionSource } from '../utils/flowImageDirectActionSource'

type PendingImageResultNodes = {
  appendPendingImageResultNode: (source: ImageDirectActionSource, sourceNodeId: string) => string
  appendPendingImageResultNodes: (
    source: ImageDirectActionSource,
    sourceNodeId: string,
    outputCount: number,
    options?: PendingImageResultOptions,
  ) => string[]
  markPendingImageResultTask: (nodeId: string, taskId: string) => void
  updatePendingImageResultNode: (nodeId: string, result: ResolvedTaskImageResult) => void
  removePendingImageResultNode: (nodeId: string) => void
}

type PendingImageResultOptions = {
  outputAnchorNodeId?: string
  targetRatio?: number
  targetWidth?: number
}

type UseFlowImageDirectActionRunnerOptions = {
  nodes: Ref<Node[]>
  pendingImageResultNodes: PendingImageResultNodes
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  getCurrentBoardId?: () => string
}

type PendingImageBatchGenerationParams = {
  resultNodeIds: string[]
  submitTask: (resultNodeId: string) => Promise<{ taskId: string }>
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  totalCount: number
  boardId: string
}

type PendingImageBatchWorkflowParams = {
  source: ImageDirectActionSource
  sourceNodeId: string
  boardId: string
  outputCount: number
  resultOptions?: PendingImageResultOptions
  preflight?: () => Promise<void>
  assertReady?: () => Promise<void>
  prepare?: () => Promise<PendingImageBatchWorkflowPrepared | void>
  submitTask: (resultNodeId: string) => Promise<{ taskId: string }>
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  cleanup?: () => Promise<void>
  fallbackErrorMessage: string
  formatErrorMessage?: (error: Error) => string
}

type PendingImageBatchWorkflowPrepared = {
  source?: ImageDirectActionSource
  sourceNodeId?: string
  resultOptions?: PendingImageResultOptions
}

type PendingImageGenerationParams = {
  source: ImageDirectActionSource
  sourceNodeId: string
  boardId: string
  assertReady?: () => Promise<void>
  submitTask: () => Promise<{ taskId: string }>
  resolveResult: (taskId: string) => Promise<ResolvedTaskImageResult>
  successMessage: string
  fallbackErrorMessage: string
}

export function useFlowImageDirectActionRunner(options: UseFlowImageDirectActionRunnerOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())
  const activeReattachKeys = new Set<string>()
  const {
    appendPendingImageResultNode,
    appendPendingImageResultNodes,
    markPendingImageResultTask,
    updatePendingImageResultNode,
    removePendingImageResultNode,
  } = options.pendingImageResultNodes

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

  function removePendingImageResultNodesIfCurrent(boardId: string, resultNodeIds: string[]) {
    if (!isBoardStillCurrent(boardId)) return
    for (const resultNodeId of resultNodeIds) removePendingImageResultNode(resultNodeId)
  }

  function warnIfBoardCurrent(boardId: string, message: string) {
    if (isBoardStillCurrent(boardId)) options.onWarning(message)
  }

  async function flushPendingTaskSnapshot() {
    try {
      await options.flushAutosave?.({ silent: true })
    } catch (error) {
      console.warn('[workspace] flush pending task snapshot failed', error)
      options.onWarning('画布保存失败，刷新页面后任务进度可能丢失')
    }
  }

  async function persistPendingImageTaskId(boardId: string, nodeId: string, taskId: string) {
    if (!isBoardStillCurrent(boardId)) return
    markPendingImageResultTask(nodeId, taskId)
    await flushPendingTaskSnapshot()
  }

  async function resolveFirstTaskImageResult(taskId: string, fallbackRatio: number) {
    const result = pickFirstValidImageResult(await waitExistingOrLiveTaskImageResults(taskId))
    if (!result) throw new Error('任务完成但未返回图片')
    const resolved = await resolveTaskImageResultDisplay(
      result,
      fallbackRatio,
      loadImageSizeFromUrl,
    )
    if (!resolved) throw new Error('任务完成但未返回图片')
    return resolved
  }

  function getPendingImageSnapshot(nodeId: string): ImageNodeItem | null {
    const node = options.nodes.value.find((item) => item.id === nodeId)
    if (!node || node.data?.kind !== 'image') return null
    const snapshot = node.data.snapshotNode
    if (!snapshot || typeof snapshot !== 'object') return null
    return snapshot as ImageNodeItem
  }

  function hasRenderableImageResult(snapshot: ImageNodeItem): boolean {
    const sourceUrl = `${snapshot.sourceImageUrl ?? ''}`.trim()
    if (sourceUrl) return true
    const outputImages = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    return outputImages.some((item) => `${item?.url ?? ''}`.trim().length > 0)
  }

  function isPendingImageTaskSnapshot(snapshot: ImageNodeItem, taskId?: string): boolean {
    const pendingTaskId = `${snapshot.pendingTaskId ?? ''}`.trim()
    if (!pendingTaskId) return false
    if (taskId && pendingTaskId !== taskId) return false
    return snapshot.outputStatus === 'running' && !hasRenderableImageResult(snapshot)
  }

  function isPendingImageTaskStillCurrent(nodeId: string, taskId: string): boolean {
    const snapshot = getPendingImageSnapshot(nodeId)
    return !!snapshot && isPendingImageTaskSnapshot(snapshot, taskId)
  }

  function collectPendingImageTasks(): Array<{
    nodeId: string
    taskId: string
    fallbackRatio: number
  }> {
    return options.nodes.value.flatMap((node) => {
      if (node.data?.kind !== 'image') return []
      const snapshot = node.data.snapshotNode
      if (!snapshot || typeof snapshot !== 'object') return []
      const imageSnapshot = snapshot as ImageNodeItem
      if (!isPendingImageTaskSnapshot(imageSnapshot)) return []
      const fallbackRatio = Number(imageSnapshot.sourceImageRatio)
      return [
        {
          nodeId: node.id,
          taskId: imageSnapshot.pendingTaskId!.trim(),
          fallbackRatio: Number.isFinite(fallbackRatio) && fallbackRatio > 0 ? fallbackRatio : 1,
        },
      ]
    })
  }

  async function runPendingImageBatchGeneration(params: PendingImageBatchGenerationParams) {
    let successCount = 0
    let pendingCount = 0
    let failureCount = 0
    let firstFailure: unknown = null
    await Promise.allSettled(
      params.resultNodeIds.map(async (resultNodeId) => {
        try {
          const { taskId } = await params.submitTask(resultNodeId)
          await persistPendingImageTaskId(params.boardId, resultNodeId, taskId)
          const resolved = await params.resolveResult(taskId, resultNodeId)
          if (isBoardStillCurrent(params.boardId)) {
            updatePendingImageResultNode(resultNodeId, resolved)
          }
          successCount += 1
        } catch (error) {
          firstFailure ??= error
          if (isTaskResultPendingError(error)) {
            pendingCount += 1
            return
          }
          failureCount += 1
          if (isBoardStillCurrent(params.boardId)) {
            removePendingImageResultNode(resultNodeId)
          }
        }
      }),
    )
    if (!isBoardStillCurrent(params.boardId)) return
    if (successCount === 0 && pendingCount > 0) {
      options.onWarning('任务进行中，稍后可在画布上查看结果')
      return
    }
    if (successCount === 0)
      throw firstFailure instanceof Error ? firstFailure : new Error('生成失败')
    if (pendingCount > 0) {
      options.onWarning(`生成完成 ${successCount}/${params.totalCount} 张，其余任务稍后可查看`)
    } else if (failureCount > 0) {
      options.onWarning(`生成完成 ${successCount}/${params.totalCount} 张，部分任务失败`)
    } else {
      options.onSuccess(successCount > 1 ? `生成完成（${successCount} 张）` : '生成完成')
    }
  }

  async function runPendingImageBatchWorkflow(
    params: PendingImageBatchWorkflowParams,
  ): Promise<void> {
    let resultNodeIds: string[] = []
    try {
      await params.preflight?.()
      const prepared = await params.prepare?.()
      await params.assertReady?.()
      resultNodeIds = appendPendingImageResultNodes(
        prepared?.source ?? params.source,
        prepared?.sourceNodeId ?? params.sourceNodeId,
        params.outputCount,
        prepared?.resultOptions ?? params.resultOptions,
      )
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: params.outputCount,
        boardId: params.boardId,
        submitTask: params.submitTask,
        resolveResult: params.resolveResult,
      })
    } catch (error) {
      removePendingImageResultNodesIfCurrent(params.boardId, resultNodeIds)
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

  async function runPendingImageGeneration(params: PendingImageGenerationParams): Promise<void> {
    let resultNodeId: string | null = null
    try {
      await params.assertReady?.()
      resultNodeId = appendPendingImageResultNode(params.source, params.sourceNodeId)
      setNodeRunning(resultNodeId, true)
      const { taskId } = await params.submitTask()
      await persistPendingImageTaskId(params.boardId, resultNodeId, taskId)
      const resolved = await params.resolveResult(taskId)
      if (!isBoardStillCurrent(params.boardId)) return
      updatePendingImageResultNode(resultNodeId, resolved)
      options.onSuccess(params.successMessage)
    } catch (error) {
      if (isTaskResultPendingError(error) && resultNodeId) {
        warnIfBoardCurrent(params.boardId, error.message)
        return
      }
      if (resultNodeId && isBoardStillCurrent(params.boardId)) {
        removePendingImageResultNode(resultNodeId)
      }
      const msg = error instanceof Error ? error.message : params.fallbackErrorMessage
      warnIfBoardCurrent(params.boardId, msg)
    } finally {
      if (resultNodeId) setNodeRunning(resultNodeId, false)
    }
  }

  async function reattachPendingImageResults(): Promise<void> {
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    const pendingTasks = collectPendingImageTasks()
    if (pendingTasks.length === 0) return
    let successCount = 0
    let failureCount = 0
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
          updatePendingImageResultNode(nodeId, resolved)
          successCount += 1
        } catch (error) {
          if (!isBoardStillCurrent(boardId) || !isPendingImageTaskStillCurrent(nodeId, taskId)) {
            return
          }
          console.warn('[workspace] reattach pending image task failed', error)
          failureCount += 1
        } finally {
          activeReattachKeys.delete(reattachKey)
          setNodeRunning(nodeId, false)
        }
      }),
    )
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
    runPendingImageGeneration,
    runPendingImageBatchWorkflow,
  }
}
