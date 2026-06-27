<template>
  <div
    class="canvas-layout"
    :class="[
      `tool-${activeTool}`,
      { 'interaction-lite': isFlowInteractionActive },
    ]"
    @mousedown.self="closeBoardHeaderEditing"
  >
    <WorkspacePageChrome
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
      :zoom-level="zoomLevel"
      @update:board-name-draft="boardNameDraft = $event"
      @toggle-board-menu="toggleBoardMenu"
      @start-board-rename="startBoardRename"
      @cancel-board-rename="cancelBoardRename"
      @confirm-board-rename="onConfirmBoardRename"
      @select-board="onSelectBoard"
      @export="onExportSelectedImages"
      @open-center="onNavigateRequest('/user/center')"
      @logout="onLogoutRequest"
      @navigate-back="onNavigateRequest('/user/boards')"
      @select-tool="setActiveTool"
      @zoom-out="onZoomOut"
      @zoom-in="onZoomIn"
    />

    <div
      ref="canvasWrapRef"
      class="workspace-flow-canvas-wrap"
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
        :delete-key-code="null"
        :default-edge-options="{ animated: false, updatable: true }"
        :min-zoom="0.1"
        :zoom-on-scroll="false"
        :pan-on-scroll="!isFlowCanvasLocked"
        :zoom-on-pinch="!isFlowCanvasLocked"
        :pan-on-scroll-mode="PanOnScrollMode.Free"
        :only-render-visible-elements="FLOW_ENABLE_VISIBLE_ELEMENTS_CULLING"
        :is-valid-connection="isValidConnection"
        @connect="onConnect"
        @connect-end="onFlowConnectEnd"
        @edge-update="onEdgeUpdate"
        @move-start="closeBoardHeaderEditing"
        @move="onFlowMove"
        @move-end="onFlowMoveEnd"
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
            :image-action-visible="
              isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeProps.id)
            "
            :image-prompt-composer-visible="
              isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeProps.id)
            "
            :image-prompt-placeholder="
              getFlowMaskEditorCommittedData(nodeProps.id)
                ? '描述如何重绘选区'
                : '输入提示词改造这张图'
            "
            :fusion-prompt-composer-visible="
              isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.fusionPrompt, nodeProps.id)
            "
            :image-direct-generating="isNodeDirectActionRunning(nodeProps.id)"
            :show-strength-control="
              !!activeStrengthAction &&
              isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeProps.id)
            "
            :strength-pct="activeStrengthValue"
            :set-strength-track-ref="setStrengthTrackRef"
            :set-image-stage-ref="setImageStageRef"
            :clear-image-stage-ref="clearImageStageRef"
            :expand-mode-active="flowExpandNodeId === nodeProps.id"
            :expand-overlay-style="
              flowExpandNodeId === nodeProps.id ? flowExpandOverlayStyle : null
            "
            :expand-handle-radius="flowExpandHandleRadius"
            :image-editor-mode="
              flowImageEditorNodeId === nodeProps.id ? imageEditorMode : null
            "
            :image-editor-filter-previews="
              flowImageEditorNodeId === nodeProps.id && imageEditorMode === 'adjust'
                ? imageAdjustmentFilterPreviews
                : []
            "
            :mask-editor-overlay-style="
              flowMaskEditorNodeId === nodeProps.id ? flowMaskEditorOverlayStyle : null
            "
            :mask-editor-canvas-width="
              Math.max(
                1,
                Math.round(
                  flowMaskEditorOverlayStyle?.width
                    ? parseFloat(flowMaskEditorOverlayStyle.width)
                    : 0,
                ),
              )
            "
            :mask-editor-canvas-height="
              Math.max(
                1,
                Math.round(
                  flowMaskEditorOverlayStyle?.height
                    ? parseFloat(flowMaskEditorOverlayStyle.height)
                    : 0,
                ),
              )
            "
            :mask-layer-opacity="maskEditorLayerOpacity"
            :image-layer-visible="true"
            :mask-editor-brush-size="flowMaskEditorBrushSize"
            :mask-editor-brush-shape="flowMaskEditorBrushShape"
            :handle-visibility-refresh-token="flowHandleVisibilityRefreshToken"
            :set-mask-canvas-ref="setFlowMaskEditorCanvasRef"
            @update-prompt-text="(value) => onFlowPromptTextUpdate(nodeProps.id, value)"
            @update-node-param="(payload) => onFlowNodeParamUpdate(nodeProps.id, payload)"
            @delete-node="() => onFlowDeleteNode(nodeProps.id)"
            @image-stage-drop="(event) => onFlowImageStageDrop(nodeProps.id, event)"
            @media-input-image-drop="onFlowMediaInputImageDrop"
            @media-input-image-open="onFlowMediaInputImageOpen"
            @media-input-image-preview="onFlowMediaInputImagePreview"
            @media-input-image-remove="onFlowMediaInputImageRemove"
            @image-open-file="() => onFlowImageOpenFile(nodeProps.id)"
            @image-open-preview="() => onFlowImageOpenPreview(nodeProps.id)"
            @image-append-to-canvas="() => onFlowImageAppendToCanvas(nodeProps.id)"
            @image-reset="() => onFlowImageReset(nodeProps.id)"
            @node-mousedown="(event) => onFlowNodeMouseDown(nodeProps.id, event)"
            @request-internals-update="() => scheduleFlowNodeInternalsRefresh([nodeProps.id])"
            @image-contextmenu="(event) => onFlowImageContextMenu(nodeProps.id, event)"
            @image-prompt-generate="
              (prompt, outputCount) => onFlowImagePromptGenerate(nodeProps.id, prompt, outputCount)
            "
            @image-expand-submit="
              (prompt, outputCount) => onFlowImageExpandSubmit(nodeProps.id, prompt, outputCount)
            "
            @fusion-contextmenu="(event) => onFlowFusionContextMenu(nodeProps.id, event)"
            @fusion-prompt-generate="
              (prompt, outputCount, aspectRatio) =>
                onFlowFusionPromptGenerate(nodeProps.id, prompt, outputCount, aspectRatio)
            "
            @prompt-generate="
              (prompt, outputCount, aspectRatio) =>
                onFlowPromptGenerate(nodeProps.id, prompt, outputCount, aspectRatio)
            "
            @image-action="(action) => onFlowImageDirectAction(nodeProps.id, action)"
            @strength-action-mousedown="onFlowImageStrengthActionMouseDown"
            @strength-track-mousedown="onFlowImageStrengthTrackMouseDown"
            @expand-handle-pointerdown="
              (payload) =>
                onFlowExpandHandlePointerDown(nodeProps.id, payload.anchor, payload.event)
            "
            @expand-handle-pointermove="onFlowExpandHandlePointerMove"
            @expand-handle-pointerup="onFlowExpandHandlePointerUp"
            @expand-frame-pointerdown="(event) => onFlowExpandFramePointerDown(nodeProps.id, event)"
            @mask-canvas-pointerdown="onFlowMaskEditorCanvasPointerDown"
          />
        </template>
      </VueFlow>
      <WorkspaceCanvasOverlays
        :image-editor-node-id="flowImageEditorNodeId"
        :image-editor-mode="imageEditorMode"
        :image-editor-chrome-anchor-rect="imageEditorChromeAnchorRect"
        :is-image-editor-chrome-ready="isImageEditorChromeReady"
        :flow-mask-editor-draw-mode="flowMaskEditorDrawMode"
        :flow-mask-editor-brush-shape="flowMaskEditorBrushShape"
        :flow-mask-editor-brush-size="flowMaskEditorBrushSize"
        :flow-mask-editor-brush-hardness="flowMaskEditorBrushHardness"
        :mask-editor-layer-opacity="maskEditorLayerOpacity"
        :active-mask-editor-output-count="activeMaskEditorOutputCount"
        :mask-editor-reference-images="maskEditorReferenceImages"
        :is-node-direct-action-running="isNodeDirectActionRunning"
        :can-undo-flow-mask-editor="canUndoFlowMaskEditor"
        :can-redo-flow-mask-editor="canRedoFlowMaskEditor"
        :active-image-editor-tool="activeImageEditorTool"
        :image-editor-layers="imageEditorLayers"
        :selected-image-editor-layer-id="selectedImageEditorLayerId"
        :selected-image-editor-adjustment-layer="selectedImageEditorAdjustmentLayer"
        :applying-image-editor-adjustments="isApplyingImageEditorAdjustments"
        :node-context-menu="nodeContextMenu"
        :canvas-node-menu-items="canvasNodeMenuItems"
        @set-draw-mode="setFlowMaskEditorDrawMode"
        @set-brush-shape="setFlowMaskEditorBrushShape"
        @set-brush-size="setFlowMaskEditorBrushSize"
        @set-brush-hardness="setFlowMaskEditorBrushHardness"
        @set-mask-layer-opacity="setMaskEditorLayerOpacity"
        @clear-mask="clearFlowMaskEditorMask"
        @invert-mask="invertFlowMaskEditorMask"
        @feather-mask="featherFlowMaskEditorMask"
        @undo-mask="undoFlowMaskEditorMask"
        @redo-mask="redoFlowMaskEditorMask"
        @update:output-count="onMaskEditorChromeOutputCountUpdate"
        @image-input-open="onMaskEditorReferenceImageOpen"
        @reference-image-remove="onMaskEditorReferenceImageRemove"
        @reference-image-preview="onMaskEditorReferenceImagePreview"
        @select-image-editor-tool="selectImageEditorTool"
        @select-image-adjustment-layer="selectImageAdjustmentLayer"
        @select-image-editor-layer="selectImageEditorLayer"
        @set-image-editor-layer-visible="setImageEditorLayerVisible"
        @add-image-editor-layer="addImageEditorLayer"
        @copy-image-editor-layer="copyImageEditorLayer"
        @merge-image-editor-layer="mergeImageEditorLayer"
        @remove-image-editor-layer="removeImageEditorLayer"
        @update-image-editor-param="updateSelectedImageEditorLayerParam"
        @apply-image-editor-adjustments="onApplyImageEditorAdjustments"
        @submit="onMaskEditorChromeSubmit"
        @create-node="handleNodeContextMenuCreate"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Ref } from 'vue'
import { useUserStore } from '../../../app/stores/user'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { uploadApi } from '../../../shared/api/upload'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useDismissableLayer } from '../../../shared/composables/useDismissableLayer'
import type { Edge, Node } from '@vue-flow/core'
import { PanOnScrollMode, useVueFlow, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowKeyboardShortcuts } from '../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../composables/useFlowLeaveGuard'
import { useFlowSelectionExport } from '../composables/useFlowSelectionExport'
import { useFlowNodeMutations } from '../composables/useFlowNodeMutations'
import { useFlowImageDirectActions } from '../execution'
import { useFlowImageActionStrengthControl } from '../composables/useFlowImageActionStrengthControl'
import { useFlowImageNodeInteractions } from '../composables/useFlowImageNodeInteractions'
import { useFlowCanvasInteractionState } from '../composables/useFlowCanvasInteractionState'
import { useFlowNodeUiState } from '../composables/useFlowNodeUiState'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import { useFlowNodeContextMenu } from '../composables/useFlowNodeContextMenu'
import { useFlowCanvasDrop } from '../composables/useFlowCanvasDrop'
import { useFlowSelectionController } from '../composables/useFlowSelectionController'
import { useFlowNodeMediaInputs } from '../composables/useFlowNodeMediaInputs'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
import { useFlowMediaInputImages } from '../composables/useFlowMediaInputImages'
import { useFlowNodeInternalsRefreshQueue } from '../composables/useFlowNodeInternalsRefreshQueue'
import { useFlowViewportControls } from '../composables/useFlowViewportControls'
import {
  ACTIVE_NODE_PANEL_KIND,
  useFlowActiveNodePanels,
} from '../composables/useFlowActiveNodePanels'
import { useFlowCanvasShellInteractions } from '../composables/useFlowCanvasShellInteractions'
import { useFlowImageActionOrchestrator } from '../composables/useFlowImageActionOrchestrator'
import { useFlowPromptGenerationOrchestrator } from '../composables/useFlowPromptGenerationOrchestrator'
import { useFlowExpandInteractionHandlers } from '../composables/useFlowExpandInteractionHandlers'
import { useMaskEditorChromeActions, useMaskEditorChromeState } from '../mask-editor'
import { useFlowWorkspaceNodeHandlers } from '../composables/useFlowWorkspaceNodeHandlers'
import { useFlowTransientUiDismissal } from '../composables/useFlowTransientUiDismissal'
import { useFlowSnapshotNodeUpdater } from '../composables/useFlowSnapshotNodeUpdater'
import { useFlowImageNodeSnapshots } from '../composables/useFlowImageNodeSnapshots'
import { useFlowImageUploadActions } from '../composables/useFlowImageUploadActions'
import { useImageCanvasExtendOverlay } from '../composables/useImageCanvasExtendOverlay'
import { useImageEditorViewportController } from '../image-editor'
import { useImageStageRegistry } from '../composables/useImageStageRegistry'
import { useWorkspaceBoardRuntime } from '../composables/useWorkspaceBoardRuntime'
import { useWorkspaceImageToolRuntime } from '../composables/useWorkspaceImageToolRuntime'
import { useAppToast } from '../../../shared/composables/useAppToast'
import WorkspaceCanvasOverlays from '../components/WorkspaceCanvasOverlays.vue'
import WorkspacePageChrome from '../components/WorkspacePageChrome.vue'

const appToast = useAppToast()
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect()
const {
  screenToFlowCoordinate,
  updateNodeInternals,
  zoomIn,
  zoomOut,
  getViewport,
  setViewport,
  setCenter,
  setNodes,
  setEdges,
  getSelectedElements,
  removeSelectedElements,
} = useVueFlow()
const { zoomLevel, onZoomIn, onZoomOut, applyFlowTransform, onPanViewport } =
  useFlowViewportControls({
    zoomIn,
    zoomOut,
    getViewport,
    setViewport: (viewport) => setViewport(viewport),
  })

function clearSelectedFlowElementsSync() {
  const selectedElements = [...getSelectedElements.value]
  if (selectedElements.length === 0) return
  removeSelectedElements(selectedElements)
}

const canvasWrapRef = ref<HTMLElement | null>(null)
const flowHandleVisibilityRefreshToken = ref(0)

const activeTool = ref<'select' | 'hand' | 'text'>('select')

const createNodeKind = ref<CanvasCreatableNodeKind>('prompt')

const FLOW_ENABLE_VISIBLE_ELEMENTS_CULLING =
  import.meta.env.VITE_WORKSPACE_FLOW_ENABLE_CULLING === 'true'

const {
  activeNodePanel,
  openActiveNodePanel,
  clearActiveNodePanel,
  isActiveNodePanel,
  getActiveNodePanelNodeId,
} = useFlowActiveNodePanels()
const duplicateModifierPressed = ref(false)
const { setImageStageRef, clearImageStageRef, getImageStageElement } = useImageStageRegistry()
const { scheduleFlowNodeInternalsRefresh } = useFlowNodeInternalsRefreshQueue({
  updateNodeInternals,
})

const {
  nodes,
  edges,
  currentBoardId,
  loadInitialOrRouteBoard,
  flushAutosave,
  flowHistory,
  flowTransactions,
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
} = useWorkspaceBoardRuntime({
  route,
  router,
  getUserIdentity: () => ({
    userId: Number(userStore.user?.id),
    userUid: `${userStore.user?.uid ?? ''}`.trim(),
  }),
  getViewport,
  setViewport: (viewport) => setViewport(viewport),
  setNodes,
  setEdges,
  logoutAndRedirect,
  message: {
    warning: (msg: string) => appToast.warning(msg),
    success: (msg: string) => appToast.success(msg),
    error: (msg: string) => appToast.error(msg),
  },
})

function onFlowMove(payload: { flowTransform?: { x?: number; y?: number; zoom?: number } }) {
  closeBoardHeaderEditing()
  const transform = payload.flowTransform
  if (!transform) return
  if (applyFlowTransform(transform)) markBoardViewportChanged()
  if (!isImageEditorChromeReady.value) return
  scheduleImageEditorChromeAnchorUpdate()
}

function onFlowMoveEnd(payload: { flowTransform?: { x?: number; y?: number; zoom?: number } }) {
  const transform = payload.flowTransform
  if (transform && applyFlowTransform(transform)) markBoardViewportChanged()
  commitPendingBoardViewportChange()
}

const flowNodeMutations = useFlowNodeMutations({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  createNodeKind,
  commitGraphMutation: flowTransactions.commitGraphMutation,
  commitNodeMutation: flowTransactions.commitNodeMutation,
  commitEdgeMutation: flowTransactions.commitEdgeMutation,
  onWarning: (text) => {
    appToast.warning(text)
  },
})

const isValidConnection = flowNodeMutations.isValidConnection
const onConnect = flowNodeMutations.onConnect
const onEdgeUpdate = flowNodeMutations.onEdgeUpdate
const createNodeAtPosition = flowNodeMutations.createNodeAtPosition
const _onFlowDeleteNode = flowNodeMutations.onFlowDeleteNode
const updateSnapshotNodeBase = flowNodeMutations.updateSnapshotNode
const _onFlowPromptTextUpdate = flowNodeMutations.onFlowPromptTextUpdate
const _onFlowNodeParamUpdate = flowNodeMutations.onFlowNodeParamUpdate
const markFlowExecutionNodeDirty = flowNodeMutations.markFlowExecutionNodeDirty
const { updateSnapshotNode } = useFlowSnapshotNodeUpdater({
  updateSnapshotNodeBase,
  scheduleFlowNodeInternalsRefresh,
})

const selectionExport = useFlowSelectionExport({ nodes, message: appToast })
const hasSelectedFlowNodes = computed(() => selectionExport.hasSelectedNodes())

function onExportSelectedImages() {
  selectionExport.exportSelectedImages()
}

function onFlowConnectEnd() {
  refreshFlowHandleVisibility()
}

function refreshFlowHandleVisibility() {
  flowHandleVisibilityRefreshToken.value += 1
}

const {
  isCanvasInteractionActive: isFlowInteractionActive,
  startCanvasInteraction: startFlowInteraction,
  stopCanvasInteraction: stopFlowInteraction,
} = useFlowCanvasInteractionState()

const { nodeContextMenu, canvasNodeMenuItems, closeNodeContextMenu, openNodeContextMenu } =
  useFlowNodeContextMenu()
const hasWorkspaceDismissableLayer = computed(
  () => nodeContextMenu.value.visible || activeNodePanel.value !== null,
)

const { getImageNodeSnapshot, getImageNodeDisplayPayload } = useFlowImageNodeSnapshots({
  nodes: nodes as Ref<Node[]>,
})
const { uploadImageToBoard, applyImageFileToImageNode, createImageNodeFromFile } =
  useFlowImageUploadActions({
    currentBoardId,
    createNodeAtPosition,
    updateSnapshotNode,
    getImageNodeSnapshot,
    uploadApi,
    sanitizeApiErrorMessage,
    onWarning: (msg) => appToast.warning(msg),
    onSuccess: (msg) => appToast.success(msg),
    onNodeMutated: (nodeId) => markFlowExecutionNodeDirty(nodeId),
  })
const { openNodeImagePreview, openImageUrlPreview } = useFlowImagePreviewLightbox()
const {
  onFlowMediaInputImageDrop,
  onFlowMediaInputImageOpen,
  onFlowMediaInputImagePreview,
  onFlowMediaInputImageRemove,
  getMediaInputImages,
  resolvePromptInputImageNodeIds,
} = useFlowMediaInputImages({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  uploadImageToBoard,
  updateSnapshotNodeBase,
  commitGraphMutation: flowTransactions.commitGraphMutation,
  markFlowExecutionNodeDirty,
  getImageNodeDisplayPayload,
  openImageUrlPreview,
  onWarning: (message) => {
    appToast.warning(message)
  },
})

async function onCanvasDrop(event: DragEvent) {
  await canvasDrop.onCanvasDrop(event)
}

function onFlowImageOpenPreview(nodeId: string) {
  const snapshot = getImageNodeSnapshot(nodeId)
  if (!snapshot) return
  void openNodeImagePreview(snapshot, 'latest-output')
}

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
} = useFlowImageDirectActions({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  commitGraphMutation: flowTransactions.commitGraphMutation,
  commitNodeMutation: flowTransactions.commitNodeMutation,
  commitEdgeMutation: flowTransactions.commitEdgeMutation,
  commitDirty: flowTransactions.commitDirty,
  flushAutosave,
  refreshNodeInternals: scheduleFlowNodeInternalsRefresh,
  getCurrentBoardId: () => currentBoardId.value ?? '',
  onSuccess: (text) => {
    appToast.success(text)
  },
  onWarning: (text) => {
    appToast.warning(text)
  },
})

const {
  activeStrengthAction,
  activeStrengthValue,
  suppressClickAction,
  setStrengthTrackRef,
  hideStrengthControl,
  getActionStrength,
  onActionMouseDown: onFlowImageStrengthActionMouseDown,
  onStrengthTrackMouseDown: onFlowImageStrengthTrackMouseDown,
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

const { dismissTransientWorkspaceUi } = useFlowTransientUiDismissal({
  cancelFlowExpandMode,
  closeBoardHeaderEditing,
  closeNodeContextMenu,
  clearActiveNodePanel,
  hideStrengthControl,
})

useDismissableLayer({
  enabled: hasWorkspaceDismissableLayer,
  ignore: [
    '.node-tool-surface',
    '.workspace-node-context-menu',
    '[data-image-mask-editor]',
    '[data-image-editor]',
  ],
  ownerDocument: () =>
    canvasWrapRef.value?.ownerDocument ?? (typeof document === 'undefined' ? null : document),
  onDismiss: () => {
    dismissTransientWorkspaceUi({ closeContextMenu: true })
  },
})

const {
  imageEditorMode,
  flowImageEditorNodeId,
  flowMaskEditorNodeId,
  flowMaskEditorOverlayStyle,
  flowMaskEditorDrawMode,
  flowMaskEditorBrushSize,
  flowMaskEditorBrushHardness,
  flowMaskEditorBrushShape,
  canUndoFlowMaskEditor,
  canRedoFlowMaskEditor,
  clearFlowMaskEditorMask,
  invertFlowMaskEditorMask,
  featherFlowMaskEditorMask,
  undoFlowMaskEditorMask,
  redoFlowMaskEditorMask,
  clearFlowMaskEditorCommittedData,
  setFlowMaskEditorCanvasRef,
  onFlowMaskEditorCanvasPointerDown,
  setFlowMaskEditorDrawMode,
  setFlowMaskEditorBrushSize,
  setFlowMaskEditorBrushHardness,
  setFlowMaskEditorBrushShape,
  getFlowMaskEditorCommittedData,
  activeImageEditorTool,
  imageEditorLayers,
  selectedImageEditorLayerId,
  selectedImageEditorAdjustmentLayer,
  selectImageEditorTool,
  selectImageAdjustmentLayer,
  selectImageEditorLayer,
  setImageEditorLayerVisible,
  addImageEditorLayer,
  copyImageEditorLayer,
  mergeImageEditorLayer,
  removeImageEditorLayer,
  updateSelectedImageEditorLayerParam,
  isApplyingImageEditorAdjustments,
  imageAdjustmentFilterPreviews,
  onApplyImageEditorAdjustments,
  cancelFlowMaskEditorMode,
  enterFlowImageEditorMode,
  commitFlowMaskEditorMode,
} = useWorkspaceImageToolRuntime({
  nodes: nodes as Ref<Node[]>,
  activeTool: activeTool as Ref<string>,
  currentBoardId,
  getImageNodeDisplayPayload,
  getImageNodeSnapshot,
  getImageStageElement,
  createImageNodeFromFile,
  connectImageNodes: onConnect,
  clearSelectedFlowElements: clearSelectedFlowElementsSync,
  startCanvasInteraction: startFlowInteraction,
  stopCanvasInteraction: stopFlowInteraction,
  message: {
    warning: (msg) => appToast.warning(msg),
    success: (msg) => appToast.success(msg),
  },
})

const {
  onFlowImageOpenFile,
  onFlowImageReset: onFlowImageResetBase,
  onFlowImageStageDrop,
} = useFlowNodeMediaInputs({
  applyImageFileToImageNode: async (nodeId, file): Promise<boolean> => {
    const updated = await applyImageFileToImageNode(nodeId, file)
    if (updated) clearFlowMaskEditorCommittedData(nodeId)
    return updated
  },
  updateSnapshotNode,
  markFlowExecutionNodeDirty,
})

const { onFlowDeleteNode, onFlowPromptTextUpdate, onFlowNodeParamUpdate, onFlowImageReset } =
  useFlowWorkspaceNodeHandlers({
    dismissTransientWorkspaceUi,
    clearFlowMaskEditorCommittedData,
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

const {
  imageEditorChromeAnchorRect,
  isImageEditorChromeReady,
  onCanvasWheel,
  focusImageEditorAndShowChrome,
  scheduleImageEditorChromeAnchorUpdate,
} = useImageEditorViewportController({
  nodes: nodes as Ref<Node[]>,
  canvasWrapRef,
  imageEditorNodeId: flowImageEditorNodeId,
  getImageStageElement,
  screenToFlowCoordinate,
  getViewport,
  setViewport: (viewport, options) => setViewport(viewport, options),
  setCenter: (x, y, options) => setCenter(x, y, options),
  onViewportTransform: (viewport) => {
    applyFlowTransform(viewport)
  },
})

const {
  maskEditorLayerOpacity,
  maskEditorReferenceImages,
  activeMaskEditorOutputCount,
  setMaskEditorLayerOpacity,
  onMaskEditorReferenceImageOpen,
  onMaskEditorReferenceImageRemove,
  onMaskEditorReferenceImagePreview,
} = useMaskEditorChromeState({
  imageEditorNodeId: flowImageEditorNodeId,
  getImageNodeSnapshot,
  uploadImageToBoard,
  onWarning: (msg) => appToast.warning(msg),
  onReferenceImagePreview: (image) => {
    void openImageUrlPreview(image.url, image.ratio)
  },
})
const isImageEditorChromeHidden = computed(() => !!flowImageEditorNodeId.value)
const isFlowCanvasLocked = computed(
  () => isFlowInteractionActive.value || !!flowImageEditorNodeId.value,
)

const { onMaskEditorChromeSubmit, onMaskEditorChromeOutputCountUpdate } =
  useMaskEditorChromeActions({
    maskEditorNodeId: flowMaskEditorNodeId,
    referenceImages: maskEditorReferenceImages,
    commitMaskEditorMode: commitFlowMaskEditorMode,
    getCommittedMaskData: getFlowMaskEditorCommittedData,
    clearActiveNodePanel,
    hideStrengthControl,
    runInpaintAction: runImageInpaintAction,
    updateOutputCount: (nodeId, outputCount) => {
      onFlowNodeParamUpdate(nodeId, {
        key: 'outputCount',
        value: outputCount,
      })
    },
    onWarning: (message) => appToast.warning(message),
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
  deleteSelectedFlowElements,
} = useFlowSelectionController({
  activeTool: activeTool as Ref<string>,
  dismissTransientWorkspaceUi,
  _deleteSelectedFlowElements: () => {
    flowNodeMutations.deleteSelectedFlowElements()
  },
})

const {
  onFlowImageAppendToCanvas,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
} = useFlowImageNodeInteractions({
  nodes: nodes as Ref<Node[]>,
  getImageNodeDisplayPayload,
  getImageNodeSnapshot: (nodeId) => {
    const snapshot = getImageNodeSnapshot(nodeId)
    if (!snapshot) return null
    return {
      x: Number(snapshot.x ?? 0),
      y: Number(snapshot.y ?? 0),
      width: Number(snapshot.width ?? 300),
      outputCount: snapshot.outputCount,
    }
  },
  commitNodeMutation: flowTransactions.commitNodeMutation,
})

const { onNodeDragStart, onNodeDragStop: stopNodeDragInteraction } = useFlowNodeDragInteractions({
  beginNodeDragTransaction: flowTransactions.beginNodeDragTransaction,
  commitNodeDragTransaction: flowTransactions.commitNodeDragTransaction,
  duplicateModifierPressed,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
  dismissTransientWorkspaceUi,
})

function onNodeDragStop() {
  try {
    stopNodeDragInteraction()
  } finally {
    refreshFlowHandleVisibility()
  }
}

const canvasDrop = useFlowCanvasDrop({
  screenToFlowCoordinate,
  onWarning: (msg) => appToast.warning(msg),
  createImageNodeFromFile,
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
  dismissTransientWorkspaceUi,
  clearFlowSelection,
  toastInfo: (message) => appToast.info(message),
  onCanvasDropDragOver: (event) => canvasDrop.onCanvasDragOver(event),
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
  runImagePromptGeneration,
  runImageInpaintAction,
  runFusionPromptGeneration,
  getFlowMaskEditorCommittedData,
  onFlowNodeParamUpdate,
  onWarning: (message) => appToast.warning(message),
  getActionStrength,
})

const { getFlowNodeUiStateCached } = useFlowNodeUiState({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
})

watch(
  () => route.query.boardId,
  async () => {
    await loadInitialOrRouteBoard()
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
  onDeleteSelected: deleteSelectedFlowElements,
  onEscape: onEscapeShortcut,
  onPanViewport,
  onToolSelect: () => {
    setActiveTool('select')
  },
  onToolHand: () => {
    setActiveTool('hand')
  },
  onToolText: () => {
    setActiveTool('text')
  },
  onDuplicateModifierChange: (pressed) => {
    duplicateModifierPressed.value = pressed
  },
})

useFlowLeaveGuard({
  onLeave: () => flushAutosave(),
})

onMounted(async () => {
  await loadInitialOrRouteBoard()
  void reattachPendingImageResults()
  refreshBoardMetas()
})
</script>

<style scoped>
@import '../components/styles/canvas-controls.css';

:global(html.workspace-overscroll-guard),
:global(body.workspace-overscroll-guard) {
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-layout {
  --workspace-control-border-width: 1.2px;
  --workspace-tool-button-gap: 7px;
  --workspace-page-chrome-fade-duration: 0.18s;
  --workspace-page-chrome-slide-duration: 0.28s;
  --workspace-page-chrome-slide-ease: cubic-bezier(0.16, 1, 0.3, 1);
  --workspace-surface-bg: rgba(255, 255, 255, 0.98);
  --workspace-surface-bg-soft: rgba(255, 255, 255, 0.94);
  --workspace-surface-muted: #f4f7fb;
  --workspace-surface-border: #d8ddea;
  --workspace-surface-border-soft: rgba(34, 42, 54, 0.08);
  --workspace-surface-shadow: 0 14px 32px rgba(15, 23, 42, 0.13);
  --workspace-surface-blur: 10px;
  --workspace-surface-radius: 10px;
  --workspace-floating-radius: 13px;
  --workspace-panel-radius: 8px;
  --workspace-panel-border-width: 1px;
  --workspace-panel-content-gap: 10px;
  --workspace-panel-border: rgba(34, 42, 54, 0.14);
  --workspace-panel-border-soft: rgba(34, 42, 54, 0.1);
  --workspace-panel-header-from: #f8fafc;
  --workspace-panel-header-to: #eef2f7;
  --workspace-text-strong: #111827;
  --workspace-text-body: #334155;
  --workspace-text-muted: #64748b;
  --workspace-text-subtle: #94a3b8;
  --workspace-accent: #9278e6;
  --workspace-accent-hover: #7a6bd5;
  --workspace-accent-strong: #6757c8;
  --workspace-accent-soft: rgba(146, 120, 230, 0.12);
  --workspace-accent-soft-strong: rgba(146, 120, 230, 0.16);
  --workspace-accent-wash: #f8f7ff;
  --workspace-accent-outline: rgba(122, 107, 213, 0.22);
  --workspace-motion-fast: 160ms;
  --workspace-motion-standard: 200ms;
  --workspace-motion-panel: 220ms;
  --workspace-motion-dock: 240ms;
  --workspace-motion-ease: cubic-bezier(0.2, 0.8, 0.2, 1);
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
.workspace-flow-canvas-wrap :deep(.vue-flow__viewport) {
  background-image: radial-gradient(circle, #e2e4e8 1px, transparent 1px);
  background-size: 12px 12px;
  background-position: 0 0;
}

.workspace-flow-canvas-wrap {
  position: absolute;
  inset: 0;
  z-index: 2;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.workspace-flow-canvas-wrap :deep(.vue-flow) {
  background: transparent;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge-path),
.workspace-flow-canvas-wrap :deep(.vue-flow__connection-path) {
  stroke: var(--flow-edge-color);
  stroke-width: var(--flow-edge-width);
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge.animated path) {
  stroke-dasharray: var(--flow-edge-dasharray);
  animation: flow-edge-dashdraw var(--flow-edge-running-duration) linear infinite;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge.animated path.vue-flow__edge-interaction) {
  stroke-dasharray: none;
  animation: none;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge.selected .vue-flow__edge-path),
.workspace-flow-canvas-wrap :deep(.vue-flow__edge:focus .vue-flow__edge-path),
.workspace-flow-canvas-wrap :deep(.vue-flow__edge:focus-visible .vue-flow__edge-path) {
  stroke: var(--flow-edge-selected-color);
}

@media (prefers-reduced-motion: reduce) {
  .workspace-flow-canvas-wrap :deep(.vue-flow__edge.animated path),
  .workspace-flow-canvas-wrap :deep(.fusion-node-connection-layer path) {
    animation: none;
  }
}

.workspace-flow-canvas-wrap :deep(.vue-flow__node) {
  background: transparent;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  padding: 0;
  cursor: default;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__node:hover),
.workspace-flow-canvas-wrap :deep(.vue-flow__node.selected),
.workspace-flow-canvas-wrap :deep(.vue-flow__node:focus),
.workspace-flow-canvas-wrap :deep(.vue-flow__node:focus-visible) {
  box-shadow: none !important;
  border: 0 !important;
  outline: none !important;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__selectionpane) {
  cursor: default;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__pane) {
  cursor: default;
}

.canvas-layout.tool-hand .workspace-flow-canvas-wrap :deep(.vue-flow__pane) {
  cursor: grab;
}

.canvas-layout.tool-hand .workspace-flow-canvas-wrap :deep(.vue-flow__pane.dragging) {
  cursor: grabbing;
}

.canvas-layout.tool-text .workspace-flow-canvas-wrap :deep(.vue-flow__pane) {
  cursor: text;
}
</style>

<style>
@keyframes flow-edge-dashdraw {
  from {
    stroke-dashoffset: 10;
  }
}
</style>
