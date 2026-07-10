import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import {
  toCanvasGraphEdgeId,
  toCanvasGraphNodeId,
  type CanvasGraphEdgeId,
  type CanvasGraphNodeId,
  type CanvasGraphTransactionSummary,
} from '../graph-runtime'
import { collectDeletedRunningTaskIds } from '../utils/flowDeletedRunningTasks'
import {
  resolveFlowEdgeCanvasConnectionId,
  resolveFlowNodeCanvasRef,
} from '../utils/flowSnapshotAdapter'
import { applyCanvasGraphCommitPolicy } from './canvasGraphCommitPolicy'
import type {
  CanvasVueFlowGraphAdapter,
  CanvasVueFlowGraphCommandResult,
} from './useCanvasVueFlowGraphAdapter'

type FlowGraphDeleteProjectionOptions = {
  graphAdapter: Pick<CanvasVueFlowGraphAdapter, 'applyGraphChanges'>
  nodes: Node[]
  edges: Edge[]
  summary: CanvasGraphTransactionSummary
  historyBefore?: unknown
  flowNodeIds?: readonly string[]
  projectionEdgeIds?: ReadonlyMap<Edge, CanvasGraphEdgeId | null>
  edgeRemoveChanges?: readonly EdgeChange[]
  onNativeRemove?: () => void
  stopDeletedRunningTasks?: (taskIds: string[]) => void
}

function resolveGraphNodeIdForFlowNode(node: Pick<Node, 'id' | 'data'>): CanvasGraphNodeId | null {
  const ref = resolveFlowNodeCanvasRef(node)
  return ref ? toCanvasGraphNodeId(ref.nodeType, ref.id) : null
}

function resolveGraphEdgeIdForFlowEdge(
  edge: Pick<Edge, 'id' | 'data'>,
  projectionEdgeIds?: ReadonlyMap<Edge, CanvasGraphEdgeId | null>,
): CanvasGraphEdgeId | null {
  const projectionEdgeId = projectionEdgeIds?.get(edge as Edge)
  if (projectionEdgeId !== undefined) return projectionEdgeId
  const connectionId = resolveFlowEdgeCanvasConnectionId(edge)
  return connectionId === null ? null : toCanvasGraphEdgeId(connectionId)
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
    if (graphNodeId && removedGraphNodeIdSet.has(graphNodeId)) {
      flowNodeIds.add(node.id)
    }
  }

  for (const graphNodeId of removedGraphNodeIdSet) {
    if (currentNodeIds.has(graphNodeId)) flowNodeIds.add(graphNodeId)
  }

  return Array.from(flowNodeIds)
}

function createNodeRemoveChanges(flowNodeIds: readonly string[]): NodeChange[] {
  return flowNodeIds.map((nodeId) => ({ id: nodeId, type: 'remove' }) as NodeChange)
}

function createEdgeRemoveChange(edge: Edge): EdgeChange {
  return {
    id: edge.id,
    type: 'remove',
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourceHandle ?? null,
    targetHandle: edge.targetHandle ?? null,
  } as EdgeChange
}

function createEdgeRemoveChanges(options: {
  edges: readonly Edge[]
  removedGraphEdgeIds: readonly CanvasGraphEdgeId[]
  removedFlowNodeIds: readonly string[]
  projectionEdgeIds?: ReadonlyMap<Edge, CanvasGraphEdgeId | null>
  edgeRemoveChanges?: readonly EdgeChange[]
}): EdgeChange[] {
  const removedGraphEdgeIds = new Set(options.removedGraphEdgeIds)
  const removedFlowNodeIds = new Set(options.removedFlowNodeIds)
  const edgeChanges: EdgeChange[] = []
  const addedEdgeIds = new Set<string>()

  function append(change: EdgeChange) {
    if (change.type !== 'remove') return
    if (addedEdgeIds.has(change.id)) return
    addedEdgeIds.add(change.id)
    edgeChanges.push(change)
  }

  options.edgeRemoveChanges?.forEach(append)

  for (const edge of options.edges) {
    const graphEdgeId = resolveGraphEdgeIdForFlowEdge(edge, options.projectionEdgeIds)
    const isRemovedDomainEdge = graphEdgeId ? removedGraphEdgeIds.has(graphEdgeId) : false
    const isConnectedToRemovedNode =
      removedFlowNodeIds.has(edge.source) || removedFlowNodeIds.has(edge.target)
    if (isRemovedDomainEdge || isConnectedToRemovedNode) {
      append(createEdgeRemoveChange(edge))
    }
  }

  return edgeChanges
}

export function applyFlowGraphDeleteProjection(
  options: FlowGraphDeleteProjectionOptions,
): CanvasVueFlowGraphCommandResult | null {
  if (!options.summary.didChange) return null

  const removedFlowNodeIds = resolveRemovedFlowNodeIds(
    options.nodes,
    options.summary.removedNodeIds,
    options.flowNodeIds,
  )
  const runningTaskIds = collectDeletedRunningTaskIds(options.nodes, removedFlowNodeIds)
  const result = applyCanvasGraphCommitPolicy(
    {
      summary: options.summary,
      historyBefore: options.historyBefore,
    },
    (commandOptions) =>
      options.graphAdapter.applyGraphChanges(
        {
          nodes: createNodeRemoveChanges(removedFlowNodeIds),
          edges: createEdgeRemoveChanges({
            edges: options.edges,
            removedGraphEdgeIds: options.summary.removedEdgeIds,
            removedFlowNodeIds,
            projectionEdgeIds: options.projectionEdgeIds,
            edgeRemoveChanges: options.edgeRemoveChanges,
          }),
        },
        commandOptions,
      ),
  )

  if (result.didChange) {
    options.onNativeRemove?.()
    if (runningTaskIds.length > 0) {
      options.stopDeletedRunningTasks?.(runningTaskIds)
    }
  }

  return result
}
