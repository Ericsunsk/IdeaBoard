import { ref, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import {
  clampImageNodeOutputCount,
  type FusionNodeItem,
  type ImageNodeItem,
} from '../types/canvasNodes'
import {
  buildPromptAspectRatioTaskParams,
  normalizePromptAspectRatio,
  type PromptAspectRatio,
} from '../config/promptAspectRatio'
import type { FlowNodeData } from '../types/flowTypes'
import { taskApi } from '../../../shared/api/task'
import { uploadApi } from '../../../shared/api/upload'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { buildExecutionTaskRequest, EXECUTION_FUSION_META } from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import { resolveImageNodeFrameSize } from '../utils/imageNodeSizing'
import { resolveFusionOutputSize } from '../utils/fusionOutputSizing'
import { computeFusionBlendWeights } from '../utils/fusionBlend'
import {
  cleanupFlowLocalInpaintAssets,
  composeFlowLocalInpaint,
  prepareFlowLocalInpaint,
  type FlowLocalInpaintCropResult,
} from '../services/flowLocalInpaint'
import { ensureFlowMaskImageUrl } from '../services/flowMediaUpload'
import { isDataUrl } from '../services/flowMediaIO'
import {
  assertCanvasExtendSubmitReady,
  assertDirectActionSubmitReady,
  assertHdUpscaleSubmitReady,
  assertPromptImageSubmitReady,
  submitCanvasExtendTask,
  submitDirectActionTask,
  submitHdUpscaleTask,
  submitImageInpaintTask,
  submitPromptImageGenerationTask,
  submitPromptTextGenerationTask,
  waitExistingOrLiveTaskImageResults,
} from '../services/flowImageDirectActionExecutor'
import { loadImageSizeFromUrl } from '../utils/imageSize'
import { usePendingImageResultNodes } from './usePendingImageResultNodes'
import {
  pickFirstValidImageResult,
  resolveTaskImageResultDisplay,
  resolveTaskImageRatio,
  type ResolvedTaskImageResult,
} from '../utils/flowImageDirectActionTaskResult'
import {
  resolveImageDirectActionSource,
  type ImageDirectActionSource,
} from '../utils/flowImageDirectActionSource'
import {
  IMAGE_NODE_ACTION,
  type ImageDirectTaskAction,
  type ImageStrengthAction,
} from '../types/imageActions'
import type { FlowTransactions } from './useFlowTransactions'
import { resolveCanvasExtendIntent, type CanvasExtendRect } from '../utils/canvasExtendGeometry'
import {
  assertTaskSubmitReady,
  resolveConnectedImageInputs,
  resolveConnectedInput,
  resolveConnectedPromptText,
} from '../services/flowTaskExecutorUtils'
import { readCurrentBoardId } from '../utils/currentBoardId'
import { isTaskResultPendingError } from '../../../shared/api/taskEvents'

type DirectActionKey = ImageDirectTaskAction | ImageStrengthAction
type RunImageDirectActionOptions = {
  strength?: number
}
type RunImageInpaintActionOptions = {
  prompt: string
  maskDataUrl: string
  outputCount: number
  referenceImageUrls?: string[]
}

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  commitDirty: FlowTransactions['commitDirty']
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  getCurrentBoardId?: () => string
}

type CanvasExtendRectIntent = {
  frameRect: CanvasExtendRect
  originalImageRect: CanvasExtendRect
  prompt?: string
  outputCount?: number
}

export function useFlowImageDirectActions(options: UseFlowImageDirectActionsOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())
  const {
    appendPendingImageResultNode,
    appendPendingImageResultNodes,
    markPendingImageResultTask,
    updatePendingImageResultNode,
    removePendingImageResultNode,
  } = usePendingImageResultNodes(options)
  const activeReattachKeys = new Set<string>()

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
    const resolved = await resolveTaskImageResultDisplay(result, fallbackRatio, loadImageSizeFromUrl)
    if (!resolved) throw new Error('任务完成但未返回图片')
    return resolved
  }

  function resolveCanvasExtendResultWidth(
    sourceWidthValue: unknown,
    frameRect: CanvasExtendRect,
    originalImageRect: CanvasExtendRect,
    fallbackRatio: number,
  ): number {
    const sourceWidth = Number(sourceWidthValue)
    const frameWidth = Number(frameRect.width)
    const originalWidth = Number(originalImageRect.width)
    if (
      Number.isFinite(sourceWidth) &&
      sourceWidth > 0 &&
      Number.isFinite(frameWidth) &&
      frameWidth > 0 &&
      Number.isFinite(originalWidth) &&
      originalWidth > 0
    ) {
      return Math.max(1, Math.round(sourceWidth * (frameWidth / originalWidth)))
    }
    return resolveImageNodeFrameSize(fallbackRatio, true).width
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

  async function runPendingImageBatchGeneration(params: {
    resultNodeIds: string[]
    submitTask: (resultNodeId: string) => Promise<{ taskId: string }>
    resolveResult: (taskId: string) => Promise<ResolvedTaskImageResult>
    totalCount: number
    boardId: string
  }) {
    let successCount = 0
    let pendingCount = 0
    let failureCount = 0
    let firstFailure: unknown = null
    await Promise.allSettled(
      params.resultNodeIds.map(async (resultNodeId) => {
        try {
          const { taskId } = await params.submitTask(resultNodeId)
          await persistPendingImageTaskId(params.boardId, resultNodeId, taskId)
          const resolved = await params.resolveResult(taskId)
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

  function getDirectActionNodeById(nodeId: string): Node<FlowNodeData> | null {
    const target = options.nodes.value.find((node) => node.id === nodeId)
    if (!target || target.data?.kind !== 'image') return null
    return target as Node<FlowNodeData>
  }

  function getFusionNodeById(nodeId: string): Node<FlowNodeData> | null {
    const target = options.nodes.value.find((node) => node.id === nodeId)
    if (!target || target.data?.kind !== 'fusion') return null
    return target as Node<FlowNodeData>
  }

  function readImageSnapshot(nodeId: string): ImageDirectActionSource | null {
    const node = getDirectActionNodeById(nodeId)
    if (!node?.data?.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
    return resolveImageDirectActionSource(node, node.data.snapshotNode as ImageNodeItem, {
      requireUrl: true,
    })
  }

  async function runImageDirectAction(
    nodeId: string,
    action: DirectActionKey,
    runOptions?: RunImageDirectActionOptions,
  ) {
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    let resultNodeId: string | null = null
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    try {
      if (action === IMAGE_NODE_ACTION.hdUpscale) {
        await assertHdUpscaleSubmitReady()
      } else {
        await assertDirectActionSubmitReady(action)
      }
      resultNodeId = appendPendingImageResultNode(source, nodeId)
      setNodeRunning(resultNodeId, true)
      const taskId =
        action === IMAGE_NODE_ACTION.hdUpscale
          ? await submitHdUpscaleTask(source, loadImageSizeFromUrl, runOptions?.strength)
          : await submitDirectActionTask(source, action, loadImageSizeFromUrl, runOptions?.strength)
      await persistPendingImageTaskId(boardId, resultNodeId, taskId)
      const resolved = await resolveFirstTaskImageResult(taskId, source.ratio)
      if (!isBoardStillCurrent(boardId)) return
      updatePendingImageResultNode(resultNodeId, resolved)
      options.onSuccess('处理完成')
    } catch (error) {
      if (isTaskResultPendingError(error) && resultNodeId) {
        warnIfBoardCurrent(boardId, error.message)
        return
      }
      if (resultNodeId && isBoardStillCurrent(boardId)) {
        removePendingImageResultNode(resultNodeId)
      }
      const msg = error instanceof Error ? error.message : '执行失败'
      warnIfBoardCurrent(boardId, msg)
    } finally {
      if (resultNodeId) {
        setNodeRunning(resultNodeId, false)
      }
    }
  }

  async function runCanvasExtendAction(nodeId: string, intent: CanvasExtendRectIntent) {
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    const resolvedIntent = resolveCanvasExtendIntent(
      intent.frameRect,
      intent.originalImageRect,
    )
    const { extendPercent, targetRatio, hasExtension } = resolvedIntent
    if (!hasExtension) {
      options.onWarning('请先拖动扩图框')
      return
    }
    const targetWidth = resolveCanvasExtendResultWidth(
      source.width,
      resolvedIntent.frameRect,
      resolvedIntent.originalImageRect,
      targetRatio,
    )

    const normalizedPrompt = `${intent.prompt ?? ''}`.trim()
    const resolvedOutputCount = clampImageNodeOutputCount(intent.outputCount ?? 1)
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    let resultNodeIds: string[] = []
    try {
      await assertCanvasExtendSubmitReady()
      resultNodeIds = appendPendingImageResultNodes(source, nodeId, resolvedOutputCount, {
        targetRatio,
        targetWidth,
      })
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        boardId,
        submitTask: async () => {
          const taskId = await submitCanvasExtendTask({
            source,
            loadImageSize: loadImageSizeFromUrl,
            extendPercent,
            prompt: normalizedPrompt,
          })
          return { taskId }
        },
        resolveResult: async (taskId) => {
          return resolveFirstTaskImageResult(taskId, targetRatio)
        },
      })
    } catch (error) {
      removePendingImageResultNodesIfCurrent(boardId, resultNodeIds)
      const msg = error instanceof Error ? error.message : '扩图失败'
      warnIfBoardCurrent(boardId, msg)
    } finally {
      for (const resultNodeId of resultNodeIds) setNodeRunning(resultNodeId, false)
    }
  }

  async function runImageInpaintAction(nodeId: string, inpaint: RunImageInpaintActionOptions) {
    const normalizedPrompt = `${inpaint.prompt ?? ''}`.trim()
    if (!normalizedPrompt) {
      options.onWarning('请输入 Prompt')
      return
    }
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }
    const rawMaskDataUrl = `${inpaint.maskDataUrl ?? ''}`.trim()
    if (!rawMaskDataUrl) {
      options.onWarning('请先标记局部重绘区域')
      return
    }
    const referenceImageUrls = (inpaint.referenceImageUrls ?? [])
      .map((url) => `${url ?? ''}`.trim())
      .filter(Boolean)
      .slice(0, 4)
    const resolvedOutputCount = clampImageNodeOutputCount(inpaint.outputCount)
    let resultNodeIds: string[] = []
    let localInpaint: FlowLocalInpaintCropResult | null = null
    let temporaryMaskUrl = ''
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    try {
      await assertPromptImageSubmitReady()
      const normalizedMaskUrl = await ensureFlowMaskImageUrl(rawMaskDataUrl, { boardId })
      if (isDataUrl(rawMaskDataUrl)) temporaryMaskUrl = normalizedMaskUrl
      localInpaint = await prepareFlowLocalInpaint({
        originalImageUrl: source.url,
        maskUrl: normalizedMaskUrl,
        boardId,
      })
      resultNodeIds = appendPendingImageResultNodes(source, nodeId, resolvedOutputCount)
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        boardId,
        submitTask: async () => {
          const taskId = await submitImageInpaintTask({
            prompt: normalizedPrompt,
            croppedImageUrl: localInpaint!.croppedImageUrl,
            croppedMaskUrl: localInpaint!.croppedMaskUrl,
            referenceImageUrls,
            cropSize: localInpaint!.cropSize,
          })
          return { taskId }
        },
        resolveResult: async (taskId) => {
          const croppedResult = pickFirstValidImageResult(
            await waitExistingOrLiveTaskImageResults(taskId),
          )
          const croppedResultUrl = croppedResult?.url
          if (!croppedResultUrl) throw new Error('任务完成但未返回图片')
          const finalUrl = await composeFlowLocalInpaint({
            originalImageUrl: source.url,
            croppedResultUrl,
            croppedMaskUrl: localInpaint!.croppedMaskUrl,
            bbox: localInpaint!.bbox,
            boardId,
          })
          const ratio = await resolveTaskImageRatio(finalUrl, source.ratio, loadImageSizeFromUrl)
          return { url: finalUrl, ratio }
        },
      })
    } catch (error) {
      removePendingImageResultNodesIfCurrent(boardId, resultNodeIds)
      warnIfBoardCurrent(boardId, error instanceof Error ? error.message : '局部重绘失败')
    } finally {
      await cleanupFlowLocalInpaintAssets(localInpaint, boardId)
      if (temporaryMaskUrl && boardId) {
        await Promise.allSettled([uploadApi.recycle(temporaryMaskUrl, boardId)])
      }
      for (const resultNodeId of resultNodeIds) setNodeRunning(resultNodeId, false)
    }
  }

  async function runImagePromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio = 'auto',
    optionsOverride: { outputAnchorNodeId?: string } = {},
  ) {
    const normalizedPrompt = prompt.trim()
    if (!normalizedPrompt) {
      options.onWarning('请输入 Prompt')
      return
    }
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }
    await runImagePromptGenerationFromSource(
      source,
      nodeId,
      normalizedPrompt,
      outputCount,
      normalizePromptAspectRatio(aspectRatio),
      { outputAnchorNodeId: optionsOverride.outputAnchorNodeId },
    )
  }

  async function runImagePromptGenerationFromSource(
    source: ImageDirectActionSource,
    sourceNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    optionsOverride: { outputAnchorNodeId?: string } = {},
  ) {
    const normalizedPrompt = prompt.trim()
    if (!normalizedPrompt) {
      options.onWarning('请输入 Prompt')
      return
    }
    if (!source.url) {
      options.onWarning('图片缺少 URL')
      return
    }
    const resolvedOutputCount = clampImageNodeOutputCount(outputCount)
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    let resultNodeIds: string[] = []
    try {
      await assertPromptImageSubmitReady()
      resultNodeIds = appendPendingImageResultNodes(source, sourceNodeId, resolvedOutputCount, {
        outputAnchorNodeId: optionsOverride.outputAnchorNodeId,
      })
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        boardId,
        submitTask: async () => {
          const taskId = await submitPromptImageGenerationTask(
            source,
            normalizedPrompt,
            loadImageSizeFromUrl,
            aspectRatio,
          )
          return { taskId }
        },
        resolveResult: async (taskId) => {
          return resolveFirstTaskImageResult(taskId, source.ratio)
        },
      })
    } catch (error) {
      removePendingImageResultNodesIfCurrent(boardId, resultNodeIds)
      const msg = error instanceof Error ? error.message : '生成失败'
      warnIfBoardCurrent(boardId, msg)
    } finally {
      for (const resultNodeId of resultNodeIds) {
        setNodeRunning(resultNodeId, false)
      }
    }
  }

  async function runTextPromptGeneration(
    anchorNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: string,
  ) {
    const normalizedPrompt = prompt.trim()
    if (!normalizedPrompt) {
      options.onWarning('请输入 Prompt')
      return
    }
    const anchorNode = options.nodes.value.find((node) => node.id === anchorNodeId)
    const snapshot = anchorNode?.data?.snapshotNode as { width?: unknown } | undefined
    const source: ImageDirectActionSource = {
      url: '',
      ratio: 1,
      x: Number(anchorNode?.position?.x ?? 0),
      y: Number(anchorNode?.position?.y ?? 0),
      width: Number(snapshot?.width) > 0 ? Number(snapshot?.width) : 280,
    }
    const resolvedOutputCount = clampImageNodeOutputCount(outputCount)
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    let resultNodeIds: string[] = []
    try {
      resultNodeIds = appendPendingImageResultNodes(source, anchorNodeId, resolvedOutputCount, {
        outputAnchorNodeId: anchorNodeId,
      })
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        boardId,
        submitTask: async () => {
          const taskId = await submitPromptTextGenerationTask(
            normalizedPrompt,
            normalizePromptAspectRatio(aspectRatio),
          )
          return { taskId }
        },
        resolveResult: async (taskId) => {
          return resolveFirstTaskImageResult(taskId, 1)
        },
      })
    } catch (error) {
      removePendingImageResultNodesIfCurrent(boardId, resultNodeIds)
      const msg = error instanceof Error ? error.message : '生成失败'
      warnIfBoardCurrent(boardId, msg)
    } finally {
      for (const resultNodeId of resultNodeIds) {
        setNodeRunning(resultNodeId, false)
      }
    }
  }

  async function runFusionPromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: string,
  ) {
    const fusionNode = getFusionNodeById(nodeId)
    if (!fusionNode?.data?.snapshotNode || typeof fusionNode.data.snapshotNode !== 'object') {
      options.onWarning('Fusion 节点不存在')
      return
    }
    const snapshot = fusionNode.data.snapshotNode as FusionNodeItem
    const taskGraph = {
      nodes: options.nodes.value as Array<{
        id: string
        data?: { kind?: string; snapshotNode?: unknown }
      }>,
      edges: options.edges.value as Array<{
        source: string
        sourceHandle?: string | null
        target: string
        targetHandle?: string | null
      }>,
    }
    const inputImages = resolveConnectedImageInputs(taskGraph, nodeId, snapshot.inputImages, 4)
    if (inputImages.length < 2) {
      options.onWarning('Fusion 节点至少需要 2 张图片')
      return
    }
    if (inputImages.length > 4) {
      options.onWarning('Fusion 节点最多支持 4 张图片')
      return
    }
    const fusionInputs = inputImages
      .map((item) => ({ ...item, url: item.url.trim() }))
      .filter((item) => item.url)
      .slice(0, 4)
    const imageUrls = fusionInputs.map((item) => item.url)
    if (imageUrls.length < 2) {
      options.onWarning('Fusion 输入图片无效')
      return
    }
    const firstInput = fusionInputs[0]
    const normalizedPrompt = `${prompt ?? ''}`.trim()
    const normalizedAspectRatio = normalizePromptAspectRatio(aspectRatio)
    const promptInput = resolveConnectedInput(taskGraph, nodeId, 'prompt')
    const connectedPromptText = promptInput ? resolveConnectedPromptText(taskGraph, nodeId) : ''
    const promptText = normalizedPrompt || connectedPromptText
    const taskPrompt = promptText
      ? `${EXECUTION_FUSION_META.promptPrefix} ${promptText}`
      : EXECUTION_FUSION_META.promptPrefix
    const resolvedOutputCount = clampImageNodeOutputCount(outputCount)
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    const fusionInputWeights = computeFusionBlendWeights(
      imageUrls.length,
      snapshot.blendPoint,
      snapshot.width,
    ).slice(0, imageUrls.length)
    let resultNodeIds: string[] = []
    try {
      await assertTaskSubmitReady(
        buildExecutionTaskRequest({
          prompt: 'submit readiness',
          feature: EXECUTION_FUSION_META.feature,
          imageUrls,
          params: {
            fusion: {
              mode: 'weighted_reference',
              input_weights: fusionInputWeights,
            },
          },
        }),
        '提交任务失败',
      )
      const imageSizes = await Promise.all(
        fusionInputs.map((input) => loadImageSizeFromUrl(input.url)),
      )
      const outputSize = resolveFusionOutputSize({
        inputImages: fusionInputs,
        imageSizes,
        blendPoint: snapshot.blendPoint,
        stageWidth: snapshot.width,
        aspectRatio: normalizedAspectRatio,
      })
      const imageWeights = outputSize.weights.slice(0, imageUrls.length)
      const targetRatio = outputSize.targetRatio
      const sizeSource = outputSize.sourceSize ?? outputSize.targetSize
      const source: ImageDirectActionSource = {
        url: firstInput.url,
        ratio: targetRatio,
        x: Number(fusionNode.position?.x ?? 0),
        y: Number(fusionNode.position?.y ?? 0),
        width: Number(snapshot.width) > 0 ? Number(snapshot.width) : 280,
      }
      resultNodeIds = appendPendingImageResultNodes(source, nodeId, resolvedOutputCount, {
        outputAnchorNodeId: nodeId,
        targetRatio,
        targetWidth: resolveImageNodeFrameSize(targetRatio, true).width,
      })
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        boardId,
        submitTask: async () => {
          const submitRes = await taskApi.submit(
            buildExecutionTaskRequest({
              prompt: taskPrompt,
              feature: EXECUTION_FUSION_META.feature,
              imageUrls,
              params: {
                ...buildExecutionSizeParams(sizeSource, outputSize.targetSize),
                ...buildPromptAspectRatioTaskParams(normalizedAspectRatio),
                fusion: {
                  mode: 'weighted_reference',
                  input_weights: imageWeights,
                },
              },
            }),
          )
          if (submitRes.code !== 0 || !submitRes.data?.task_id) {
            throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Fusion 任务失败'))
          }
          return { taskId: submitRes.data.task_id }
        },
        resolveResult: async (taskId) => {
          return resolveFirstTaskImageResult(taskId, targetRatio)
        },
      })
    } catch (error) {
      removePendingImageResultNodesIfCurrent(boardId, resultNodeIds)
      const msg =
        error instanceof Error
          ? sanitizeApiErrorMessage(error.message, 'Fusion 生成失败')
          : 'Fusion 生成失败'
      warnIfBoardCurrent(boardId, msg)
    } finally {
      for (const resultNodeId of resultNodeIds) setNodeRunning(resultNodeId, false)
    }
  }

  function isNodeDirectActionRunning(nodeId: string): boolean {
    return runningNodeIds.value.has(nodeId)
  }

  return {
    isNodeDirectActionRunning,
    reattachPendingImageResults,
    runImageDirectAction,
    runCanvasExtendAction,
    runImagePromptGeneration,
    runImagePromptGenerationFromSource,
    runImageInpaintAction,
    runFusionPromptGeneration,
    runTextPromptGeneration,
  }
}
