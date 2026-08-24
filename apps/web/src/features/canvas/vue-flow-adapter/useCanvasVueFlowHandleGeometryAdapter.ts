import { useVueFlow } from '@vue-flow/core'
import { createCanvasVueFlowRendererMetricsPort } from './canvasRendererMetrics'
import type {
  CanvasHandleProximityDirection,
  CanvasHandleProximityGeometry,
  CanvasHandleProximityPoint,
} from './canvasHandleProximityCoordinator'

type CanvasSurfaceOrigin = CanvasHandleProximityPoint | null

function isUsableCoordinate(value: number | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function resolveAnchorOffset(
  direction: CanvasHandleProximityDirection,
  width: number,
  height: number,
): CanvasHandleProximityPoint {
  return {
    x: direction === 'output' ? width : 0,
    y: height / 2,
  }
}

/**
 * Resolves the aggregate Handle anchor from Vue Flow's node projection.
 *
 * The canvas surface rectangle is read once per coordinator evaluation. Each Handle then uses
 * the already-maintained node dimensions, computed position, and viewport transform only.
 */
export function useCanvasVueFlowHandleGeometryAdapter(): CanvasHandleProximityGeometry {
  const { findNode, viewport, vueFlowRef } = useVueFlow()
  const rendererMetrics = createCanvasVueFlowRendererMetricsPort(findNode)
  let surfaceOrigin: CanvasSurfaceOrigin = null

  function prepare() {
    const surface = vueFlowRef.value
    if (!surface) {
      surfaceOrigin = null
      return
    }

    const bounds = surface.getBoundingClientRect()
    surfaceOrigin = { x: bounds.left, y: bounds.top }
  }

  function resolveAnchor(
    nodeId: string,
    direction: CanvasHandleProximityDirection,
  ): CanvasHandleProximityPoint | null {
    const origin = surfaceOrigin
    const metrics = rendererMetrics.readNodeMetrics(nodeId)
    if (!origin || !metrics || !metrics.measured || !metrics.computedPosition) return null

    const { width, height } = metrics.dimensions
    const { x, y } = metrics.computedPosition
    const activeViewport = viewport.value
    if (
      !isUsableCoordinate(width) ||
      !isUsableCoordinate(height) ||
      width <= 0 ||
      height <= 0 ||
      !isUsableCoordinate(x) ||
      !isUsableCoordinate(y) ||
      !isUsableCoordinate(activeViewport.x) ||
      !isUsableCoordinate(activeViewport.y) ||
      !isUsableCoordinate(activeViewport.zoom) ||
      activeViewport.zoom <= 0
    ) {
      return null
    }

    const offset = resolveAnchorOffset(direction, width, height)
    return {
      x: origin.x + activeViewport.x + (x + offset.x) * activeViewport.zoom,
      y: origin.y + activeViewport.y + (y + offset.y) * activeViewport.zoom,
    }
  }

  return {
    prepare,
    resolveAnchor,
  }
}
