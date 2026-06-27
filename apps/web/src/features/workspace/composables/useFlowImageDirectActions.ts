import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import {
  clampImageNodeOutputCount,
  getCanvasNodeImageInputLimit,
  normalizePromptAspectRatio,
  resolveImageNodeFrameSize,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptAspectRatio,
} from '../../canvas-board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import { uploadApi } from '../../../shared/api/upload'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { EXECUTION_FUSION_META } from '../config/executionContract'
import { resolveFusionOutputSize } from '../utils/fusionOutputSizing'
import {
  cleanupFlowLocalInpaintAssets,
  composeFlowLocalInpaint,
  prepareFlowLocalInpaint,
  type FlowLocalInpaintCropResult,
} from '../services/flowLocalInpaint'
import { ensureFlowMaskImageUrl } from '../services/flowMediaUpload'
import { isDataUrl } from '../services/flowMediaIO'
import { waitExistingOrLiveTaskImageResults } from '../services/flowImageDirectActionExecutor'
import { loadImageSizeFromUrl } from '../utils/imageSize'
import { usePendingImageResultNodes } from './usePendingImageResultNodes'
import {
  pickFirstValidImageResult,
  resolveTaskImageRatio,
} from '../utils/flowImageDirectActionTaskResult'
import {
  resolveImageDirectActionSource,
  type ImageDirectActionSource,
} from '../utils/flowImageDirectActionSource'
import type { ImageTaskAction } from '../config/imageActionRegistry'
import type { FlowTransactions } from './useFlowTransactions'
import { resolveCanvasExtendIntent, type CanvasExtendRect } from '../utils/canvasExtendGeometry'
import {
  resolveConnectedImageInputs,
  resolveConnectedInput,
  resolveConnectedPromptText,
} from '../services/flowTaskExecutorUtils'
import { readCurrentBoardId } from '../utils/currentBoardId'
import {
  assertImageInpaintTaskSubmitReady,
  buildCanvasExtendTaskCommand,
  buildFusionTaskCommand,
  buildImageDirectTaskCommand,
  buildImageInpaintTaskCommand,
  buildPromptImageTaskCommand,
  buildTextPromptTaskCommand,
} from '../services/flowImageDirectActionCommands'
import { useFlowImageDirectActionRunner } from './useFlowImageDirectActionRunner'

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
  const pendingImageResultNodes = usePendingImageResultNodes(options)
  const {
    isNodeDirectActionRunning,
    reattachPendingImageResults,
    resolveFirstTaskImageResult,
    runPendingImageBatchWorkflow,
    runPendingImageGeneration,
  } = useFlowImageDirectActionRunner({
    nodes: options.nodes,
    pendingImageResultNodes,
    flushAutosave: options.flushAutosave,
    getCurrentBoardId: options.getCurrentBoardId,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
  })

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
    action: ImageTaskAction,
    runOptions?: RunImageDirectActionOptions,
  ) {
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    const command = buildImageDirectTaskCommand({
      source,
      action,
      loadImageSize: loadImageSizeFromUrl,
      strength: runOptions?.strength,
    })
    if (!command) {
      options.onWarning('当前图片工具不可执行')
      return
    }
    await runPendingImageGeneration({
      source,
      sourceNodeId: nodeId,
      boardId,
      assertReady: command.assertReady,
      submitTask: () => command.submit(),
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, source.ratio),
      successMessage: '处理完成',
      fallbackErrorMessage: '执行失败',
    })
  }

  async function runCanvasExtendAction(nodeId: string, intent: CanvasExtendRectIntent) {
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    const resolvedIntent = resolveCanvasExtendIntent(intent.frameRect, intent.originalImageRect)
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
    const command = buildCanvasExtendTaskCommand({
      source,
      loadImageSize: loadImageSizeFromUrl,
      extendPercent,
      prompt: normalizedPrompt,
    })
    await runPendingImageBatchWorkflow({
      source,
      sourceNodeId: nodeId,
      boardId,
      outputCount: resolvedOutputCount,
      resultOptions: {
        targetRatio,
        targetWidth,
      },
      assertReady: command.assertReady,
      submitTask: () => command.submit(),
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, targetRatio),
      fallbackErrorMessage: '扩图失败',
    })
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
    const referenceImageLimit = getCanvasNodeImageInputLimit('prompt')
    const referenceImageUrls = (inpaint.referenceImageUrls ?? [])
      .map((url) => `${url ?? ''}`.trim())
      .filter(Boolean)
      .slice(0, referenceImageLimit)
    const resolvedOutputCount = clampImageNodeOutputCount(inpaint.outputCount)
    let localInpaint: FlowLocalInpaintCropResult | null = null
    let temporaryMaskUrl = ''
    let command: ReturnType<typeof buildImageInpaintTaskCommand> | null = null
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    await runPendingImageBatchWorkflow({
      source,
      sourceNodeId: nodeId,
      boardId,
      outputCount: resolvedOutputCount,
      preflight: assertImageInpaintTaskSubmitReady,
      prepare: async () => {
        const normalizedMaskUrl = await ensureFlowMaskImageUrl(rawMaskDataUrl, { boardId })
        if (isDataUrl(rawMaskDataUrl)) temporaryMaskUrl = normalizedMaskUrl
        localInpaint = await prepareFlowLocalInpaint({
          originalImageUrl: source.url,
          maskUrl: normalizedMaskUrl,
          boardId,
        })
        command = buildImageInpaintTaskCommand({
          prompt: normalizedPrompt,
          croppedImageUrl: localInpaint.croppedImageUrl,
          croppedMaskUrl: localInpaint.croppedMaskUrl,
          referenceImageUrls,
          cropSize: localInpaint.cropSize,
        })
      },
      submitTask: () => {
        if (!command) throw new Error('局部重绘任务未准备完成')
        return command.submit()
      },
      resolveResult: async (taskId) => {
        if (!localInpaint) throw new Error('局部重绘任务未准备完成')
        const croppedResult = pickFirstValidImageResult(
          await waitExistingOrLiveTaskImageResults(taskId),
        )
        const croppedResultUrl = croppedResult?.url
        if (!croppedResultUrl) throw new Error('任务完成但未返回图片')
        const finalUrl = await composeFlowLocalInpaint({
          originalImageUrl: source.url,
          croppedResultUrl,
          croppedMaskUrl: localInpaint.croppedMaskUrl,
          bbox: localInpaint.bbox,
          boardId,
        })
        const ratio = await resolveTaskImageRatio(finalUrl, source.ratio, loadImageSizeFromUrl)
        return { url: finalUrl, ratio }
      },
      cleanup: async () => {
        await cleanupFlowLocalInpaintAssets(localInpaint, boardId)
        if (temporaryMaskUrl && boardId) {
          await Promise.allSettled([uploadApi.recycle(temporaryMaskUrl, boardId)])
        }
      },
      fallbackErrorMessage: '局部重绘失败',
    })
  }

  async function runImagePromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio = 'auto',
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
    const command = buildPromptImageTaskCommand({
      source,
      prompt: normalizedPrompt,
      loadImageSize: loadImageSizeFromUrl,
      aspectRatio,
    })
    await runPendingImageBatchWorkflow({
      source,
      sourceNodeId,
      boardId,
      outputCount: resolvedOutputCount,
      resultOptions: {
        outputAnchorNodeId: optionsOverride.outputAnchorNodeId,
      },
      assertReady: command.assertReady,
      submitTask: () => command.submit(),
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, source.ratio),
      fallbackErrorMessage: '生成失败',
    })
  }

  async function runTextPromptGeneration(
    anchorNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
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
    const command = buildTextPromptTaskCommand({
      prompt: normalizedPrompt,
      aspectRatio: normalizePromptAspectRatio(aspectRatio),
    })
    await runPendingImageBatchWorkflow({
      source,
      sourceNodeId: anchorNodeId,
      boardId,
      outputCount: resolvedOutputCount,
      resultOptions: {
        outputAnchorNodeId: anchorNodeId,
      },
      assertReady: command.assertReady,
      submitTask: () => command.submit(),
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, 1),
      fallbackErrorMessage: '生成失败',
    })
  }

  async function runFusionPromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
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
    const fusionInputLimit = getCanvasNodeImageInputLimit('fusion')
    const inputImages = resolveConnectedImageInputs(
      taskGraph,
      nodeId,
      snapshot.inputImages,
      fusionInputLimit,
    )
    if (inputImages.length < 2) {
      options.onWarning('Fusion 节点至少需要 2 张图片')
      return
    }
    const fusionInputs = inputImages
      .map((item) => ({ ...item, url: item.url.trim() }))
      .filter((item) => item.url)
      .slice(0, fusionInputLimit)
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
    const source: ImageDirectActionSource = {
      url: firstInput.url,
      ratio: firstInput.ratio,
      x: Number(fusionNode.position?.x ?? 0),
      y: Number(fusionNode.position?.y ?? 0),
      width: Number(snapshot.width) > 0 ? Number(snapshot.width) : 280,
    }
    let targetRatio = firstInput.ratio
    let command: ReturnType<typeof buildFusionTaskCommand> | null = null
    await runPendingImageBatchWorkflow({
      source,
      sourceNodeId: nodeId,
      boardId,
      outputCount: resolvedOutputCount,
      prepare: async () => {
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
        targetRatio = outputSize.targetRatio
        command = buildFusionTaskCommand({
          prompt: taskPrompt,
          imageUrls,
          inputWeights: imageWeights,
          sizeSource: outputSize.sourceSize ?? outputSize.targetSize,
          targetSize: outputSize.targetSize,
          aspectRatio: normalizedAspectRatio,
        })
        return {
          source: {
            ...source,
            ratio: targetRatio,
          },
          resultOptions: {
            outputAnchorNodeId: nodeId,
            targetRatio,
            targetWidth: resolveImageNodeFrameSize(targetRatio, true).width,
          },
        }
      },
      assertReady: () => {
        if (!command) throw new Error('Fusion 任务未准备完成')
        return command.assertReady()
      },
      submitTask: () => {
        if (!command) throw new Error('Fusion 任务未准备完成')
        return command.submit()
      },
      resolveResult: (taskId) => resolveFirstTaskImageResult(taskId, targetRatio),
      fallbackErrorMessage: 'Fusion 生成失败',
      formatErrorMessage: (error) => sanitizeApiErrorMessage(error.message, 'Fusion 生成失败'),
    })
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
