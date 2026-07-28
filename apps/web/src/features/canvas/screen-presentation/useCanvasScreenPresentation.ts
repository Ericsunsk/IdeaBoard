import { computed, type ComputedRef, type Ref } from 'vue'
import { PanOnScrollMode, type NodeProps } from '@vue-flow/core'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import type { CanvasChromeAccountState, CanvasChromeBoardState } from '../components/chrome'
import type { CanvasGroupPresentationInteraction } from '../group-presentation'
import type {
  FusionNodePresentationBinding,
  ImageNodePresentationBinding,
} from '../node-presentation'
import type { CanvasImageTools } from '../image-tools'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'
import type { CanvasWorkspaceRuntime } from '../workspace-runtime'
import type { CanvasScreenPresentationBinding } from './screenPresentationContract'
import { CANVAS_VIEWPORT_MAX_ZOOM, CANVAS_VIEWPORT_MIN_ZOOM } from '../utils/canvasViewportStorage'

type CanvasImageToolSession = {
  viewModel: Pick<CanvasImageTools['viewModel'], 'chrome'>
  commands: Pick<CanvasImageTools['commands'], 'chrome'>
}

type CanvasScreenWorkspace = {
  viewModel: Pick<CanvasWorkspaceRuntime['viewModel'], 'flowPane'> & {
    graphEditing: Pick<CanvasWorkspaceRuntime['viewModel']['graphEditing'], 'hasSelectedFlowNodes'>
    viewportControls: Pick<CanvasWorkspaceRuntime['viewModel']['viewportControls'], 'zoomLevel'>
  }
}

type CanvasScreenPresentationOptions = {
  workspace: CanvasScreenWorkspace
  chrome: {
    board: ComputedRef<CanvasChromeBoardState>
    account: ComputedRef<CanvasChromeAccountState>
    activeTool: Ref<'select' | 'hand'>
    minimapEnabled: Ref<boolean>
    isHidden: Ref<boolean>
    closeBoardHeaderEditing: () => void
    commands: CanvasScreenPresentationBinding['chrome']['commands']
  }
  canvas: {
    containerRef: Ref<HTMLElement | null>
    isInteractionActive: Ref<boolean>
    isViewportPanActive: Ref<boolean>
    isLocked: Ref<boolean>
    isShortcutSuppressed: ComputedRef<boolean>
    connectionBindings: CanvasWorkspaceRuntime['commands']['flowPane']['connectionAdapter']['vueFlowBindings']
    surface: CanvasScreenPresentationBinding['canvas']['surface']
    flow: CanvasScreenPresentationBinding['canvas']['flow']
    group: CanvasGroupPresentationInteraction
    node: {
      copyMode: Ref<boolean>
      getUiState: (nodeId: string) => NodeUiState | PromptNodeUiState | undefined
      imagePresentationFor: (nodeId: string) => ImageNodePresentationBinding
      fusionPresentationFor: (nodeId: string) => FusionNodePresentationBinding
      generationAvailable: Ref<boolean>
      setImageStageRef: (nodeId: string, element: Element | null) => void
      clearImageStageRef: (nodeId: string, element?: Element | null) => void
      expand: {
        nodeId: Ref<string | null>
        overlayStyle: Ref<Record<string, string> | null>
        handleRadius: number
        visibilityRefreshToken: Ref<number>
      }
      listeners: CanvasScreenPresentationBinding['canvas']['node']['listeners']
    }
    minimap: {
      enabled: Ref<boolean>
      isHidden: Ref<boolean>
      commands: CanvasScreenPresentationBinding['canvas']['minimap']['commands']
    }
    overlays: {
      imageToolSession: CanvasImageToolSession
      nodeContextMenu: Ref<{ visible: boolean; x: number; y: number }>
      canvasNodeMenuItems: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
      createNode: (kind: CanvasCreatableNodeKind) => void
    }
  }
}

export function useCanvasScreenPresentation(
  options: CanvasScreenPresentationOptions,
): ComputedRef<CanvasScreenPresentationBinding> {
  const { workspace, chrome, canvas } = options

  return computed(() => {
    const isSelectTool = chrome.activeTool.value === 'select'
    const isHandTool = chrome.activeTool.value === 'hand'
    const isCanvasLocked = canvas.isLocked.value

    return {
      layout: {
        activeTool: chrome.activeTool.value,
        isInteractionActive: canvas.isInteractionActive.value,
        isViewportPanActive: canvas.isViewportPanActive.value,
        onBackgroundMouseDown: chrome.closeBoardHeaderEditing,
      },
      chrome: {
        props: {
          inert: chrome.isHidden.value,
          isHidden: chrome.isHidden.value,
          board: chrome.board.value,
          account: chrome.account.value,
          tools: {
            activeTool: chrome.activeTool.value,
            minimapEnabled: chrome.minimapEnabled.value,
          },
          status: {
            exportDisabled: !workspace.viewModel.graphEditing.hasSelectedFlowNodes.value,
            zoomLevel: workspace.viewModel.viewportControls.zoomLevel.value,
          },
        },
        commands: chrome.commands,
      },
      canvas: {
        containerRef: canvas.containerRef,
        nodes: workspace.viewModel.flowPane.nodes,
        edges: workspace.viewModel.flowPane.edges,
        props: {
          panOnDrag: !isCanvasLocked && isHandTool,
          nodesDraggable: !isCanvasLocked && !isHandTool,
          nodesConnectable: !isCanvasLocked && !isHandTool,
          elementsSelectable: !isCanvasLocked && !isHandTool,
          selectNodesOnDrag: !isCanvasLocked && isSelectTool,
          selectionKeyCode: !isCanvasLocked && isSelectTool,
          multiSelectionKeyCode: !isCanvasLocked && isSelectTool ? 'Shift' : null,
          deleteKeyCode:
            isCanvasLocked || canvas.isShortcutSuppressed.value ? null : ['Backspace', 'Delete'],
          defaultEdgeOptions: { animated: false, updatable: true },
          applyDefault: false,
          minZoom: CANVAS_VIEWPORT_MIN_ZOOM,
          maxZoom: CANVAS_VIEWPORT_MAX_ZOOM,
          zoomOnScroll: false,
          panOnScroll: !isCanvasLocked,
          zoomOnPinch: !isCanvasLocked,
          panOnScrollMode: PanOnScrollMode.Free,
          onlyRenderVisibleElements: true,
          ...canvas.connectionBindings,
        },
        surface: canvas.surface,
        flow: canvas.flow,
        group: {
          props: {
            presentation: canvas.group,
            actionTarget: canvas.containerRef.value,
          },
        },
        node: {
          propsFor: (node: NodeProps<FlowNodeData>) => ({
            nodeId: node.id,
            data: node.data,
            selected: node.selected,
            dragging: node.dragging,
            copyMode: canvas.node.copyMode.value,
            uiState: canvas.node.getUiState(node.id),
            imagePresentation: canvas.node.imagePresentationFor(node.id),
            fusionPresentation: canvas.node.fusionPresentationFor(node.id),
            generationAvailable: canvas.node.generationAvailable.value,
            setImageStageRef: canvas.node.setImageStageRef,
            clearImageStageRef: canvas.node.clearImageStageRef,
            expandModeActive: canvas.node.expand.nodeId.value === node.id,
            expandOverlayStyle:
              canvas.node.expand.nodeId.value === node.id
                ? canvas.node.expand.overlayStyle.value
                : null,
            expandHandleRadius: canvas.node.expand.handleRadius,
            handleVisibilityRefreshToken: canvas.node.expand.visibilityRefreshToken.value,
          }),
          listeners: canvas.node.listeners,
        },
        minimap: {
          props: {
            enabled: canvas.minimap.enabled.value,
            inert: canvas.minimap.isHidden.value,
            isHidden: canvas.minimap.isHidden.value,
          },
          commands: canvas.minimap.commands,
        },
        overlays: {
          props: {
            imageToolSession: canvas.overlays.imageToolSession,
            nodeContextMenu: canvas.overlays.nodeContextMenu.value,
            canvasNodeMenuItems: canvas.overlays.canvasNodeMenuItems,
          },
          commands: {
            createNode: canvas.overlays.createNode,
          },
        },
      },
    }
  })
}

export type { CanvasScreenPresentationOptions }
