import type { Connection } from '@vue-flow/core'
import { isCanvasConnectionAllowed } from '../config/canvasConnections'
import type { CanvasConnectionContractNode } from '../config/canvasNodeConnectionContract'
import type { CanvasConnectionNodeType, CanvasConnectionPort } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'

interface ParsedNodeRef {
  nodeType: CanvasConnectionNodeType
  nodeId: number
}

interface ParsedHandleRef {
  direction: 'in' | 'out'
  port: CanvasConnectionPort
}

function parseNodeRef(nodeId: string | null | undefined, fallbackType?: CanvasConnectionNodeType): ParsedNodeRef | null {
  if (!nodeId) return null
  const [prefix, idPart] = nodeId.split('-')
  const parsedId = Number(idPart)
  const nodeType = isCanvasConnectionNodeType(prefix) ? prefix : fallbackType
  if (!nodeType || Number.isNaN(parsedId)) return null
  return { nodeType, nodeId: parsedId }
}

function isCanvasConnectionNodeType(value: unknown): value is CanvasConnectionNodeType {
  return value === 'image'
    || value === 'prompt'
    || value === 'mask'
    || value === 'color'
    || value === 'model'
    || value === 'viewpoint'
    || value === 'edit'
    || value === 'fusion'
    || value === 'enhance'
    || value === 'wear'
}

function parseHandleRef(handleId: string | null | undefined): ParsedHandleRef | null {
  if (!handleId) return null
  const [direction, rawPort] = handleId.split(':')
  if ((direction !== 'in' && direction !== 'out') || !rawPort) return null
  const port = rawPort as CanvasConnectionPort
  return { direction, port }
}

export function isFlowConnectionAllowed(
  connection: Connection,
  lookupData: (nodeId: string) => FlowNodeData | undefined,
): boolean {
  const sourceNodeData = connection.source ? lookupData(connection.source) : undefined
  const targetNodeData = connection.target ? lookupData(connection.target) : undefined
  const sourceNodeType = isCanvasConnectionNodeType(sourceNodeData?.nodeType) ? sourceNodeData?.nodeType : undefined
  const targetNodeType = isCanvasConnectionNodeType(targetNodeData?.nodeType) ? targetNodeData?.nodeType : undefined
  const sourceNodeRef = parseNodeRef(connection.source, sourceNodeType)
  const targetNodeRef = parseNodeRef(connection.target, targetNodeType)
  const sourceHandle = parseHandleRef(connection.sourceHandle)
  const targetHandle = parseHandleRef(connection.targetHandle)

  if (!sourceNodeRef || !targetNodeRef || !sourceHandle || !targetHandle) return false
  if (sourceHandle.direction !== 'out' || targetHandle.direction !== 'in') return false

  const from = { ...sourceNodeRef, port: sourceHandle.port }
  const to = { ...targetNodeRef, port: targetHandle.port }

  const lookupNode = (nodeType: CanvasConnectionNodeType, nodeId: number): CanvasConnectionContractNode | null => {
    const key = `${nodeType}-${nodeId}`
    const data = lookupData(key)
    if (!data) return null
    return {
      id: nodeId,
      type: nodeType,
      mode: data.mode ?? (nodeType === 'edit' ? 'partial-edit' : 'hd-upscale'),
    } as CanvasConnectionContractNode
  }

  return isCanvasConnectionAllowed(from, to, lookupNode)
}
