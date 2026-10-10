import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { i18n } from '../../../../core/i18n'
import {
  commitCanvasGraphDeleteIntent,
  isCanvasGraphNodeId,
  type CanvasGraphEdgeId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
} from '../../domain/graph'
import {
  createFlowEdgeIdentityFromRemoveChange,
  createFlowEdgeProjectionIndex,
  type FlowEdgeIdentity,
  type FlowEdgeProjectionIndex,
} from './flowConnectionIdentity'
import { releaseCanvasGraphDeleteExecutions } from './flowGraphDeleteProjection'
import type { PendingImageExecutionLease } from '../../application/execution/pending-execution'

type EdgeRemoveChange = Extract<EdgeChange, { type: 'remove' }>

type ResolvedFlowNodeChanges = {
  graphNodeIds: CanvasGraphNodeId[]
  flowNodeIds: string[]
}

export type CanvasDeleteCommandLifecycleOptions = {
  readNodes: () => Node[]
  readEdges: () => Edge[]
  readGraphStore: () => CanvasGraphStore
  resolveRemovedFlowNodes: (changes: readonly NodeChange[]) => ResolvedFlowNodeChanges | null
  scheduleMicrotask: (callback: () => void) => void
  onWarning: (message: string) => void
  captureActiveNodeExecutions: (nodeIds: readonly string[]) => PendingImageExecutionLease[]
  releaseNodeExecutions: (leases: readonly PendingImageExecutionLease[]) => void
}

export type CanvasDeleteCommandLifecycle = {
  deleteNode: (nodeId: string) => boolean
  handleNodeChanges: (changes: NodeChange[]) => boolean
  handleEdgeChanges: (changes: EdgeChange[]) => boolean
}

function hasRemoveChange(changes: Array<NodeChange | EdgeChange>): boolean {
  return changes.some((change) => change.type === 'remove')
}

function dedupeRemoveChanges<T extends NodeChange | EdgeChange>(changes: T[]): T[] {
  const removedIds = new Set<string>()
  return changes.filter((change) => {
    if (change.type !== 'remove') return true
    if (removedIds.has(change.id)) return false
    removedIds.add(change.id)
    return true
  })
}

export function createCanvasDeleteCommandLifecycle(
  options: CanvasDeleteCommandLifecycleOptions,
): CanvasDeleteCommandLifecycle {
  let isCommitScheduled = false
  let pendingNodeChanges: NodeChange[] = []
  let pendingEdgeChanges: EdgeChange[] = []

  function resolveNativeRemovedEdgeIds(
    projectionIndex: FlowEdgeProjectionIndex,
    edgeChanges: EdgeChange[],
    removedNodeIds: Set<string>,
  ): CanvasGraphEdgeId[] | null {
    const edgeIds: CanvasGraphEdgeId[] = []
    for (const change of edgeChanges) {
      if (change.type !== 'remove') continue
      const edge = projectionIndex.getFlowEdge(change.id)
      if (edge && (removedNodeIds.has(edge.source) || removedNodeIds.has(edge.target))) continue
      const edgeIdentity: FlowEdgeIdentity =
        edge ?? createFlowEdgeIdentityFromRemoveChange(change as EdgeRemoveChange)
      const edgeId = projectionIndex.resolveCanvasEdgeId(edgeIdentity)
      if (!edgeId) return null
      edgeIds.push(edgeId)
    }
    return Array.from(new Set(edgeIds))
  }

  function collectPendingResultNodeIds(sourceFlowNodeIds: readonly string[]): CanvasGraphNodeId[] {
    // Live execution state comes from the PendingImageExecutionCore lease, never
    // from snapshot running fields (those are hydration-only facts). Candidates
    // are pure topology: image-kind targets fed by a deleted source.
    const sourceIds = new Set(sourceFlowNodeIds)
    const store = options.readGraphStore()
    const candidateIds = new Set<CanvasGraphNodeId>()
    for (const edge of options.readEdges()) {
      if (!sourceIds.has(edge.source) || edge.targetHandle !== 'in:image') continue
      if (!isCanvasGraphNodeId(edge.target)) continue
      if (store.getNode(edge.target)?.kind !== 'image') continue
      candidateIds.add(edge.target)
    }
    if (candidateIds.size === 0) return []
    const activeIds = new Set<string>(
      options.captureActiveNodeExecutions([...candidateIds]).map((lease) => lease.resultNodeId),
    )
    return [...candidateIds].filter((nodeId) => activeIds.has(nodeId))
  }

  function applyDelete(command: {
    store: CanvasGraphStore
    nodeIds: CanvasGraphNodeId[]
    edgeIds?: CanvasGraphEdgeId[]
    flowNodeIds: string[]
    acceptNoop?: boolean
  }): boolean {
    const pendingResultNodeIds = collectPendingResultNodeIds(command.flowNodeIds)
    const nodeIds = [...new Set([...command.nodeIds, ...pendingResultNodeIds])]
    const summary = commitCanvasGraphDeleteIntent(command.store, {
      nodeIds,
      edgeIds: command.edgeIds,
      removeConnectedEdges: true,
    })
    if (!summary.didChange) return command.acceptNoop ?? false

    releaseCanvasGraphDeleteExecutions({
      summary,
      nodes: options.readNodes(),
      edges: options.readEdges(),
      flowNodeIds: [...command.flowNodeIds, ...pendingResultNodeIds],
      captureActiveNodeExecutions: options.captureActiveNodeExecutions,
      releaseNodeExecutions: options.releaseNodeExecutions,
    })
    return true
  }

  function deleteNode(nodeId: string): boolean {
    if (!isCanvasGraphNodeId(nodeId)) {
      options.onWarning(i18n.global.t('canvas.edges.deleteWriteFailedWarning'))
      return false
    }
    const store = options.readGraphStore()
    if (!store.getNode(nodeId)) {
      options.onWarning(i18n.global.t('canvas.edges.deleteWriteFailedWarning'))
      return false
    }
    try {
      return applyDelete({
        store,
        nodeIds: [nodeId],
        flowNodeIds: [nodeId],
      })
    } catch (error) {
      console.error('[Canvas] Failed to commit node delete transaction', error)
      options.onWarning(i18n.global.t('canvas.edges.deleteWriteFailedWarning'))
      return false
    }
  }

  function hasApplicableRemoveChanges(nodeChanges: NodeChange[], edgeChanges: EdgeChange[]) {
    const currentNodeIds = new Set(options.readNodes().map((node) => node.id))
    const currentEdgeIds = new Set(options.readEdges().map((edge) => edge.id))
    return (
      nodeChanges.some((change) => change.type === 'remove' && currentNodeIds.has(change.id)) ||
      edgeChanges.some((change) => change.type === 'remove' && currentEdgeIds.has(change.id))
    )
  }

  function commitNativeRemove(nodeChanges: NodeChange[], edgeChanges: EdgeChange[]): boolean {
    const removedNodes = options.resolveRemovedFlowNodes(nodeChanges)
    if (!removedNodes) return false
    const removedNodeIds = new Set<string>([
      ...removedNodes.flowNodeIds,
      ...removedNodes.graphNodeIds,
    ])
    const store = options.readGraphStore()
    const projectionIndex = createFlowEdgeProjectionIndex(store, options.readEdges())
    const edgeIds = resolveNativeRemovedEdgeIds(projectionIndex, edgeChanges, removedNodeIds)
    if (!edgeIds) return false

    return applyDelete({
      store,
      nodeIds: removedNodes.graphNodeIds,
      edgeIds,
      flowNodeIds: removedNodes.flowNodeIds,
      acceptNoop: true,
    })
  }

  function commitPendingNativeRemove() {
    isCommitScheduled = false
    const nodeChanges = dedupeRemoveChanges(pendingNodeChanges)
    const edgeChanges = dedupeRemoveChanges(pendingEdgeChanges)
    pendingNodeChanges = []
    pendingEdgeChanges = []
    if (!hasApplicableRemoveChanges(nodeChanges, edgeChanges)) {
      return
    }
    try {
      if (commitNativeRemove(nodeChanges, edgeChanges)) return
    } catch (error) {
      console.error('[Canvas] Failed to commit native delete transaction', error)
    }
    options.onWarning(i18n.global.t('canvas.edges.deleteApplyFailedWarning'))
  }

  function scheduleCommit() {
    if (isCommitScheduled) return
    isCommitScheduled = true
    options.scheduleMicrotask(() => {
      options.scheduleMicrotask(commitPendingNativeRemove)
    })
  }

  function handleNodeChanges(changes: NodeChange[]): boolean {
    if (!hasRemoveChange(changes)) return false
    pendingNodeChanges = pendingNodeChanges.concat(changes)
    scheduleCommit()
    return true
  }

  function handleEdgeChanges(changes: EdgeChange[]): boolean {
    if (!hasRemoveChange(changes)) return false
    pendingEdgeChanges = pendingEdgeChanges.concat(changes)
    scheduleCommit()
    return true
  }

  return {
    deleteNode,
    handleNodeChanges,
    handleEdgeChanges,
  }
}
