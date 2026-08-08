<template>
  <div
    class="canvas-layout"
    :class="[
      `tool-${activeTool}`,
      {
        'interaction-lite': isFlowInteractionActive,
        'viewport-pan-lite': isFlowViewportPanInteractionActive,
      },
    ]"
    @mousedown.self="closeBoardHeaderEditing"
  >
    <CanvasChrome
      v-bind="chromeProps"
      @update:board-name-draft="updateBoardNameDraft"
      @toggle-board-menu="toggleBoardMenu"
      @start-board-rename="startBoardRename"
      @cancel-board-rename="cancelBoardRename"
      @confirm-board-rename="confirmBoardRename"
      @reload-board-catalog="reloadBoardCatalog"
      @select-board="selectBoard"
      @export="exportSelectedImages"
      @navigate-back="navigateBack"
      @select-tool="setActiveTool"
      @toggle-minimap="toggleCanvasMinimap"
      @zoom-out="onZoomOut"
      @zoom-in="onZoomIn"
    />

    <div
      ref="canvasWrapRef"
      class="canvas-flow-wrap ib-page-dot-bg"
      @dragover.prevent="onCanvasDragOver"
      @drop.prevent="onCanvasDrop"
      @pointerdown.capture="onCanvasPointerDownCapture"
      @contextmenu.capture="onCanvasContextMenuCapture"
      @wheel="onCanvasWheel"
    >
      <VueFlow
        :nodes="nodes"
        :edges="edges"
        v-bind="vueFlowProps"
        @nodes-change="commands.flowPane.onNodesChange"
        @edges-change="vueFlowSelectionBindings.onEdgesChange"
        @connect-end="onFlowConnectEnd"
        @move-start="onFlowMoveStart"
        @move="onFlowMove"
        @move-end="onFlowMoveEnd"
        @viewport-change-end="onViewportChangeEnd"
        @node-click="onFlowNodeClick"
        @node-drag-start="onNodeDragStart"
        @node-drag-stop="onNodeDragStop"
        @selection-start="vueFlowSelectionBindings.onSelectionStart"
        @selection-end="vueFlowSelectionBindings.onSelectionEnd"
        @pane-click="onCanvasPaneClick"
        @pane-context-menu="onPaneContextMenu"
      >
        <template #zoom-pane>
          <CanvasGroupPresentation
            :presentation="groupPresentation"
            :action-target="canvasWrapRef"
          />
        </template>
        <template #node-canvas="nodeProps">
          <FlowCanvasNode
            v-bind="nodePropsFor(nodeProps)"
            @node-intent="nodePresentationIntentRouter.dispatch"
            @node-mousedown="selectionCommands.onFlowNodeMouseDown"
            @request-internals-update="onFlowNodeInternalsUpdate"
          />
        </template>
        <CanvasChromeMiniMap
          v-bind="minimapProps"
          @center-requested="onMiniMapCenterRequested"
          @drag-start="onMiniMapDragStart"
          @drag-move="handleMiniMapDragMove"
          @drag-end="onMiniMapDragEnd"
        />
      </VueFlow>
      <CanvasOverlays v-bind="overlayProps" @create-node="handleNodeContextMenuCreate" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { uploadApi } from '../../../shared/api/upload'
import { USER_BOARDS_PATH } from '../../../shared/constants/routePaths'
import { PanOnScrollMode, VueFlow, type NodeMouseEvent, type NodeProps } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowKeyboardShortcuts } from '../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../composables/useFlowLeaveGuard'
import { useFlowImageActionStrengthControl } from '../composables/useFlowImageActionStrengthControl'
import type { CanvasConnectionPort, CanvasCreatableNodeKind } from '../../canvas-board/domain'
import type { CanvasGraphNodeId } from '../graph-runtime'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
import { provideCanvasViewportContext } from '../composables/useCanvasViewportContext'
import {
  ACTIVE_NODE_PANEL_KIND,
  useFlowActiveNodePanels,
} from '../composables/useFlowActiveNodePanels'
import { useFlowCanvasShellInteractions } from '../composables/useFlowCanvasShellInteractions'
import {
  CanvasGroupPresentation,
  useCanvasGroupPresentationInteraction,
} from '../group-presentation'
import { useFlowExpandInteractionHandlers } from '../composables/useFlowExpandInteractionHandlers'
import {
  useFlowTransientUiDismissal,
  type DismissTransientCanvasUi,
} from '../composables/useFlowTransientUiDismissal'
import { useImageStageRegistry } from '../composables/useImageStageRegistry'
import { useFlowPaneSelectionAdapter, useVueFlowPaneAdapter } from '../flow-pane'
import { useCanvasWorkspaceRuntime } from '../workspace-runtime'
import {
  createFusionNodePresentationResolver,
  createImageNodePresentationResolver,
} from '../node-presentation'
import { createCanvasNodePresentationIntentRouter } from './createCanvasNodePresentationIntentRouter'
import { useCanvasNodeContextMenu } from './useCanvasNodeContextMenu'
import { useCanvasCustomLayerOverlay } from '../overlay-lifecycle'
import FlowCanvasNode from '../components/FlowCanvasNode.vue'
import CanvasOverlays from '../components/CanvasOverlays.vue'
import {
  CanvasChrome,
  CanvasChromeMiniMap,
  type CanvasChromeAccountState,
  type CanvasChromeBoardState,
} from '../components/chrome'
import type { FlowNodeData } from '../types/flowTypes'
import { CANVAS_VIEWPORT_MAX_ZOOM, CANVAS_VIEWPORT_MIN_ZOOM } from '../utils/canvasViewportStorage'
import {
  useUserAccountControls,
  type UserShellNavigationAdapter,
} from '../../user-shell/useUserAccountControls'

const props = defineProps<{
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
}>()

const { route, router, getUserIdentity, logoutAndRedirect } = props

function openRecharge() {
  accountControls.commands.openAccount('recharge')
}

const flowPaneAdapter = useVueFlowPaneAdapter()
const { screenToFlowCoordinate, viewport: flowViewport } = flowPaneAdapter

const canvasWrapRef = ref<HTMLElement | null>(null)
const flowHandleVisibilityRefreshToken = ref(0)

const activeTool = ref<'select' | 'hand'>('select')
const isCanvasMinimapEnabled = ref(false)
let dismissTransientCanvasUi: DismissTransientCanvasUi = () => {}
const {
  nodeContextMenu,
  canvasNodeMenuItems,
  close: closeNodeContextMenu,
  open: openNodeContextMenu,
  createNode: createNodeFromContextMenu,
} = useCanvasNodeContextMenu()

const createNodeKind = ref<CanvasCreatableNodeKind>('prompt')

const {
  activeNodePanel,
  openActiveNodePanel,
  clearActiveNodePanel,
  isActiveNodePanel,
  getActiveNodePanelNodeId,
} = useFlowActiveNodePanels()
const duplicateModifierPressed = ref(false)
const { setImageStageRef, clearImageStageRef, getImageStageElement } = useImageStageRegistry()
const { openNodeImagePreview, openImageUrlPreview } = useFlowImagePreviewLightbox()

const workspace = useCanvasWorkspaceRuntime({
  route,
  router,
  getUserIdentity,
  createNodeKind,
  flowPaneAdapter,
  uploadApi,
  openNodeImagePreview,
  openImageUrlPreview,
  logoutAndRedirect,
  startNodeDragInteraction: () => startNodeDragInteraction(),
  stopNodeDragInteraction: () => stopNodeDragInteraction(),
  refreshFlowHandleVisibility: () => refreshFlowHandleVisibility(),
  onNativeRemove: () => {
    dismissTransientCanvasUi({ closeExpand: true, closeContextMenu: true })
  },
  openRecharge,
})
const viewModel = workspace.viewModel
const commands = workspace.commands
const isGenerationAvailable = computed(() => commands.imageWorkflow.isGenerationAvailable())

const { nodes, edges } = viewModel.flowPane
const { connectionAdapter } = commands.flowPane

type CanvasBenchmarkConnectDeleteTarget = {
  sourceNodeId: CanvasGraphNodeId
  targetNodeId: CanvasGraphNodeId
  sourcePort: CanvasConnectionPort
  targetPort: CanvasConnectionPort
  deleteNodeId: CanvasGraphNodeId
}

type CanvasBenchmarkWindow = Window & {
  __ideaboardCanvasBenchmarkActions?: {
    connectDelete: (target: CanvasBenchmarkConnectDeleteTarget) => void
  }
}

const isCanvasBenchmarkActionsEnabled = computed(() => route.query.canvasBenchmarkActions === '1')

if (import.meta.env.DEV && isCanvasBenchmarkActionsEnabled.value) {
  const benchmarkWindow = window as CanvasBenchmarkWindow
  benchmarkWindow.__ideaboardCanvasBenchmarkActions = {
    connectDelete: (target) => {
      connectionAdapter.connectIntent({
        sourceNodeId: target.sourceNodeId,
        targetNodeId: target.targetNodeId,
        sourcePort: target.sourcePort,
        targetPort: target.targetPort,
      })
      commands.flowPane.onNodesChange([{ id: target.deleteNodeId, type: 'remove' }])
    },
  }
}

onBeforeUnmount(() => {
  delete (window as CanvasBenchmarkWindow).__ideaboardCanvasBenchmarkActions
})

const {
  currentBoardId,
  isBoardMenuOpen,
  isRenamingBoard,
  boardNameDraft,
  isRenamePending,
  catalogRecovery,
  currentBoardName,
  boardOptions,
} = viewModel.boardChrome
const {
  setBoardNameInputRef,
  toggleBoardMenu,
  startBoardRename,
  cancelBoardRename,
  closeBoardHeaderEditing,
  onConfirmBoardRename,
  reloadCatalog,
  onSelectBoard,
  onNavigateRequest,
  onLogoutRequest,
} = commands.boardSession

const canvasNavigationAdapter: UserShellNavigationAdapter = {
  navigate: onNavigateRequest,
  logout: onLogoutRequest,
}
const accountControls = useUserAccountControls({
  navigation: canvasNavigationAdapter,
  showHome: false,
})

const canvasChromeBoard = computed<CanvasChromeBoardState>(() => ({
  currentName: currentBoardName.value,
  options: boardOptions.value,
  currentId: currentBoardId.value,
  isMenuOpen: isBoardMenuOpen.value,
  isRenaming: isRenamingBoard.value,
  isRenamePending: isRenamePending.value,
  catalogRecovery: catalogRecovery.value,
  setNameInputRef: setBoardNameInputRef,
}))
const canvasChromeAccount = computed<CanvasChromeAccountState>(() => ({
  avatarUrl: accountControls.viewModel.avatarUrl.value,
  balance: accountControls.viewModel.balance.value,
  menuItems: accountControls.viewModel.menuItems.value,
}))
const { history: flowHistory } = viewModel.persistence

const { flushAutosave, loadCanvasBoardAndProbe, reattachPendingImageResults } = commands.persistence

const { zoomLevel } = viewModel.viewportControls
const {
  onZoomIn,
  onZoomOut,
  onPanViewport,
  onFlowMoveStart: onFlowMoveStartBase,
  onFlowMove: syncFlowMove,
  onFlowMoveEnd,
  onViewportChangeEnd,
  onMiniMapCenterRequested,
  onMiniMapDragStart: onMiniMapDragStartBase,
  onMiniMapDragMove,
  onMiniMapDragEnd,
  onNodeDragStart: onNodeDragStartBase,
  onNodeDragStop,
} = commands.viewportControls
provideCanvasViewportContext({ zoomLevel })

const { isFlowInteractionActive, isFlowViewportPanInteractionActive, isFlowInteractionLocked } =
  viewModel.graphEditing
const { groupFrames, selectionGroupAction } = viewModel.graphEditing.grouping
const {
  scheduleFlowNodeInternalsRefresh,
  createNodeAtPosition,
  onExportSelectedImages,
  onFlowMediaInputImageDrop,
  onFlowMediaInputImageOpen,
  onFlowMediaInputImagePreview,
  onFlowMediaInputImageRemove,
  onCanvasDrop,
  onCanvasDragOver: onFlowCanvasDropDragOver,
  onFlowImageOpenPreview,
  onFlowImageAppendToCanvas,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
  getFlowNodeUiStateCached,
} = commands.graphEditing
const {
  selectGroupAtPoint,
  handlePaneClick,
  handleNodeClick,
  groupSelectedNodes,
  ungroupSelectedGroup,
  deleteSelectedGroup,
  groupDrag,
  groupResize,
} = commands.graphEditing.grouping

function onFlowConnectEnd() {
  refreshFlowHandleVisibility()
}

function onFlowMove(payload: Parameters<typeof syncFlowMove>[0]) {
  syncFlowMove(payload)
  if (!payload.flowTransform || !imageTools.viewModel.chrome.value.isReady) return
  scheduleImageEditorChromeAnchorUpdate()
}

function handleMiniMapDragMove(payload: { delta: Parameters<typeof onMiniMapDragMove>[0] }) {
  onMiniMapDragMove(payload.delta)
}

function refreshFlowHandleVisibility() {
  flowHandleVisibilityRefreshToken.value += 1
}

function onFlowNodeInternalsUpdate(nodeId: string) {
  scheduleFlowNodeInternalsRefresh([nodeId])
}

const hasCanvasDismissableLayer = computed(
  () => nodeContextMenu.value.visible || activeNodePanel.value !== null,
)

const { runImageDirectAction } = commands.imageWorkflow

const {
  activeStrengthAction,
  consumeSuppressedStrengthClick,
  setStrengthRangeRef,
  setStrengthTriggerRef,
  hideStrengthControl,
  getActionStrength,
  onStrengthRangePointerDown,
  onStrengthInput,
  onStrengthChange,
  onStrengthPointerUp,
  onStrengthClick,
} = useFlowImageActionStrengthControl({
  onCommit: (action, strength) => {
    const targetNodeId = getActiveNodePanelNodeId(ACTIVE_NODE_PANEL_KIND.imageTools)
    if (!targetNodeId) return
    clearActiveNodePanel()
    void runImageDirectAction(targetNodeId, action, { strength })
  },
  onCancel: () => {
    clearActiveNodePanel()
  },
})

const {
  expandNodeId: flowExpandNodeId,
  expandOverlayStyle: flowExpandOverlayStyle,
  enterExpandMode: enterFlowExpandMode,
  cancelExpandMode: cancelFlowExpandMode,
  beginHandleResize: beginFlowExpandHandleResize,
  beginFrameDrag: beginFlowExpandFrameDrag,
  onHandlePointerMove: onFlowExpandHandlePointerMove,
  onHandlePointerUp: onFlowExpandHandlePointerUp,
  commitExpandMode: commitFlowExpandMode,
  handleRadius: flowExpandHandleRadius,
} = commands.imageWorkflow.createCanvasExtendOverlay({
  activeTool: activeTool as Ref<string>,
  getImageStageElement,
})

const imageTools = commands.imageWorkflow.createImageTools({
  activeTool: activeTool as Ref<string>,
  canvasWrapRef,
  isCanvasInteractionLocked: isFlowInteractionLocked,
  getImageStageElement,
  clearActiveNodePanel,
  hideStrengthControl,
  isImageActionPanelVisible: (nodeId) =>
    isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeId),
  isImagePromptComposerVisible: (nodeId) =>
    isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeId),
  strengthControl: {
    activeAction: activeStrengthAction,
    getValue: getActionStrength,
    setRangeRef: setStrengthRangeRef,
    setTriggerRef: setStrengthTriggerRef,
    onRangePointerDown: onStrengthRangePointerDown,
    onInput: onStrengthInput,
    onChange: onStrengthChange,
    onPointerUp: onStrengthPointerUp,
    onClick: onStrengthClick,
  },
})

const {
  viewModel: {
    active: { nodeId: flowImageEditorNodeId, maskNodeId: flowMaskEditorNodeId },
    locks: { isChromeHidden: isImageEditorChromeHidden, isCanvasLocked: isFlowCanvasLocked },
    nodeBindings: { stageFor: imageToolStageBindingFor, actionFor: imageToolActionBindingFor },
  },
  commands: {
    lifecycle: {
      enter: enterFlowImageEditorMode,
      cancel: cancelFlowMaskEditorMode,
      discardNodeEdits: discardFlowImageToolNodeEdits,
    },
    editor: {
      onCanvasWheel: onCanvasWheelBase,
      focusAndShowChrome: focusImageEditorAndShowChrome,
      scheduleChromeAnchorUpdate: scheduleImageEditorChromeAnchorUpdate,
    },
    prompt: { runPromptAction: runFlowImageToolPromptAction },
  },
} = imageTools

const imagePresentationFor = createImageNodePresentationResolver({
  actionBindingFor: imageToolActionBindingFor,
  stageBindingFor: imageToolStageBindingFor,
}).forNode

const fusionPresentationFor = createFusionNodePresentationResolver({
  promptComposerVisibleFor: (nodeId) =>
    isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.fusionPrompt, nodeId),
}).forNode

const isCanvasShortcutSuppressed = computed(
  () => !!flowImageEditorNodeId.value || !!flowMaskEditorNodeId.value,
)
const isCanvasGroupInteractionEnabled = computed(
  () =>
    activeTool.value === 'select' &&
    !isFlowCanvasLocked.value &&
    !flowImageEditorNodeId.value &&
    !flowMaskEditorNodeId.value &&
    !flowExpandNodeId.value,
)

function onFlowNodeClick(payload: NodeMouseEvent) {
  handleNodeClick({
    node: payload.node,
    selectNode: () => selectionCommands.onFlowNodeClick(),
  })
}

function onDeleteInlineSelection() {
  if (!isCanvasGroupInteractionEnabled.value) return false
  return deleteSelectedGroup()
}

function performSelectionGroupAction() {
  const action = selectionGroupAction.value
  if (action?.kind === 'group') {
    groupSelectedNodes()
    return
  }
  if (action?.kind === 'ungroup') {
    ungroupSelectedGroup()
  }
}

function toggleCanvasMinimapBase() {
  isCanvasMinimapEnabled.value = !isCanvasMinimapEnabled.value
}

function handleNodeContextMenuCreate(kind: CanvasCreatableNodeKind) {
  createNodeFromContextMenu(kind, createNodeAtPosition)
  dismissTransientCanvasUi({ closeExpand: true })
}

dismissTransientCanvasUi = useFlowTransientUiDismissal({
  cancelFlowExpandMode,
  closeBoardHeaderEditing,
  closeNodeContextMenu,
  clearActiveNodePanel,
  hideStrengthControl,
}).dismissTransientCanvasUi

useCanvasCustomLayerOverlay({
  visible: hasCanvasDismissableLayer,
  ignore: [
    '.node-tool-surface',
    '.canvas-node-context-menu',
    '[data-image-mask-editor]',
    '[data-image-editor]',
  ],
  ownerDocument: () =>
    canvasWrapRef.value?.ownerDocument ?? (typeof document === 'undefined' ? null : document),
  onDismiss: () => {
    dismissTransientCanvasUi({ closeContextMenu: true })
  },
})

const {
  onFlowImageOpenFile,
  onFlowImageReset: onFlowImageResetBase,
  onFlowImageStageDrop,
} = commands.nodeInteraction.createNodeMediaInputs({
  discardNodeEdits: discardFlowImageToolNodeEdits,
})

const { onFlowDeleteNode, onFlowPromptTextUpdate, onFlowNodeParamUpdate, onFlowImageReset } =
  commands.nodeInteraction.createNodeHandlers({
    dismissTransientCanvasUi,
    discardFlowImageToolNodeEdits,
    _onFlowImageReset: onFlowImageResetBase,
  })

const { onFlowImageExpandSubmit, onFlowExpandHandlePointerDown, onFlowExpandFramePointerDown } =
  useFlowExpandInteractionHandlers({
    flowExpandNodeId,
    clearActiveNodePanel,
    hideStrengthControl,
    commitFlowExpandMode,
    beginFlowExpandHandleResize,
    beginFlowExpandFrameDrag,
  })

const { onFlowPromptGenerate } = commands.imageWorkflow.createPromptGenerationHandlers({
  cancelFlowExpandMode,
  cancelFlowMaskEditorMode,
})

const {
  paneSelectionBindings,
  vueFlowSelectionBindings,
  commands: selectionCommands,
} = useFlowPaneSelectionAdapter({
  activeTool: activeTool as Ref<string>,
  dismissTransientCanvasUi,
  onEdgesChange: (changes) => commands.flowPane.onEdgesChange(changes),
})

const groupPresentation = useCanvasGroupPresentationInteraction({
  canvas: {
    host: canvasWrapRef,
    nodes,
    viewport: flowViewport,
    isInteractionEnabled: isCanvasGroupInteractionEnabled,
  },
  grouping: {
    frames: groupFrames,
    action: selectionGroupAction,
    selectAtPoint: selectGroupAtPoint,
    drag: groupDrag,
    resize: groupResize,
    onAction: performSelectionGroupAction,
  },
  pane: {
    toFlowPoint: screenToFlowCoordinate,
    onPointerDownCapture: paneSelectionBindings.onPointerDownCapture,
    dismiss: (options) => dismissTransientCanvasUi(options),
  },
})
const { bindClosePolicyHandlers, onCanvasContextMenuCapture, onCanvasPointerDownCapture } =
  groupPresentation

const { onNodeDragStart: startNodeDragInteraction, onNodeDragStop: stopNodeDragInteraction } =
  useFlowNodeDragInteractions({
    beginNodeDragTransaction: commands.flowPane.nodeDragTransactions.begin,
    commitNodeDragTransaction: commands.flowPane.nodeDragTransactions.commit,
    duplicateModifierPressed,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
    isNodeDragLocked: () => isFlowCanvasLocked.value,
  })

const {
  onPaneContextMenu: onPaneContextMenuBase,
  setActiveTool: setActiveToolBase,
  onEscapeShortcut,
  onCanvasDragOver: onCanvasDragOverBase,
} = useFlowCanvasShellInteractions({
  canvasWrapRef,
  activeTool,
  screenToFlowCoordinate,
  openNodeContextMenu,
  dismissTransientCanvasUi,
  clearFlowSelection: selectionCommands.clearSelection,
  onCanvasDropDragOver: onFlowCanvasDropDragOver,
  isFlowExpandActive: () => !!flowExpandNodeId.value,
  isBoardHeaderEditingActive: () => isBoardMenuOpen.value || isRenamingBoard.value,
})

const {
  onCanvasWheel,
  onFlowMoveStart,
  onMiniMapDragStart,
  onNodeDragStart,
  onCanvasPaneClick,
  toggleCanvasMinimap,
  onPaneContextMenu,
  setActiveTool,
  onCanvasDragOver,
} = bindClosePolicyHandlers({
  onCanvasWheel: onCanvasWheelBase,
  onFlowMoveStart: onFlowMoveStartBase,
  onMiniMapDragStart: onMiniMapDragStartBase,
  onNodeDragStart: onNodeDragStartBase,
  onCanvasPaneClick: () => handlePaneClick(selectionCommands.onPaneClick),
  toggleCanvasMinimap: toggleCanvasMinimapBase,
  onPaneContextMenu: onPaneContextMenuBase,
  setActiveTool: setActiveToolBase,
  onCanvasDragOver: onCanvasDragOverBase,
})

const {
  onFlowImageContextMenu,
  onFlowFusionContextMenu,
  onFlowImageDirectAction,
  onFlowImagePromptGenerate,
  onFlowFusionPromptGenerate,
} = commands.imageWorkflow.createImageActionHandlers({
  flowImageEditorNodeId,
  consumeSuppressedStrengthClick,
  openActiveNodePanel,
  clearActiveNodePanel,
  hideStrengthControl,
  selectFlowNodeForContextMenu: selectionCommands.selectNodeForContextMenu,
  cancelFlowExpandMode,
  cancelFlowMaskEditorMode,
  enterFlowExpandMode,
  enterFlowImageEditorMode,
  focusImageEditorAndShowChrome,
  runImagePromptAction: runFlowImageToolPromptAction,
  getActionStrength,
})

const nodePresentationIntentRouter = createCanvasNodePresentationIntentRouter({
  prompt: {
    updateText: onFlowPromptTextUpdate,
    updateParameters: onFlowNodeParamUpdate,
    generate: onFlowPromptGenerate,
  },
  node: { delete: onFlowDeleteNode },
  mediaInput: {
    drop: onFlowMediaInputImageDrop,
    open: onFlowMediaInputImageOpen,
    preview: onFlowMediaInputImagePreview,
    remove: onFlowMediaInputImageRemove,
  },
  image: {
    dropStage: onFlowImageStageDrop,
    openFile: onFlowImageOpenFile,
    preview: onFlowImageOpenPreview,
    appendToCanvas: onFlowImageAppendToCanvas,
    reset: onFlowImageReset,
    openContextMenu: onFlowImageContextMenu,
    generateFromPrompt: onFlowImagePromptGenerate,
    runAction: onFlowImageDirectAction,
  },
  canvasExtend: {
    submit: onFlowImageExpandSubmit,
    beginHandle: onFlowExpandHandlePointerDown,
    moveHandle: onFlowExpandHandlePointerMove,
    endHandle: onFlowExpandHandlePointerUp,
    beginFrame: onFlowExpandFramePointerDown,
  },
  fusion: {
    openContextMenu: onFlowFusionContextMenu,
    generate: onFlowFusionPromptGenerate,
  },
})

const chromeProps = computed(() => ({
  inert: isImageEditorChromeHidden.value,
  isHidden: isImageEditorChromeHidden.value,
  board: canvasChromeBoard.value,
  account: canvasChromeAccount.value,
  tools: {
    activeTool: activeTool.value,
    minimapEnabled: isCanvasMinimapEnabled.value,
  },
  status: {
    exportDisabled: !viewModel.graphEditing.hasSelectedFlowNodes.value,
    zoomLevel: viewModel.viewportControls.zoomLevel.value,
  },
}))

const vueFlowProps = computed(() => {
  const isSelectTool = activeTool.value === 'select'
  const isHandTool = activeTool.value === 'hand'
  const isCanvasLocked = isFlowCanvasLocked.value

  return {
    panOnDrag: !isCanvasLocked && isHandTool,
    nodesDraggable: !isCanvasLocked && !isHandTool,
    nodesConnectable: !isCanvasLocked && !isHandTool,
    elementsSelectable: !isCanvasLocked && !isHandTool,
    selectNodesOnDrag: !isCanvasLocked && isSelectTool,
    selectionKeyCode: !isCanvasLocked && isSelectTool,
    multiSelectionKeyCode: !isCanvasLocked && isSelectTool ? 'Shift' : null,
    deleteKeyCode:
      isCanvasLocked || isCanvasShortcutSuppressed.value ? null : ['Backspace', 'Delete'],
    defaultEdgeOptions: { animated: false, updatable: true },
    applyDefault: false,
    minZoom: CANVAS_VIEWPORT_MIN_ZOOM,
    maxZoom: CANVAS_VIEWPORT_MAX_ZOOM,
    zoomOnScroll: false,
    panOnScroll: !isCanvasLocked,
    zoomOnPinch: !isCanvasLocked,
    panOnScrollMode: PanOnScrollMode.Free,
    onlyRenderVisibleElements: true,
    ...connectionAdapter.vueFlowBindings,
  }
})

const minimapProps = computed(() => ({
  enabled: isCanvasMinimapEnabled.value,
  inert: isImageEditorChromeHidden.value,
  isHidden: isImageEditorChromeHidden.value,
}))

const overlayProps = computed(() => ({
  imageToolSession: imageTools,
  nodeContextMenu: nodeContextMenu.value,
  canvasNodeMenuItems,
}))

function nodePropsFor(node: NodeProps<FlowNodeData>) {
  const isExpandModeActive = flowExpandNodeId.value === node.id

  return {
    nodeId: node.id,
    data: node.data,
    selected: node.selected,
    dragging: node.dragging,
    copyMode: duplicateModifierPressed.value,
    uiState: getFlowNodeUiStateCached(node.id),
    imagePresentation: imagePresentationFor(node.id),
    fusionPresentation: fusionPresentationFor(node.id),
    generationAvailable: isGenerationAvailable.value,
    setImageStageRef,
    clearImageStageRef,
    expandModeActive: isExpandModeActive,
    expandOverlayStyle: isExpandModeActive ? flowExpandOverlayStyle.value : null,
    expandHandleRadius: flowExpandHandleRadius,
    handleVisibilityRefreshToken: flowHandleVisibilityRefreshToken.value,
  }
}

function updateBoardNameDraft(value: string) {
  boardNameDraft.value = value
}

function confirmBoardRename() {
  void onConfirmBoardRename()
}

function reloadBoardCatalog() {
  void reloadCatalog()
}

function selectBoard(boardId: string) {
  void onSelectBoard(boardId)
}

function exportSelectedImages() {
  void onExportSelectedImages()
}

function navigateBack() {
  canvasNavigationAdapter.navigate(USER_BOARDS_PATH)
}

watch(
  () => route.query.boardId,
  async () => {
    await loadCanvasBoardAndProbe()
    void reattachPendingImageResults()
  },
)

useFlowKeyboardShortcuts({
  onSave: () => {
    void flushAutosave({ silent: false })
  },
  onUndo: flowHistory.undo,
  onRedo: flowHistory.redo,
  onDeleteInlineSelection,
  isCanvasShortcutSuppressed: () => isCanvasShortcutSuppressed.value,
  onEscape: onEscapeShortcut,
  onPanViewport,
  onToolSelect: () => {
    setActiveTool('select')
  },
  onToolHand: () => {
    setActiveTool('hand')
  },
  onDuplicateModifierChange: (pressed) => {
    duplicateModifierPressed.value = pressed
  },
})

useFlowLeaveGuard({
  onLeave: () => flushAutosave(),
})

onMounted(async () => {
  await loadCanvasBoardAndProbe()
  void reattachPendingImageResults()
})
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
