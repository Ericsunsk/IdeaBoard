import {
  getCanvasNodeImageInputLimit,
  normalizePromptAspectRatio,
  resolveImageNodeFrameSize,
  resolveImageNodeLatestOutput,
  type FusionBlendPoint,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptAspectRatio,
  type PromptNodeItem,
} from '../../../../board'
import { parseRequestedOutputCount } from '../../../../../shared/domain/generationContract'
import {
  normalizeOutputResolution,
  type OutputResolution,
} from '../../../../../shared/domain/generationContract'
import { i18n } from '../../../../../shared/i18n'
import { EXECUTION_FUSION_META } from '../../../domain/execution-contracts/executionContract'
import type { ImageTaskAction } from '../../../domain/execution-contracts/imageActionExecution'
import { resolveOutpaintIntent, type OutpaintRect } from '../../../domain/graph/outpaintGeometry'
import {
  resolveImageNodeLayoutAnchor,
  type ImageNodeLayoutAnchor,
} from '../../../infrastructure/vue-flow-adapter/flowImageNodeSelectors'
import {
  resolveConnectedImageInputs,
  resolveConnectedInput,
  resolveConnectedPromptText,
  type FlowExecutionGraphOptions,
  type FlowGraphNode,
  type ResolvedFlowInputImage,
} from '../../../infrastructure/vue-flow-adapter/flowNodeSourceResolution'
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
  strength?: number | undefined
  outputResolution?: OutputResolution | undefined
  resultOptions: ImageExecutionResultOptions
}

export type OutpaintExecutionPlan = {
  kind: 'outpaint'
  sourceNodeId: string
  source: ImageExecutionInput
  prompt: string
  outputCount: number
  frameRect: OutpaintRect
  originalImageRect: OutpaintRect
  targetRatio: number
  targetWidth: number
  resultOptions: ImageExecutionResultOptions
}

export type ImageInpaintExecutionPlan = {
  kind: 'inpaint'
  sourceNodeId: string
  source: ImageExecutionInput
  prompt: string
  rawMaskDataUrl: string
  outputCount: number
  resultOptions: ImageExecutionResultOptions
}

export type GenerationExecutionPlan = {
  kind: 'generation'
  sourceNodeId: string
  source: ImageExecutionInput
  referenceImages: ImageExecutionInput[]
  prompt: string
  outputCount: number
  aspectRatio: PromptAspectRatio
  outputResolution?: OutputResolution | undefined
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
  outputResolution?: OutputResolution | undefined
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
  | OutpaintExecutionPlan
  | ImageInpaintExecutionPlan
  | GenerationExecutionPlan
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
  if (outputCount === null) failPlanning(i18n.global.t('canvas.execution.invalidOutputCount'))
  return outputCount
}

function normalizeExecutionRatio(value: unknown): number {
  const ratio = Number(value)
  return Number.isFinite(ratio) && ratio > 0 ? ratio : 1
}

function toImageExecutionInput(
  anchor: Pick<ImageNodeLayoutAnchor, 'url' | 'ratio' | 'assetId'>,
  imageNode: Pick<ImageNodeItem, 'sourceMediaAssetId' | 'outputImages'>,
): ImageExecutionInput {
  // Submission still addresses numeric board asset IDs (backend int64
  // contract); delivery uses the string asset identity from the MediaRef.
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  const rawNumeric = latestOutput ? latestOutput.mediaAssetId : imageNode.sourceMediaAssetId
  const numericId = Number(rawNumeric)
  return {
    url: anchor.url.trim(),
    ratio: normalizeExecutionRatio(anchor.ratio),
    ...(Number.isInteger(numericId) && numericId > 0 ? { mediaAssetId: numericId } : {}),
    ...(anchor.assetId ? { assetId: anchor.assetId } : {}),
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
    input: toImageExecutionInput(anchor, imageNode),
    resultAnchor: toImageResultAnchor(anchor),
  }
}

function requireImageExecutionImage(
  graph: FlowExecutionGraphOptions,
  nodeId: string,
): ResolvedImageExecutionImage {
  const node = graph.nodes.find((item) => item.id === nodeId)
  if (!node || node.data?.kind !== 'image') {
    failPlanning(i18n.global.t('canvas.execution.imageNodeMissingUrl'))
  }
  const snapshot = node.data.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') {
    failPlanning(i18n.global.t('canvas.execution.imageNodeMissingUrl'))
  }
  const source = resolveImageExecutionImage(node, snapshot as ImageNodeItem, {
    requireUrl: true,
  })
  if (!source?.input.url) failPlanning(i18n.global.t('canvas.execution.imageNodeMissingUrl'))
  return source
}

function readNodeWidth(node: FlowGraphNode | undefined, fallbackWidth: number): number {
  const snapshot = node?.data?.snapshotNode as { width?: unknown } | undefined
  const width = Number(snapshot?.width)
  return Number.isFinite(width) && width > 0 ? width : fallbackWidth
}

function resolveOutpaintResultWidth(
  sourceWidthValue: unknown,
  frameRect: OutpaintRect,
  originalImageRect: OutpaintRect,
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

  function planOutpaintAction(
    nodeId: string,
    intent: {
      frameRect: OutpaintRect
      originalImageRect: OutpaintRect
      prompt?: string
    },
  ): OutpaintExecutionPlan {
    const graph = readGraph()
    const source = requireImageExecutionImage(graph, nodeId)
    const resolvedIntent = resolveOutpaintIntent(intent.frameRect, intent.originalImageRect)
    if (!resolvedIntent.hasExtension)
      failPlanning(i18n.global.t('canvas.execution.dragOutpaintFrameFirst'))
    const { targetRatio } = resolvedIntent
    const targetWidth = resolveOutpaintResultWidth(
      source.resultAnchor.width,
      resolvedIntent.frameRect,
      resolvedIntent.originalImageRect,
      targetRatio,
    )
    return {
      kind: 'outpaint',
      sourceNodeId: nodeId,
      source: source.input,
      prompt: `${intent.prompt ?? ''}`.trim(),
      outputCount: 1,
      frameRect: resolvedIntent.frameRect,
      originalImageRect: resolvedIntent.originalImageRect,
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
    },
  ): ImageInpaintExecutionPlan {
    const prompt = `${inpaint.prompt ?? ''}`.trim()
    if (!prompt) failPlanning(i18n.global.t('canvas.execution.enterPromptFirst'))
    const graph = readGraph()
    const source = requireImageExecutionImage(graph, nodeId)
    const rawMaskDataUrl = `${inpaint.maskDataUrl ?? ''}`.trim()
    if (!rawMaskDataUrl) failPlanning(i18n.global.t('canvas.execution.markInpaintAreaFirst'))
    return {
      kind: 'inpaint',
      sourceNodeId: nodeId,
      source: source.input,
      prompt,
      rawMaskDataUrl,
      outputCount: 1,
      resultOptions: {
        anchor: withOutputAnchorNodeId(source.resultAnchor, nodeId),
      },
    }
  }

  function planGeneration(
    anchorNodeId: string,
    referenceImages: readonly ImageExecutionInput[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ): GenerationExecutionPlan {
    const normalizedPrompt = `${prompt ?? ''}`.trim()
    if (referenceImages.length > 4)
      failPlanning(i18n.global.t('canvas.execution.tooManyReferenceImages'))
    const normalizedReferences = referenceImages.map((image) => ({
      url: `${image.url ?? ''}`.trim(),
      ratio: normalizeExecutionRatio(image.ratio),
      ...(image.mediaAssetId ? { mediaAssetId: image.mediaAssetId } : {}),
      ...(image.assetId ? { assetId: image.assetId } : {}),
    }))
    if (normalizedReferences.some((image) => !image.url))
      failPlanning(i18n.global.t('canvas.execution.referenceImageMissingUrl'))
    const graph = readGraph()
    const promptNode = graph.nodes.find((node) => node.id === anchorNodeId)
    const outputResolution =
      promptNode?.data?.kind === 'prompt' && promptNode.data.snapshotNode
        ? normalizeOutputResolution(
            (promptNode.data.snapshotNode as PromptNodeItem).outputResolution,
          )
        : '1k'
    const source = normalizedReferences[0] ?? { url: '', ratio: 1 }
    return {
      kind: 'generation',
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

  function planFusionGeneration(
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
      failPlanning(i18n.global.t('canvas.execution.fusionNodeMissing'))
    }
    const snapshot = fusionNode.data.snapshotNode as FusionNodeItem
    const fusionInputLimit = getCanvasNodeImageInputLimit('fusion')
    const inputImages = resolveConnectedImageInputs(
      graph,
      nodeId,
      snapshot.inputImages,
      fusionInputLimit,
    )
    if (inputImages.length < 2) failPlanning(i18n.global.t('canvas.execution.fusionNeedsTwoImages'))
    const fusionInputs = inputImages
      .map((item) => ({ ...item, url: item.url.trim() }))
      .filter((item) => item.url)
      .slice(0, fusionInputLimit)
    const imageUrls = fusionInputs.map((item) => item.url)
    if (imageUrls.length < 2) failPlanning(i18n.global.t('canvas.execution.fusionInputsInvalid'))

    const normalizedPrompt = `${prompt ?? ''}`.trim()
    const normalizedAspectRatio = normalizePromptAspectRatio(aspectRatio)
    const promptInput = resolveConnectedInput(graph, nodeId, 'prompt')
    const connectedPromptText = promptInput ? resolveConnectedPromptText(graph, nodeId) : ''
    const promptText = normalizedPrompt || connectedPromptText
    const taskPrompt = promptText
      ? `${EXECUTION_FUSION_META.promptPrefix} ${promptText}`
      : EXECUTION_FUSION_META.promptPrefix
    const firstInput = fusionInputs[0]
    if (!firstInput) failPlanning(i18n.global.t('canvas.execution.fusionInputsInvalid'))
    const resultAnchor = resolveGraphNodeResultAnchor(graph, nodeId)
    return {
      kind: 'fusion',
      sourceNodeId: nodeId,
      source: {
        url: firstInput.url,
        ratio: firstInput.ratio,
        ...(firstInput.assetId ? { assetId: firstInput.assetId } : {}),
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
    planOutpaintAction,
    planFusionGeneration,
    planImageDirectAction,
    planImageInpaintAction,
    planGeneration,
  }
}
