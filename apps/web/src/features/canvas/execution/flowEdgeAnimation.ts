import type { Edge } from '@vue-flow/core'
import { createFlowGraphCommandOptions } from '../composables/flowGraphAdapterCommandSideEffects'
import type { CanvasVueFlowGraphAdapter } from '../composables/useCanvasVueFlowGraphAdapter'

type SyncImageInputEdgeAnimationOptions = {
  edges: Edge[]
  graphAdapter: Pick<CanvasVueFlowGraphAdapter, 'replaceEdges'>
  targetNodeIds: Iterable<string>
  shouldAnimate: boolean
}

export function resolveImageInputEdgeAnimation(
  edges: Edge[],
  targetNodeIds: Iterable<string>,
  shouldAnimate: boolean,
): { edges: Edge[]; didChange: boolean } {
  const targetNodeIdSet = new Set(targetNodeIds)
  let didChange = false
  const nextEdges = edges.map((edge) => {
    if (!targetNodeIdSet.has(edge.target) || edge.targetHandle !== 'in:image') return edge
    if (edge.animated === shouldAnimate) return edge
    didChange = true
    return { ...edge, animated: shouldAnimate }
  })
  return { edges: didChange ? nextEdges : edges, didChange }
}

export function syncImageInputEdgeAnimation(options: SyncImageInputEdgeAnimationOptions): boolean {
  const result = resolveImageInputEdgeAnimation(
    options.edges,
    options.targetNodeIds,
    options.shouldAnimate,
  )
  if (!result.didChange) return false
  options.graphAdapter.replaceEdges(
    result.edges,
    createFlowGraphCommandOptions({ recordHistory: false, markDirty: false }),
  )
  return true
}
