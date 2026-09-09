import type { Edge } from '@vue-flow/core'
import type { PendingImageExecutionSnapshot } from './core'
import { projectCanvasRenderEdges } from '../../vue-flow-adapter/canvasEdgeProjection'

export type PendingImageExecutionEdgeProjection = Readonly<{
  edges: Edge[]
  didChange: boolean
}>

const ACTIVE_STATUSES = new Set(['submitting', 'running'])

/**
 * Vue Flow is only a rendering projection: pending facts stay in the Core.
 */
export function projectPendingImageExecutionEdges(
  edges: Edge[],
  snapshot: PendingImageExecutionSnapshot,
): PendingImageExecutionEdgeProjection {
  const activeNodeIds = new Set(
    snapshot.executions
      .filter((execution) => ACTIVE_STATUSES.has(execution.status))
      .map((execution) => execution.resultNodeId),
  )
  const projectedEdges = projectCanvasRenderEdges(edges, (edge) =>
    activeNodeIds.has(edge.target as `image-${number}`),
  )
  const didChange = projectedEdges !== edges
  return { edges: didChange ? projectedEdges : edges, didChange }
}
