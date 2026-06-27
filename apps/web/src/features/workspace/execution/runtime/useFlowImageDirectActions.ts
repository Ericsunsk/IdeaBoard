import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { SubmitTaskData } from '../../../../shared/api/task'
import {
  clampImageNodeOutputCount,
  getCanvasNodeImageInputLimit,
  normalizePromptAspectRatio,
  resolveImageNodeFrameSize,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptAspectRatio,
} from '../../../canvas-board/domain'
import type { FlowNodeData } from '../../types/flowTypes'
import { uploadApi } from '../../../../shared/api/upload'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import { EXECUTION_FUSION_META } from '../contracts/executionContract'
import { resolveFusionOutputSize } from '../commands/fusionOutputSizing'
import {
  cleanupFlowLocalInpaintAssets,
  composeFlowLocalInpaint,
  prepareFlowLocalInpaint,
  type FlowLocalInpaintCropResult,
} from '../media/flowLocalInpaint'
import { ensureFlowMaskImageUrl } from '../media/flowMediaUpload'
import { isDataUrl } from '../../services/flowMediaIO'
import {
  assertTaskRequestReady,
  submitTaskRequest,
  waitExistingOrLiveTaskImageResults,
} from './flowImageDirectActionExecutor'
import { loadImageSizeFromUrl } from '../media/imageSize'
import { usePendingImageResultNodes } from '../results/usePendingImageResultNodes'
import {
  pickFirstValidImageResult,
  resolveTaskImageRatio,
} from '../results/flowImageDirectActionTaskResult'
import {
  resolveImageDirectActionSource,
  type ImageDirectActionSource,
} from '../media/flowImageDirectActionSource'
import type { FlowTransactions } from '../../composables/useFlowTransactions'
import { resolveCanvasExtendIntent, type CanvasExtendRect } from '../../utils/canvasExtendGeometry'
import {
  resolveConnectedImageInputs,
  resolveConnectedInput,
  resolveConnectedPromptText,
} from '../media/flowExecutionInputs'
import { readCurrentBoardId } from '../../utils/currentBoardId'
import {
  buildCanvasExtendReadinessRequest,
  buildCanvasExtendTaskRequest,
  buildFusionTaskRequest,
  buildImageInpaintTaskRequest,
  buildImageActionReadinessRequest,
  buildImageTaskRequest,
  buildPromptImageReadinessRequest,
  buildPromptImageTaskRequest,
  buildTextPromptTaskRequest,
} from '../commands/flowImageTaskRequests'
import { useFlowImageDirectActionRunner } from './useFlowImageDirectActionRunner'
import type { ImageTaskAction } from '../contracts/imageActionExecution'

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

type TaskSubmissionCallbacks = {
  assertReady?: () => Promise<void>
  submitTask: () => Promise<{ taskId: string }>
}

function createTaskSubmissionCallbacks(options: {
  readinessRequest?: () => SubmitTaskData
  buildRequest: () => SubmitTaskData | Promise<SubmitTaskData>
  fallbackMessage?: string
}): TaskSubmissionCallbacks {
  const { readinessRequest } = options
  const assertReady = readinessRequest
    ? () => assertTaskRequestReady(readinessRequest(), options.fallbackMessage)
    : undefined
  return {
    assertReady,
    submitTask: async () =>
      submitTaskRequest(await options.buildRequest(), options.fallbackMessage),
  }
}

function requirePreparedTaskRequest(
  request: SubmitTaskData | null,
  fallbackMessage: string,
): SubmitTaskData {
  if (!request) throw new Error(fallbackMessage)
  return request
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
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: () => buildImageActionReadinessRequest(action),
      buildRequest: async () =>
        buildImageTaskRequest({
          source,
          action,
          imageSize: await loadImageSizeFromUrl(source.url),
          strength: runOptions?.strength,
        }),
    })
    await runPendingImageGeneration({
      source,
      sourceNodeId: nodeId,
      boardId,
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
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
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: buildCanvasExtendReadinessRequest,
      buildRequest: async () =>
        buildCanvasExtendTaskRequest({
          source,
          imageSize: await loadImageSizeFromUrl(source.url),
          extendPercent,
          prompt: normalizedPrompt,
        }),
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
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
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
    let request: ReturnType<typeof buildImageInpaintTaskRequest> | null = null
    const boardId = readCurrentBoardId(options.getCurrentBoardId)
    const taskSubmission = createTaskSubmissionCallbacks({
      buildRequest: () => requirePreparedTaskRequest(request, '局部重绘任务未准备完成'),
      fallbackMessage: '提交局部重绘任务失败',
    })
    await runPendingImageBatchWorkflow({
      source,
      sourceNodeId: nodeId,
      boardId,
      outputCount: resolvedOutputCount,
      preflight: () => assertTaskRequestReady(buildPromptImageReadinessRequest()),
      prepare: async () => {
        const normalizedMaskUrl = await ensureFlowMaskImageUrl(rawMaskDataUrl, { boardId })
        if (isDataUrl(rawMaskDataUrl)) temporaryMaskUrl = normalizedMaskUrl
        localInpaint = await prepareFlowLocalInpaint({
          originalImageUrl: source.url,
          maskUrl: normalizedMaskUrl,
          boardId,
        })
        request = buildImageInpaintTaskRequest({
          prompt: normalizedPrompt,
          croppedImageUrl: localInpaint.croppedImageUrl,
          croppedMaskUrl: localInpaint.croppedMaskUrl,
          referenceImageUrls,
          cropSize: localInpaint.cropSize,
        })
      },
      submitTask: taskSubmission.submitTask,
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
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: buildPromptImageReadinessRequest,
      buildRequest: async () =>
        buildPromptImageTaskRequest({
          source,
          prompt: normalizedPrompt,
          imageSize: await loadImageSizeFromUrl(source.url),
          aspectRatio,
        }),
    })
    await runPendingImageBatchWorkflow({
      source,
      sourceNodeId,
      boardId,
      outputCount: resolvedOutputCount,
      resultOptions: {
        outputAnchorNodeId: optionsOverride.outputAnchorNodeId,
      },
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
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
    const taskSubmission = createTaskSubmissionCallbacks({
      buildRequest: () =>
        buildTextPromptTaskRequest({
          prompt: normalizedPrompt,
          aspectRatio: normalizePromptAspectRatio(aspectRatio),
        }),
    })
    await runPendingImageBatchWorkflow({
      source,
      sourceNodeId: anchorNodeId,
      boardId,
      outputCount: resolvedOutputCount,
      resultOptions: {
        outputAnchorNodeId: anchorNodeId,
      },
      submitTask: taskSubmission.submitTask,
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
    let request: ReturnType<typeof buildFusionTaskRequest> | null = null
    const taskSubmission = createTaskSubmissionCallbacks({
      readinessRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
      buildRequest: () => requirePreparedTaskRequest(request, 'Fusion 任务未准备完成'),
      fallbackMessage: '提交 Fusion 任务失败',
    })
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
        request = buildFusionTaskRequest({
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
      assertReady: taskSubmission.assertReady,
      submitTask: taskSubmission.submitTask,
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
