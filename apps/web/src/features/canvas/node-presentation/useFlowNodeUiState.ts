import { computed, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { i18n } from '../../../shared/i18n'
import {
  getCanvasNodePrimaryOutputPort,
  getCanvasNodeRequiredInputPorts,
  getCanvasNodeVisibleInputPorts,
  isCanvasConnectionNodeType,
  isCanvasExecutionOutputNodeType,
} from '../../canvas-board/domain'
import {
  formatCanvasNodePortLabel,
  getCanvasNodeExecutionActionLabels,
} from '../config/canvasNodeRegistry'
import {
  resolveWorkflowNodeOutputStatus,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
  type CanvasExecutionOutputNodeItem,
  type CanvasExecutionOutputNodeType,
  type CanvasNodeItem,
} from '../../canvas-board/domain'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import { parseHandlePort, resolveSourceImageFromNode } from '../utils/flowNodeSourceResolution'
import { measureCanvasPerf } from '../utils/canvasPerfProbe'

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
}
type FlowNodeUiState = NodeUiState | PromptNodeUiState

type FlowNodeConnectionStatus = {
  hasAnyConnection: boolean
  incoming: Partial<Record<CanvasConnectionPort, boolean>>
  pendingIncoming: Partial<Record<CanvasConnectionPort, boolean>>
  incomingSourceKind: Partial<Record<CanvasConnectionPort, CanvasConnectionNodeType>>
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

function arePrimaryActionsEqual(
  left: NodeUiState['primaryAction'],
  right: NodeUiState['primaryAction'],
) {
  if (!left || !right) return left === right
  return left.label === right.label && left.disabled === right.disabled
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
    arePrimaryActionsEqual(left.primaryAction, right.primaryAction) &&
    left.cacheStatus === right.cacheStatus &&
    left.isRunning === right.isRunning
  )
}

export function useFlowNodeUiState(options: UseFlowNodeUiStateOptions) {
  let previousUiStateByNodeId = new Map<string, FlowNodeUiState>()

  const connectionIndexCache = computed(() =>
    createFlowConnectionIndex(options.edges.value as FlowEdgeLike[]),
  )

  function recomputeFlowNodeUiState() {
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

    function getSnapshotNode(nodeId: string): CanvasNodeItem | null {
      const node = nodeMap.get(nodeId)
      if (!node?.data?.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
      return node.data.snapshotNode as CanvasNodeItem
    }

    function getNodeType(nodeId: string): CanvasConnectionNodeType | null {
      const node = nodeMap.get(nodeId)
      return isCanvasConnectionNodeType(node?.data?.kind) ? node.data.kind : null
    }

    function resolveConnectedImageReady(nodeId: string): boolean {
      const node = nodeMap.get(nodeId)
      if (!node?.data?.snapshotNode || typeof node.data.snapshotNode !== 'object') return false
      const snapshotNode = node.data.snapshotNode as Record<string, unknown>
      const nodeType = getNodeType(nodeId)
      if (nodeType === 'image') {
        const outputUrl = resolveSourceImageFromNode(nodeType, snapshotNode).trim()
        return !options.isNodeExecutionActive(nodeId) && !!outputUrl
      }
      if (!nodeType || !isCanvasExecutionOutputNodeType(nodeType)) return false
      const outputUrl = resolveSourceImageFromNode(nodeType, snapshotNode).trim()
      if (!outputUrl) return false
      const outputImages = Array.isArray(snapshotNode.outputImages) ? snapshotNode.outputImages : []
      const outputStatus = resolveWorkflowNodeOutputStatus(snapshotNode.outputStatus, outputImages)
      return outputStatus === 'ready'
    }

    function getNodeConnectionStatus(
      nodeType: CanvasConnectionNodeType,
      nodeId: string,
    ): FlowNodeConnectionStatus {
      const incoming: Partial<Record<CanvasConnectionPort, boolean>> = {}
      const pendingIncoming: Partial<Record<CanvasConnectionPort, boolean>> = {}
      const incomingSourceKind: Partial<Record<CanvasConnectionPort, CanvasConnectionNodeType>> = {}
      const outgoing: Partial<Record<CanvasConnectionPort, boolean>> = {}
      const incomingEdges = connectionIndex.incomingByNodeId.get(nodeId) ?? []
      const outgoingEdges = connectionIndex.outgoingByNodeId.get(nodeId) ?? []

      getCanvasNodeVisibleInputPorts(nodeType).forEach((portContract) => {
        const matchedEdges = incomingEdges.filter((edge) => edge.targetPort === portContract.port)
        const connected = matchedEdges.length > 0
        incoming[portContract.port] = connected
        if (!connected) {
          pendingIncoming[portContract.port] = false
          return
        }
        const sourceType = getNodeType(matchedEdges[0].source)
        if (sourceType) incomingSourceKind[portContract.port] = sourceType
        if (portContract.port === 'image') {
          pendingIncoming[portContract.port] = matchedEdges.some(
            (edge) => !resolveConnectedImageReady(edge.source),
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

    function buildPrimaryAction(
      nodeType: CanvasExecutionOutputNodeType,
      nodeId: string,
      status: FlowNodeConnectionStatus,
      isRunning: boolean,
    ) {
      const actionLabels = getCanvasNodeExecutionActionLabels(nodeType)
      if (isRunning) {
        return {
          label: actionLabels.running,
          disabled: true,
        }
      }

      const requiredPorts = getCanvasNodeRequiredInputPorts(nodeType).map(
        (port) => port.port as CanvasConnectionPort,
      )
      const disconnectedPorts = requiredPorts.filter((port) => !status.incoming[port])
      if (disconnectedPorts.length > 0) {
        const missingLabels = disconnectedPorts.map((port) => formatCanvasNodePortLabel(port))
        return {
          label:
            disconnectedPorts.length === 1
              ? i18n.global.t('canvas.edges.connectRequiredInputLabel', {
                  label: missingLabels[0],
                })
              : i18n.global.t('canvas.edges.connectRequiredInputsLabel', {
                  count: disconnectedPorts.length,
                }),
          disabled: true,
        }
      }

      const pendingPorts = getCanvasNodeVisibleInputPorts(nodeType)
        .map((port) => port.port)
        .filter((port) => !!status.incoming[port] && !!status.pendingIncoming[port])
      if (pendingPorts.length > 0) {
        const pendingLabels = pendingPorts.map((port) => formatCanvasNodePortLabel(port))
        return {
          label:
            pendingPorts.length === 1
              ? i18n.global.t('canvas.edges.waitForInputLabel', { label: pendingLabels[0] })
              : i18n.global.t('canvas.edges.waitForInputsLabel', { count: pendingPorts.length }),
          disabled: true,
        }
      }

      return {
        label: actionLabels.ready,
        disabled: false,
      }
    }

    flowNodes.forEach((node) => {
      const nodeType = isCanvasConnectionNodeType(node.data?.kind) ? node.data.kind : null
      if (!nodeType) {
        setFlowNodeUiState(node.id, {
          connection: { hasAnyConnection: false, connected: {} },
        })
        return
      }
      const snapshotNode = getSnapshotNode(node.id)
      if (!snapshotNode) {
        setFlowNodeUiState(node.id, {
          connection: { hasAnyConnection: false, connected: {} },
        })
        return
      }

      const status = getNodeConnectionStatus(nodeType, node.id)
      const connection = {
        hasAnyConnection: status.hasAnyConnection,
        connected: {
          ...status.incoming,
          output: !!status.outgoing[getCanvasNodePrimaryOutputPort(nodeType)],
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
        primaryAction: buildPrimaryAction(nodeType, node.id, status, isRunning),
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
