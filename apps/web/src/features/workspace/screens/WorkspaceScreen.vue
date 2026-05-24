<template>
  <div
    class="canvas-layout"
    :class="[
      `tool-${activeTool}`,
      { 'interaction-lite': isFlowInteractionActive, 'is-mask-editing': isMaskEditorChromeHidden },
    ]"
    @mousedown.self="closeBoardHeaderEditing"
  >
    <HomeCanvasHeader
      class="canvas-chrome canvas-chrome-header"
      :inert="isMaskEditorChromeHidden"
      :current-board-name="currentBoardName"
      :board-options="boardOptions"
      :current-board-id="currentBoardId"
      :is-board-menu-open="isBoardMenuOpen"
      :is-renaming-board="isRenamingBoard"
      :set-board-name-input-ref="setBoardNameInputRef"
      :export-disabled="!hasSelectedFlowNodes"
      :avatar-url="userStore.user?.avatar"
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
    />

    <WorkspaceToolPalette
      :active-tool="activeTool"
      :inert="isMaskEditorChromeHidden"
      @select-tool="setActiveTool"
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
        :selection-key-code="!isFlowCanvasLocked && activeTool === 'select' ? 'Shift' : false"
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
        @edge-update="onEdgeUpdate"
        @move-start="closeBoardHeaderEditing"
        @move="onFlowMove"
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
            :image-action-visible="activeImageActionNodeId === nodeProps.id"
            :image-prompt-composer-visible="activeImagePromptNodeId === nodeProps.id"
            :image-prompt-placeholder="
              getFlowMaskEditorCommittedData(nodeProps.id)
                ? '描述如何重绘选区'
                : '输入提示词改造这张图'
            "
            :fusion-prompt-composer-visible="activeFusionPromptNodeId === nodeProps.id"
            :image-direct-generating="isNodeDirectActionRunning(nodeProps.id)"
            :show-strength-control="
              !!activeStrengthAction && activeImageActionNodeId === nodeProps.id
            "
            :strength-pct="activeStrengthValue"
            :set-strength-track-ref="setStrengthTrackRef"
            :set-image-stage-ref="setImageStageRef"
            :clear-image-stage-ref="clearImageStageRef"
            :expand-mode-active="flowExpandNodeId === nodeProps.id"
            :expand-overlay-style="flowExpandNodeId === nodeProps.id ? flowExpandOverlayStyle : null"
            :expand-handle-radius="flowExpandHandleRadius"
            :mask-editor-mode-active="flowMaskEditorNodeId === nodeProps.id"
            :mask-editor-overlay-style="flowMaskEditorNodeId === nodeProps.id ? flowMaskEditorOverlayStyle : null"
            :mask-editor-canvas-width="Math.max(1, Math.round(flowMaskEditorOverlayStyle?.width ? parseFloat(flowMaskEditorOverlayStyle.width) : 0))"
            :mask-editor-canvas-height="Math.max(1, Math.round(flowMaskEditorOverlayStyle?.height ? parseFloat(flowMaskEditorOverlayStyle.height) : 0))"
            :mask-layer-opacity="maskEditorLayerOpacity"
            :image-layer-visible="true"
            :set-mask-canvas-ref="setFlowMaskEditorCanvasRef"
            @update-prompt-text="(value) => onFlowPromptTextUpdate(nodeProps.id, value)"
            @update-node-param="(payload) => onFlowNodeParamUpdate(nodeProps.id, payload)"
            @delete-node="() => onFlowDeleteNode(nodeProps.id)"
            @generate-model="() => onFlowModelGenerate(nodeProps.id)"
            @generate-viewpoint="() => onFlowViewpointGenerate(nodeProps.id)"
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
            @image-prompt-generate="(prompt, outputCount) => onFlowImagePromptGenerate(nodeProps.id, prompt, outputCount)"
            @image-expand-submit="(prompt, outputCount) => onFlowImageExpandSubmit(nodeProps.id, prompt, outputCount)"
            @fusion-contextmenu="(event) => onFlowFusionContextMenu(nodeProps.id, event)"
            @fusion-prompt-generate="
              (prompt, outputCount, aspectRatio) =>
                onFlowFusionPromptGenerate(nodeProps.id, prompt, outputCount, aspectRatio)
            "
            @prompt-generate="(prompt, outputCount, aspectRatio) => onFlowPromptGenerate(nodeProps.id, prompt, outputCount, aspectRatio)"
            @image-action="(action) => onFlowImageDirectAction(nodeProps.id, action)"
            @strength-action-mousedown="onFlowImageStrengthActionMouseDown"
            @strength-track-mousedown="onFlowImageStrengthTrackMouseDown"
            @expand-handle-pointerdown="
              (payload) => onFlowExpandHandlePointerDown(nodeProps.id, payload.anchor, payload.event)
            "
            @expand-handle-pointermove="onFlowExpandHandlePointerMove"
            @expand-handle-pointerup="onFlowExpandHandlePointerUp"
            @expand-frame-pointerdown="
              (event) => onFlowExpandFramePointerDown(nodeProps.id, event)
            "
            @mask-canvas-pointerdown="onFlowMaskEditorCanvasPointerDown"
          />
        </template>
      </VueFlow>
      <ImageMaskEditorChrome
        v-if="flowMaskEditorNodeId && maskEditorChromeAnchorRect && isMaskEditorChromeReady"
        :anchor-rect="maskEditorChromeAnchorRect"
        :draw-mode="flowMaskEditorDrawMode"
        :brush-size="flowMaskEditorBrushSize"
        :mask-layer-opacity="maskEditorLayerOpacity"
        :output-count="activeMaskEditorOutputCount"
        :reference-images="maskEditorReferenceImages"
        :direct-generating="isNodeDirectActionRunning(flowMaskEditorNodeId)"
        @set-draw-mode="setFlowMaskEditorDrawMode"
        @set-brush-size="setFlowMaskEditorBrushSize"
        @set-mask-layer-opacity="setMaskEditorLayerOpacity"
        @clear-mask="clearFlowMaskEditorMask"
        @invert-mask="invertFlowMaskEditorMask"
        @update:output-count="onMaskEditorChromeOutputCountUpdate"
        @image-input-open="onMaskEditorReferenceImageOpen"
        @reference-image-remove="onMaskEditorReferenceImageRemove"
        @reference-image-preview="onMaskEditorReferenceImagePreview"
        @submit="onMaskEditorChromeSubmit"
      />
      <WorkspaceNodeContextMenu
        :visible="nodeContextMenu.visible"
        :x="nodeContextMenu.x"
        :y="nodeContextMenu.y"
        :items="canvasNodeMenuItems"
        @create="handleNodeContextMenuCreate"
      />
    </div>

    <WorkspaceZoomControls
      :zoom-level="zoomLevel"
      :inert="isMaskEditorChromeHidden"
      @zoom-out="onZoomOut"
      @zoom-in="onZoomIn"
    />

  </div>
</template>

<script setup lang="ts">
import type { Ref } from 'vue'
import { useUserStore } from '../../../app/stores/user'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { uploadApi } from '../../../shared/api/upload'
import type { Edge, Node } from '@vue-flow/core'
import { PanOnScrollMode, useVueFlow, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowHistory } from '../composables/useFlowHistory'
import { useFlowTransactions, type FlowHistorySnapshot } from '../composables/useFlowTransactions'
import { useFlowBoardPersistence } from '../composables/useFlowBoardPersistence'
import { useFlowKeyboardShortcuts } from '../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../composables/useFlowLeaveGuard'
import { useFlowSelectionExport } from '../composables/useFlowSelectionExport'
import {
  useFlowNodeMutations,
  type FlowCreatableNodeKind,
} from '../composables/useFlowNodeMutations'
import { useFlowGenerationHandlers } from '../composables/useFlowGenerationHandlers'
import { useFlowImageDirectActions } from '../composables/useFlowImageDirectActions'
import {
  useFlowImageActionStrengthControl,
} from '../composables/useFlowImageActionStrengthControl'
import { useFlowImageNodeInteractions } from '../composables/useFlowImageNodeInteractions'
import { useFlowCanvasInteractionState } from '../composables/useFlowCanvasInteractionState'
import { useFlowNodeUiState } from '../composables/useFlowNodeUiState'
import { createEmptyCanvasBoardSnapshot } from '../utils/flowSnapshotAdapter'
import {
  clampImageNodeOutputCount,
  type CanvasBoardSnapshot,
} from '../../workspace/types/canvasNodes'
import { type ImageNodeAction } from '../types/imageActions'
import { useFlowNodeContextMenu } from '../composables/useFlowNodeContextMenu'
import { useFlowCanvasDrop } from '../composables/useFlowCanvasDrop'
import { useFlowBoardHeaderActions } from '../composables/useFlowBoardHeaderActions'
import { useFlowSelectionController } from '../composables/useFlowSelectionController'
import { useFlowNodeMediaInputs } from '../composables/useFlowNodeMediaInputs'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
import { useFlowMediaInputImages } from '../composables/useFlowMediaInputImages'
import { useFlowNodeInternalsRefreshQueue } from '../composables/useFlowNodeInternalsRefreshQueue'
import { useFlowViewportControls } from '../composables/useFlowViewportControls'
import { useFlowActiveNodePanels } from '../composables/useFlowActiveNodePanels'
import { useFlowCanvasShellInteractions } from '../composables/useFlowCanvasShellInteractions'
import { useFlowImageActionOrchestrator } from '../composables/useFlowImageActionOrchestrator'
import {
  useImageCanvasExtendOverlay,
  type CanvasExtendDragAnchor,
} from '../composables/useImageCanvasExtendOverlay'
import { useImageMaskEditorOverlay } from '../composables/useImageMaskEditorOverlay'
import { useImageStageRegistry } from '../composables/useImageStageRegistry'
import { normalizePromptAspectRatio } from '../config/promptAspectRatio'
import { useFlowAutosave } from '../composables/useFlowAutosave'
import { useAppToast } from '../../../shared/composables/useAppToast'
import ImageMaskEditorChrome from '../components/nodes/ImageMaskEditorChrome.vue'
import {
  useMaskEditorViewportController,
} from '../composables/useMaskEditorViewportController'
import WorkspaceToolPalette from '../components/WorkspaceToolPalette.vue'
import WorkspaceZoomControls from '../components/WorkspaceZoomControls.vue'

const appToast = useAppToast()
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
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

const nodes = ref<Node[]>([])
const edges = ref<Edge[]>([])
const canvasWrapRef = ref<HTMLElement | null>(null)
const boardBaseSnapshot = ref<CanvasBoardSnapshot>(createEmptyCanvasBoardSnapshot())

const createNodeOptions: readonly { key: string; label: string; value: FlowCreatableNodeKind }[] = [
  { key: 'prompt', label: 'Prompt', value: 'prompt' },
  { key: 'image', label: 'Image', value: 'image' },
  { key: 'fusion', label: 'Fusion', value: 'fusion' },
  { key: 'model', label: 'Model', value: 'model' },
  { key: 'viewpoint', label: 'Viewpoint', value: 'viewpoint' },
]
const isLoadingBoardSnapshot = ref(false)
const isSavingBoard = ref(false)
const isDirty = ref(false)
const saveStatus = ref<'idle' | 'saved' | 'failed'>('idle')
const activeTool = ref<'select' | 'hand' | 'text'>('select')

const createNodeKind = ref<FlowCreatableNodeKind>('prompt')

const FLOW_ENABLE_VISIBLE_ELEMENTS_CULLING =
  import.meta.env.VITE_WORKSPACE_FLOW_ENABLE_CULLING === 'true'

const {
  activeImageActionNodeId,
  activeImagePromptNodeId,
  activeFusionPromptNodeId,
  closeActiveImagePromptComposer,
  clearActiveNodePanels,
} = useFlowActiveNodePanels()
const duplicateModifierPressed = ref(false)
const { setImageStageRef, clearImageStageRef, getImageStageElement } = useImageStageRegistry()
const { scheduleFlowNodeInternalsRefresh } = useFlowNodeInternalsRefreshQueue({
  updateNodeInternals,
})

async function onNavigateRequest(path: string) {
  void flushAutosave()
  await router.push(path)
}

async function onLogoutRequest() {
  void flushAutosave()
  await userStore.logout('/user/home')
}

function onFlowMove(payload: { flowTransform?: { x?: number; y?: number; zoom?: number } }) {
  closeBoardHeaderEditing()
  const transform = payload.flowTransform
  if (!transform) return
  applyFlowTransform(transform)
  if (!isMaskEditorChromeReady.value) return
  scheduleMaskEditorChromeAnchorUpdate()
}

function markDirty() {
  if (canSaveBoard.value) {
    isDirty.value = true
    if (saveStatus.value !== 'failed') saveStatus.value = 'idle'
    markDirtyAndSchedule()
  }
}

const flowHistory = useFlowHistory({
  nodes,
  edges,
  maxSteps: 10,
  onRestore: () => {
    markDirty()
  },
  commitNodes: (nodes) => commitFlowNodes(nodes as Node[]),
  commitEdges: (edges) => commitFlowEdges(edges as Edge[]),
})

function refreshSnapshot() {
  // Snapshot preview UI has been removed; callers keep this hook for flow parity.
}

const boardPersistence = useFlowBoardPersistence({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  boardBaseSnapshot,
  isDirty,
  saveStatus,
  isLoadingBoardSnapshot,
  isSavingBoard,
  getBoardIdQuery: () => route.query.boardId,
  onWarning: (msg) => {
    appToast.warning(msg)
  },
  onSuccess: (msg) => {
    appToast.success(msg)
  },
  onError: (msg) => {
    appToast.error(msg)
  },
  refreshSnapshot,
  clearFlowHistoryBaseline: flowHistory.clearBaseline,
  getUserIdentity: () => ({
    userId: Number(userStore.user?.id),
    userUid: `${userStore.user?.uid ?? ''}`.trim(),
  }),
  getViewport,
  setViewport,
  commitNodes: (nodes) => commitFlowNodes(nodes as Node[]),
  commitEdges: (edges) => commitFlowEdges(edges as Edge[]),
})

const currentBoardId = boardPersistence.currentBoardId
const canSaveBoard = boardPersistence.canSaveBoard
const loadInitialOrRouteBoard = boardPersistence.loadInitialOrRouteBoard
const saveCurrentBoardSnapshot = boardPersistence.saveCurrentBoardSnapshot
const { markDirtyAndSchedule, flushAutosave } = useFlowAutosave({
  isDirty,
  isSaving: isSavingBoard,
  save: (options) => saveCurrentBoardSnapshot({ waitForPreview: false, silent: options?.silent }),
})

const {
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
} = useFlowBoardHeaderActions({
  currentBoardId,
  message: {
    warning: (msg: string) => appToast.warning(msg),
    success: (msg: string) => appToast.success(msg),
  },
  router,
  route,
  flushAutosave: () => {
    void flushAutosave()
  },
})

function commitFlowNodes(nextNodes: Node[]) {
  nodes.value = nextNodes
  setNodes(nextNodes)
}

function commitFlowEdges(nextEdges: Edge[]) {
  edges.value = nextEdges
  setEdges(nextEdges)
}

const flowTransactions = useFlowTransactions({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  captureHistory: flowHistory.capture,
  pushHistory: flowHistory.push,
  beginDragHistory: flowHistory.onNodeDragStart,
  commitDragHistory: flowHistory.onNodeDragStop,
  markDirty,
  refreshSnapshot,
  commitNodes: (nextNodes) => {
    commitFlowNodes(nextNodes)
  },
  commitEdges: (nextEdges) => {
    commitFlowEdges(nextEdges)
  },
})

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
function onFlowDeleteNode(nodeId: string) {
  clearActiveNodePanels()
  clearFlowMaskEditorCommittedData(nodeId)
  hideStrengthControl()
  closeNodeContextMenu()
  _onFlowDeleteNode(nodeId)
}
const updateSnapshotNodeBase = flowNodeMutations.updateSnapshotNode
const _onFlowPromptTextUpdate = flowNodeMutations.onFlowPromptTextUpdate
const _onFlowNodeParamUpdate = flowNodeMutations.onFlowNodeParamUpdate
function onFlowPromptTextUpdate(nodeId: string, value: string) {
  _onFlowPromptTextUpdate(nodeId, value)
  markFlowExecutionNodeDirty(nodeId)
}
function onFlowNodeParamUpdate(nodeId: string, payload: { key: string; value: unknown }) {
  _onFlowNodeParamUpdate(nodeId, payload)
  markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
}
const markFlowExecutionNodeDirty = flowNodeMutations.markFlowExecutionNodeDirty

function updateSnapshotNode(
  nodeId: string,
  mutate: (snapshotNode: Record<string, unknown>) => Record<string, unknown>,
  options?: { history?: boolean },
) {
  updateSnapshotNodeBase(nodeId, mutate, options)
  scheduleFlowNodeInternalsRefresh([nodeId])
}

const selectionExport = useFlowSelectionExport({ nodes, message: appToast })
const hasSelectedFlowNodes = computed(() => selectionExport.hasSelectedNodes())

function onExportSelectedImages() {
  selectionExport.exportSelectedImages()
}

const {
  isCanvasInteractionActive: isFlowInteractionActive,
  startCanvasInteraction: startFlowInteraction,
  stopCanvasInteraction: stopFlowInteraction,
} = useFlowCanvasInteractionState()

const { nodeContextMenu, canvasNodeMenuItems, closeNodeContextMenu, openNodeContextMenu } =
  useFlowNodeContextMenu()

let canvasDropRef: ReturnType<typeof useFlowCanvasDrop> | null = null
const {
  getImageNodeSnapshot,
  getImageNodeDisplayPayload,
  onFlowImageOpenFile,
  onFlowImageReset: _onFlowImageReset,
  onFlowImageStageDrop,
} = useFlowNodeMediaInputs({
  nodes: nodes as Ref<Node[]>,
  canvasDrop: {
    applyImageFileToImageNode: async (nodeId, file): Promise<boolean> => {
      if (!canvasDropRef) return false
      const updated = await canvasDropRef.applyImageFileToImageNode(nodeId, file)
      if (updated) clearFlowMaskEditorCommittedData(nodeId)
      return updated
    },
  },
  updateSnapshotNode,
  markFlowExecutionNodeDirty,
})

function onFlowImageReset(nodeId: string) {
  clearFlowMaskEditorCommittedData(nodeId)
  _onFlowImageReset(nodeId)
}
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
  uploadImageToBoard: async (file) => {
    if (!canvasDropRef) throw new Error('上传能力暂不可用')
    return canvasDropRef.uploadImageToBoard(file)
  },
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
  onFlowModelGenerate,
  onFlowViewpointGenerate,
  isFlowExecutionNodeRunning,
} = useFlowGenerationHandlers({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  currentBoardId,
  updateSnapshotNode,
  captureHistory: flowTransactions.captureHistory as () => FlowHistorySnapshot,
  onWarning: (text) => {
    appToast.warning(text)
  },
  onSuccess: (text) => {
    appToast.success(text)
  },
  onError: (text) => {
    appToast.error(text)
  },
  onOutputCommitted: (nodeId) => {
    markFlowExecutionNodeDirty(nodeId)
  },
})

const {
  isNodeDirectActionRunning,
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
    const targetNodeId = activeImageActionNodeId.value
    if (!targetNodeId) return
    clearActiveNodePanels()
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

const {
  maskEditorNodeId: flowMaskEditorNodeId,
  maskEditorOverlayStyle: flowMaskEditorOverlayStyle,
  drawMode: flowMaskEditorDrawMode,
  brushSize: flowMaskEditorBrushSize,
  enterMaskEditorMode: enterFlowMaskEditorMode,
  cancelMaskEditorMode: cancelFlowMaskEditorMode,
  commitMaskEditorMode: commitFlowMaskEditorMode,
  clearMask: clearFlowMaskEditorMask,
  invertMask: invertFlowMaskEditorMask,
  clearCommittedMaskData: clearFlowMaskEditorCommittedData,
  setMaskCanvasRef: setFlowMaskEditorCanvasRef,
  onCanvasPointerDown: onFlowMaskEditorCanvasPointerDown,
  setDrawMode: setFlowMaskEditorDrawMode,
  setBrushSize: setFlowMaskEditorBrushSize,
  getCommittedMaskData: getFlowMaskEditorCommittedData,
} = useImageMaskEditorOverlay({
  nodes: nodes as Ref<Node[]>,
  activeTool: activeTool as Ref<string>,
  getImageNodeDisplayPayload,
  getImageStageElement,
  onBeforePointerCancel: clearSelectedFlowElementsSync,
  startCanvasInteraction: startFlowInteraction,
  stopCanvasInteraction: stopFlowInteraction,
})

const {
  maskEditorChromeAnchorRect,
  isMaskEditorChromeReady,
  maskEditorLayerOpacity,
  maskEditorReferenceImages,
  activeMaskEditorOutputCount,
  setMaskEditorLayerOpacity,
  onMaskEditorReferenceImageOpen,
  onMaskEditorReferenceImageRemove,
  onMaskEditorReferenceImagePreview,
  onCanvasWheel,
  focusMaskEditorAndShowChrome,
  scheduleMaskEditorChromeAnchorUpdate,
} = useMaskEditorViewportController({
  nodes: nodes as Ref<Node[]>,
  canvasWrapRef,
  flowMaskEditorNodeId,
  getImageNodeSnapshot,
  getImageStageElement,
  screenToFlowCoordinate,
  getViewport,
  setViewport: (viewport, options) => setViewport(viewport, options),
  setCenter: (x, y, options) => setCenter(x, y, options),
  uploadImageToBoard: async (file: File) => {
    const uploaded = await canvasDrop.uploadImageToBoard(file)
    return { url: uploaded.url, ratio: uploaded.ratio }
  },
  onWarning: (msg) => appToast.warning(msg),
  onReferenceImagePreview: (image) => {
    void openImageUrlPreview(image.url, image.ratio)
  },
  onViewportTransform: (viewport) => {
    applyFlowTransform(viewport)
  },
})
const isMaskEditorChromeHidden = computed(() => !!flowMaskEditorNodeId.value)
const isFlowCanvasLocked = computed(
  () => isFlowInteractionActive.value || !!flowMaskEditorNodeId.value,
)

function onMaskEditorChromeSubmit(prompt: string, outputCount: number) {
  const nodeId = flowMaskEditorNodeId.value
  if (!nodeId) return
  const didCommitMask = commitFlowMaskEditorMode()
  const committed = getFlowMaskEditorCommittedData(nodeId)
  if (!didCommitMask || !committed?.executionMaskDataUrl) {
    appToast.warning('请先标记局部重绘区域')
    return
  }
  clearActiveNodePanels()
  hideStrengthControl()
  void runImageInpaintAction(nodeId, {
    prompt,
    outputCount,
    maskDataUrl: committed.executionMaskDataUrl,
    referenceImageUrls: maskEditorReferenceImages.value.map((image) => image.url),
  })
}

function onMaskEditorChromeOutputCountUpdate(value: number) {
  const nodeId = flowMaskEditorNodeId.value
  if (!nodeId) return
  onFlowNodeParamUpdate(nodeId, { key: 'outputCount', value: clampImageNodeOutputCount(value) })
}

function onFlowImageExpandSubmit(nodeId: string, prompt: string, outputCount: number) {
  if (flowExpandNodeId.value !== nodeId) return
  closeActiveImagePromptComposer()
  activeImageActionNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  void commitFlowExpandMode(prompt, outputCount)
}

function onFlowPromptGenerate(
  promptNodeId: string,
  prompt: string,
  outputCount: number,
  aspectRatio: string,
) {
  cancelFlowExpandMode()
  cancelFlowMaskEditorMode()
  const normalizedAspectRatio = normalizePromptAspectRatio(aspectRatio)
  const inputImages = getMediaInputImages(promptNodeId)
  if (inputImages.length === 0) {
    void runTextPromptGeneration(promptNodeId, prompt, outputCount, normalizedAspectRatio)
    return
  }
  const linkedSourceNodeIds = new Set(resolvePromptInputImageNodeIds(promptNodeId))
  inputImages.forEach((item) => {
    if (item.sourceType === 'node' && linkedSourceNodeIds.has(item.sourceId)) {
      void runImagePromptGeneration(item.sourceId, prompt, outputCount, normalizedAspectRatio, {
        outputAnchorNodeId: promptNodeId,
      })
      return
    }
    void runImagePromptGenerationFromSource(
      {
        url: item.url,
        ratio: item.ratio,
        x: 0,
        y: 0,
        width: 280,
      },
      item.id,
      prompt,
      outputCount,
      normalizedAspectRatio,
      { outputAnchorNodeId: promptNodeId },
    )
  })
}

function onFlowExpandHandlePointerDown(
  nodeId: string,
  anchor: CanvasExtendDragAnchor,
  event: PointerEvent,
) {
  if (flowExpandNodeId.value !== nodeId) return
  beginFlowExpandHandleResize(event, anchor)
}

function onFlowExpandFramePointerDown(nodeId: string, event: PointerEvent) {
  if (flowExpandNodeId.value !== nodeId) return
  beginFlowExpandFrameDrag(event)
}

const {
  clearFlowSelection,
  selectFlowNodeForContextMenu,
  onPaneClick,
  onFlowNodeClick,
  onFlowNodeMouseDown,
  deleteSelectedFlowElements,
} = useFlowSelectionController({
  activeTool: activeTool as Ref<string>,
  activeImageActionNodeId,
  activeImagePromptNodeId,
  activeFusionPromptNodeId,
  closeImagePromptComposer: closeActiveImagePromptComposer,
  cancelFlowExpandMode,
  hideStrengthControl,
  closeBoardHeaderEditing,
  closeNodeContextMenu,
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

const { onNodeDragStart, onNodeDragStop } = useFlowNodeDragInteractions({
  beginNodeDragTransaction: flowTransactions.beginNodeDragTransaction,
  commitNodeDragTransaction: flowTransactions.commitNodeDragTransaction,
  duplicateModifierPressed,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
  activeImageActionNodeId,
  activeImagePromptNodeId,
  activeFusionPromptNodeId,
  closeImagePromptComposer: closeActiveImagePromptComposer,
  cancelFlowExpandMode,
  hideStrengthControl,
})

const canvasDrop = useFlowCanvasDrop({
  currentBoardId,
  screenToFlowCoordinate,
  createNodeAtPosition,
  updateSnapshotNode,
  getImageNodeSnapshot,
  uploadApi,
  sanitizeApiErrorMessage,
  onWarning: (msg) => appToast.warning(msg),
  onSuccess: (msg) => appToast.success(msg),
  onNodeMutated: (nodeId) => markFlowExecutionNodeDirty(nodeId),
  createNodeOptions: createNodeOptions as unknown as ReadonlyArray<{ value?: unknown }>,
})
canvasDropRef = canvasDrop

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
  closeNodeContextMenu,
  createNodeAtPosition,
  cancelFlowExpandMode,
  closeBoardHeaderEditing,
  clearActiveNodePanels,
  hideStrengthControl,
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
  flowMaskEditorNodeId,
  activeImageActionNodeId,
  activeImagePromptNodeId,
  activeFusionPromptNodeId,
  suppressClickAction,
  clearActiveNodePanels,
  closeActiveImagePromptComposer,
  hideStrengthControl,
  selectFlowNodeForContextMenu,
  cancelFlowExpandMode,
  cancelFlowMaskEditorMode,
  enterFlowExpandMode,
  enterFlowMaskEditorMode,
  focusMaskEditorAndShowChrome,
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
  isFlowExecutionNodeRunning,
})

watch(
  () => route.query.boardId,
  async () => {
    await loadInitialOrRouteBoard()
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
  onLeave: () => {
    void flushAutosave()
  },
})

onMounted(async () => {
  await loadInitialOrRouteBoard()
  refreshBoardMetas()
  refreshSnapshot()
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
  --canvas-control-border-width: 1.2px;
  --canvas-control-icon-stroke: 1.5;
  --canvas-chrome-slide-duration: 0.2s;
  --canvas-chrome-slide-ease: cubic-bezier(0.22, 1, 0.36, 1);
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

.canvas-chrome {
  transition: transform var(--canvas-chrome-slide-duration) var(--canvas-chrome-slide-ease);
  will-change: transform;
}

.canvas-layout.is-mask-editing .canvas-chrome {
  pointer-events: none;
}

.canvas-layout.is-mask-editing .canvas-chrome * {
  pointer-events: none;
}

.canvas-layout.is-mask-editing .canvas-chrome-header {
  transform: translateY(-72px);
}

.canvas-layout.is-mask-editing .canvas-chrome-left {
  transform: translate(-72px, -50%);
}

.canvas-layout.is-mask-editing .canvas-chrome-status {
  transform: translateY(72px);
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
  background-image: radial-gradient(circle, #e2e4e8 1.5px, transparent 1.5px);
  background-size: 24px 24px;
  background-position: 0 0;
}

.toolbar-divider {
  width: 24px;
  height: 1px;
  background: #f1f3f9;
  margin: 4px auto;
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
