import type { Ref } from 'vue'
import type { Edge, FlowEvents, Node, NodeProps } from '@vue-flow/core'
import { VueFlow } from '@vue-flow/core'
import type {
  CanvasChromeAccountState,
  CanvasChromeBoardState,
  CanvasChromeStatusState,
  CanvasChromeToolState,
} from '../components/chrome'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import type { CanvasGroupPresentationInteraction } from '../group-presentation'
import type {
  CanvasNodePresentationIntent,
  FusionNodePresentationBinding,
  ImageNodePresentationBinding,
} from '../node-presentation'
import type { CanvasImageTools } from '../image-tools'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'

type CanvasChromeProps = {
  inert: boolean
  isHidden: boolean
  board: CanvasChromeBoardState
  account: CanvasChromeAccountState
  tools: CanvasChromeToolState
  status: CanvasChromeStatusState
}

type CanvasFlowNodeProps = {
  nodeId: string
  data: FlowNodeData
  selected?: boolean
  dragging?: boolean
  copyMode?: boolean
  uiState?: NodeUiState | PromptNodeUiState
  fusionPresentation?: FusionNodePresentationBinding
  generationAvailable?: boolean
  imagePresentation: ImageNodePresentationBinding
  setImageStageRef?: (nodeId: string, element: Element | null) => void
  clearImageStageRef?: (nodeId: string, element?: Element | null) => void
  expandModeActive?: boolean
  expandOverlayStyle?: Record<string, string> | null
  expandHandleRadius?: number
  handleVisibilityRefreshToken?: number
}

type CanvasImageToolSession = {
  viewModel: Pick<CanvasImageTools['viewModel'], 'chrome'>
  commands: Pick<CanvasImageTools['commands'], 'chrome'>
}

export type CanvasScreenPresentationBinding = {
  layout: {
    activeTool: string
    isInteractionActive: boolean
    isViewportPanActive: boolean
    onBackgroundMouseDown: (event: MouseEvent) => void
  }
  chrome: {
    props: CanvasChromeProps
    commands: {
      updateBoardNameDraft: (value: string) => void
      toggleBoardMenu: () => void
      startBoardRename: () => void
      cancelBoardRename: () => void
      confirmBoardRename: () => void
      reloadBoardCatalog: () => void
      selectBoard: (boardId: string) => void
      exportSelectedImages: () => void
      navigateBack: () => void
      selectTool: (tool: 'select' | 'hand') => void
      toggleMinimap: () => void
      zoomOut: () => void
      zoomIn: () => void
    }
  }
  canvas: {
    containerRef: Ref<HTMLElement | null>
    nodes: Ref<Node<FlowNodeData>[]>
    edges: Ref<Edge[]>
    props: Partial<InstanceType<typeof VueFlow>['$props']>
    surface: {
      onDragOver: (event: DragEvent) => void
      onDrop: (event: DragEvent) => void
      onPointerDownCapture: (event: PointerEvent) => void
      onContextMenuCapture: (event: MouseEvent) => void
      onWheel: (event: WheelEvent) => void
    }
    flow: {
      onNodesChange: (changes: FlowEvents['nodesChange']) => void
      onEdgesChange: (changes: FlowEvents['edgesChange']) => void
      onConnectEnd: () => void
      onMoveStart: () => void
      onMove: (payload: FlowEvents['move']) => void
      onMoveEnd: (payload: FlowEvents['moveEnd']) => void
      onViewportChangeEnd: () => void
      onNodeClick: (payload: FlowEvents['nodeClick']) => void
      onNodeDragStart: () => void
      onNodeDragStop: () => void
      onSelectionStart: (payload: FlowEvents['selectionStart']) => void
      onSelectionEnd: (payload: FlowEvents['selectionEnd']) => void
      onPaneClick: () => void
      onPaneContextMenu: (payload: FlowEvents['paneContextMenu']) => void
    }
    group: {
      props: {
        presentation: CanvasGroupPresentationInteraction
        actionTarget: HTMLElement | null
      }
    }
    node: {
      propsFor: (node: NodeProps<FlowNodeData>) => CanvasFlowNodeProps
      listeners: {
        intent: (intent: CanvasNodePresentationIntent) => void
        nodeMousedown: (nodeId: string, event: MouseEvent) => void
        requestInternalsUpdate: (nodeId: string) => void
      }
    }
    minimap: {
      props: { enabled: boolean; inert: boolean; isHidden: boolean }
      commands: {
        centerRequested: (payload: { position: { x: number; y: number } }) => void
        dragStart: () => void
        dragMove: (payload: { delta: { x: number; y: number } }) => void
        dragEnd: () => void
      }
    }
    overlays: {
      props: {
        imageToolSession: CanvasImageToolSession
        nodeContextMenu: { visible: boolean; x: number; y: number }
        canvasNodeMenuItems: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
      }
      commands: {
        createNode: (kind: CanvasCreatableNodeKind) => void
      }
    }
  }
}
