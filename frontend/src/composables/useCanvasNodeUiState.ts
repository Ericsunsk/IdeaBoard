import {
  getCanvasNodeRequiredInputPorts,
  getCanvasNodeVisibleInputPorts,
  getEditNodePortLabel,
} from '../config/canvasNodeConnectionContract'
import { hasColorNodeColors, resolveWorkflowNodeOutputStatus } from '../types/canvasNodes'
import type { NodeConnectionStatus } from './useCanvasNodeConnections'
import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasExecutionOutputNodeItem,
  CanvasExecutionOutputNodeType,
  CanvasNodeItem,
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'
import type {
  NodeConnectionState,
  NodePrimaryActionState,
  PromptNodeUiState,
  NodeUiState,
} from '../types/canvasNodeUi'

function resolveExecutionNodeCacheStatus(node: CanvasExecutionOutputNodeItem) {
  return resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages)
}

type UseCanvasNodeUiStateOptions = {
  getNodeConnectionStatus: (
    nodeType: CanvasConnectionNodeType,
    node: CanvasNodeItem & { id: number },
  ) => NodeConnectionStatus
  lookupNodeByType: (nodeType: CanvasConnectionNodeType, nodeId: number) => CanvasNodeItem | null
  isExecutionNodeRunning?: (nodeType: CanvasExecutionOutputNodeType, nodeId: number) => boolean
  isNodeLastRunFailed?: (nodeType: CanvasExecutionOutputNodeType, nodeId: number) => boolean
}

type ExecutionActionLabels = { ready: string; running: string }

type ExecutionActionState = {
  primaryAction: NodePrimaryActionState
  secondaryStatus?: string
}

export function useCanvasNodeUiState(options: UseCanvasNodeUiStateOptions) {
  function resolveExecutionActionLabels(nodeType: CanvasExecutionOutputNodeType): ExecutionActionLabels {
    switch (nodeType) {
      case 'model':
      case 'viewpoint':
        return { ready: '生成', running: '生成中...' }
      case 'color':
        return { ready: '应用', running: '应用中...' }
      case 'edit':
        return { ready: '编辑', running: '编辑中...' }
      case 'fusion':
        return { ready: '融合', running: '融合中...' }
      case 'enhance':
        return { ready: '增强', running: '增强中...' }
      case 'tryon':
        return { ready: '上身', running: '上身中...' }
    }
  }

  function formatNodePortLabel(
    nodeType: CanvasConnectionNodeType,
    port: CanvasConnectionPort,
    node?: CanvasNodeItem | null,
  ): string {
    if (
      nodeType === 'edit' &&
      node &&
      node.type === 'edit' &&
      (port === 'image' || port === 'imageB' || port === 'mask' || port === 'prompt')
    ) {
      return getEditNodePortLabel(node.mode, port)
    }
    if (nodeType === 'model') {
      if (port === 'image') return 'Image'
      if (port === 'imageA') return 'Model'
      if (port === 'imageB') return 'Pose'
    }
    if (port === 'image') return 'Image'
    if (port === 'mask') return 'Mask'
    if (port === 'prompt') return 'Prompt'
    if (port === 'imageA') {
      if (nodeType === 'tryon') return 'Body'
      return 'Image A'
    }
    if (port === 'imageB') {
      if (nodeType === 'tryon') {
        return 'Texture'
      }
      return 'Image B'
    }
    return port
  }

  function getNodeRequiredPortIssues(
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    candidatePorts?: string[],
  ) {
    const node = options.lookupNodeByType(nodeType, nodeId)
    if (!node) {
      return {
        node: null,
        disconnectedPorts: candidatePorts ?? [],
        pendingPorts: [] as string[],
      }
    }
    const status = options.getNodeConnectionStatus(nodeType, node)
    const targetPorts = candidatePorts ?? getCanvasNodeRequiredInputPorts(nodeType, node).map(port => port.port)
    const disconnectedPorts: string[] = []
    const pendingPorts: string[] = []

    targetPorts.forEach((port) => {
      const typedPort = port as CanvasConnectionPort
      if (!status.incoming[typedPort]) {
        disconnectedPorts.push(port)
        return
      }
      if (status.pendingIncoming[typedPort]) {
        pendingPorts.push(port)
      }
    })

    return {
      node,
      disconnectedPorts,
      pendingPorts,
    }
  }

  function getNodeConnectedPendingPortIssues(
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    candidatePorts?: string[],
  ) {
    const node = options.lookupNodeByType(nodeType, nodeId)
    if (!node) {
      return {
        node: null,
        pendingPorts: [] as string[],
      }
    }

    const status = options.getNodeConnectionStatus(nodeType, node)
    const targetPorts = candidatePorts ?? getCanvasNodeVisibleInputPorts(nodeType, node).map(port => port.port)
    const pendingPorts: string[] = []

    targetPorts.forEach((port) => {
      const typedPort = port as CanvasConnectionPort
      if (!status.incoming[typedPort]) return
      if (status.pendingIncoming[typedPort]) {
        pendingPorts.push(port)
      }
    })

    return {
      node,
      pendingPorts,
    }
  }

  function getNodeOutputPort(nodeType: CanvasConnectionNodeType): CanvasConnectionPort {
    if (nodeType === 'prompt') return 'prompt'
    if (nodeType === 'mask') return 'mask'
    return 'image'
  }

  function getNodeConnectionState(
    nodeType: CanvasConnectionNodeType,
    node: CanvasNodeItem & { id: number },
  ): NodeConnectionState {
    const status = options.getNodeConnectionStatus(nodeType, node)
    return {
      hasAnyConnection: status.hasAnyConnection,
      connected: {
        ...status.incoming,
        output: !!status.outgoing[getNodeOutputPort(nodeType)],
      },
      pending: {
        ...status.pendingIncoming,
      },
    }
  }

  function buildExecutionActionState(
    nodeType: CanvasExecutionOutputNodeType,
    node: CanvasExecutionOutputNodeItem,
  ): ExecutionActionState {
    const actionLabels = resolveExecutionActionLabels(nodeType)
    if (options.isExecutionNodeRunning?.(nodeType, node.id)) {
      return {
        primaryAction: {
          label: actionLabels.running,
          disabled: true,
        },
      }
    }

    const { node: resolvedNode, disconnectedPorts } = getNodeRequiredPortIssues(nodeType, node.id)
    if (disconnectedPorts.length > 0) {
      const missingLabels = disconnectedPorts.map(
        port => formatNodePortLabel(nodeType, port as CanvasConnectionPort, resolvedNode),
      )
      return {
        primaryAction: {
          label: disconnectedPorts.length === 1
            ? `连接 ${missingLabels[0]}`
            : `连接 ${disconnectedPorts.length} 个输入`,
          disabled: true,
        },
        secondaryStatus: `缺少：${missingLabels.join(' / ')}`,
      }
    }

    const { node: pendingNode, pendingPorts } = getNodeConnectedPendingPortIssues(nodeType, node.id)
    if (pendingPorts.length > 0) {
      const pendingLabels = pendingPorts.map(
        port => formatNodePortLabel(nodeType, port as CanvasConnectionPort, pendingNode),
      )
      return {
        primaryAction: {
          label: pendingPorts.length === 1
            ? `等待 ${pendingLabels[0]}`
            : `等待 ${pendingPorts.length} 个输入`,
          disabled: true,
        },
        secondaryStatus: `等待上游：${pendingLabels.join(' / ')}`,
      }
    }

    const blockedLabel = resolveExecutionBlockedLabel(nodeType, node)
    if (blockedLabel) {
      return {
        primaryAction: {
          label: blockedLabel,
          disabled: true,
        },
      }
    }

    const cacheStatus = resolveExecutionNodeCacheStatus(node)
    if (cacheStatus === 'stale') {
      return {
        primaryAction: {
          label: '刷新缓存',
          disabled: false,
        },
        secondaryStatus: '当前结果已过期，需刷新',
      }
    }

    if (options.isNodeLastRunFailed?.(nodeType, node.id)) {
      return {
        primaryAction: {
          label: '重试',
          disabled: false,
        },
        secondaryStatus: '上次执行失败，可重试',
      }
    }

    if (cacheStatus === 'ready' && node.outputImages.length > 0) {
      return {
        primaryAction: {
          label: '重新生成',
          disabled: false,
        },
        secondaryStatus: '当前缓存可用',
      }
    }

    return {
      primaryAction: {
        label: actionLabels.ready,
        disabled: false,
      },
    }
  }

  function resolveExecutionBlockedLabel(nodeType: CanvasExecutionOutputNodeType, node: CanvasExecutionOutputNodeItem): string | null {
    switch (nodeType) {
      case 'viewpoint':
        return (node as ViewpointNodeItem).selectedViewpoint ? null : '选择视角'
      case 'color':
        return hasColorNodeColors(node as ColorNodeItem) ? null : '添加颜色'
      default:
        return null
    }
  }

  function getNodeUiState(nodeType: 'model', node: ModelNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'viewpoint', node: ViewpointNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'color', node: ColorNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'edit', node: EditNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'fusion', node: FusionNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'enhance', node: EnhanceNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'tryon', node: TryOnNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'image', node: CanvasNodeItem & { id: number }): NodeUiState
  function getNodeUiState(nodeType: 'mask', node: CanvasNodeItem & { id: number }): NodeUiState
  function getNodeUiState(nodeType: 'prompt', node: PromptNodeItem): PromptNodeUiState
  function getNodeUiState(
    nodeType: CanvasExecutionOutputNodeType | 'image' | 'mask' | 'prompt',
    node: CanvasNodeItem & { id: number },
  ): NodeUiState {
    const connection = getNodeConnectionState(nodeType, node)
    if (nodeType === 'image' || nodeType === 'mask' || nodeType === 'prompt') {
      return { connection }
    }

    const executionNode = node as CanvasExecutionOutputNodeItem
    const executionState = buildExecutionActionState(nodeType, executionNode)

    return {
      connection,
      primaryAction: executionState.primaryAction,
      secondaryStatus: executionState.secondaryStatus,
      cacheStatus: resolveExecutionNodeCacheStatus(executionNode),
    }
  }

  return {
    formatNodePortLabel,
    getNodeRequiredPortIssues,
    getNodeConnectedPendingPortIssues,
    getNodeConnectionState,
    getNodeUiState,
  }
}
