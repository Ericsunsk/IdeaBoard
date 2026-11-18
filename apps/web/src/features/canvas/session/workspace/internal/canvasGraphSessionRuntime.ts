import type { Edge, Node } from '@vue-flow/core'
import { onScopeDispose, type Ref } from 'vue'
import type { BoardSnapshot } from '../../../../board/ports/canvas'
import type { BoardSession } from '../../board-session'
import type { CanvasGraphHistory } from '../../graph-editing'
import type { CanvasVueFlowGraphAdapter } from '../../../session/flow-adapter'
import { bindCanvasGraphSessionProjection } from '../../../session/flow-adapter'
import { boardSnapshotToFlow, projectCanvasNodeReplacement } from '../../../session/flow-adapter'
import type { CanvasGraphStore } from '../../../core/graph'
import { CanvasGraphSession } from '../../board-session'
import { canvasGraphStoreTelemetryProbe } from '../../graph-editing'
import { BoardGraphSession, type PendingImageExecutionCore } from '../../execution'

type CanvasGraphSessionRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  boardBaseSnapshot: Ref<BoardSnapshot>
  boardSession: BoardSession
  graphAdapter: CanvasVueFlowGraphAdapter
  executionCore: PendingImageExecutionCore
  hydratePendingImageLeases?: ((snapshot: BoardSnapshot) => void) | undefined
  history: CanvasGraphHistory
  markDirty: () => void
  isNodeExecutionActive: (nodeId: string) => boolean
  preserveCurrentSelection: (graph: { nodes: Node[]; edges: Edge[] }) => {
    nodes: Node[]
    edges: Edge[]
  }
  projectCurrentGroupNodeLayers: (nodes: Node[]) => Node[]
  projectCommittedGroupNodeLayer: (node: Node) => Node
  onGroupsChanged: (groups: BoardSnapshot['groups']) => void
  scheduleMicrotask?: ((callback: () => void) => void) | undefined
  onLocalSnapshot?:
    | ((snapshot: BoardSnapshot, nodeIds: readonly string[], edgeIds: readonly string[]) => void)
    | undefined
  onLocalEdit?: (() => void) | undefined
  canWriteGraph?: (() => boolean) | undefined
}

export type CanvasGraphSessionRuntime = {
  isAttached: () => boolean
  readCanvasGraphStore: () => CanvasGraphStore
  replaceGraphSnapshot: (snapshot: BoardSnapshot, reason: 'hydrate' | 'save-reconcile') => void
  applyRemoteSnapshot: (snapshot: BoardSnapshot) => void
  exportGraphSnapshot: (baseSnapshot: BoardSnapshot) => BoardSnapshot
  commitGraph: (
    graph: { nodes: Node[]; edges: Edge[] },
    source: 'hydrate-replace' | 'save-reconcile',
  ) => void
  dispose: () => void
}

function createFlowEdgeEndpointKey(edge: Edge): string {
  return JSON.stringify([
    edge.source,
    edge.sourceHandle ?? null,
    edge.target,
    edge.targetHandle ?? null,
  ])
}

export function createCanvasGraphSessionRuntime(
  options: CanvasGraphSessionRuntimeOptions,
): CanvasGraphSessionRuntime {
  let graphSession: CanvasGraphSession | null = null
  let unsubscribeGraphSessionCommits: (() => void) | null = null

  function dispose() {
    unsubscribeGraphSessionCommits?.()
    unsubscribeGraphSessionCommits = null
    graphSession?.dispose()
    graphSession = null
    // Drop undo/redo stacks synchronously with the session so a mid-switch
    // undo can neither throw on the disposed session nor apply a
    // cross-board patch. The history module additionally guards with
    // isAttached() and board-id stamps as defense in depth.
    options.history.clearBaseline()
  }

  function requireGraphSession(): CanvasGraphSession {
    if (!isAttached()) {
      throw new Error('Canvas Graph Session is not attached to the current Board')
    }
    return graphSession!
  }

  function isAttached(): boolean {
    const boardId = options.boardSession.currentBoardId.value
    return !!graphSession && !!boardId && graphSession.boardId === boardId
  }

  function readCanvasGraphStore(): CanvasGraphStore {
    return requireGraphSession().store
  }

  function rebaseSavedFlowProjection(savedGraph: { nodes: Node[]; edges: Edge[] }) {
    const savedNodesById = new Map(savedGraph.nodes.map((node) => [node.id, node] as const))
    const savedEdgesById = new Map(savedGraph.edges.map((edge) => [edge.id, edge] as const))
    const savedEdgeEndpointKeys = new Set(savedGraph.edges.map(createFlowEdgeEndpointKey))
    return {
      nodes: [
        ...projectCanvasNodeReplacement({
          current: isAttached() ? options.nodes.value : null,
          next: savedGraph.nodes,
        }),
        ...options.nodes.value.filter(
          (node) => !savedNodesById.has(node.id) && options.isNodeExecutionActive(node.id),
        ),
      ],
      edges: [
        ...savedGraph.edges,
        ...options.edges.value.filter(
          (edge) =>
            !savedEdgesById.has(edge.id) &&
            !savedEdgeEndpointKeys.has(createFlowEdgeEndpointKey(edge)) &&
            options.isNodeExecutionActive(edge.target),
        ),
      ],
    }
  }

  function commitGraph(
    nextGraph: { nodes: Node[]; edges: Edge[] },
    source: 'hydrate-replace' | 'save-reconcile',
  ) {
    const semanticGraph =
      source === 'save-reconcile' ? rebaseSavedFlowProjection(nextGraph) : nextGraph
    const projectedGraph = {
      nodes: options.projectCurrentGroupNodeLayers(semanticGraph.nodes),
      edges: semanticGraph.edges,
    }
    const resolvedGraph =
      source === 'save-reconcile'
        ? options.preserveCurrentSelection(projectedGraph)
        : projectedGraph
    options.graphAdapter.replaceGraph(resolvedGraph)
  }

  function replaceGraphSnapshot(snapshot: BoardSnapshot, reason: 'hydrate' | 'save-reconcile') {
    const boardId = options.boardSession.currentBoardId.value
    if (!boardId) {
      dispose()
      options.executionCore.detachBoard()
      return
    }

    if (reason === 'save-reconcile' && graphSession?.boardId === boardId) {
      graphSession.replace(snapshot)
      return
    }

    dispose()
    const session = CanvasGraphSession.hydrate(boardId, snapshot, canvasGraphStoreTelemetryProbe)
    session.store.setDomainWritePolicy(options.canWriteGraph ?? (() => true))
    graphSession = session
    const unsubscribeHistory = session.subscribe((event) => {
      if (session !== graphSession || session.boardId !== options.boardSession.currentBoardId.value)
        return
      if (event.sideEffects.shouldMarkDirty) {
        if (event.summary.origin === 'user') options.onLocalEdit?.()
        options.onLocalSnapshot?.(
          session.exportSnapshot(options.boardBaseSnapshot.value),
          [
            ...event.summary.changedNodeIds,
            ...event.summary.removedNodeIds,
            ...event.summary.changedGroupIds,
            ...event.summary.removedGroupIds,
          ],
          [...event.summary.changedEdgeIds, ...event.summary.removedEdgeIds],
        )
      }
      if (event.sideEffects.shouldRecordHistory) options.history.record(event.patch)
      if (event.sideEffects.shouldMarkDirty) options.markDirty()
    })
    const unsubscribeProjection = bindCanvasGraphSessionProjection({
      session,
      graphAdapter: options.graphAdapter,
      readCurrent: () => ({ nodes: options.nodes.value, edges: options.edges.value }),
      projectFull: () => {
        const flow = boardSnapshotToFlow(session.exportSnapshot(options.boardBaseSnapshot.value))
        return {
          nodes: options.projectCurrentGroupNodeLayers(
            projectCanvasNodeReplacement({
              current: isAttached() ? options.nodes.value : null,
              next: flow.nodes as Node[],
            }),
          ),
          edges: flow.edges as Edge[],
        }
      },
      isCurrentAttachment: () =>
        session === graphSession && session.boardId === options.boardSession.currentBoardId.value,
      readGroups: () =>
        session.store.getGroupIds().flatMap((groupId) => {
          const group = session.store.getGroup(groupId)
          return group ? [group] : []
        }),
      onGroupsChanged: options.onGroupsChanged,
      projectNode: options.projectCommittedGroupNodeLayer,
      scheduleMicrotask: options.scheduleMicrotask,
    })
    unsubscribeGraphSessionCommits = () => {
      unsubscribeProjection()
      unsubscribeHistory()
    }
    options.executionCore.attachBoard(BoardGraphSession.create(boardId, { graphSession: session }))
    options.hydratePendingImageLeases?.(snapshot)
  }

  function exportGraphSnapshot(baseSnapshot: BoardSnapshot): BoardSnapshot {
    return requireGraphSession().exportSnapshot(baseSnapshot)
  }

  function applyRemoteSnapshot(snapshot: BoardSnapshot): void {
    const session = requireGraphSession()
    if (snapshot.schemaVersion !== options.boardBaseSnapshot.value.schemaVersion) return
    options.boardBaseSnapshot.value = snapshot
    session.replace(snapshot)
    const flow = boardSnapshotToFlow(session.exportSnapshot(snapshot))
    options.graphAdapter.replaceFullProjection(
      options.preserveCurrentSelection({
        nodes: options.projectCurrentGroupNodeLayers(
          projectCanvasNodeReplacement({
            current: isAttached() ? options.nodes.value : null,
            next: flow.nodes as Node[],
          }),
        ),
        edges: flow.edges as Edge[],
      }),
    )
    options.onGroupsChanged(snapshot.groups)
  }

  const runtime = {
    isAttached,
    readCanvasGraphStore,
    replaceGraphSnapshot,
    applyRemoteSnapshot,
    exportGraphSnapshot,
    commitGraph,
    dispose,
  }
  onScopeDispose(() => {
    runtime.dispose()
    options.executionCore.detachBoard()
  })
  return runtime
}
