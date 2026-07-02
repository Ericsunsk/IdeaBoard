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
    <CanvasPageChrome
      :active-tool="activeTool"
      :inert="isImageEditorChromeHidden"
      :chrome-hidden="isImageEditorChromeHidden"
      :current-board-name="currentBoardName"
      :board-options="boardOptions"
      :current-board-id="currentBoardId"
      :is-board-menu-open="isBoardMenuOpen"
      :is-renaming-board="isRenamingBoard"
      :set-board-name-input-ref="setBoardNameInputRef"
      :export-disabled="!hasSelectedFlowNodes"
      :avatar-url="userStore.user?.avatar ?? undefined"
      :minimap-enabled="isCanvasMinimapEnabled"
      :zoom-level="zoomLevel"
      @update:board-name-draft="boardNameDraft = $event"
      @toggle-board-menu="toggleBoardMenu"
      @start-board-rename="startBoardRename"
      @cancel-board-rename="cancelBoardRename"
      @confirm-board-rename="onConfirmBoardRename"
      @select-board="onSelectBoard"
      @export="onExportSelectedImages"
      @open-center="onNavigateRequest(USER_CENTER_PATH)"
      @logout="onLogoutRequest"
      @navigate-back="onNavigateRequest(USER_BOARDS_PATH)"
      @select-tool="setActiveTool"
      @toggle-minimap="toggleCanvasMinimap"
      @zoom-out="onZoomOut"
      @zoom-in="onZoomIn"
    />

    <div
      ref="canvasWrapRef"
      class="canvas-flow-wrap"
      @dragover.prevent="onCanvasDragOver"
      @drop.prevent="onCanvasDrop"
      @wheel="onCanvasWheel"
    >
      <VueFlow
        v-model:nodes="nodes"
        v-model:edges="edges"
        :pan-on-drag="!isFlowCanvasLocked && activeTool === 'hand'"
        :nodes-draggable="!isFlowCanvasLocked && activeTool !== 'hand'"
        :nodes-connectable="!isFlowCanvasLocked && activeTool !== 'hand'"
        :elements-selectable="!isFlowCanvasLocked && activeTool !== 'hand'"
        :select-nodes-on-drag="!isFlowCanvasLocked && activeTool === 'select'"
        :selection-key-code="!isFlowCanvasLocked && activeTool === 'select'"
        :multi-selection-key-code="!isFlowCanvasLocked && activeTool === 'select' ? 'Shift' : null"
        :delete-key-code="nativeDeleteKeyCode"
        :default-edge-options="{ animated: false, updatable: true }"
        :min-zoom="0.1"
        :zoom-on-scroll="false"
        :pan-on-scroll="!isFlowCanvasLocked"
        :zoom-on-pinch="!isFlowCanvasLocked"
        :pan-on-scroll-mode="PanOnScrollMode.Free"
        :only-render-visible-elements="true"
        :is-valid-connection="isValidConnection"
        @nodes-change="onNodesChange"
        @edges-change="onEdgesChange"
        @connect="onConnect"
        @connect-end="onFlowConnectEnd"
        @edge-update="onEdgeUpdate"
        @move-start="onFlowMoveStart"
        @move="onFlowMove"
        @move-end="onFlowMoveEnd"
        @viewport-change-end="onViewportChangeEnd"
        @node-click="onFlowNodeClick"
        @node-drag-start="onNodeDragStart"
        @node-drag-stop="onNodeDragStop"
        @pane-click="onPaneClick"
        @pane-context-menu="onPaneContextMenu"
      >
        <template #node-canvas="nodeProps">
          <FlowCanvasNode
            :node-id="nodeProps.id"
            :data="nodeProps.data"
            :selected="!!nodeProps.selected"
            :dragging="!!nodeProps.dragging"
            :copy-mode="duplicateModifierPressed"
            :ui-state="getFlowNodeUiStateCached(nodeProps.id)"
            :image-action-binding="imageToolActionBindingFor(nodeProps.id)"
            :fusion-prompt-composer-visible="
              isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.fusionPrompt, nodeProps.id)
            "
            :set-image-stage-ref="setImageStageRef"
            :clear-image-stage-ref="clearImageStageRef"
            :expand-mode-active="flowExpandNodeId === nodeProps.id"
            :expand-overlay-style="
              flowExpandNodeId === nodeProps.id ? flowExpandOverlayStyle : null
            "
            :expand-handle-radius="flowExpandHandleRadius"
            :image-tool-stage-binding="imageToolStageBindingFor(nodeProps.id)"
            :handle-visibility-refresh-token="flowHandleVisibilityRefreshToken"
            @update-prompt-text="onFlowPromptTextUpdate"
            @update-node-param="onFlowNodeParamUpdate"
            @delete-node="onFlowDeleteNode"
            @image-stage-drop="onFlowImageStageDrop"
            @media-input-image-drop="onFlowMediaInputImageDrop"
            @media-input-image-open="onFlowMediaInputImageOpen"
            @media-input-image-preview="onFlowMediaInputImagePreview"
            @media-input-image-remove="onFlowMediaInputImageRemove"
            @image-open-file="onFlowImageOpenFile"
            @image-open-preview="onFlowImageOpenPreview"
            @image-append-to-canvas="onFlowImageAppendToCanvas"
            @image-reset="onFlowImageReset"
            @node-mousedown="onFlowNodeMouseDown"
            @request-internals-update="onFlowNodeInternalsUpdate"
            @image-contextmenu="onFlowImageContextMenu"
            @image-prompt-generate="onFlowImagePromptGenerate"
            @image-expand-submit="onFlowImageExpandSubmit"
            @fusion-contextmenu="onFlowFusionContextMenu"
            @fusion-prompt-generate="onFlowFusionPromptGenerate"
            @prompt-generate="onFlowPromptGenerate"
            @image-action="onFlowImageDirectAction"
            @expand-handle-pointerdown="onFlowExpandHandlePointerDown"
            @expand-handle-pointermove="onFlowExpandHandlePointerMove"
            @expand-handle-pointerup="onFlowExpandHandlePointerUp"
            @expand-frame-pointerdown="onFlowExpandFramePointerDown"
          />
        </template>
        <CanvasMiniMap
          v-if="isCanvasMinimapEnabled"
          @center-requested="onMiniMapCenterRequested"
          @drag-start="onMiniMapDragStart"
          @drag-move="handleMiniMapDragMove"
          @drag-end="onMiniMapDragEnd"
        />
      </VueFlow>
      <CanvasOverlays
        :chrome-model="imageToolChromeModel"
        :chrome-commands="imageToolChromeCommands"
        :node-context-menu="nodeContextMenu"
        :canvas-node-menu-items="canvasNodeMenuItems"
        @create-node="handleNodeContextMenuCreate"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../../../app/stores/user'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { uploadApi } from '../../../shared/api/upload'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useDismissableLayer } from '../../../shared/composables/useDismissableLayer'
import { USER_BOARDS_PATH, USER_CENTER_PATH } from '../../../shared/constants/routePaths'
import type { Edge, Node } from '@vue-flow/core'
import { PanOnScrollMode, useVueFlow, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowKeyboardShortcuts } from '../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../composables/useFlowLeaveGuard'
import { useFlowImageActionStrengthControl } from '../composables/useFlowImageActionStrengthControl'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import { useFlowSelectionController } from '../composables/useFlowSelectionController'
import { useFlowNodeMediaInputs } from '../composables/useFlowNodeMediaInputs'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
import { provideCanvasViewportContext } from '../composables/useCanvasViewportContext'
import { useCanvasViewportRuntime } from '../composables/useCanvasViewportRuntime'
import {
  ACTIVE_NODE_PANEL_KIND,
  useFlowActiveNodePanels,
} from '../composables/useFlowActiveNodePanels'
import { useFlowCanvasShellInteractions } from '../composables/useFlowCanvasShellInteractions'
import { useFlowImageActionOrchestrator } from '../composables/useFlowImageActionOrchestrator'
import { useFlowPromptGenerationOrchestrator } from '../composables/useFlowPromptGenerationOrchestrator'
import { useFlowExpandInteractionHandlers } from '../composables/useFlowExpandInteractionHandlers'
import { useFlowCanvasNodeHandlers } from '../composables/useFlowCanvasNodeHandlers'
import {
  useFlowTransientUiDismissal,
  type DismissTransientCanvasUi,
} from '../composables/useFlowTransientUiDismissal'
import { useImageCanvasExtendOverlay } from '../composables/useImageCanvasExtendOverlay'
import { useImageStageRegistry } from '../composables/useImageStageRegistry'
import { useCanvasBoardRuntime } from '../composables/useCanvasBoardRuntime'
import { useCanvasGraphRuntime } from '../composables/useCanvasGraphRuntime'
import { useCanvasImageTools } from '../image-tools'
import { useCanvasTaskRuntime } from '../composables/useCanvasTaskRuntime'
import { useCanvasFeedback } from '../feedback/useCanvasFeedback'
import FlowCanvasNode from '../components/FlowCanvasNode.vue'
import CanvasMiniMap from '../components/CanvasMiniMap.vue'
import CanvasOverlays from '../components/CanvasOverlays.vue'
import CanvasPageChrome from '../components/CanvasPageChrome.vue'

const canvasFeedback = useCanvasFeedback()
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect()
const {
  screenToFlowCoordinate,
  updateNodeInternals,
  zoomIn,
  zoomOut,
  viewport,
  panBy,
  getViewport,
  setViewport,
  setCenter,
  getSelectedElements,
  removeSelectedElements,
} = useVueFlow()

const canvasWrapRef = ref<HTMLElement | null>(null)
const flowHandleVisibilityRefreshToken = ref(0)

const activeTool = ref<'select' | 'hand'>('select')
const isCanvasMinimapEnabled = ref(false)
let dismissTransientCanvasUi: DismissTransientCanvasUi = () => {}

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

const {
  nodes,
  edges,
  fullNodes,
  fullEdges,
  currentBoardId,
  loadInitialOrRouteBoard,
  flushAutosave,
  flowHistory,
  flowTransactions,
  onNodesChange,
  onEdgesChange,
  isBoardMenuOpen,
  isRenamingBoard,
  boardNameDraft,
  currentBoardName,
  boardOptions,
  setBoardNameInputRef,
  refreshBoardMetas,
  toggleBoardMenu,
  startBoardRename,
  cancelBoardRename,
  closeBoardHeaderEditing,
  onConfirmBoardRename,
  onSelectBoard,
  onNavigateRequest,
  onLogoutRequest,
  markBoardViewportChanged,
  commitPendingBoardViewportChange,
} = useCanvasBoardRuntime({
  route,
  router,
  getUserIdentity: () => ({
    userId: Number(userStore.user?.id),
    userUid: `${userStore.user?.uid ?? ''}`.trim(),
  }),
  getViewport,
  setViewport: (viewport) => setViewport(viewport),
  logoutAndRedirect,
  message: canvasFeedback.boardSession.runtimeMessage,
  onNativeRemove: () => {
    dismissTransientCanvasUi({ closeExpand: true, closeContextMenu: true })
  },
})

const {
  zoomLevel,
  onZoomIn,
  onZoomOut,
  onPanViewport,
  onFlowMoveStart,
  onFlowMove: syncFlowMove,
  onFlowMoveEnd,
  onViewportChangeEnd,
  onMiniMapCenterRequested,
  onMiniMapDragStart,
  onMiniMapDragMove,
  onMiniMapDragEnd,
  onNodeDragStart,
  onNodeDragStop,
  loadCanvasBoardAndProbe,
} = useCanvasViewportRuntime({
  nodes,
  edges,
  fullNodes,
  fullEdges,
  currentBoardId,
  viewport,
  getViewport,
  setViewport: (nextViewport, options) => setViewport(nextViewport, options),
  setCenter: (x, y, options) => setCenter(x, y, options),
  zoomIn,
  zoomOut,
  panBy,
  markBoardViewportChanged,
  commitPendingBoardViewportChange,
  loadInitialOrRouteBoard,
  closeBoardHeaderEditing,
  startCanvasInteraction: (key) => startFlowInteraction(key),
  stopCanvasInteraction: (key) => stopFlowInteraction(key),
  startNodeDragInteraction: () => startNodeDragInteraction(),
  stopNodeDragInteraction: () => stopNodeDragInteraction(),
  refreshFlowHandleVisibility: () => refreshFlowHandleVisibility(),
})
provideCanvasViewportContext({ zoomLevel })

const { openNodeImagePreview, openImageUrlPreview } = useFlowImagePreviewLightbox()
const {
  scheduleFlowNodeInternalsRefresh,
  clearSelectedFlowElementsSync,
  isValidConnection,
  onConnect,
  onEdgeUpdate,
  createNodeAtPosition,
  _onFlowDeleteNode,
  _onFlowPromptTextUpdate,
  _onFlowNodeParamUpdate,
  markFlowExecutionNodeDirty,
  updateSnapshotNode,
  hasSelectedFlowNodes,
  onExportSelectedImages,
  isFlowInteractionActive,
  isFlowViewportPanInteractionActive,
  isFlowInteractionLocked,
  startFlowInteraction,
  stopFlowInteraction,
  nodeContextMenu,
  canvasNodeMenuItems,
  closeNodeContextMenu,
  openNodeContextMenu,
  getImageNodeSnapshot,
  getImageNodeDisplayPayload,
  uploadImageToBoard,
  applyImageFileToImageNode,
  createImageNodeFromFile,
  onFlowMediaInputImageDrop,
  onFlowMediaInputImageOpen,
  onFlowMediaInputImagePreview,
  onFlowMediaInputImageRemove,
  getMediaInputImages,
  resolvePromptInputImageNodeIds,
  onCanvasDrop,
  onCanvasDragOver: onFlowCanvasDropDragOver,
  onFlowImageOpenPreview,
  onFlowImageAppendToCanvas,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
  getFlowNodeUiStateCached,
} = useCanvasGraphRuntime({
  nodes: fullNodes as Ref<Node[]>,
  edges: fullEdges as Ref<Edge[]>,
  currentBoardId,
  createNodeKind,
  flowTransactions,
  updateNodeInternals,
  screenToFlowCoordinate,
  getSelectedElements,
  removeSelectedElements,
  uploadApi,
  sanitizeApiErrorMessage,
  openNodeImagePreview,
  openImageUrlPreview,
  message: canvasFeedback.graph.runtimeMessage,
})

function onFlowConnectEnd() {
  refreshFlowHandleVisibility()
}

function onFlowMove(payload: Parameters<typeof syncFlowMove>[0]) {
  syncFlowMove(payload)
  if (!payload.flowTransform || !imageToolChromeModel.value.isReady) return
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

const {
  isNodeDirectActionRunning,
  reattachPendingImageResults,
  runImageDirectAction,
  runCanvasExtendAction,
  runImageInpaintAction,
  runImagePromptGeneration,
  runImagePromptGenerationFromSource,
  runFusionPromptGeneration,
  runTextPromptGeneration,
} = useCanvasTaskRuntime({
  nodes: fullNodes as Ref<Node[]>,
  edges: fullEdges as Ref<Edge[]>,
  flowTransactions,
  flushAutosave,
  currentBoardId,
  refreshNodeInternals: scheduleFlowNodeInternalsRefresh,
  message: canvasFeedback.taskExecution.runtimeMessage,
})

const {
  activeStrengthAction,
  suppressClickAction,
  setStrengthRangeRef,
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
} = useImageCanvasExtendOverlay({
  nodes: nodes as Ref<Node[]>,
  activeTool: activeTool as Ref<string>,
  getImageNodeDisplayPayload,
  getImageStageElement,
  onBeforePointerCancel: clearSelectedFlowElementsSync,
  onCommit: async (payload) => {
    await runCanvasExtendAction(payload.nodeId, {
      frameRect: payload.frameRect,
      originalImageRect: payload.originalImageRect,
      prompt: payload.prompt,
      outputCount: payload.outputCount,
    })
  },
})

const imageTools = useCanvasImageTools({
  nodes: nodes as Ref<Node[]>,
  activeTool: activeTool as Ref<string>,
  canvasWrapRef,
  isCanvasInteractionLocked: isFlowInteractionLocked,
  currentBoardId,
  getImageNodeDisplayPayload,
  getImageNodeSnapshot,
  getImageStageElement,
  screenToFlowCoordinate,
  getViewport,
  setViewport: (viewport, options) => setViewport(viewport, options),
  setCenter: (x, y, options) => setCenter(x, y, options),
  createImageNodeFromFile,
  connectImageNodes: onConnect,
  uploadImageToBoard,
  openImageUrlPreview,
  clearActiveNodePanel,
  hideStrengthControl,
  updateOutputCount: (nodeId, payload) => {
    _onFlowNodeParamUpdate(nodeId, payload)
    markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
  },
  runInpaintAction: runImageInpaintAction,
  runPromptGeneration: runImagePromptGeneration,
  isImageActionPanelVisible: (nodeId) =>
    isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeId),
  isImagePromptComposerVisible: (nodeId) =>
    isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeId),
  isDirectActionRunning: isNodeDirectActionRunning,
  activeStrengthAction,
  getActionStrength,
  setStrengthRangeRef,
  onStrengthRangePointerDown,
  onStrengthInput,
  onStrengthChange,
  onStrengthPointerUp,
  onStrengthClick,
  clearSelectedFlowElements: clearSelectedFlowElementsSync,
  startCanvasInteraction: startFlowInteraction,
  stopCanvasInteraction: stopFlowInteraction,
  message: canvasFeedback.imageTools.runtimeMessage,
})

const {
  activeNodeId: flowImageEditorNodeId,
  activeMaskNodeId: flowMaskEditorNodeId,
  chromeModel: imageToolChromeModel,
  chromeCommands: imageToolChromeCommands,
  isChromeHidden: isImageEditorChromeHidden,
  isCanvasLocked: isFlowCanvasLocked,
  enter: enterFlowImageEditorMode,
  cancel: cancelFlowMaskEditorMode,
  stageBindingFor: imageToolStageBindingFor,
  actionBindingFor: imageToolActionBindingFor,
  discardNodeEdits: discardFlowImageToolNodeEdits,
  runPromptAction: runFlowImageToolPromptAction,
  onCanvasWheel,
  focusAndShowChrome: focusImageEditorAndShowChrome,
  scheduleChromeAnchorUpdate: scheduleImageEditorChromeAnchorUpdate,
} = imageTools

const isCanvasShortcutSuppressed = computed(
  () => !!flowImageEditorNodeId.value || !!flowMaskEditorNodeId.value,
)
const nativeDeleteKeyCode = computed(() =>
  isFlowCanvasLocked.value || isCanvasShortcutSuppressed.value ? null : ['Backspace', 'Delete'],
)

function toggleCanvasMinimap() {
  isCanvasMinimapEnabled.value = !isCanvasMinimapEnabled.value
}

dismissTransientCanvasUi = useFlowTransientUiDismissal({
  cancelFlowExpandMode,
  closeBoardHeaderEditing,
  closeNodeContextMenu,
  clearActiveNodePanel,
  hideStrengthControl,
}).dismissTransientCanvasUi

useDismissableLayer({
  enabled: hasCanvasDismissableLayer,
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
} = useFlowNodeMediaInputs({
  applyImageFileToImageNode: async (nodeId, file): Promise<boolean> => {
    const updated = await applyImageFileToImageNode(nodeId, file)
    if (updated) discardFlowImageToolNodeEdits(nodeId)
    return updated
  },
  updateSnapshotNode,
  markFlowExecutionNodeDirty,
})

const { onFlowDeleteNode, onFlowPromptTextUpdate, onFlowNodeParamUpdate, onFlowImageReset } =
  useFlowCanvasNodeHandlers({
    dismissTransientCanvasUi,
    discardFlowImageToolNodeEdits,
    _onFlowDeleteNode,
    _onFlowPromptTextUpdate,
    _onFlowNodeParamUpdate,
    _onFlowImageReset: onFlowImageResetBase,
    markFlowExecutionNodeDirty,
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

const { onFlowPromptGenerate } = useFlowPromptGenerationOrchestrator({
  cancelFlowExpandMode,
  cancelFlowMaskEditorMode,
  getMediaInputImages,
  resolvePromptInputImageNodeIds,
  runTextPromptGeneration,
  runImagePromptGeneration,
  runImagePromptGenerationFromSource,
})

const {
  clearFlowSelection,
  selectFlowNodeForContextMenu,
  onPaneClick,
  onFlowNodeClick,
  onFlowNodeMouseDown,
} = useFlowSelectionController({
  activeTool: activeTool as Ref<string>,
  dismissTransientCanvasUi,
})

const { onNodeDragStart: startNodeDragInteraction, onNodeDragStop: stopNodeDragInteraction } =
  useFlowNodeDragInteractions({
    beginNodeDragTransaction: flowTransactions.beginNodeDragTransaction,
    commitNodeDragTransaction: flowTransactions.commitNodeDragTransaction,
    duplicateModifierPressed,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
  })

const {
  onPaneContextMenu,
  handleNodeContextMenuCreate,
  setActiveTool,
  onEscapeShortcut,
  onCanvasDragOver,
} = useFlowCanvasShellInteractions({
  canvasWrapRef,
  nodeContextMenu,
  activeTool,
  screenToFlowCoordinate,
  openNodeContextMenu,
  createNodeAtPosition,
  dismissTransientCanvasUi,
  clearFlowSelection,
  onCanvasDropDragOver: onFlowCanvasDropDragOver,
  isFlowExpandActive: () => !!flowExpandNodeId.value,
  isBoardHeaderEditingActive: () => isBoardMenuOpen.value || isRenamingBoard.value,
})

const {
  onFlowImageContextMenu,
  onFlowFusionContextMenu,
  onFlowImageDirectAction,
  onFlowImagePromptGenerate,
  onFlowFusionPromptGenerate,
} = useFlowImageActionOrchestrator({
  flowImageEditorNodeId,
  suppressClickAction,
  openActiveNodePanel,
  clearActiveNodePanel,
  hideStrengthControl,
  selectFlowNodeForContextMenu,
  cancelFlowExpandMode,
  cancelFlowMaskEditorMode,
  enterFlowExpandMode,
  enterFlowImageEditorMode,
  focusImageEditorAndShowChrome,
  runImageDirectAction,
  runImagePromptAction: runFlowImageToolPromptAction,
  runFusionPromptGeneration,
  onFlowNodeParamUpdate,
  onWarning: canvasFeedback.imageTools.actionUnavailable,
  getActionStrength,
})

watch(
  () => route.query.boardId,
  async () => {
    await loadCanvasBoardAndProbe()
    void reattachPendingImageResults()
    refreshBoardMetas()
  },
)

useFlowKeyboardShortcuts({
  onSave: () => {
    void flushAutosave({ silent: false })
  },
  onUndo: flowHistory.undo,
  onRedo: flowHistory.redo,
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
  refreshBoardMetas()
})
</script>

<style scoped>
@import '../components/styles/canvas-controls.css';

:global(html.canvas-overscroll-guard),
:global(body.canvas-overscroll-guard) {
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-layout {
  --canvas-control-border-width: 1.2px;
  --canvas-tool-button-gap: 7px;
  --canvas-page-chrome-fade-duration: 0.18s;
  --canvas-page-chrome-slide-duration: 0.28s;
  --canvas-page-chrome-slide-ease: cubic-bezier(0.16, 1, 0.3, 1);
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
  --canvas-motion-fast: 160ms;
  --canvas-motion-standard: 200ms;
  --canvas-motion-panel: 220ms;
  --canvas-motion-dock: 240ms;
  --canvas-motion-ease: cubic-bezier(0.2, 0.8, 0.2, 1);
  --flow-edge-color: rgb(177 177 183 / 50%);
  --flow-edge-active-color: rgb(146 120 230 / 50%);
  --flow-edge-selected-color: rgb(123 135 152 / 50%);
  --flow-edge-width: 2;
  --flow-edge-dasharray: 5;
  --flow-edge-running-duration: 0.5s;
  width: 100%;
  height: 100vh;
  background: #ffffff;
  position: relative;
  overflow: hidden;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-layout.interaction-lite :deep(.vue-flow__node),
.canvas-layout.interaction-lite :deep(.workflow-node),
.canvas-layout.interaction-lite :deep(.vue-flow__edge-path),
.canvas-layout.interaction-lite :deep(.fusion-node-connection-layer) {
  transition: none !important;
}

.canvas-layout.interaction-lite :deep(.workflow-node *) {
  transition-duration: 0s !important;
}

.canvas-layout.interaction-lite :deep(.vue-flow__edge.selected .vue-flow__edge-path) {
  filter: none !important;
}

.canvas-layout.viewport-pan-lite :deep(.vue-flow__transformationpane) {
  will-change: transform;
}

.canvas-layout.viewport-pan-lite :deep(.vue-flow__nodes),
.canvas-layout.viewport-pan-lite :deep(.vue-flow__edges),
.canvas-layout.viewport-pan-lite :deep(.vue-flow__edge-labels) {
  pointer-events: none;
}

.canvas-flow-wrap :deep(.vue-flow__viewport) {
  background-image: radial-gradient(circle, #e2e4e8 1px, transparent 1px);
  background-size: 12px 12px;
  background-position: 0 0;
}

.canvas-flow-wrap {
  position: absolute;
  inset: 0;
  z-index: 2;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-flow-wrap :deep(.vue-flow) {
  background: transparent;
}

.canvas-flow-wrap :deep(.vue-flow__edge-path),
.canvas-flow-wrap :deep(.vue-flow__connection-path) {
  stroke: var(--flow-edge-color);
  stroke-width: var(--flow-edge-width);
}

.canvas-flow-wrap :deep(.vue-flow__edge.animated path) {
  stroke-dasharray: var(--flow-edge-dasharray);
  animation: flow-edge-dashdraw var(--flow-edge-running-duration) linear infinite;
}

.canvas-flow-wrap :deep(.vue-flow__edge.animated path.vue-flow__edge-interaction) {
  stroke-dasharray: none;
  animation: none;
}

.canvas-flow-wrap :deep(.vue-flow__edge.selected .vue-flow__edge-path),
.canvas-flow-wrap :deep(.vue-flow__edge:focus-visible .vue-flow__edge-path) {
  stroke: var(--flow-edge-selected-color);
}

@media (prefers-reduced-motion: reduce) {
  .canvas-flow-wrap :deep(.vue-flow__edge.animated path),
  .canvas-flow-wrap :deep(.fusion-node-connection-layer path) {
    animation: none;
  }
}

.canvas-flow-wrap :deep(.vue-flow__node) {
  background: transparent;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  padding: 0;
  cursor: default;
  transition: none;
}

.canvas-flow-wrap :deep(.vue-flow__node:hover),
.canvas-flow-wrap :deep(.vue-flow__node.selected),
.canvas-flow-wrap :deep(.vue-flow__node:focus),
.canvas-flow-wrap :deep(.vue-flow__node:focus-visible) {
  box-shadow: none !important;
  border: 0 !important;
  outline: none !important;
}

.canvas-flow-wrap :deep(.vue-flow__selectionpane) {
  cursor: default;
}

.canvas-flow-wrap :deep(.vue-flow__pane) {
  cursor: default;
}

.canvas-layout.tool-hand .canvas-flow-wrap :deep(.vue-flow__pane) {
  cursor: grab;
}

.canvas-layout.tool-hand .canvas-flow-wrap :deep(.vue-flow__pane.dragging) {
  cursor: grabbing;
}
</style>

<style>
@keyframes flow-edge-dashdraw {
  from {
    stroke-dashoffset: 10;
  }
}
</style>
