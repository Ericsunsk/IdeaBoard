import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  type CanvasConnectionContractNode,
} from './canvasNodeConnectionContract'
import type { CanvasConnectionNodeType, CanvasConnectionPort } from '../types/canvasNodes'

export type CanvasConnectionTone = 'image' | 'mask' | 'prompt'

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
  'model',
  'viewpoint',
  'color',
  'edit',
  'fusion',
  'enhance',
  'wear',
])

export function resolveCanvasConnectionTone(port: CanvasConnectionPort): CanvasConnectionTone {
  if (port === 'mask') return 'mask'
  if (port === 'prompt') return 'prompt'
  return 'image'
}

export function isCanvasConnectionAllowed(
  from: CanvasConnectionEndpointLike,
  to: CanvasConnectionEndpointLike,
  lookupNode: CanvasConnectionNodeLookup,
): boolean {
  if (from.nodeType === to.nodeType && from.nodeId === to.nodeId) return false

  const fromNode = lookupNode(from.nodeType, from.nodeId)
  const toNode = lookupNode(to.nodeType, to.nodeId)
  if (!fromNode || !toNode) return false

  const fromPort = getCanvasNodeVisibleOutputPorts(from.nodeType, fromNode).find(
    (port) => port.port === from.port,
  )
  const toPort = getCanvasNodeVisibleInputPorts(to.nodeType, toNode).find(
    (port) => port.port === to.port,
  )
  if (!fromPort || !toPort) return false

  if (from.nodeType === 'prompt') {
    return fromPort.kind === 'prompt' && toPort.kind === 'prompt'
  }

  if (from.nodeType === 'mask') {
    if (fromPort.kind !== 'mask') return false
    return toPort.kind === 'mask' || (to.nodeType === 'image' && to.port === 'image')
  }

  if (from.nodeType === 'image') {
    return fromPort.kind === 'image' && toPort.kind === 'image'
  }

  if (RELAY_IMAGE_OUTPUT_NODE_TYPES.has(from.nodeType)) {
    return fromPort.kind === 'image' && toPort.kind === 'image'
  }

  return false
}
