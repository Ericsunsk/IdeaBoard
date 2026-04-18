import {
  getCanvasNodeRequiredInputPorts,
  paletteModeUsesReference,
} from '../config/canvasNodeConnectionContract'
import { resolveWorkflowNodeOutputStatus } from '../types/canvasNodes'
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
  FaceSwapNodeItem,
  FusionNodeItem,
  GenerateNodeItem,
  MannequinNodeItem,
  ModelNodeItem,
  PaletteNodeItem,
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
  generateModeSupportsStyleModel: (mode: GenerateNodeItem['mode']) => boolean
}

export function useCanvasNodeUiState(options: UseCanvasNodeUiStateOptions) {
  function formatNodePortLabel(
    nodeType: CanvasConnectionNodeType,
    port: CanvasConnectionPort,
    node?: CanvasNodeItem | null,
  ): string {
    if (port === 'image') return 'Image'
    if (port === 'mask') return 'Mask'
    if (port === 'prompt') return 'Prompt'
    if (port === 'reference') return 'Ref'
    if (port === 'imageA') {
      if (nodeType === 'tryon') return 'Base'
      if (nodeType === 'faceswap') return 'Target'
      return 'Image A'
    }
    if (port === 'imageB') {
      if (nodeType === 'tryon') {
        return node && node.type === 'tryon' && node.mode === 'pattern-on-body' ? 'Pattern' : 'Fabric'
      }
      if (nodeType === 'faceswap') return 'Face'
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

  function buildExecutionPrimaryActionState(
    nodeType: CanvasConnectionNodeType,
    node: CanvasNodeItem & { id: number },
    readyLabel: string,
    blockedLabel?: string | null,
  ): NodePrimaryActionState {
    const { node: resolvedNode, disconnectedPorts, pendingPorts } = getNodeRequiredPortIssues(nodeType, node.id)
    if (disconnectedPorts.length > 0) {
      return {
        label: disconnectedPorts.length === 1
          ? `连接 ${formatNodePortLabel(nodeType, disconnectedPorts[0] as CanvasConnectionPort, resolvedNode)}`
          : `连接 ${disconnectedPorts.length} 个输入`,
        disabled: true,
      }
    }
    if (pendingPorts.length > 0) {
      return {
        label: pendingPorts.length === 1
          ? `等待 ${formatNodePortLabel(nodeType, pendingPorts[0] as CanvasConnectionPort, resolvedNode)}`
          : `等待 ${pendingPorts.length} 个输入`,
        disabled: true,
      }
    }
    if (blockedLabel) {
      return {
        label: blockedLabel,
        disabled: true,
      }
    }
    if ('outputImages' in node && Array.isArray(node.outputImages)) {
      const cacheStatus = resolveExecutionNodeCacheStatus(node as CanvasExecutionOutputNodeItem)
      if (cacheStatus === 'stale') {
        return {
          label: '刷新缓存',
          disabled: false,
        }
      }
    }
    return {
      label: readyLabel,
      disabled: false,
    }
  }

  function getNodePrimaryActionState(nodeType: 'model', node: ModelNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'mannequin', node: MannequinNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'viewpoint', node: ViewpointNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'color', node: ColorNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'edit', node: EditNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'fusion', node: FusionNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'enhance', node: EnhanceNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'tryon', node: TryOnNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'faceswap', node: FaceSwapNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'palette', node: PaletteNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(nodeType: 'generate', node: GenerateNodeItem): NodePrimaryActionState
  function getNodePrimaryActionState(
    nodeType: CanvasExecutionOutputNodeType,
    node: CanvasExecutionOutputNodeItem,
  ): NodePrimaryActionState {
    switch (nodeType) {
      case 'model': {
        const modelNode = node as ModelNodeItem
        return buildExecutionPrimaryActionState(
          nodeType,
          modelNode,
          '生成多角度',
          modelNode.selectedAngles.length === 0 ? '选择角度' : null,
        )
      }
      case 'mannequin':
        return buildExecutionPrimaryActionState(nodeType, node, '生成人模')
      case 'viewpoint': {
        const viewpointNode = node as ViewpointNodeItem
        return buildExecutionPrimaryActionState(
          nodeType,
          viewpointNode,
          '生成',
          viewpointNode.selectedViewpoints.length === 0 ? '选择视角' : null,
        )
      }
      case 'color':
        return buildExecutionPrimaryActionState(nodeType, node, '应用换色')
      case 'edit':
      case 'fusion':
      case 'enhance':
      case 'tryon':
      case 'faceswap':
        return buildExecutionPrimaryActionState(nodeType, node, '生成')
      case 'palette': {
        const paletteNode = node as PaletteNodeItem
        return buildExecutionPrimaryActionState(
          nodeType,
          paletteNode,
          '生成',
          !paletteModeUsesReference(paletteNode.mode) && paletteNode.colors.length === 0 ? '添加颜色' : null,
        )
      }
      case 'generate': {
        const generateNode = node as GenerateNodeItem
        return buildExecutionPrimaryActionState(
          nodeType,
          generateNode,
          '生成',
          options.generateModeSupportsStyleModel(generateNode.mode) && !generateNode.styleModel ? '选择 Style' : null,
        )
      }
    }
  }

  function getNodeUiState(nodeType: 'model', node: ModelNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'mannequin', node: MannequinNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'viewpoint', node: ViewpointNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'color', node: ColorNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'edit', node: EditNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'fusion', node: FusionNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'enhance', node: EnhanceNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'tryon', node: TryOnNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'faceswap', node: FaceSwapNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'palette', node: PaletteNodeItem): NodeUiState
  function getNodeUiState(nodeType: 'generate', node: GenerateNodeItem): NodeUiState
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

    let primaryAction: NodePrimaryActionState
    switch (nodeType) {
      case 'model':
        primaryAction = getNodePrimaryActionState(nodeType, node as ModelNodeItem)
        break
      case 'mannequin':
        primaryAction = getNodePrimaryActionState(nodeType, node as MannequinNodeItem)
        break
      case 'viewpoint':
        primaryAction = getNodePrimaryActionState(nodeType, node as ViewpointNodeItem)
        break
      case 'color':
        primaryAction = getNodePrimaryActionState(nodeType, node as ColorNodeItem)
        break
      case 'edit':
        primaryAction = getNodePrimaryActionState(nodeType, node as EditNodeItem)
        break
      case 'fusion':
        primaryAction = getNodePrimaryActionState(nodeType, node as FusionNodeItem)
        break
      case 'enhance':
        primaryAction = getNodePrimaryActionState(nodeType, node as EnhanceNodeItem)
        break
      case 'tryon':
        primaryAction = getNodePrimaryActionState(nodeType, node as TryOnNodeItem)
        break
      case 'faceswap':
        primaryAction = getNodePrimaryActionState(nodeType, node as FaceSwapNodeItem)
        break
      case 'palette':
        primaryAction = getNodePrimaryActionState(nodeType, node as PaletteNodeItem)
        break
      case 'generate':
        primaryAction = getNodePrimaryActionState(nodeType, node as GenerateNodeItem)
        break
    }

    return {
      connection,
      primaryAction,
      cacheStatus: resolveExecutionNodeCacheStatus(node as CanvasExecutionOutputNodeItem),
    }
  }

  return {
    formatNodePortLabel,
    getNodeRequiredPortIssues,
    getNodeConnectionState,
    getNodePrimaryActionState,
    getNodeUiState,
  }
}
