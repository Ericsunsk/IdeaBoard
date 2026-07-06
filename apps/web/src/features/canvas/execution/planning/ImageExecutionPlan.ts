import {
  clampImageNodeOutputCount,
  getCanvasNodeImageInputLimit,
  normalizePromptReferenceImageUrls,
  normalizePromptAspectRatio,
  resolveImageNodeFrameSize,
  type FusionBlendPoint,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptAspectRatio,
} from '../../../canvas-board/domain'
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

export type {
  CanvasExtendPercent,
  ImageExecutionInput,
  ImageExecutionResultOptions,
  ImageResultAnchor,
}

export type ImageActionExecutionPlan = {
  kind: 'image-action'
  sourceNodeId: string
  source: ImageExecutionInput
  action: ImageTaskAction
  strength?: number
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
  resultOptions: ImageExecutionResultOptions
}

export type ImagePromptExecutionPlan = {
  kind: 'image-prompt'
  sourceNodeId: string
  source: ImageExecutionInput
  prompt: string
  outputCount: number
  aspectRatio: PromptAspectRatio
  resultOptions: ImageExecutionResultOptions
}

export type TextPromptExecutionPlan = {
  kind: 'text-prompt'
  sourceNodeId: string
  source: ImageExecutionInput
  prompt: string
  outputCount: number
  aspectRatio: PromptAspectRatio
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
  | ImagePromptExecutionPlan
  | TextPromptExecutionPlan
  | FusionExecutionPlan

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

function normalizeExecutionRatio(value: unknown): number {
  const ratio = Number(value)
  return Number.isFinite(ratio) && ratio > 0 ? ratio : 1
}

function toImageExecutionInput(anchor: Pick<ImageNodeLayoutAnchor, 'url' | 'ratio'>): ImageExecutionInput {
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
    options: { strength?: number } = {},
  ): ImageActionExecutionPlan {
    const source = requireImageExecutionImage(readGraph(), nodeId)
    return {
      kind: 'image-action',
      sourceNodeId: nodeId,
      source: source.input,
      action,
      strength: options.strength,
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
      outputCount: clampImageNodeOutputCount(intent.outputCount ?? 1),
      extendPercent,
      targetRatio,
      targetWidth,
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
    const source = requireImageExecutionImage(readGraph(), nodeId)
    const rawMaskDataUrl = `${inpaint.maskDataUrl ?? ''}`.trim()
    if (!rawMaskDataUrl) failPlanning('请先标记局部重绘区域')
    return {
      kind: 'image-inpaint',
      sourceNodeId: nodeId,
      source: source.input,
      prompt,
      rawMaskDataUrl,
      outputCount: clampImageNodeOutputCount(inpaint.outputCount),
      referenceImageUrls: normalizePromptReferenceImageUrls(inpaint.referenceImageUrls),
      resultOptions: {
        anchor: withOutputAnchorNodeId(source.resultAnchor, nodeId),
      },
    }
  }

  function planImagePromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio = 'auto',
    options: { outputAnchorNodeId?: string } = {},
  ): ImagePromptExecutionPlan {
    const graph = readGraph()
    const source = requireImageExecutionImage(graph, nodeId)
    return planImagePromptGenerationFromSource(
      source.input,
      nodeId,
      prompt,
      outputCount,
      aspectRatio,
      {
        ...options,
        anchor: options.outputAnchorNodeId
          ? resolveGraphNodeResultAnchor(graph, options.outputAnchorNodeId)
          : source.resultAnchor,
      },
    )
  }

  function planImagePromptGenerationFromSource(
    source: ImageExecutionInput,
    sourceNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    options: { outputAnchorNodeId?: string; anchor?: ImageResultAnchor } = {},
  ): ImagePromptExecutionPlan {
    const normalizedPrompt = `${prompt ?? ''}`.trim()
    if (!normalizedPrompt) failPlanning('请输入 Prompt')
    if (!source.url) failPlanning('图片缺少 URL')
    return {
      kind: 'image-prompt',
      source,
      sourceNodeId,
      prompt: normalizedPrompt,
      outputCount: clampImageNodeOutputCount(outputCount),
      aspectRatio: normalizePromptAspectRatio(aspectRatio),
      resultOptions: {
        ...(options.anchor
          ? { anchor: withOutputAnchorNodeId(options.anchor, options.outputAnchorNodeId) }
          : options.outputAnchorNodeId
            ? { anchor: resolveGraphNodeResultAnchor(readGraph(), options.outputAnchorNodeId) }
            : {}),
      },
    }
  }

  function planTextPromptGeneration(
    anchorNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ): TextPromptExecutionPlan {
    const normalizedPrompt = `${prompt ?? ''}`.trim()
    if (!normalizedPrompt) failPlanning('请输入 Prompt')
    const graph = readGraph()
    const source: ImageExecutionInput = {
      url: '',
      ratio: 1,
    }
    return {
      kind: 'text-prompt',
      sourceNodeId: anchorNodeId,
      source,
      prompt: normalizedPrompt,
      outputCount: clampImageNodeOutputCount(outputCount),
      aspectRatio: normalizePromptAspectRatio(aspectRatio),
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
      outputCount: clampImageNodeOutputCount(outputCount),
      aspectRatio: normalizedAspectRatio,
      inputImages: fusionInputs,
      imageUrls,
      blendPoint: snapshot.blendPoint,
      stageWidth: snapshot.width,
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
    planImagePromptGeneration,
    planImagePromptGenerationFromSource,
    planTextPromptGeneration,
  }
}
