import type { Connection } from '@vue-flow/core'
import { isCanvasConnectionAllowed } from '../../../core/nodes/canvasConnections.ts'
import type { FlowNodeData } from '../../../core/nodes/flowTypes.ts'
import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  isCanvasConnectionNodeType,
  type CanvasConnection,
  type CanvasConnectionEndpoint,
  type CanvasConnectionNodeKind,
  type CanvasConnectionPort,
  type CanvasConnectionContractNode,
  type CanvasNodePortContract,
} from '../../../../board/ports/canvas'

const AUTO_HANDLE_PORT = '__auto'

interface ParsedNodeRef {
  nodeType: CanvasConnectionNodeKind
  nodeId: number
}

interface ParsedHandleRef {
  direction: 'in' | 'out'
  port: CanvasConnectionPort | null
  shouldAuto: boolean
}

type FlowEdgeLike = {
  id?: string
  target?: string
  targetHandle?: string | null
}

function parseNodeRef(
  nodeId: string | null | undefined,
  fallbackType?: CanvasConnectionNodeKind,
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
  if (rawPort === AUTO_HANDLE_PORT) return { direction, port: null, shouldAuto: true }
  const port = rawPort as CanvasConnectionPort
  return { direction, port, shouldAuto: false }
}

function buildContractNode(
  nodeType: CanvasConnectionNodeKind,
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
    nodeType: CanvasConnectionNodeKind,
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
  if (visibleOutputPortContracts.length !== 1) return null
  const singlePort = visibleOutputPortContracts[0]
  return singlePort ? singlePort.port : null
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
  const openPort =
    allowedPorts.find((targetPort) => !usedPorts.has(targetPort.port)) ?? allowedPorts[0]
  if (!openPort) return null
  return openPort.port
}

type ParsedFlowConnectionRefs = {
  sourceNodeRef: ParsedNodeRef
  targetNodeRef: ParsedNodeRef
  sourceHandle: ParsedHandleRef
  targetHandle: ParsedHandleRef
}

function parseFlowConnectionRefs(
  connection: Connection,
  lookupData: (nodeId: string) => FlowNodeData | undefined,
): ParsedFlowConnectionRefs | null {
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
  return { sourceNodeRef, targetNodeRef, sourceHandle, targetHandle }
}

export function normalizeFlowConnection(
  connection: Connection,
  lookupData: (nodeId: string) => FlowNodeData | undefined,
  existingEdges: readonly FlowEdgeLike[] = [],
): Connection | null {
  const refs = parseFlowConnectionRefs(connection, lookupData)
  if (!refs) return null
  const { sourceNodeRef, targetNodeRef, sourceHandle, targetHandle } = refs

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

export function flowConnectionToCanvasConnectionEndpoint(
  connection: Connection,
): CanvasConnectionEndpoint | null {
  const sourceNodeRef = parseNodeRef(connection.source)
  const targetNodeRef = parseNodeRef(connection.target)
  const sourceHandle = parseHandleRef(connection.sourceHandle)
  const targetHandle = parseHandleRef(connection.targetHandle)

  if (!sourceNodeRef || !targetNodeRef || !sourceHandle || !targetHandle) return null
  if (sourceHandle.direction !== 'out' || targetHandle.direction !== 'in') return null
  if (!sourceHandle.port || !targetHandle.port) return null

  return {
    fromNodeType: sourceNodeRef.nodeType,
    fromNodeId: sourceNodeRef.nodeId,
    fromPort: sourceHandle.port,
    toNodeType: targetNodeRef.nodeType,
    toNodeId: targetNodeRef.nodeId,
    toPort: targetHandle.port,
  }
}

export function flowConnectionToCanvasConnection(
  id: number,
  connection: Connection,
): CanvasConnection | null {
  const endpoint = flowConnectionToCanvasConnectionEndpoint(connection)
  return endpoint ? { ...endpoint, id } : null
}

export function isFlowConnectionAllowed(
  connection: Connection,
  lookupData: (nodeId: string) => FlowNodeData | undefined,
): boolean {
  return !!normalizeFlowConnection(connection, lookupData)
}
