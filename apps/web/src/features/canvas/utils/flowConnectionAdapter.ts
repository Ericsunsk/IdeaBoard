import type { Connection } from '@vue-flow/core'
import { isCanvasConnectionAllowed } from '../config/canvasConnections'
import type { FlowNodeData } from '../types/flowTypes'
import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  isCanvasConnectionNodeType,
  type CanvasConnection,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
  type CanvasConnectionContractNode,
  type CanvasNodePortContract,
} from '../../canvas-board/domain'

const AUTO_HANDLE_PORT = '__auto'

interface ParsedNodeRef {
  nodeType: CanvasConnectionNodeType
  nodeId: number
}

interface ParsedHandleRef {
  direction: 'in' | 'out'
  port: CanvasConnectionPort | null
  auto: boolean
}

type FlowEdgeLike = {
  id?: string
  target?: string
  targetHandle?: string | null
}

function parseNodeRef(
  nodeId: string | null | undefined,
  fallbackType?: CanvasConnectionNodeType,
): ParsedNodeRef | null {
  if (!nodeId) return null
  const [prefix, idPart] = nodeId.split('-')
  const parsedId = Number(idPart)
  const nodeType = isCanvasConnectionNodeType(prefix) ? prefix : fallbackType
  if (!nodeType || Number.isNaN(parsedId)) return null
  return { nodeType, nodeId: parsedId }
}

function parseHandleRef(handleId: string | null | undefined): ParsedHandleRef | null {
  if (!handleId) return null
  const [direction, rawPort] = handleId.split(':')
  if ((direction !== 'in' && direction !== 'out') || !rawPort) return null
  if (rawPort === AUTO_HANDLE_PORT) return { direction, port: null, auto: true }
  const port = rawPort as CanvasConnectionPort
  return { direction, port, auto: false }
}

function buildContractNode(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  data: FlowNodeData | undefined,
): CanvasConnectionContractNode {
  const snapshotNode = data?.snapshotNode
  if (snapshotNode && typeof snapshotNode === 'object') {
    return {
      ...(snapshotNode as Record<string, unknown>),
      id: nodeId,
      type: nodeType,
    } as CanvasConnectionContractNode
  }
  return {
    id: nodeId,
    type: nodeType,
  } as CanvasConnectionContractNode
}

function createNodeLookup(lookupData: (nodeId: string) => FlowNodeData | undefined) {
  return (
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
  ): CanvasConnectionContractNode | null => {
    const key = `${nodeType}-${nodeId}`
    const data = lookupData(key)
    if (!data) return null
    return buildContractNode(nodeType, nodeId, data)
  }
}

function getVisiblePorts(
  ref: ParsedNodeRef,
  direction: 'input' | 'output',
): CanvasNodePortContract[] {
  return direction === 'input'
    ? getCanvasNodeVisibleInputPorts(ref.nodeType)
    : getCanvasNodeVisibleOutputPorts(ref.nodeType)
}

function resolveSourcePort(
  sourceRef: ParsedNodeRef,
  sourceHandle: ParsedHandleRef,
): CanvasConnectionPort | null {
  if (sourceHandle.port) return sourceHandle.port
  const visibleOutputPortContracts = getVisiblePorts(sourceRef, 'output')
  return visibleOutputPortContracts.length === 1 ? visibleOutputPortContracts[0].port : null
}

function collectUsedTargetPorts(targetNodeId: string, existingEdges: readonly FlowEdgeLike[]) {
  const usedPorts = new Set<CanvasConnectionPort>()
  for (const edge of existingEdges) {
    if (edge.target !== targetNodeId) continue
    const targetHandle = parseHandleRef(edge.targetHandle)
    if (!targetHandle || targetHandle.direction !== 'in' || !targetHandle.port) continue
    usedPorts.add(targetHandle.port)
  }
  return usedPorts
}

function resolveTargetPort(
  sourceRef: ParsedNodeRef,
  sourcePort: CanvasConnectionPort,
  targetRef: ParsedNodeRef,
  targetNodeId: string,
  targetHandle: ParsedHandleRef,
  lookupData: (nodeId: string) => FlowNodeData | undefined,
  existingEdges: readonly FlowEdgeLike[],
): CanvasConnectionPort | null {
  if (targetHandle.port) return targetHandle.port
  const lookupNode = createNodeLookup(lookupData)
  const targetPorts = getVisiblePorts(targetRef, 'input')
  const allowedPorts = targetPorts.filter((targetPort) =>
    isCanvasConnectionAllowed(
      { ...sourceRef, port: sourcePort },
      { ...targetRef, port: targetPort.port },
      lookupNode,
    ),
  )
  if (allowedPorts.length === 0) return null
  const usedPorts = collectUsedTargetPorts(targetNodeId, existingEdges)
  const openPort = allowedPorts.find((targetPort) => !usedPorts.has(targetPort.port))
  return (openPort ?? allowedPorts[0]).port
}

export function normalizeFlowConnection(
  connection: Connection,
  lookupData: (nodeId: string) => FlowNodeData | undefined,
  existingEdges: readonly FlowEdgeLike[] = [],
): Connection | null {
  const sourceNodeData = connection.source ? lookupData(connection.source) : undefined
  const targetNodeData = connection.target ? lookupData(connection.target) : undefined
  const sourceNodeType = isCanvasConnectionNodeType(sourceNodeData?.nodeType)
    ? sourceNodeData?.nodeType
    : undefined
  const targetNodeType = isCanvasConnectionNodeType(targetNodeData?.nodeType)
    ? targetNodeData?.nodeType
    : undefined
  const sourceNodeRef = parseNodeRef(connection.source, sourceNodeType)
  const targetNodeRef = parseNodeRef(connection.target, targetNodeType)
  const sourceHandle = parseHandleRef(connection.sourceHandle)
  const targetHandle = parseHandleRef(connection.targetHandle)

  if (!sourceNodeRef || !targetNodeRef || !sourceHandle || !targetHandle) return null
  if (sourceHandle.direction !== 'out' || targetHandle.direction !== 'in') return null

  const sourcePort = resolveSourcePort(sourceNodeRef, sourceHandle)
  if (!sourcePort || !connection.target) return null
  const targetPort = resolveTargetPort(
    sourceNodeRef,
    sourcePort,
    targetNodeRef,
    connection.target,
    targetHandle,
    lookupData,
    existingEdges,
  )
  if (!targetPort) return null

  const lookupNode = createNodeLookup(lookupData)
  const from = { ...sourceNodeRef, port: sourcePort }
  const to = { ...targetNodeRef, port: targetPort }
  if (!isCanvasConnectionAllowed(from, to, lookupNode)) return null

  return {
    ...connection,
    sourceHandle: `out:${sourcePort}`,
    targetHandle: `in:${targetPort}`,
  }
}

export function flowConnectionToCanvasConnection(
  id: number,
  connection: Connection,
): CanvasConnection | null {
  const sourceNodeRef = parseNodeRef(connection.source)
  const targetNodeRef = parseNodeRef(connection.target)
  const sourceHandle = parseHandleRef(connection.sourceHandle)
  const targetHandle = parseHandleRef(connection.targetHandle)

  if (!sourceNodeRef || !targetNodeRef || !sourceHandle || !targetHandle) return null
  if (sourceHandle.direction !== 'out' || targetHandle.direction !== 'in') return null
  if (!sourceHandle.port || !targetHandle.port) return null

  return {
    id,
    fromNodeType: sourceNodeRef.nodeType,
    fromNodeId: sourceNodeRef.nodeId,
    fromPort: sourceHandle.port,
    toNodeType: targetNodeRef.nodeType,
    toNodeId: targetNodeRef.nodeId,
    toPort: targetHandle.port,
  }
}

export function isFlowConnectionAllowed(
  connection: Connection,
  lookupData: (nodeId: string) => FlowNodeData | undefined,
): boolean {
  return !!normalizeFlowConnection(connection, lookupData)
}
