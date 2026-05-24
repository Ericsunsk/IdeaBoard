import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  type CanvasConnectionContractNode,
} from './canvasNodeConnectionContract'
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

const RELAY_IMAGE_OUTPUT_NODE_TYPES = new Set<CanvasConnectionNodeType>([
  'viewpoint',
  'fusion',
])

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
  const toPort = getCanvasNodeVisibleInputPorts(to.nodeType).find(
    (port) => port.port === to.port,
  )
  if (!fromPort || !toPort) return false

  if (from.nodeType === 'prompt') {
    return fromPort.kind === toPort.kind
  }

  if (from.nodeType === 'image') {
    return fromPort.kind === 'image' && toPort.kind === 'image'
  }

  if (RELAY_IMAGE_OUTPUT_NODE_TYPES.has(from.nodeType)) {
    return fromPort.kind === 'image' && toPort.kind === 'image'
  }

  return false
}
