import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
  type CanvasConnectionContractNode,
} from '../../../board/domain'

export interface CanvasConnectionEndpointLike {
  nodeType: CanvasConnectionNodeType
  nodeId: number
  port: CanvasConnectionPort
}

export type CanvasConnectionNodeLookup = (
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
) => CanvasConnectionContractNode | null

export function isCanvasConnectionAllowed(
  from: CanvasConnectionEndpointLike,
  to: CanvasConnectionEndpointLike,
  lookupNode: CanvasConnectionNodeLookup,
): boolean {
  if (from.nodeType === to.nodeType && from.nodeId === to.nodeId) return false

  if (!lookupNode(from.nodeType, from.nodeId) || !lookupNode(to.nodeType, to.nodeId)) return false

  const fromPort = getCanvasNodeVisibleOutputPorts(from.nodeType).find(
    (port) => port.port === from.port,
  )
  const toPort = getCanvasNodeVisibleInputPorts(to.nodeType).find((port) => port.port === to.port)
  if (!fromPort || !toPort) return false

  return fromPort.kind === toPort.kind
}
