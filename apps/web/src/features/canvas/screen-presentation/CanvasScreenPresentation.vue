<template>
  <div
    class="canvas-layout"
    :class="[
      `tool-${activeTool}`,
      {
        'interaction-lite': canvas.isInteractionActive.value,
        'viewport-pan-lite': canvas.isViewportPanActive.value,
      },
    ]"
    @mousedown.self="chrome.closeBoardHeaderEditing"
  >
    <CanvasChrome
      v-bind="chromeProps"
      @update:board-name-draft="chrome.commands.updateBoardNameDraft"
      @toggle-board-menu="chrome.commands.toggleBoardMenu"
      @start-board-rename="chrome.commands.startBoardRename"
      @cancel-board-rename="chrome.commands.cancelBoardRename"
      @confirm-board-rename="chrome.commands.confirmBoardRename"
      @reload-board-catalog="chrome.commands.reloadBoardCatalog"
      @select-board="chrome.commands.selectBoard"
      @export="chrome.commands.exportSelectedImages"
      @navigate-back="chrome.commands.navigateBack"
      @select-tool="chrome.commands.selectTool"
      @toggle-minimap="chrome.commands.toggleMinimap"
      @zoom-out="chrome.commands.zoomOut"
      @zoom-in="chrome.commands.zoomIn"
    />

    <div
      :ref="canvas.containerRef"
      class="canvas-flow-wrap ib-page-dot-bg"
      @dragover.prevent="canvas.surface.onDragOver"
      @drop.prevent="canvas.surface.onDrop"
      @pointerdown.capture="canvas.surface.onPointerDownCapture"
      @contextmenu.capture="canvas.surface.onContextMenuCapture"
      @wheel="canvas.surface.onWheel"
    >
      <VueFlow
        :nodes="workspace.viewModel.flowPane.nodes.value"
        :edges="workspace.viewModel.flowPane.edges.value"
        v-bind="vueFlowProps"
        @nodes-change="canvas.flow.onNodesChange"
        @edges-change="canvas.flow.onEdgesChange"
        @connect-end="canvas.flow.onConnectEnd"
        @move-start="canvas.flow.onMoveStart"
        @move="canvas.flow.onMove"
        @move-end="canvas.flow.onMoveEnd"
        @viewport-change-end="canvas.flow.onViewportChangeEnd"
        @node-click="canvas.flow.onNodeClick"
        @node-drag-start="canvas.flow.onNodeDragStart"
        @node-drag-stop="canvas.flow.onNodeDragStop"
        @selection-start="canvas.flow.onSelectionStart"
        @selection-end="canvas.flow.onSelectionEnd"
        @pane-click="canvas.flow.onPaneClick"
        @pane-context-menu="canvas.flow.onPaneContextMenu"
      >
        <template #zoom-pane>
          <CanvasGroupPresentation
            :presentation="canvas.group"
            :action-target="canvas.containerRef.value"
          />
        </template>
        <template #node-canvas="nodeProps">
          <FlowCanvasNode
            v-bind="nodePropsFor(nodeProps)"
            @node-intent="canvas.node.dispatchIntent"
            @node-mousedown="canvas.node.listeners.nodeMousedown"
            @request-internals-update="canvas.node.listeners.requestInternalsUpdate"
          />
        </template>
        <CanvasChromeMiniMap
          v-bind="minimapProps"
          @center-requested="canvas.minimap.commands.centerRequested"
          @drag-start="canvas.minimap.commands.dragStart"
          @drag-move="canvas.minimap.commands.dragMove"
          @drag-end="canvas.minimap.commands.dragEnd"
        />
      </VueFlow>
      <CanvasOverlays v-bind="overlayProps" @create-node="canvas.overlays.createNode" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, type ComputedRef, type Ref } from 'vue'
import { PanOnScrollMode, VueFlow, type FlowEvents, type NodeProps } from '@vue-flow/core'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import FlowCanvasNode from '../components/FlowCanvasNode.vue'
import CanvasOverlays from '../components/CanvasOverlays.vue'
import {
  CanvasChrome,
  CanvasChromeMiniMap,
  type CanvasChromeAccountState,
  type CanvasChromeBoardState,
} from '../components/chrome'
import {
  CanvasGroupPresentation,
  type CanvasGroupPresentationInteraction,
} from '../group-presentation'
import type { CanvasImageTools } from '../image-tools'
import type {
  CanvasNodePresentationIntent,
  FusionNodePresentationBinding,
  ImageNodePresentationBinding,
} from '../node-presentation'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'
import type { CanvasWorkspaceRuntime } from '../workspace-runtime'
import { CANVAS_VIEWPORT_MAX_ZOOM, CANVAS_VIEWPORT_MIN_ZOOM } from '../utils/canvasViewportStorage'

type CanvasImageToolSession = {
  viewModel: Pick<CanvasImageTools['viewModel'], 'chrome'>
  commands: Pick<CanvasImageTools['commands'], 'chrome'>
}

type CanvasScreenWorkspace = {
  viewModel: {
    flowPane: Pick<CanvasWorkspaceRuntime['viewModel']['flowPane'], 'nodes' | 'edges'>
    graphEditing: { hasSelectedFlowNodes: Ref<boolean> }
    viewportControls: { zoomLevel: Ref<number> }
  }
}

type CanvasScreenPresentationSource = {
  workspace: CanvasScreenWorkspace
  chrome: {
    board: ComputedRef<CanvasChromeBoardState>
    account: ComputedRef<CanvasChromeAccountState>
    activeTool: Ref<'select' | 'hand'>
    minimapEnabled: Ref<boolean>
    isHidden: Ref<boolean>
    closeBoardHeaderEditing: () => void
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
    isInteractionActive: Ref<boolean>
    isViewportPanActive: Ref<boolean>
    isLocked: Ref<boolean>
    isShortcutSuppressed: ComputedRef<boolean>
    connectionBindings: CanvasWorkspaceRuntime['commands']['flowPane']['connectionAdapter']['vueFlowBindings']
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
      dispatchIntent: (intent: CanvasNodePresentationIntent) => void
      listeners: {
        nodeMousedown: (nodeId: string, event: MouseEvent) => void
        requestInternalsUpdate: (nodeId: string) => void
      }
    }
    minimap: {
      enabled: Ref<boolean>
      isHidden: Ref<boolean>
      commands: {
        centerRequested: (payload: { position: { x: number; y: number } }) => void
        dragStart: () => void
        dragMove: (payload: { delta: { x: number; y: number } }) => void
        dragEnd: () => void
      }
    }
    overlays: {
      imageToolSession: CanvasImageToolSession
      nodeContextMenu: Ref<{ visible: boolean; x: number; y: number }>
      canvasNodeMenuItems: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
      createNode: (kind: CanvasCreatableNodeKind) => void
    }
  }
}

const props = defineProps<{
  presentation: CanvasScreenPresentationSource
}>()

const { workspace, chrome, canvas } = props.presentation
const activeTool = computed(() => chrome.activeTool.value)
const chromeProps = computed(() => ({
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
}))
const vueFlowProps = computed(() => {
  const isSelectTool = chrome.activeTool.value === 'select'
  const isHandTool = chrome.activeTool.value === 'hand'
  const isCanvasLocked = canvas.isLocked.value

  return {
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
  }
})
const minimapProps = computed(() => ({
  enabled: canvas.minimap.enabled.value,
  inert: canvas.minimap.isHidden.value,
  isHidden: canvas.minimap.isHidden.value,
}))
const overlayProps = computed(() => ({
  imageToolSession: canvas.overlays.imageToolSession,
  nodeContextMenu: canvas.overlays.nodeContextMenu.value,
  canvasNodeMenuItems: canvas.overlays.canvasNodeMenuItems,
}))

function nodePropsFor(node: NodeProps<FlowNodeData>) {
  const isExpandModeActive = canvas.node.expand.nodeId.value === node.id

  return {
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
    expandModeActive: isExpandModeActive,
    expandOverlayStyle: isExpandModeActive ? canvas.node.expand.overlayStyle.value : null,
    expandHandleRadius: canvas.node.expand.handleRadius,
    handleVisibilityRefreshToken: canvas.node.expand.visibilityRefreshToken.value,
  }
}
</script>
<style>
@import '../components/styles/canvas-vue-flow-adapter-skin.css';
@import '../components/styles/canvas-style-contracts.css';
</style>

<style scoped>
:global(html.canvas-overscroll-guard),
:global(body.canvas-overscroll-guard) {
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-layout {
  --canvas-control-border-width: 1.2px;
  --canvas-tool-button-gap: 7px;
  --canvas-node-floating-ui-width: 287px;
  --canvas-surface-bg: rgba(255, 255, 255, 0.98);
  --canvas-surface-bg-soft: rgba(255, 255, 255, 0.94);
  --canvas-surface-muted: #f4f7fb;
  --canvas-surface-border: #d8ddea;
  --canvas-surface-border-soft: rgba(34, 42, 54, 0.08);
  --canvas-minimap-node: #c9ced8;
  --canvas-minimap-viewport: #c9ced8;
  --canvas-minimap-mask: rgb(201 206 216 / 0.18);
  --canvas-surface-shadow: 0 14px 32px rgba(15, 23, 42, 0.13);
  --canvas-surface-blur: 10px;
  --canvas-surface-radius: 10px;
  --canvas-floating-radius: 13px;
  --canvas-panel-radius: 8px;
  --canvas-panel-border-width: 1px;
  --canvas-panel-content-gap: 10px;
  --canvas-panel-border: rgba(34, 42, 54, 0.14);
  --canvas-panel-border-soft: rgba(34, 42, 54, 0.1);
  --canvas-panel-header-from: #f8fafc;
  --canvas-panel-header-to: #eef2f7;
  --canvas-text-strong: #111827;
  --canvas-text-body: #334155;
  --canvas-text-muted: #64748b;
  --canvas-text-subtle: #94a3b8;
  --canvas-accent: #9278e6;
  --canvas-accent-hover: #7a6bd5;
  --canvas-accent-strong: #6757c8;
  --canvas-accent-soft: rgba(146, 120, 230, 0.12);
  --canvas-accent-soft-strong: rgba(146, 120, 230, 0.16);
  --canvas-accent-wash: #f8f7ff;
  --canvas-accent-outline: rgba(122, 107, 213, 0.22);
  --flow-edge-color: rgb(177 177 183 / 50%);
  --flow-edge-active-color: rgb(146 120 230 / 50%);
  --flow-edge-selected-color: rgb(123 135 152 / 50%);
  --flow-edge-width: 2;
  --flow-edge-dasharray: 5;
  /* Execution edges need a continuous cycle rather than a finite interaction duration. */
  --flow-edge-running-duration: 0.5s;
  width: 100%;
  height: 100vh;
  background: #ffffff;
  position: relative;
  overflow: hidden;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-flow-wrap {
  position: absolute;
  inset: 0;
  z-index: 2;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}
</style>
