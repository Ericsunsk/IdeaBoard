import { computed, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import {
  getCanvasNodeRequiredInputPorts,
  getCanvasNodeVisibleInputPorts,
} from '../config/canvasNodeConnectionContract'
import {
  isCanvasConnectionNodeType,
  isCanvasExecutionOutputNodeType,
  resolveWorkflowNodeOutputStatus,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
  type CanvasExecutionOutputNodeItem,
  type CanvasExecutionOutputNodeType,
  type CanvasNodeItem,
  type ViewpointNodeItem,
} from '../types/canvasNodes'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import { parseHandlePort, resolveSourceImageFromNode } from '../services/flowTaskExecutorUtils'

type FlowEdgeLike = {
  source: string
  sourceHandle?: string | null
  target: string
  targetHandle?: string | null
}
type FlowNodeLike = { id: string; data?: { kind?: string; snapshotNode?: unknown } }

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

type ExecutionActionLabels = { ready: string; running: string }

function getNodeOutputPort(nodeType: CanvasConnectionNodeType): CanvasConnectionPort {
  if (nodeType === 'prompt') return 'prompt'
  return 'image'
}

function resolveExecutionActionLabels(
  nodeType: CanvasExecutionOutputNodeType,
): ExecutionActionLabels {
  switch (nodeType) {
    case 'viewpoint':
      return { ready: '生成', running: '生成中...' }
    case 'fusion':
      return { ready: '融合', running: '融合中...' }
  }
}

function formatNodePortLabel(
  nodeType: CanvasConnectionNodeType,
  port: CanvasConnectionPort,
): string {
  void nodeType
  if (port === 'image') return 'Image'
  if (port === 'prompt') return 'Prompt'
  return port
}

export function useFlowNodeUiState(options: UseFlowNodeUiStateOptions) {
  const flowNodeUiStateCache = computed(() => {
    const result: Record<string, NodeUiState | PromptNodeUiState> = {}
    const flowNodes = options.nodes.value as FlowNodeLike[]
    const flowEdges = options.edges.value as FlowEdgeLike[]
    const nodeMap = new Map(flowNodes.map((node) => [node.id, node]))

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
        return outputStatus !== 'running' && !!`${snapshotNode.sourceImageUrl ?? ''}`.trim()
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

      getCanvasNodeVisibleInputPorts(nodeType).forEach((portContract) => {
        const matchedEdge = flowEdges.find(
          (edge) =>
            edge.target === nodeId && parseHandlePort(edge.targetHandle) === portContract.port,
        )
        const connected = !!matchedEdge
        incoming[portContract.port] = connected
        if (!connected || !matchedEdge) {
          pendingIncoming[portContract.port] = false
          return
        }
        const sourceType = getNodeType(matchedEdge.source)
        if (sourceType) incomingSourceKind[portContract.port] = sourceType
        if (portContract.port === 'image') {
          pendingIncoming[portContract.port] = !resolveConnectedImageReady(matchedEdge.source)
          return
        }
        pendingIncoming[portContract.port] = false
      })

      const outputPort = getNodeOutputPort(nodeType)
      outgoing[outputPort] = flowEdges.some(
        (edge) => edge.source === nodeId && parseHandlePort(edge.sourceHandle) === outputPort,
      )

      return {
        hasAnyConnection: flowEdges.some(
          (edge) => edge.source === nodeId || edge.target === nodeId,
        ),
        incoming,
        pendingIncoming,
        incomingSourceKind,
        outgoing,
      }
    }

    function resolveExecutionBlockedLabel(
      nodeType: CanvasExecutionOutputNodeType,
      node: CanvasExecutionOutputNodeItem,
    ): string | null {
      switch (nodeType) {
        case 'viewpoint':
          return (node as ViewpointNodeItem).selectedViewpoint ? null : '选择视角'
        default:
          return null
      }
    }

    function buildPrimaryAction(
      nodeType: CanvasExecutionOutputNodeType,
      nodeId: string,
      executionNode: CanvasExecutionOutputNodeItem,
      status: FlowNodeConnectionStatus,
    ) {
      const actionLabels = resolveExecutionActionLabels(nodeType)
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
        const missingLabels = disconnectedPorts.map((port) => formatNodePortLabel(nodeType, port))
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
        const pendingLabels = pendingPorts.map((port) => formatNodePortLabel(nodeType, port))
        return {
          label:
            pendingPorts.length === 1
              ? `等待 ${pendingLabels[0]}`
              : `等待 ${pendingPorts.length} 个输入`,
          disabled: true,
        }
      }

      const blockedLabel = resolveExecutionBlockedLabel(nodeType, executionNode)
      if (blockedLabel) {
        return {
          label: blockedLabel,
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
          output: !!status.outgoing[getNodeOutputPort(nodeType)],
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
        primaryAction: buildPrimaryAction(nodeType, node.id, executionNode, status),
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
