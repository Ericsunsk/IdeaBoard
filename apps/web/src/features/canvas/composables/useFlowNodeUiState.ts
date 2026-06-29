import { computed, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
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
  isFlowExecutionNodeRunning?: (nodeType: CanvasExecutionOutputNodeType, nodeId: string) => boolean
}

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

export function useFlowNodeUiState(options: UseFlowNodeUiStateOptions) {
  const flowNodeUiStateCache = computed(() => {
    const result: Record<string, NodeUiState | PromptNodeUiState> = {}
    const flowNodes = options.nodes.value as FlowNodeLike[]
    const flowEdges = options.edges.value as FlowEdgeLike[]
    const nodeMap = new Map(flowNodes.map((node) => [node.id, node]))
    const connectionIndex = createFlowConnectionIndex(flowEdges)

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
        const outputImages = Array.isArray(snapshotNode.outputImages)
          ? snapshotNode.outputImages
          : []
        const outputStatus = resolveWorkflowNodeOutputStatus(
          snapshotNode.outputStatus,
          outputImages,
        )
        const outputUrl = resolveSourceImageFromNode(nodeType, snapshotNode).trim()
        return outputStatus !== 'running' && !!outputUrl
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
    ) {
      const actionLabels = getCanvasNodeExecutionActionLabels(nodeType)
      if (options.isFlowExecutionNodeRunning?.(nodeType, nodeId)) {
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
              ? `连接 ${missingLabels[0]}`
              : `连接 ${disconnectedPorts.length} 个输入`,
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
              ? `等待 ${pendingLabels[0]}`
              : `等待 ${pendingPorts.length} 个输入`,
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
        result[node.id] = { connection: { hasAnyConnection: false, connected: {} } }
        return
      }
      const snapshotNode = getSnapshotNode(node.id)
      if (!snapshotNode) {
        result[node.id] = { connection: { hasAnyConnection: false, connected: {} } }
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
        result[node.id] = { connection }
        return
      }

      const executionNode = snapshotNode as CanvasExecutionOutputNodeItem
      result[node.id] = {
        connection,
        primaryAction: buildPrimaryAction(nodeType, node.id, status),
        cacheStatus: resolveWorkflowNodeOutputStatus(
          executionNode.outputStatus,
          executionNode.outputImages,
        ),
        isRunning: options.isFlowExecutionNodeRunning?.(nodeType, node.id) ?? false,
      }
    })

    return result
  })

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
