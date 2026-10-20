import { computed, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import {
  getCanvasNodePrimaryOutputPort,
  getCanvasNodeVisibleInputPorts,
  isCanvasConnectionNodeType,
  isCanvasExecutionOutputNodeType,
} from '../../../board/ports/canvas'
import {
  resolveWorkflowNodeOutputStatus,
  type CanvasConnectionNodeKind,
  type CanvasConnectionPort,
  type CanvasExecutionOutputNodeItem,
  type CanvasNodeItem,
} from '../../../board/ports/canvas'
import type { NodeUiState, PromptNodeUiState } from '../../core/nodes/canvasNodeUi.ts'
import { parseHandlePort, resolveSourceImageFromNode } from '../../session/flow-adapter'
import { measureCanvasPerf } from '../../session/browser/canvasPerfProbe.ts'

type FlowEdgeLike = {
  source: string
  sourceHandle?: string | null
  target: string
  targetHandle?: string | null
}
type FlowNodeLike = { id: string; data?: { kind?: string; snapshotNode?: unknown } }
type IndexedFlowEdge = FlowEdgeLike & {
  sourcePort: string
  targetPort: string
}

type UseFlowNodeUiStateOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  isNodeExecutionActive: (nodeId: string) => boolean
  /**
   * Reactive execution epoch. `isNodeExecutionActive` reads non-reactive
   * lease state, so the ui-state cache must also track this ref or a running
   * node keeps a stale "ready" label until the next graph change.
   */
  executionEpoch?: Ref<unknown>
}
type FlowNodeUiState = NodeUiState | PromptNodeUiState

type FlowNodeConnectionStatus = {
  hasAnyConnection: boolean
  incoming: Partial<Record<CanvasConnectionPort, boolean>>
  pendingIncoming: Partial<Record<CanvasConnectionPort, boolean>>
  incomingSourceKind: Partial<Record<CanvasConnectionPort, CanvasConnectionNodeKind>>
  outgoing: Partial<Record<CanvasConnectionPort, boolean>>
}

type FlowConnectionIndex = {
  incomingByNodeId: Map<string, IndexedFlowEdge[]>
  outgoingByNodeId: Map<string, IndexedFlowEdge[]>
}

function appendIndexedEdge(
  edgeMap: Map<string, IndexedFlowEdge[]>,
  nodeId: string,
  edge: IndexedFlowEdge,
) {
  const existing = edgeMap.get(nodeId)
  if (existing) {
    existing.push(edge)
    return
  }
  edgeMap.set(nodeId, [edge])
}

function createFlowConnectionIndex(flowEdges: FlowEdgeLike[]): FlowConnectionIndex {
  const incomingByNodeId = new Map<string, IndexedFlowEdge[]>()
  const outgoingByNodeId = new Map<string, IndexedFlowEdge[]>()

  flowEdges.forEach((edge) => {
    const indexedEdge: IndexedFlowEdge = {
      ...edge,
      sourcePort: parseHandlePort(edge.sourceHandle),
      targetPort: parseHandlePort(edge.targetHandle),
    }
    appendIndexedEdge(incomingByNodeId, edge.target, indexedEdge)
    appendIndexedEdge(outgoingByNodeId, edge.source, indexedEdge)
  })

  return { incomingByNodeId, outgoingByNodeId }
}

function areRecordsEqual(
  left: Partial<Record<string, unknown>> | undefined,
  right: Partial<Record<string, unknown>> | undefined,
) {
  const leftRecord = left ?? {}
  const rightRecord = right ?? {}
  const leftKeys = Object.keys(leftRecord)
  const rightKeys = Object.keys(rightRecord)
  if (leftKeys.length !== rightKeys.length) return false
  return leftKeys.every((key) => Object.is(leftRecord[key], rightRecord[key]))
}

function areConnectionStatesEqual(
  left: FlowNodeUiState['connection'],
  right: FlowNodeUiState['connection'],
) {
  return (
    left.hasAnyConnection === right.hasAnyConnection &&
    areRecordsEqual(left.connected, right.connected) &&
    areRecordsEqual(left.pending, right.pending) &&
    areRecordsEqual(left.sourceKind, right.sourceKind)
  )
}

function areFlowNodeUiStatesEqual(left: FlowNodeUiState, right: FlowNodeUiState) {
  return (
    areConnectionStatesEqual(left.connection, right.connection) &&
    left.cacheStatus === right.cacheStatus &&
    left.isRunning === right.isRunning
  )
}

function getSnapshotNode(node: FlowNodeLike | undefined): CanvasNodeItem | null {
  if (!node?.data?.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
  return node.data.snapshotNode as CanvasNodeItem
}

function getNodeType(node: FlowNodeLike | undefined): CanvasConnectionNodeKind | null {
  return isCanvasConnectionNodeType(node?.data?.kind) ? node.data.kind : null
}

function resolveConnectedImageReady(
  node: FlowNodeLike | undefined,
  isNodeExecutionActive: (nodeId: string) => boolean,
): boolean {
  if (!node?.data?.snapshotNode || typeof node.data.snapshotNode !== 'object') return false
  const snapshotNode = node.data.snapshotNode as Record<string, unknown>
  const nodeType = getNodeType(node)
  if (nodeType === 'image') {
    const outputUrl = resolveSourceImageFromNode(nodeType, snapshotNode).trim()
    return !isNodeExecutionActive(node.id) && !!outputUrl
  }
  if (!nodeType || !isCanvasExecutionOutputNodeType(nodeType)) return false
  const outputUrl = resolveSourceImageFromNode(nodeType, snapshotNode).trim()
  if (!outputUrl) return false
  const outputImages = Array.isArray(snapshotNode.outputImages) ? snapshotNode.outputImages : []
  const outputStatus = resolveWorkflowNodeOutputStatus(snapshotNode.outputStatus, outputImages)
  return outputStatus === 'ready'
}

function getNodeConnectionStatus(
  nodeType: CanvasConnectionNodeKind,
  nodeId: string,
  connectionIndex: FlowConnectionIndex,
  nodeMap: ReadonlyMap<string, FlowNodeLike>,
  isNodeExecutionActive: (nodeId: string) => boolean,
): FlowNodeConnectionStatus {
  const incoming: Partial<Record<CanvasConnectionPort, boolean>> = {}
  const pendingIncoming: Partial<Record<CanvasConnectionPort, boolean>> = {}
  const incomingSourceKind: Partial<Record<CanvasConnectionPort, CanvasConnectionNodeKind>> = {}
  const outgoing: Partial<Record<CanvasConnectionPort, boolean>> = {}
  const incomingEdges = connectionIndex.incomingByNodeId.get(nodeId) ?? []
  const outgoingEdges = connectionIndex.outgoingByNodeId.get(nodeId) ?? []

  getCanvasNodeVisibleInputPorts(nodeType).forEach((portContract) => {
    const matchedEdges = incomingEdges.filter((edge) => edge.targetPort === portContract.port)
    const isConnected = matchedEdges.length > 0
    incoming[portContract.port] = isConnected
    if (!isConnected) {
      pendingIncoming[portContract.port] = false
      return
    }
    const matchedEdge = matchedEdges[0]
    if (!matchedEdge) {
      pendingIncoming[portContract.port] = false
      return
    }
    const sourceType = getNodeType(nodeMap.get(matchedEdge.source))
    if (sourceType) incomingSourceKind[portContract.port] = sourceType
    if (portContract.port === 'image') {
      pendingIncoming[portContract.port] = matchedEdges.some(
        (edge) => !resolveConnectedImageReady(nodeMap.get(edge.source), isNodeExecutionActive),
      )
      return
    }
    pendingIncoming[portContract.port] = false
  })

  const outputPort = getCanvasNodePrimaryOutputPort(nodeType)
  outgoing[outputPort] = outgoingEdges.some((edge) => edge.sourcePort === outputPort)

  return {
    hasAnyConnection: incomingEdges.length > 0 || outgoingEdges.length > 0,
    incoming,
    pendingIncoming,
    incomingSourceKind,
    outgoing,
  }
}

export function useFlowNodeUiState(options: UseFlowNodeUiStateOptions) {
  let previousUiStateByNodeId = new Map<string, FlowNodeUiState>()

  const connectionIndexCache = computed(() =>
    createFlowConnectionIndex(options.edges.value as FlowEdgeLike[]),
  )

  function recomputeFlowNodeUiState() {
    // Track the execution epoch so lease-only transitions (no graph change)
    // invalidate the cached ui state.
    void options.executionEpoch?.value
    const result: Record<string, FlowNodeUiState> = {}
    const nextUiStateByNodeId = new Map<string, FlowNodeUiState>()
    const flowNodes = options.nodes.value as FlowNodeLike[]
    const nodeMap = new Map(flowNodes.map((node) => [node.id, node]))
    const connectionIndex = connectionIndexCache.value

    function setFlowNodeUiState(nodeId: string, nextState: FlowNodeUiState) {
      const previousState = previousUiStateByNodeId.get(nodeId)
      const state =
        previousState && areFlowNodeUiStatesEqual(previousState, nextState)
          ? previousState
          : nextState
      result[nodeId] = state
      nextUiStateByNodeId.set(nodeId, state)
    }

    flowNodes.forEach((node) => {
      const nodeType = getNodeType(node)
      if (!nodeType) {
        setFlowNodeUiState(node.id, {
          connection: { hasAnyConnection: false, connected: {} },
        })
        return
      }
      const snapshotNode = getSnapshotNode(node)
      if (!snapshotNode) {
        setFlowNodeUiState(node.id, {
          connection: { hasAnyConnection: false, connected: {} },
        })
        return
      }

      const status = getNodeConnectionStatus(
        nodeType,
        node.id,
        connectionIndex,
        nodeMap,
        options.isNodeExecutionActive,
      )
      const connection = {
        hasAnyConnection: status.hasAnyConnection,
        connected: {
          ...status.incoming,
          isOutput: !!status.outgoing[getCanvasNodePrimaryOutputPort(nodeType)],
        },
        pending: {
          ...status.pendingIncoming,
        },
        sourceKind: {
          ...status.incomingSourceKind,
        },
      }

      if (!isCanvasExecutionOutputNodeType(nodeType)) {
        setFlowNodeUiState(node.id, { connection })
        return
      }

      const executionNode = snapshotNode as CanvasExecutionOutputNodeItem
      const isRunning = options.isNodeExecutionActive(node.id)
      setFlowNodeUiState(node.id, {
        connection,
        cacheStatus: resolveWorkflowNodeOutputStatus(
          executionNode.outputStatus,
          executionNode.outputImages,
        ),
        isRunning,
      })
    })

    previousUiStateByNodeId = nextUiStateByNodeId
    return result
  }

  const flowNodeUiStateCache = computed(() =>
    measureCanvasPerf(
      'node-ui.recompute',
      () => ({ nodes: options.nodes.value.length, edges: options.edges.value.length }),
      recomputeFlowNodeUiState,
    ),
  )

  function getFlowNodeUiStateCached(nodeId: string): NodeUiState | PromptNodeUiState {
    return (
      flowNodeUiStateCache.value[nodeId] || {
        connection: { hasAnyConnection: false, connected: {} },
      }
    )
  }

  return {
    getFlowNodeUiStateCached,
  }
}
