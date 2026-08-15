import {
  getCanvasNodeImageInputLimit,
  normalizePromptReferenceImageUrls,
  normalizePromptAspectRatio,
  resolveImageNodeFrameSize,
  type FusionBlendPoint,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptAspectRatio,
  type PromptNodeItem,
} from '../../../canvas-board/domain'
import { parseRequestedOutputCount } from '../../../../shared/domain/generationContract'
import { normalizeOutputResolution, type OutputResolution } from '../../../../shared/domain/generationContract'
import { EXECUTION_FUSION_META } from '../contracts/executionContract'
import type { ImageTaskAction } from '../contracts/imageActionExecution'
import {
  resolveCanvasExtendIntent,
  type CanvasExtendPercent,
  type CanvasExtendRect,
} from '../../utils/canvasExtendGeometry'
import {
  resolveImageNodeLayoutAnchor,
  type ImageNodeLayoutAnchor,
} from '../../utils/flowImageNodeSelectors'
import {
  resolveConnectedImageInputs,
  resolveConnectedInput,
  resolveConnectedPromptText,
  type FlowExecutionGraphOptions,
  type FlowGraphNode,
  type ResolvedFlowInputImage,
} from '../../utils/flowNodeSourceResolution'
import { resolveFusionOutputSize, type FusionOutputImageSize } from './fusionOutputSizing'
import type {
  ImageExecutionInput,
  ImageExecutionResultOptions,
  ImageResultAnchor,
} from './imageExecutionResultTypes'

export type { ImageExecutionInput, ImageExecutionResultOptions, ImageResultAnchor }

export type ImageActionExecutionPlan = {
  kind: 'image-action'
  sourceNodeId: string
  source: ImageExecutionInput
  action: ImageTaskAction
  prompt: string
  outputCount: number
  strength?: number
  outputResolution?: OutputResolution
  resultOptions: ImageExecutionResultOptions
}

export type CanvasExtendExecutionPlan = {
  kind: 'canvas-extend'
  sourceNodeId: string
  source: ImageExecutionInput
  prompt: string
  outputCount: number
  extendPercent: CanvasExtendPercent
  targetRatio: number
  targetWidth: number
  outputResolution?: OutputResolution
  resultOptions: ImageExecutionResultOptions
}

export type ImageInpaintExecutionPlan = {
  kind: 'image-inpaint'
  sourceNodeId: string
  source: ImageExecutionInput
  prompt: string
  rawMaskDataUrl: string
  outputCount: number
  referenceImageUrls: string[]
  outputResolution?: OutputResolution
  resultOptions: ImageExecutionResultOptions
}

export type PromptGenerationExecutionPlan = {
  kind: 'prompt-generation'
  sourceNodeId: string
  source: ImageExecutionInput
  referenceImages: ImageExecutionInput[]
  prompt: string
  outputCount: number
  aspectRatio: PromptAspectRatio
  outputResolution?: OutputResolution
  resultOptions: ImageExecutionResultOptions
}

export type FusionExecutionInputImage = ResolvedFlowInputImage

export type FusionExecutionPlan = {
  kind: 'fusion'
  sourceNodeId: string
  source: ImageExecutionInput
  taskPrompt: string
  outputCount: number
  aspectRatio: PromptAspectRatio
  inputImages: FusionExecutionInputImage[]
  imageUrls: string[]
  blendPoint: FusionBlendPoint
  stageWidth: number
  outputResolution?: OutputResolution
  resultOptions: ImageExecutionResultOptions
}

export type PreparedFusionExecutionPlan = FusionExecutionPlan & {
  source: ImageExecutionInput
  inputWeights: number[]
  sizeSource: FusionOutputImageSize | null
  targetSize: FusionOutputImageSize | null
  targetRatio: number
  resultOptions: ImageExecutionResultOptions
}

export type ImageExecutionPlan =
  | ImageActionExecutionPlan
  | CanvasExtendExecutionPlan
  | ImageInpaintExecutionPlan
  | PromptGenerationExecutionPlan
  | FusionExecutionPlan

export type ActivatedImageExecutionPlan = ImageExecutionPlan & {
  activationId: string
}

export class ImageExecutionPlanError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'ImageExecutionPlanError'
  }
}

export function isImageExecutionPlanError(error: unknown): error is ImageExecutionPlanError {
  return error instanceof ImageExecutionPlanError
}

function failPlanning(message: string): never {
  throw new ImageExecutionPlanError(message)
}

function resolveNodeOutputResolution(
  graph: FlowExecutionGraphOptions,
  nodeId: string,
): OutputResolution {
  const node = graph.nodes.find((item) => item.id === nodeId)
  const snapshot = node?.data?.snapshotNode
  return snapshot && typeof snapshot === 'object' && 'outputResolution' in snapshot
    ? normalizeOutputResolution((snapshot as { outputResolution?: unknown }).outputResolution)
    : '1k'
}

function requireRequestedOutputCount(value: unknown): number {
  const outputCount = parseRequestedOutputCount(value)
  if (outputCount === null) failPlanning('生成数量无效')
  return outputCount
}

function normalizeExecutionRatio(value: unknown): number {
  const ratio = Number(value)
  return Number.isFinite(ratio) && ratio > 0 ? ratio : 1
}

function toImageExecutionInput(
  anchor: Pick<ImageNodeLayoutAnchor, 'url' | 'ratio'>,
): ImageExecutionInput {
  return {
    url: anchor.url.trim(),
    ratio: normalizeExecutionRatio(anchor.ratio),
  }
}

function toImageResultAnchor(
  anchor: Pick<ImageNodeLayoutAnchor, 'x' | 'y' | 'width'>,
  outputAnchorNodeId?: string,
): ImageResultAnchor {
  return {
    x: Number(anchor.x),
    y: Number(anchor.y),
    width: Math.max(1, Math.round(Number(anchor.width))),
    ...(outputAnchorNodeId ? { outputAnchorNodeId } : {}),
  }
}

function withOutputAnchorNodeId(
  anchor: ImageResultAnchor,
  outputAnchorNodeId?: string,
): ImageResultAnchor {
  const normalizedAnchorNodeId = `${outputAnchorNodeId ?? anchor.outputAnchorNodeId ?? ''}`.trim()
  return {
    ...anchor,
    ...(normalizedAnchorNodeId ? { outputAnchorNodeId: normalizedAnchorNodeId } : {}),
  }
}

function resolveGraphNodeResultAnchor(
  graph: FlowExecutionGraphOptions,
  nodeId: string,
  fallbackWidth = 280,
): ImageResultAnchor {
  const node = graph.nodes.find((item) => item.id === nodeId)
  return {
    x: Number(node?.position?.x ?? 0),
    y: Number(node?.position?.y ?? 0),
    width: readNodeWidth(node, fallbackWidth),
    outputAnchorNodeId: nodeId,
  }
}

type ResolvedImageExecutionImage = {
  input: ImageExecutionInput
  resultAnchor: ImageResultAnchor
}

function resolveImageExecutionImage(
  node: Pick<FlowGraphNode, 'position'>,
  imageNode: ImageNodeItem,
  options: { requireUrl?: boolean } = {},
): ResolvedImageExecutionImage | null {
  const anchor = resolveImageNodeLayoutAnchor(node, imageNode, options)
  if (!anchor) return null
  return {
    input: toImageExecutionInput(anchor),
    resultAnchor: toImageResultAnchor(anchor),
  }
}

function requireImageExecutionImage(
  graph: FlowExecutionGraphOptions,
  nodeId: string,
): ResolvedImageExecutionImage {
  const node = graph.nodes.find((item) => item.id === nodeId)
  if (!node || node.data?.kind !== 'image') {
    failPlanning('图片节点不存在或缺少 URL')
  }
  const snapshot = node.data.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') {
    failPlanning('图片节点不存在或缺少 URL')
  }
  const source = resolveImageExecutionImage(node, snapshot as ImageNodeItem, {
    requireUrl: true,
  })
  if (!source?.input.url) failPlanning('图片节点不存在或缺少 URL')
  return source
}

function readNodeWidth(node: FlowGraphNode | undefined, fallbackWidth: number): number {
  const snapshot = node?.data?.snapshotNode as { width?: unknown } | undefined
  const width = Number(snapshot?.width)
  return Number.isFinite(width) && width > 0 ? width : fallbackWidth
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

export function prepareFusionExecutionPlan(
  plan: FusionExecutionPlan,
  imageSizes: Array<FusionOutputImageSize | null>,
): PreparedFusionExecutionPlan {
  const outputSize = resolveFusionOutputSize({
    inputImages: plan.inputImages,
    imageSizes,
    blendPoint: plan.blendPoint,
    stageWidth: plan.stageWidth,
    aspectRatio: plan.aspectRatio,
  })
  const targetRatio = outputSize.targetRatio
  const targetWidth = resolveImageNodeFrameSize(targetRatio, true).width
  return {
    ...plan,
    source: {
      url: plan.source.url,
      ratio: targetRatio,
    },
    inputWeights: outputSize.weights.slice(0, plan.imageUrls.length),
    sizeSource: outputSize.sourceSize ?? outputSize.targetSize,
    targetSize: outputSize.targetSize,
    targetRatio,
    resultOptions: {
      ...plan.resultOptions,
      ...(plan.resultOptions.anchor
        ? { anchor: withOutputAnchorNodeId(plan.resultOptions.anchor, plan.sourceNodeId) }
        : {}),
      targetRatio,
      targetWidth,
    },
  }
}

export function createImageExecutionPlanner(readGraph: () => FlowExecutionGraphOptions) {
  function planImageDirectAction(
    nodeId: string,
    action: ImageTaskAction,
    options: { prompt?: string; outputCount?: number; strength?: number } = {},
  ): ImageActionExecutionPlan {
    const graph = readGraph()
    const source = requireImageExecutionImage(graph, nodeId)
    return {
      kind: 'image-action',
      sourceNodeId: nodeId,
      source: source.input,
      action,
      prompt: `${options.prompt ?? ''}`.trim(),
      outputCount: requireRequestedOutputCount(options.outputCount ?? 1),
      strength: options.strength,
      outputResolution: resolveNodeOutputResolution(graph, nodeId),
      resultOptions: {
        anchor: withOutputAnchorNodeId(source.resultAnchor, nodeId),
      },
    }
  }

  function planCanvasExtendAction(
    nodeId: string,
    intent: {
      frameRect: CanvasExtendRect
      originalImageRect: CanvasExtendRect
      prompt?: string
      outputCount?: number
    },
  ): CanvasExtendExecutionPlan {
    const graph = readGraph()
    const source = requireImageExecutionImage(graph, nodeId)
    const resolvedIntent = resolveCanvasExtendIntent(intent.frameRect, intent.originalImageRect)
    if (!resolvedIntent.hasExtension) failPlanning('请先拖动扩图框')
    const { extendPercent, targetRatio } = resolvedIntent
    const targetWidth = resolveCanvasExtendResultWidth(
      source.resultAnchor.width,
      resolvedIntent.frameRect,
      resolvedIntent.originalImageRect,
      targetRatio,
    )
    return {
      kind: 'canvas-extend',
      sourceNodeId: nodeId,
      source: source.input,
      prompt: `${intent.prompt ?? ''}`.trim(),
      outputCount: requireRequestedOutputCount(intent.outputCount ?? 1),
      extendPercent,
      targetRatio,
      targetWidth,
      outputResolution: resolveNodeOutputResolution(graph, nodeId),
      resultOptions: {
        anchor: withOutputAnchorNodeId(source.resultAnchor, nodeId),
        targetRatio,
        targetWidth,
      },
    }
  }

  function planImageInpaintAction(
    nodeId: string,
    inpaint: {
      prompt: string
      maskDataUrl: string
      outputCount: number
      referenceImageUrls?: string[]
    },
  ): ImageInpaintExecutionPlan {
    const prompt = `${inpaint.prompt ?? ''}`.trim()
    if (!prompt) failPlanning('请输入 Prompt')
    const graph = readGraph()
    const source = requireImageExecutionImage(graph, nodeId)
    const rawMaskDataUrl = `${inpaint.maskDataUrl ?? ''}`.trim()
    if (!rawMaskDataUrl) failPlanning('请先标记局部重绘区域')
    return {
      kind: 'image-inpaint',
      sourceNodeId: nodeId,
      source: source.input,
      prompt,
      rawMaskDataUrl,
      outputCount: requireRequestedOutputCount(inpaint.outputCount),
      referenceImageUrls: normalizePromptReferenceImageUrls(inpaint.referenceImageUrls),
      outputResolution: resolveNodeOutputResolution(graph, nodeId),
      resultOptions: {
        anchor: withOutputAnchorNodeId(source.resultAnchor, nodeId),
      },
    }
  }

  function planPromptGeneration(
    anchorNodeId: string,
    referenceImages: readonly ImageExecutionInput[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ): PromptGenerationExecutionPlan {
    const normalizedPrompt = `${prompt ?? ''}`.trim()
    if (referenceImages.length > 4) failPlanning('Prompt 节点最多支持 4 张参考图')
    const normalizedReferences = referenceImages.map((image) => ({
      url: `${image.url ?? ''}`.trim(),
      ratio: normalizeExecutionRatio(image.ratio),
    }))
    if (normalizedReferences.some((image) => !image.url)) failPlanning('参考图缺少 URL')
    const graph = readGraph()
    const promptNode = graph.nodes.find((node) => node.id === anchorNodeId)
    const outputResolution =
      promptNode?.data?.kind === 'prompt' && promptNode.data.snapshotNode
        ? normalizeOutputResolution((promptNode.data.snapshotNode as PromptNodeItem).outputResolution)
        : '1k'
    const source = normalizedReferences[0] ?? { url: '', ratio: 1 }
    return {
      kind: 'prompt-generation',
      sourceNodeId: anchorNodeId,
      source,
      referenceImages: normalizedReferences,
      prompt: normalizedPrompt,
      outputCount: requireRequestedOutputCount(outputCount),
      aspectRatio: normalizePromptAspectRatio(aspectRatio),
      outputResolution,
      resultOptions: {
        anchor: resolveGraphNodeResultAnchor(graph, anchorNodeId),
      },
    }
  }

  function planFusionPromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ): FusionExecutionPlan {
    const graph = readGraph()
    const fusionNode = graph.nodes.find((node) => node.id === nodeId)
    if (
      !fusionNode ||
      fusionNode.data?.kind !== 'fusion' ||
      !fusionNode.data.snapshotNode ||
      typeof fusionNode.data.snapshotNode !== 'object'
    ) {
      failPlanning('Fusion 节点不存在')
    }
    const snapshot = fusionNode.data.snapshotNode as FusionNodeItem
    const fusionInputLimit = getCanvasNodeImageInputLimit('fusion')
    const inputImages = resolveConnectedImageInputs(
      graph,
      nodeId,
      snapshot.inputImages,
      fusionInputLimit,
    )
    if (inputImages.length < 2) failPlanning('Fusion 节点至少需要 2 张图片')
    const fusionInputs = inputImages
      .map((item) => ({ ...item, url: item.url.trim() }))
      .filter((item) => item.url)
      .slice(0, fusionInputLimit)
    const imageUrls = fusionInputs.map((item) => item.url)
    if (imageUrls.length < 2) failPlanning('Fusion 输入图片无效')

    const normalizedPrompt = `${prompt ?? ''}`.trim()
    const normalizedAspectRatio = normalizePromptAspectRatio(aspectRatio)
    const promptInput = resolveConnectedInput(graph, nodeId, 'prompt')
    const connectedPromptText = promptInput ? resolveConnectedPromptText(graph, nodeId) : ''
    const promptText = normalizedPrompt || connectedPromptText
    const taskPrompt = promptText
      ? `${EXECUTION_FUSION_META.promptPrefix} ${promptText}`
      : EXECUTION_FUSION_META.promptPrefix
    const firstInput = fusionInputs[0]
    const resultAnchor = resolveGraphNodeResultAnchor(graph, nodeId)
    return {
      kind: 'fusion',
      sourceNodeId: nodeId,
      source: {
        url: firstInput.url,
        ratio: firstInput.ratio,
      },
      taskPrompt,
      outputCount: requireRequestedOutputCount(outputCount),
      aspectRatio: normalizedAspectRatio,
      inputImages: fusionInputs,
      imageUrls,
      blendPoint: snapshot.blendPoint,
      stageWidth: snapshot.width,
      outputResolution: resolveNodeOutputResolution(graph, nodeId),
      resultOptions: {
        anchor: resultAnchor,
      },
    }
  }

  return {
    planCanvasExtendAction,
    planFusionPromptGeneration,
    planImageDirectAction,
    planImageInpaintAction,
    planPromptGeneration,
  }
}
