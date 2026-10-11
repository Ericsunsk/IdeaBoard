import type { Edge, Node } from '@vue-flow/core'
import {
  toCanvasGraphNodeId,
  type CanvasGraphNodeId,
  type CanvasGraphTransactionSummary,
} from '../../../core/graph'
import { resolveFlowNodeCanvasRef } from './flowSnapshotAdapter.ts'
import type { PendingImageExecutionLease } from '../../execution/pending-execution'

type CanvasGraphDeleteExecutionReleaseOptions = {
  summary: CanvasGraphTransactionSummary
  nodes: readonly Node[]
  edges: readonly Edge[]
  flowNodeIds?: readonly string[]
  captureActiveNodeExecutions: (nodeIds: readonly string[]) => PendingImageExecutionLease[]
  releaseNodeExecutions: (leases: readonly PendingImageExecutionLease[]) => void
}

function resolveGraphNodeIdForFlowNode(node: Pick<Node, 'id' | 'data'>): CanvasGraphNodeId | null {
  const ref = resolveFlowNodeCanvasRef(node)
  return ref ? toCanvasGraphNodeId(ref.nodeType, ref.id) : null
}

function resolveRemovedFlowNodeIds(
  nodes: readonly Node[],
  removedGraphNodeIds: readonly CanvasGraphNodeId[],
  explicitFlowNodeIds: readonly string[] = [],
): string[] {
  const removedGraphNodeIdSet = new Set(removedGraphNodeIds)
  const currentNodeIds = new Set(nodes.map((node) => node.id))
  const flowNodeIds = new Set(explicitFlowNodeIds)
  for (const node of nodes) {
    const graphNodeId = resolveGraphNodeIdForFlowNode(node)
    if (graphNodeId && removedGraphNodeIdSet.has(graphNodeId)) flowNodeIds.add(node.id)
  }
  for (const graphNodeId of removedGraphNodeIds) {
    if (currentNodeIds.has(graphNodeId)) flowNodeIds.add(graphNodeId)
  }
  return [...flowNodeIds]
}

function resolveImageTargetsForRemovedSources(
  nodes: readonly Node[],
  edges: readonly Edge[],
  removedFlowNodeIds: readonly string[],
): string[] {
  const removedNodeIds = new Set(removedFlowNodeIds)
  const nodesById = new Map(nodes.map((node) => [node.id, node]))
  return [
    ...new Set(
      edges.flatMap((edge) => {
        if (
          !removedNodeIds.has(edge.source) ||
          edge.sourceHandle !== 'out:image' ||
          edge.targetHandle !== 'in:image'
        ) {
          return []
        }
        return nodesById.get(edge.target)?.data?.kind === 'image' ? [edge.target] : []
      }),
    ),
  ]
}

/**
 * Flow projection is performed by the Session commit subscriber. Deletion
 * keeps only execution cleanup which depends on the pre-commit Flow view.
 */
export function releaseCanvasGraphDeleteExecutions(
  options: CanvasGraphDeleteExecutionReleaseOptions,
): boolean {
  if (!options.summary.didChange) return false
  const removedFlowNodeIds = resolveRemovedFlowNodeIds(
    options.nodes,
    options.summary.removedNodeIds,
    options.flowNodeIds,
  )
  const candidateSlotNodeIds = resolveImageTargetsForRemovedSources(
    options.nodes,
    options.edges,
    removedFlowNodeIds,
  )
  const leases = options.captureActiveNodeExecutions([
    ...new Set([...removedFlowNodeIds, ...candidateSlotNodeIds]),
  ])
  options.releaseNodeExecutions(leases)
  return true
}
