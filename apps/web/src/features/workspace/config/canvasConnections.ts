import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  type CanvasConnectionContractNode,
} from './canvasNodeRegistry'
import type { CanvasConnectionNodeType, CanvasConnectionPort } from '../types/canvasNodes'

export type CanvasConnectionTone = 'image' | 'prompt'

export interface CanvasConnectionEndpointLike {
  nodeType: CanvasConnectionNodeType
  nodeId: number
  port: CanvasConnectionPort
}

export type CanvasConnectionNodeLookup = (
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
) => CanvasConnectionContractNode | null

export function resolveCanvasConnectionTone(port: CanvasConnectionPort): CanvasConnectionTone {
  if (port === 'prompt') return 'prompt'
  return 'image'
}

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
