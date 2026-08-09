import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import {
  commitCanvasGraphDeleteIntent,
  isCanvasGraphNodeId,
  type CanvasGraphEdgeId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
} from '../graph-runtime'
import {
  createFlowEdgeIdentityFromRemoveChange,
  createFlowEdgeProjectionIndex,
  type FlowEdgeIdentity,
  type FlowEdgeProjectionIndex,
} from './flowConnectionIdentity'
import { applyFlowGraphDeleteProjection } from './flowGraphDeleteProjection'
import type { CanvasVueFlowGraphAdapter } from '../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'

type EdgeRemoveChange = Extract<EdgeChange, { type: 'remove' }>

type ResolvedFlowNodeChanges = {
  graphNodeIds: CanvasGraphNodeId[]
  flowNodeIds: string[]
}

export type CanvasDeleteCommandLifecycleOptions<HistorySnapshot> = {
  readNodes: () => Node[]
  readEdges: () => Edge[]
  readBoardBaseSnapshot: () => CanvasBoardSnapshot
  createGraphStore: () => CanvasGraphStore
  graphAdapter: Pick<CanvasVueFlowGraphAdapter, 'applyGraphChanges'>
  captureHistory: () => HistorySnapshot
  commitGroups: (groups: CanvasBoardSnapshot['groups']) => void
  resolveRemovedFlowNodes: (changes: readonly NodeChange[]) => ResolvedFlowNodeChanges | null
  scheduleMicrotask: (callback: () => void) => void
  onWarning: (message: string) => void
  onNativeRemove?: () => void
  stopDeletedRunningTasks?: (taskIds: string[]) => void
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

export function createCanvasDeleteCommandLifecycle<HistorySnapshot>(
  options: CanvasDeleteCommandLifecycleOptions<HistorySnapshot>,
): CanvasDeleteCommandLifecycle {
  let pendingHistory: { before: HistorySnapshot } | null = null
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

  function applyDelete(command: {
    store: CanvasGraphStore
    nodeIds: CanvasGraphNodeId[]
    edgeIds?: CanvasGraphEdgeId[]
    flowNodeIds: string[]
    edgeRemoveChanges?: EdgeChange[]
    historyBefore: HistorySnapshot
    projectionIndex?: FlowEdgeProjectionIndex
    acceptNoop?: boolean
  }): boolean {
    const projectionIndex =
      command.projectionIndex ?? createFlowEdgeProjectionIndex(command.store, options.readEdges())
    const summary = commitCanvasGraphDeleteIntent(command.store, {
      nodeIds: command.nodeIds,
      edgeIds: command.edgeIds,
      removeConnectedEdges: true,
    })
    if (!summary.didChange) return command.acceptNoop ?? false

    options.commitGroups(command.store.exportSnapshot(options.readBoardBaseSnapshot()).groups)
    const result = applyFlowGraphDeleteProjection({
      graphAdapter: options.graphAdapter,
      nodes: options.readNodes(),
      edges: options.readEdges(),
      commitPolicy: { summary, historyBefore: command.historyBefore },
      flowNodeIds: command.flowNodeIds,
      projectionIndex,
      edgeRemoveChanges: command.edgeRemoveChanges,
      onNativeRemove: options.onNativeRemove,
      stopDeletedRunningTasks: options.stopDeletedRunningTasks,
    })
    return result?.didChange ?? false
  }

  function deleteNode(nodeId: string): boolean {
    if (!isCanvasGraphNodeId(nodeId)) {
      options.onWarning('删除操作未能写入画布，请刷新后重试')
      return false
    }
    const store = options.createGraphStore()
    if (!store.getNode(nodeId)) {
      options.onWarning('删除操作未能写入画布，请刷新后重试')
      return false
    }
    try {
      return applyDelete({
        store,
        nodeIds: [nodeId],
        flowNodeIds: [nodeId],
        historyBefore: options.captureHistory(),
      })
    } catch (error) {
      console.error('[Canvas] Failed to commit node delete transaction', error)
      options.onWarning('删除操作未能写入画布，请刷新后重试')
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

  function commitNativeRemove(
    capturedHistory: HistorySnapshot,
    nodeChanges: NodeChange[],
    edgeChanges: EdgeChange[],
  ): boolean {
    const removedNodes = options.resolveRemovedFlowNodes(nodeChanges)
    if (!removedNodes) return false
    const removedNodeIds = new Set<string>([
      ...removedNodes.flowNodeIds,
      ...removedNodes.graphNodeIds,
    ])
    const store = options.createGraphStore()
    const projectionIndex = createFlowEdgeProjectionIndex(store, options.readEdges())
    const edgeIds = resolveNativeRemovedEdgeIds(projectionIndex, edgeChanges, removedNodeIds)
    if (!edgeIds) return false

    return applyDelete({
      store,
      nodeIds: removedNodes.graphNodeIds,
      edgeIds,
      flowNodeIds: removedNodes.flowNodeIds,
      edgeRemoveChanges: edgeChanges,
      historyBefore: capturedHistory,
      projectionIndex,
      acceptNoop: true,
    })
  }

  function commitPendingNativeRemove() {
    isCommitScheduled = false
    const capturedHistory = pendingHistory
    const nodeChanges = dedupeRemoveChanges(pendingNodeChanges)
    const edgeChanges = dedupeRemoveChanges(pendingEdgeChanges)
    pendingHistory = null
    pendingNodeChanges = []
    pendingEdgeChanges = []
    if (!capturedHistory || !hasApplicableRemoveChanges(nodeChanges, edgeChanges)) {
      return
    }
    try {
      if (commitNativeRemove(capturedHistory.before, nodeChanges, edgeChanges)) return
    } catch (error) {
      console.error('[Canvas] Failed to commit native delete transaction', error)
    }
    options.onWarning('删除操作未能应用，请刷新画布后重试')
  }

  function scheduleCommit() {
    if (!pendingHistory) pendingHistory = { before: options.captureHistory() }
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
