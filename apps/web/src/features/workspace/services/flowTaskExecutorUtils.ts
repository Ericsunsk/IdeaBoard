import type { CanvasConnectionNodeType, ImageNodeOutputItem } from '../types/canvasNodes'
import { sanitizeTaskErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api'

export interface FlowTaskExecutorGraphOptions {
  nodes: Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
  edges: Array<{ source: string; sourceHandle?: string | null; target: string; targetHandle?: string | null }>
}

export interface FlowConnectedInput {
  sourceType: CanvasConnectionNodeType
  snapshotNode: Record<string, unknown>
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
    head === 'prompt' || head === 'image' || head === 'mask'
    || head === 'model' || head === 'viewpoint' || head === 'edit'
    || head === 'fusion' || head === 'enhance' || head === 'wear' || head === 'color'
  ) {
    return head
  }
  return null
}

export function readLatestOutputImage(snapshotNode: Record<string, unknown>): ImageNodeOutputItem | null {
  const outputImages = Array.isArray(snapshotNode.outputImages) ? snapshotNode.outputImages as ImageNodeOutputItem[] : []
  return outputImages.length > 0 ? outputImages[outputImages.length - 1] : null
}

export function resolveSourceImageFromNode(nodeType: CanvasConnectionNodeType, snapshotNode: Record<string, unknown>): string {
  if (nodeType === 'image') {
    const latestOutput = readLatestOutputImage(snapshotNode)
    const sourceImageUrl = typeof snapshotNode.sourceImageUrl === 'string' ? snapshotNode.sourceImageUrl.trim() : ''
    return (latestOutput?.url || sourceImageUrl || '').trim()
  }
  const latestOutput = readLatestOutputImage(snapshotNode)
  return (latestOutput?.url || '').trim()
}

export function resolveConnectedInput(
  options: FlowTaskExecutorGraphOptions,
  targetNodeId: string,
  targetPort: 'image' | 'mask' | 'prompt' | 'imageA' | 'imageB',
): FlowConnectedInput | null {
  const incoming = options.edges.find(edge => edge.target === targetNodeId && parseHandlePort(edge.targetHandle) === targetPort)
  if (!incoming) return null
  const sourceNode = options.nodes.find(node => node.id === incoming.source)
  if (!sourceNode || !sourceNode.data?.snapshotNode || typeof sourceNode.data.snapshotNode !== 'object') return null
  const sourceType = resolveNodeType(sourceNode.id)
  if (!sourceType) return null
  return {
    sourceType,
    snapshotNode: sourceNode.data.snapshotNode as Record<string, unknown>,
  }
}

export function resolveConnectedExecutionInput(
  options: FlowTaskExecutorGraphOptions,
  targetNodeId: string,
  targetPort: 'image' | 'mask' | 'prompt' | 'imageA' | 'imageB',
): FlowConnectedInput | null {
  const incoming = options.edges.find(edge => edge.target === targetNodeId && parseHandlePort(edge.targetHandle) === targetPort)
  if (!incoming) return null
  const sourceNode = options.nodes.find(node => node.id === incoming.source)
  if (!sourceNode || !sourceNode.data?.snapshotNode || typeof sourceNode.data.snapshotNode !== 'object') return null
  const sourceKind = sourceNode.data.kind
  if (!sourceKind || !isFlowExecutionSourceKind(sourceKind)) return null
  return {
    sourceType: sourceKind as CanvasConnectionNodeType,
    snapshotNode: sourceNode.data.snapshotNode as Record<string, unknown>,
  }
}

export function extractImageUrlsFromTaskResult(result: string | null | undefined): string[] {
  if (!result) return []
  try {
    const parsed = JSON.parse(result)
    if (Array.isArray(parsed?.image_urls)) {
      return parsed.image_urls.filter((url: unknown): url is string => typeof url === 'string' && url.trim().length > 0)
    }
    if (typeof parsed?.image_url === 'string' && parsed.image_url.trim()) return [parsed.image_url.trim()]
    return []
  } catch {
    return []
  }
}

export async function waitTaskResult(taskId: string): Promise<string[]> {
  const maxRetries = 180
  for (let attempt = 0; attempt < maxRetries; attempt += 1) {
    const taskRes = await taskApi.get(taskId)
    if (taskRes.code === 0 && taskRes.data?.task) {
      const task = taskRes.data.task
      if (task.status === 'completed') {
        return extractImageUrlsFromTaskResult(task.result)
      }
      if (task.status === 'failed') {
        throw new Error(sanitizeTaskErrorMessage(task.error_msg, '生成失败'))
      }
    }
    await new Promise(resolve => window.setTimeout(resolve, 2000))
  }
  throw new Error('任务轮询超时，请稍后查看结果')
}

/** All node kinds that can produce image output usable as downstream source. */
export const FLOW_EXECUTION_SOURCE_KINDS = new Set<CanvasConnectionNodeType>([
  'image', 'edit', 'enhance', 'color', 'fusion', 'wear', 'model', 'viewpoint',
])

export function isFlowExecutionSourceKind(kind: string): kind is CanvasConnectionNodeType {
  return FLOW_EXECUTION_SOURCE_KINDS.has(kind as CanvasConnectionNodeType)
}

export function resolveFlowSourceImageRatio(sourceKind: CanvasConnectionNodeType, snapshotNode: Record<string, unknown>): number {
  if (sourceKind === 'image') {
    const sourceImageRatio = Number(snapshotNode.sourceImageRatio)
    if (Number.isFinite(sourceImageRatio) && sourceImageRatio > 0) return sourceImageRatio
  }
  const outputImages = Array.isArray(snapshotNode.outputImages) ? snapshotNode.outputImages as Array<{ ratio?: unknown }> : []
  const latestOutput = outputImages.length > 0 ? outputImages[outputImages.length - 1] : null
  const outputRatio = Number(latestOutput?.ratio)
  if (Number.isFinite(outputRatio) && outputRatio > 0) return outputRatio
  return 1
}
