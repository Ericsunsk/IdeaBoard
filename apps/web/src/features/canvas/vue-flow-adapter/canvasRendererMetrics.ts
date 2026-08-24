import type { Node } from '@vue-flow/core'

export type CanvasRendererPoint = {
  x: number
  y: number
}

export type CanvasRendererDimensions = {
  width: number
  height: number
}

export type CanvasRendererNodeMetrics = {
  nodeId: string
  position: CanvasRendererPoint
  dimensions: CanvasRendererDimensions
  computedPosition: CanvasRendererPoint | null
  measured: boolean
}

export type CanvasRendererMetricsPort = {
  readNodeMetrics: (nodeId: string) => CanvasRendererNodeMetrics | null
}

type NodeFinder = (nodeId: string) => Node | undefined

function readFinite(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function readPoint(value: unknown): CanvasRendererPoint | null {
  if (!value || typeof value !== 'object') return null
  const point = value as { x?: unknown; y?: unknown }
  const x = readFinite(point.x)
  const y = readFinite(point.y)
  return x === null || y === null ? null : { x, y }
}

function readDimensions(value: unknown): CanvasRendererDimensions {
  if (!value || typeof value !== 'object') return { width: 0, height: 0 }
  const dimensions = value as { width?: unknown; height?: unknown }
  return {
    width: readFinite(dimensions.width) ?? 0,
    height: readFinite(dimensions.height) ?? 0,
  }
}

/**
 * Adapts Vue Flow's renderer-owned node state to a small, explicit metrics
 * port. The returned `measured` flag is intentionally strict: a node with
 * missing or zero dimensions is not treated as measured by layout consumers.
 */
export function createCanvasVueFlowRendererMetricsPort(
  findNode: NodeFinder,
): CanvasRendererMetricsPort {
  return {
    readNodeMetrics(nodeId) {
      const node = findNode(nodeId)
      if (!node) return null

      const position = readPoint(node.position) ?? { x: 0, y: 0 }
      const computedPosition = readPoint(
        (node as Node & { computedPosition?: unknown }).computedPosition,
      )
      const dimensions = readDimensions(
        (node as Node & { dimensions?: unknown }).dimensions,
      )
      return {
        nodeId,
        position,
        dimensions,
        computedPosition,
        measured:
          computedPosition !== null && dimensions.width > 0 && dimensions.height > 0,
      }
    },
  }
}
