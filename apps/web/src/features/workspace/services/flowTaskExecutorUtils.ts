import type { CanvasConnectionNodeType, ImageNodeOutputItem } from '../types/canvasNodes'
import { waitTaskResultWithSse } from '../../../shared/api/taskEvents'

export interface FlowTaskExecutorGraphOptions {
  nodes: Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
  edges: Array<{
    source: string
    sourceHandle?: string | null
    target: string
    targetHandle?: string | null
  }>
}

export interface FlowConnectedInput {
  sourceType: CanvasConnectionNodeType
  snapshotNode: Record<string, unknown>
}

type FlowConnectedInputPort = 'image' | 'mask' | 'prompt' | 'imageA' | 'imageB'
type FlowGraphNode = FlowTaskExecutorGraphOptions['nodes'][number]
type FlowSourceTypeResolver = (sourceNode: FlowGraphNode) => CanvasConnectionNodeType | null

function resolvePromptInputSourceNodeId(
  options: FlowTaskExecutorGraphOptions,
  targetNodeId: string,
): string {
  const incoming = options.edges.find(
    (edge) => edge.target === targetNodeId && parseHandlePort(edge.targetHandle) === 'prompt',
  )
  return incoming?.source || ''
}

function resolvePromptChainText(
  options: FlowTaskExecutorGraphOptions,
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
  options: FlowTaskExecutorGraphOptions,
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
  if (
    head === 'prompt' ||
    head === 'image' ||
    head === 'mask' ||
    head === 'model' ||
    head === 'viewpoint' ||
    head === 'edit' ||
    head === 'fusion' ||
    head === 'enhance' ||
    head === 'wear' ||
    head === 'color'
  ) {
    return head
  }
  return null
}

export function readLatestOutputImage(
  snapshotNode: Record<string, unknown>,
): ImageNodeOutputItem | null {
  const outputImages = Array.isArray(snapshotNode.outputImages)
    ? (snapshotNode.outputImages as ImageNodeOutputItem[])
    : []
  return outputImages.length > 0 ? outputImages[outputImages.length - 1] : null
}

export function resolveSourceImageFromNode(
  nodeType: CanvasConnectionNodeType,
  snapshotNode: Record<string, unknown>,
): string {
  if (nodeType === 'image') {
    const latestOutput = readLatestOutputImage(snapshotNode)
    const sourceImageUrl =
      typeof snapshotNode.sourceImageUrl === 'string' ? snapshotNode.sourceImageUrl.trim() : ''
    return (latestOutput?.url || sourceImageUrl || '').trim()
  }
  const latestOutput = readLatestOutputImage(snapshotNode)
  return (latestOutput?.url || '').trim()
}

function resolveConnectedNodeInput(
  options: FlowTaskExecutorGraphOptions,
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
    snapshotNode: sourceNode.data.snapshotNode as Record<string, unknown>,
  }
}

export function resolveConnectedInput(
  options: FlowTaskExecutorGraphOptions,
  targetNodeId: string,
  targetPort: FlowConnectedInputPort,
): FlowConnectedInput | null {
  return resolveConnectedNodeInput(options, targetNodeId, targetPort, (sourceNode) =>
    resolveNodeType(sourceNode.id),
  )
}

export function resolveConnectedExecutionInput(
  options: FlowTaskExecutorGraphOptions,
  targetNodeId: string,
  targetPort: FlowConnectedInputPort,
): FlowConnectedInput | null {
  return resolveConnectedNodeInput(options, targetNodeId, targetPort, (sourceNode) => {
    const sourceKind = sourceNode.data?.kind
    return sourceKind && isFlowExecutionSourceKind(sourceKind) ? sourceKind : null
  })
}

export async function waitTaskResult(taskId: string): Promise<string[]> {
  return waitTaskResultWithSse(taskId)
}

/** All node kinds that can produce image output usable as downstream source. */
export const FLOW_EXECUTION_SOURCE_KINDS = new Set<CanvasConnectionNodeType>([
  'image',
  'edit',
  'enhance',
  'color',
  'fusion',
  'wear',
  'model',
  'viewpoint',
])

export function isFlowExecutionSourceKind(kind: string): kind is CanvasConnectionNodeType {
  return FLOW_EXECUTION_SOURCE_KINDS.has(kind as CanvasConnectionNodeType)
}

export function resolveFlowSourceImageRatio(
  sourceKind: CanvasConnectionNodeType,
  snapshotNode: Record<string, unknown>,
): number {
  if (sourceKind === 'image') {
    const sourceImageRatio = Number(snapshotNode.sourceImageRatio)
    if (Number.isFinite(sourceImageRatio) && sourceImageRatio > 0) return sourceImageRatio
  }
  const outputImages = Array.isArray(snapshotNode.outputImages)
    ? (snapshotNode.outputImages as Array<{ ratio?: unknown }>)
    : []
  const latestOutput = outputImages.length > 0 ? outputImages[outputImages.length - 1] : null
  const outputRatio = Number(latestOutput?.ratio)
  if (Number.isFinite(outputRatio) && outputRatio > 0) return outputRatio
  return 1
}
