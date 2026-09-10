import {
  isCanvasConnectionNodeType,
  isCanvasImageInputSourceNodeType,
  resolveImageNodeDisplayRatio,
  type CanvasConnectionNodeType,
  type ImageNodeItem,
  type ImageNodeOutputItem,
} from '../../board/domain'
import { resolveImageNodeDisplayUrl } from './flowImageNodeSelectors'

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
  mediaAssetId?: number | undefined
  /** String delivery identity resolved from the source node's MediaRef. */
  assetId?: string | undefined
}

export type FlowConnectedInputPort = 'image' | 'prompt'
export type FlowGraphNode = FlowExecutionGraphOptions['nodes'][number]
type FlowSourceTypeResolver = (sourceNode: FlowGraphNode) => CanvasConnectionNodeType | null

export type FlowSnapshotNodeLike = {
  type?: unknown
  outputImages?: unknown
  sourceImageUrl?: unknown
  sourceMediaAssetId?: unknown
  sourceImageRatio?: unknown
  outputStatus?: unknown
}

function isImageNodeSnapshot(snapshotNode: FlowSnapshotNodeLike): snapshotNode is ImageNodeItem {
  return snapshotNode.type === 'image-node'
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

function resolveNodeType(nodeId: string): CanvasConnectionNodeType | null {
  const [head] = nodeId.split('-')
  return isCanvasConnectionNodeType(head) ? head : null
}

function readLatestOutputImage(snapshotNode: FlowSnapshotNodeLike): ImageNodeOutputItem | null {
  const outputImages = Array.isArray(snapshotNode.outputImages)
    ? (snapshotNode.outputImages as ImageNodeOutputItem[])
    : []
  return outputImages.length > 0 ? (outputImages[outputImages.length - 1] ?? null) : null
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

function resolveSourceAssetIdFromNode(
  nodeType: CanvasConnectionNodeType,
  snapshotNode: FlowSnapshotNodeLike,
): string | undefined {
  const ref =
    nodeType === 'image'
      ? isImageNodeSnapshot(snapshotNode)
        ? snapshotNode.sourceMediaRef
        : undefined
      : readLatestOutputImage(snapshotNode)?.mediaRef
  const assetId = ref?.assetId?.trim()
  return assetId ? assetId : undefined
}

function resolveSourceMediaAssetIdFromNode(
  nodeType: CanvasConnectionNodeType,
  snapshotNode: FlowSnapshotNodeLike,
): number | undefined {
  const raw =
    nodeType === 'image'
      ? isImageNodeSnapshot(snapshotNode)
        ? snapshotNode.sourceMediaAssetId
        : undefined
      : readLatestOutputImage(snapshotNode)?.mediaAssetId
  const mediaAssetId = Number(raw)
  return Number.isInteger(mediaAssetId) && mediaAssetId > 0 ? mediaAssetId : undefined
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

function isFlowExecutionSourceKind(kind: string): kind is CanvasConnectionNodeType {
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

type SnapshotInputRecord = {
  id?: unknown
  sourceType?: unknown
  sourceId?: unknown
  sourceNodeId?: unknown
  url?: unknown
  ratio?: unknown
  mediaAssetId?: unknown
  mediaRef?: unknown
}

function readSnapshotInputIdentity(record: SnapshotInputRecord):
  | (Pick<ResolvedFlowInputImage, 'id' | 'sourceType' | 'sourceId'> & {
      sourceNodeId?: string
    })
  | null {
  const id = typeof record.id === 'string' ? record.id : ''
  const sourceType =
    record.sourceType === 'node' || record.sourceType === 'external' ? record.sourceType : null
  const sourceId = typeof record.sourceId === 'string' ? record.sourceId : ''
  const sourceNodeId = typeof record.sourceNodeId === 'string' ? record.sourceNodeId : undefined
  if (!id || !sourceType || !sourceId) return null
  return {
    id,
    sourceType,
    sourceId,
    ...(sourceNodeId ? { sourceNodeId } : {}),
  }
}

function readSnapshotInputMedia(
  record: SnapshotInputRecord,
): (Pick<ResolvedFlowInputImage, 'url' | 'ratio'> & {
  mediaAssetId?: number
  assetId?: string
}) | null {
  const url = typeof record.url === 'string' ? record.url.trim() : ''
  const ratio = Number(record.ratio)
  const mediaAssetId = Number(record.mediaAssetId)
  const mediaRef = record.mediaRef as { assetId?: unknown } | undefined
  const assetId = typeof mediaRef?.assetId === 'string' ? mediaRef.assetId.trim() : ''
  if (!url || !Number.isFinite(ratio) || ratio <= 0) return null
  return {
    url,
    ratio,
    ...(Number.isInteger(mediaAssetId) && mediaAssetId > 0 ? { mediaAssetId } : {}),
    ...(assetId ? { assetId } : {}),
  }
}

function normalizeSnapshotInputImage(item: unknown): ResolvedFlowInputImage | null {
  const record = item as SnapshotInputRecord
  const identity = readSnapshotInputIdentity(record)
  const media = readSnapshotInputMedia(record)
  if (!identity || !media) return null
  return { ...identity, ...media }
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
      const normalized = normalizeSnapshotInputImage(item)
      if (normalized) normalizedSnapshot.push(normalized)
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
      ...(resolveSourceMediaAssetIdFromNode(sourceKind, snapshotNode)
        ? { mediaAssetId: resolveSourceMediaAssetIdFromNode(sourceKind, snapshotNode) }
        : {}),
      ...(resolveSourceAssetIdFromNode(sourceKind, snapshotNode)
        ? { assetId: resolveSourceAssetIdFromNode(sourceKind, snapshotNode) }
        : {}),
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
