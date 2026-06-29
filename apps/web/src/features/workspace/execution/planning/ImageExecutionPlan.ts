import {
  clampImageNodeOutputCount,
  getCanvasNodeImageInputLimit,
  isCanvasConnectionNodeType,
  isCanvasImageInputSourceNodeType,
  normalizePromptAspectRatio,
  resolveImageNodeFrameSize,
  type CanvasConnectionNodeType,
  type FusionBlendPoint,
  type FusionNodeItem,
  type ImageNodeItem,
  type ImageNodeOutputItem,
  type PromptAspectRatio,
} from '../../../canvas-board/domain'
import { EXECUTION_FUSION_META } from '../contracts/executionContract'
import type { ImageTaskAction } from '../contracts/imageActionExecution'
import { resolveCanvasExtendIntent, type CanvasExtendRect } from '../../utils/canvasExtendGeometry'
import {
  resolveImageNodeDisplayRatio,
  resolveImageNodeDisplayUrl,
  resolveImageNodeLayoutAnchor,
  type ImageNodeLayoutAnchor,
} from '../../utils/flowImageNodeSelectors'
import { resolveFusionOutputSize, type FusionOutputImageSize } from './fusionOutputSizing'

export interface FlowExecutionGraphOptions {
  nodes: Array<{
    id: string
    position?: { x?: number; y?: number }
    data?: { kind?: string; snapshotNode?: unknown }
  }>
  edges: Array<{
    source: string
    sourceHandle?: string | null
    target: string
    targetHandle?: string | null
  }>
}

export interface FlowConnectedInput {
  sourceType: CanvasConnectionNodeType
  snapshotNode: FlowSnapshotNodeLike
}
export interface ResolvedFlowInputImage {
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string
  url: string
  ratio: number
}

export type FlowConnectedInputPort = 'image' | 'prompt'
type FlowGraphNode = FlowExecutionGraphOptions['nodes'][number]
type FlowSourceTypeResolver = (sourceNode: FlowGraphNode) => CanvasConnectionNodeType | null
export type ImageExecutionInput = {
  url: string
  ratio: number
}
export type ImageResultAnchor = {
  x: number
  y: number
  width: number
  outputAnchorNodeId?: string
}
export type FlowSnapshotNodeLike = {
  type?: unknown
  outputImages?: unknown
  sourceImageUrl?: unknown
  sourceImageRatio?: unknown
  outputStatus?: unknown
}

export type ImageExecutionResultOptions = {
  anchor?: ImageResultAnchor
  targetRatio?: number
  targetWidth?: number
}

export type CanvasExtendPercent = {
  top: number
  right: number
  bottom: number
  left: number
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

function isImageNodeSnapshot(snapshotNode: FlowSnapshotNodeLike): snapshotNode is ImageNodeItem {
  return snapshotNode.type === 'image-node'
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

function resolvePromptInputSourceNodeId(
  options: FlowExecutionGraphOptions,
  targetNodeId: string,
): string {
  const incoming = options.edges.find(
    (edge) => edge.target === targetNodeId && parseHandlePort(edge.targetHandle) === 'prompt',
  )
  return incoming?.source || ''
}

function resolvePromptChainText(
  options: FlowExecutionGraphOptions,
  promptNodeId: string,
  visited: Set<string>,
): string {
  if (!promptNodeId || visited.has(promptNodeId)) return ''
  visited.add(promptNodeId)
  const promptNode = options.nodes.find((node) => node.id === promptNodeId)
  if (
    !promptNode ||
    promptNode.data?.kind !== 'prompt' ||
    !promptNode.data.snapshotNode ||
    typeof promptNode.data.snapshotNode !== 'object'
  ) {
    return ''
  }

  const snapshotNode = promptNode.data.snapshotNode as { text?: unknown }
  const selfText =
    typeof snapshotNode.text === 'string'
      ? snapshotNode.text.trim()
      : `${snapshotNode.text ?? ''}`.trim()

  const upstreamPromptNodeId = resolvePromptInputSourceNodeId(options, promptNodeId)
  const upstreamText = upstreamPromptNodeId
    ? resolvePromptChainText(options, upstreamPromptNodeId, visited)
    : ''

  return [upstreamText, selfText].filter((item) => item.length > 0).join('\n')
}

export function resolveConnectedPromptText(
  options: FlowExecutionGraphOptions,
  targetNodeId: string,
): string {
  const promptSourceNodeId = resolvePromptInputSourceNodeId(options, targetNodeId)
  if (!promptSourceNodeId) return ''
  return resolvePromptChainText(options, promptSourceNodeId, new Set<string>())
}

export function parseHandlePort(handle: string | null | undefined): string {
  if (!handle) return ''
  const [prefix, port] = handle.split(':')
  if (prefix === 'in' || prefix === 'out') return port || ''
  return handle
}

export function resolveNodeType(nodeId: string): CanvasConnectionNodeType | null {
  const [head] = nodeId.split('-')
  return isCanvasConnectionNodeType(head) ? head : null
}

export function readLatestOutputImage(
  snapshotNode: FlowSnapshotNodeLike,
): ImageNodeOutputItem | null {
  const outputImages = Array.isArray(snapshotNode.outputImages)
    ? (snapshotNode.outputImages as ImageNodeOutputItem[])
    : []
  return outputImages.length > 0 ? outputImages[outputImages.length - 1] : null
}

export function resolveSourceImageFromNode(
  nodeType: CanvasConnectionNodeType,
  snapshotNode: FlowSnapshotNodeLike,
): string {
  if (nodeType === 'image') {
    return isImageNodeSnapshot(snapshotNode) ? resolveImageNodeDisplayUrl(snapshotNode) : ''
  }
  const latestOutput = readLatestOutputImage(snapshotNode)
  return (latestOutput?.url || '').trim()
}

function resolveConnectedNodeInput(
  options: FlowExecutionGraphOptions,
  targetNodeId: string,
  targetPort: FlowConnectedInputPort,
  resolveSourceType: FlowSourceTypeResolver,
): FlowConnectedInput | null {
  const incoming = options.edges.find(
    (edge) => edge.target === targetNodeId && parseHandlePort(edge.targetHandle) === targetPort,
  )
  if (!incoming) return null
  const sourceNode = options.nodes.find((node) => node.id === incoming.source)
  if (
    !sourceNode ||
    !sourceNode.data?.snapshotNode ||
    typeof sourceNode.data.snapshotNode !== 'object'
  )
    return null
  const sourceType = resolveSourceType(sourceNode)
  if (!sourceType) return null
  return {
    sourceType,
    snapshotNode: sourceNode.data.snapshotNode as FlowSnapshotNodeLike,
  }
}

export function resolveConnectedInput(
  options: FlowExecutionGraphOptions,
  targetNodeId: string,
  targetPort: FlowConnectedInputPort,
): FlowConnectedInput | null {
  return resolveConnectedNodeInput(options, targetNodeId, targetPort, (sourceNode) =>
    resolveNodeType(sourceNode.id),
  )
}

export function isFlowExecutionSourceKind(kind: string): kind is CanvasConnectionNodeType {
  return isCanvasImageInputSourceNodeType(kind)
}

export function resolveFlowSourceImageRatio(
  sourceKind: CanvasConnectionNodeType,
  snapshotNode: FlowSnapshotNodeLike,
): number {
  if (sourceKind === 'image') {
    return isImageNodeSnapshot(snapshotNode) ? resolveImageNodeDisplayRatio(snapshotNode) : 1
  }
  const outputImages = Array.isArray(snapshotNode.outputImages)
    ? (snapshotNode.outputImages as Array<{ ratio?: unknown }>)
    : []
  const latestOutput = outputImages.length > 0 ? outputImages[outputImages.length - 1] : null
  const outputRatio = Number(latestOutput?.ratio)
  if (Number.isFinite(outputRatio) && outputRatio > 0) return outputRatio
  return 1
}

export function resolveConnectedImageInputs(
  options: FlowExecutionGraphOptions,
  targetNodeId: string,
  snapshotInputImages: unknown,
  maxImages = 4,
): ResolvedFlowInputImage[] {
  const normalizedSnapshot: ResolvedFlowInputImage[] = []
  if (Array.isArray(snapshotInputImages)) {
    snapshotInputImages.forEach((item) => {
      const record = item as {
        id?: unknown
        sourceType?: unknown
        sourceId?: unknown
        sourceNodeId?: unknown
        url?: unknown
        ratio?: unknown
      }
      const id = typeof record.id === 'string' ? record.id : ''
      const sourceType =
        record.sourceType === 'node' || record.sourceType === 'external' ? record.sourceType : null
      const sourceId = typeof record.sourceId === 'string' ? record.sourceId : ''
      const sourceNodeId = typeof record.sourceNodeId === 'string' ? record.sourceNodeId : undefined
      const url = typeof record.url === 'string' ? record.url.trim() : ''
      const ratio = Number(record.ratio)
      if (!id || !sourceType || !sourceId || !url || !Number.isFinite(ratio) || ratio <= 0) return
      if (sourceNodeId) {
        normalizedSnapshot.push({ id, sourceType, sourceId, sourceNodeId, url, ratio })
        return
      }
      normalizedSnapshot.push({ id, sourceType, sourceId, url, ratio })
    })
  }

  const edgeConnectedNodeInputs: ResolvedFlowInputImage[] = []
  options.edges.forEach((edge) => {
    if (edge.target !== targetNodeId || parseHandlePort(edge.targetHandle) !== 'image') return
    const sourceNode = options.nodes.find((node) => node.id === edge.source)
    const sourceKind = sourceNode?.data?.kind
    if (
      !sourceNode ||
      !sourceNode.data?.snapshotNode ||
      typeof sourceNode.data.snapshotNode !== 'object' ||
      !sourceKind ||
      !isFlowExecutionSourceKind(sourceKind)
    ) {
      return
    }
    const snapshotNode = sourceNode.data.snapshotNode as FlowSnapshotNodeLike
    const url = resolveSourceImageFromNode(sourceKind, snapshotNode).trim()
    if (!url) return
    edgeConnectedNodeInputs.push({
      id: `node:${sourceNode.id}`,
      sourceType: 'node',
      sourceId: sourceNode.id,
      sourceNodeId: sourceNode.id,
      url,
      ratio: resolveFlowSourceImageRatio(sourceKind, snapshotNode),
    })
  })

  const edgeBySourceId = new Map(edgeConnectedNodeInputs.map((item) => [item.sourceId, item]))
  const orderedInputsFromSnapshot: ResolvedFlowInputImage[] = []
  normalizedSnapshot.forEach((item) => {
    if (item.sourceType === 'external') {
      orderedInputsFromSnapshot.push(item)
      return
    }
    const connected = edgeBySourceId.get(item.sourceId)
    if (connected) orderedInputsFromSnapshot.push(connected)
  })
  const usedNodeSourceIds = new Set(
    orderedInputsFromSnapshot
      .filter((item) => item.sourceType === 'node')
      .map((item) => item.sourceId),
  )
  const remainingNodeInputs = edgeConnectedNodeInputs.filter(
    (item) => !usedNodeSourceIds.has(item.sourceId),
  )

  return [...orderedInputsFromSnapshot, ...remainingNodeInputs].slice(0, maxImages)
}

export function resolveImageExecutionInput(
  node: Pick<FlowGraphNode, 'position'>,
  imageNode: ImageNodeItem,
  options: { requireUrl?: boolean } = {},
): ImageExecutionInput | null {
  const anchor = resolveImageNodeLayoutAnchor(node, imageNode, options)
  return anchor ? toImageExecutionInput(anchor) : null
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
    const referenceImageLimit = getCanvasNodeImageInputLimit('prompt')
    const referenceImageUrls = (inpaint.referenceImageUrls ?? [])
      .map((url) => `${url ?? ''}`.trim())
      .filter(Boolean)
      .slice(0, referenceImageLimit)
    return {
      kind: 'image-inpaint',
      sourceNodeId: nodeId,
      source: source.input,
      prompt,
      rawMaskDataUrl,
      outputCount: clampImageNodeOutputCount(inpaint.outputCount),
      referenceImageUrls,
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
