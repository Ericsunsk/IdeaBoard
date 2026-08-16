import type { Edge } from '@vue-flow/core'
import type { PendingImageExecutionSnapshot } from './PendingImageExecutionCore'

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
  let didChange = false
  const projectedEdges = edges.map((edge) => {
    if (edge.targetHandle !== 'in:image') return edge
    const animated = activeNodeIds.has(edge.target as `image-${number}`)
    if (edge.animated === animated) return edge
    didChange = true
    return { ...edge, animated }
  })
  return { edges: didChange ? projectedEdges : edges, didChange }
}
