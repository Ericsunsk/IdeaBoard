import type { CanvasConnectionNodeType, ImageNodeOutputItem } from '../types/canvasNodes'
import { waitTaskResultWithSse } from '../../../shared/api/taskEvents'
import { taskApi, type SubmitTaskData } from '../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'

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
export interface ResolvedFlowInputImage {
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string
  url: string
  ratio: number
}

type FlowConnectedInputPort = 'image' | 'prompt' | 'imageA' | 'imageB'
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
    head === 'model' ||
    head === 'viewpoint' ||
    head === 'fusion'
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

export async function assertTaskSubmitReady(
  request: SubmitTaskData,
  fallbackMessage = '提交任务失败',
): Promise<void> {
  const readiness = await taskApi.checkSubmitReadiness(request)
  if (readiness.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(readiness.msg, fallbackMessage))
  }
  if (!readiness.data?.ready) {
    throw new Error(sanitizeApiErrorMessage(readiness.data?.message, fallbackMessage))
  }
}

/** All node kinds that can produce image output usable as downstream source. */
export const FLOW_EXECUTION_SOURCE_KINDS = new Set<CanvasConnectionNodeType>([
  'image',
  'fusion',
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

export function resolveConnectedImageInputs(
  options: FlowTaskExecutorGraphOptions,
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
    const snapshotNode = sourceNode.data.snapshotNode as Record<string, unknown>
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
