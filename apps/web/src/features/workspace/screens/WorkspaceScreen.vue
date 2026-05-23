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

    <aside
      class="floating-toolbar canvas-chrome canvas-chrome-left canvas-control-group canvas-control-group--vertical"
      :inert="isMaskEditorChromeHidden"
      @wheel.prevent.stop
    >
      <div
        class="canvas-icon-button"
        :class="{ 'is-active': activeTool === 'select' }"
        title="Select (V)"
        @click="setActiveTool('select')"
      >
        <OneFingerSelectHandGesture class="canvas-control-icon" />
      </div>
      <div
        class="canvas-icon-button"
        :class="{ 'is-active': activeTool === 'hand' }"
        title="Hand (H)"
        @click="setActiveTool('hand')"
      >
        <SpockHandGesture class="canvas-control-icon" />
      </div>
      <div
        class="canvas-icon-button"
        :class="{ 'is-active': activeTool === 'text' }"
        title="Text (T)"
        @click="setActiveTool('text')"
      >
        <Text class="canvas-control-icon" />
      </div>
    </aside>

    <div
      ref="canvasWrapRef"
      class="workspace-flow-canvas-wrap"
      @dragover.prevent="onCanvasDragOver"
      @drop.prevent="onCanvasDrop"
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
            :mask-editor-draw-mode="flowMaskEditorDrawMode"
            :mask-editor-brush-size="flowMaskEditorBrushSize"
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
            @mask-set-draw-mode="setFlowMaskEditorDrawMode"
            @mask-set-brush-size="setFlowMaskEditorBrushSize"
            @mask-clear="clearFlowMaskEditorMask"
            @mask-cancel="cancelFlowMaskEditorMode"
            @mask-done="onFlowMaskEditorDone"
          />
        </template>
      </VueFlow>
      <WorkspaceNodeContextMenu
        :visible="nodeContextMenu.visible"
        :x="nodeContextMenu.x"
        :y="nodeContextMenu.y"
        :items="canvasNodeMenuItems"
        @create="handleNodeContextMenuCreate"
      />
    </div>

    <div
      class="floating-status canvas-chrome canvas-chrome-status"
      :inert="isMaskEditorChromeHidden"
      @wheel.prevent.stop
    >
      <div class="zoom-controls">
        <button type="button" class="zoom-btn" title="缩小" @click="onZoomOut">
          <Minus class="zoom-btn-icon" />
        </button>
        <span class="zoom-level">{{ zoomLevel }}%</span>
        <button type="button" class="zoom-btn" title="放大" @click="onZoomIn">
          <Plus class="zoom-btn-icon" />
        </button>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import type { Ref } from 'vue'
import { useUserStore } from '../../../app/stores/user'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { uploadApi } from '../../../shared/api/upload'
import { IMAGE_UPLOAD_ACCEPT } from '../../../shared/constants/media'
import type { Edge, Node } from '@vue-flow/core'
import { Minus, OneFingerSelectHandGesture, Plus, SpockHandGesture, Text } from '@iconoir/vue'
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
import type { CanvasBoardSnapshot } from '../../workspace/types/canvasNodes'
import { IMAGE_NODE_ACTION, isImageStrengthAction, type ImageNodeAction } from '../types/imageActions'
import { useFlowNodeContextMenu } from '../composables/useFlowNodeContextMenu'
import { useFlowCanvasDrop } from '../composables/useFlowCanvasDrop'
import { useFlowBoardHeaderActions } from '../composables/useFlowBoardHeaderActions'
import { useFlowSelectionController } from '../composables/useFlowSelectionController'
import { useFlowNodeMediaInputs } from '../composables/useFlowNodeMediaInputs'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
import {
  useImageCanvasExtendOverlay,
  type CanvasExtendDragAnchor,
} from '../composables/useImageCanvasExtendOverlay'
import { useImageMaskEditorOverlay } from '../composables/useImageMaskEditorOverlay'
import { useImageStageRegistry } from '../composables/useImageStageRegistry'
import { normalizePromptAspectRatio } from '../config/promptAspectRatio'
import { useFlowAutosave } from '../composables/useFlowAutosave'
import { useAppToast } from '../../../shared/composables/useAppToast'

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

const zoomLevel = ref(100)
let lastFlowZoom = 100
let maskEditorReturnViewport: { x: number; y: number; zoom: number } | null = null
let maskEditorFocusRequestId = 0

function applyFlowTransform(transform: { x?: number; y?: number; zoom?: number }): boolean {
  let zoomChanged = false
  if (Number.isFinite(transform.zoom)) {
    const nextZoom = Math.max(10, Math.round((transform.zoom as number) * 100))
    if (lastFlowZoom !== nextZoom) zoomChanged = true
    zoomLevel.value = nextZoom
    lastFlowZoom = nextZoom
  }
  return zoomChanged
}

function onZoomIn() {
  zoomIn()
}
function onZoomOut() {
  zoomOut()
}

function resolveFlowNodeCenter(nodeId: string): { x: number; y: number } | null {
  const stage = getImageStageElement(nodeId)
  if (stage) {
    const rect = stage.getBoundingClientRect()
    return screenToFlowCoordinate({
      x: rect.left + rect.width / 2,
      y: rect.top + rect.height / 2,
    })
  }

  const node = (
    nodes.value as Array<{
      id: string
      position: { x: number; y: number }
      data?: { snapshotNode?: unknown }
    }>
  ).find((item) => item.id === nodeId)
  if (!node) return null
  const snapshot = node.data?.snapshotNode as { width?: unknown } | undefined
  const width = Number(snapshot?.width)
  return {
    x: Number(node.position.x) + (Number.isFinite(width) && width > 0 ? width / 2 : 140),
    y: Number(node.position.y) + 140,
  }
}

function isMaskEditorFocusRequestCurrent(nodeId: string, requestId: number) {
  return requestId === maskEditorFocusRequestId && flowMaskEditorNodeId.value === nodeId
}

async function focusImageStageForMaskEditor(nodeId: string, requestId: number) {
  await nextTick()
  if (!isMaskEditorFocusRequestCurrent(nodeId, requestId)) return
  const center = resolveFlowNodeCenter(nodeId)
  if (!center) return
  if (!isMaskEditorFocusRequestCurrent(nodeId, requestId)) return
  await setCenter(center.x, center.y, {
    zoom: 2,
    duration: 220,
    interpolate: 'smooth',
  })
  if (!isMaskEditorFocusRequestCurrent(nodeId, requestId)) return
  applyFlowTransform(getViewport())
}

function restoreMaskEditorReturnViewport() {
  maskEditorFocusRequestId += 1
  const viewport = maskEditorReturnViewport
  if (!viewport) return
  maskEditorReturnViewport = null
  void setViewport({ ...viewport }, { duration: 220, interpolate: 'smooth' })
  applyFlowTransform(viewport)
}

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

let pendingNodeInternalsRefresh = false
let queuedNodeInternalsIds: Set<string> | null = null
const activeImageActionNodeId = ref<string | null>(null)
const activeImagePromptNodeId = ref<string | null>(null)
const activeFusionPromptNodeId = ref<string | null>(null)
const duplicateModifierPressed = ref(false)
const { setImageStageRef, clearImageStageRef, getImageStageElement } = useImageStageRegistry()

function scheduleFlowNodeInternalsRefresh(nodeIds: string[]) {
  const nextIds = nodeIds.filter((id) => typeof id === 'string' && id.length > 0)
  if (nextIds.length === 0) return
  if (!queuedNodeInternalsIds) queuedNodeInternalsIds = new Set()
  nextIds.forEach((id) => queuedNodeInternalsIds?.add(id))
  if (pendingNodeInternalsRefresh) return
  pendingNodeInternalsRefresh = true
  void nextTick(() => {
    requestAnimationFrame(() => {
      const ids = readQueuedNodeInternalsRefreshIds()
      if (ids.length > 0) {
        updateNodeInternals(ids)
      }
      const pendingIds = readQueuedNodeInternalsRefreshIds()
      pendingNodeInternalsRefresh = false
      if (pendingIds.length > 0) {
        scheduleFlowNodeInternalsRefresh(pendingIds)
      }
    })
  })
}

function readQueuedNodeInternalsRefreshIds(): string[] {
  const ids = queuedNodeInternalsIds ? Array.from(queuedNodeInternalsIds) : []
  queuedNodeInternalsIds = null
  return ids
}

function onPaneContextMenu(event: MouseEvent) {
  event.preventDefault()
  cancelFlowExpandMode()
  closeBoardHeaderEditing()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  clearFlowSelection()
  const flowPoint = screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
  const rect = canvasWrapRef.value?.getBoundingClientRect()
  const x = rect ? event.clientX - rect.left : event.clientX
  const y = rect ? event.clientY - rect.top : event.clientY
  openNodeContextMenu(x, y, flowPoint.x, flowPoint.y)
}

function handleNodeContextMenuCreate(kind: FlowCreatableNodeKind) {
  createNodeAtPosition(kind, nodeContextMenu.value.flowX, nodeContextMenu.value.flowY)
  cancelFlowExpandMode()
  closeNodeContextMenu()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
}

function setActiveTool(tool: 'select' | 'hand' | 'text') {
  cancelFlowExpandMode()
  closeBoardHeaderEditing()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  activeTool.value = tool
  if (tool === 'text') {
    appToast.info('文本工具稍后接入')
  }
}

function onEscapeShortcut() {
  if (flowExpandNodeId.value) {
    cancelFlowExpandMode()
    return true
  }
  if (isBoardMenuOpen.value || isRenamingBoard.value) {
    closeBoardHeaderEditing()
    return true
  }
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  return clearFlowSelection()
}

function onPanViewport(delta: { x: number; y: number }) {
  const viewport = getViewport()
  setViewport({
    x: viewport.x + delta.x,
    y: viewport.y + delta.y,
    zoom: viewport.zoom,
  })
}

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
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
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
  onFlowImageReset,
  onFlowImageStageDrop,
} = useFlowNodeMediaInputs({
  nodes: nodes as Ref<Node[]>,
  canvasDrop: {
    applyImageFileToImageNode: async (nodeId, file) => {
      if (!canvasDropRef) return
      await canvasDropRef.applyImageFileToImageNode(nodeId, file)
    },
  },
  updateSnapshotNode,
  markFlowExecutionNodeDirty,
})
const { openNodeImagePreview, openImageUrlPreview } = useFlowImagePreviewLightbox()

function resolveNodePreviewImageById(
  nodeId: string,
): { id: string; sourceType: 'node'; sourceId: string; sourceNodeId: string; url: string; ratio: number } | null {
  const node = (nodes.value as Node[]).find((item) => item.id === nodeId)
  const kind = node?.data?.kind
  const snapshot = node?.data?.snapshotNode as Record<string, unknown> | undefined
  if (!kind || !snapshot) return null
  if (kind === 'image') {
    const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    const latest = outputs[outputs.length - 1] as { url?: string; ratio?: number } | undefined
    const outputUrl = typeof latest?.url === 'string' ? latest.url.trim() : ''
    if (outputUrl) {
      return { id: `node:${nodeId}`, sourceType: 'node', sourceId: nodeId, sourceNodeId: nodeId, url: outputUrl, ratio: Number(latest?.ratio) || 1 }
    }
    const sourceUrl = String(snapshot.sourceImageUrl ?? '').trim()
    if (!sourceUrl) return null
    return { id: `node:${nodeId}`, sourceType: 'node', sourceId: nodeId, sourceNodeId: nodeId, url: sourceUrl, ratio: Number(snapshot.sourceImageRatio) || 1 }
  }
  const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
  const latest = outputs[outputs.length - 1] as { url?: string; ratio?: number } | undefined
  const outputUrl = typeof latest?.url === 'string' ? latest.url.trim() : ''
  return outputUrl
    ? { id: `node:${nodeId}`, sourceType: 'node', sourceId: nodeId, sourceNodeId: nodeId, url: outputUrl, ratio: Number(latest?.ratio) || 1 }
    : null
}

watch(
  [nodes, edges],
  () => {
    const flowNodes = nodes.value as Node[]
    const flowEdges = edges.value as Edge[]
    flowNodes
      .filter((node) => node.data?.kind === 'prompt' || node.data?.kind === 'fusion')
      .forEach((mediaInputNode) => {
        const linkedImages = flowEdges
          .filter((edge) => edge.target === mediaInputNode.id && edge.targetHandle === 'in:image')
          .map((edge) => resolveNodePreviewImageById(edge.source))
          .filter(
            (item): item is { id: string; sourceType: 'node'; sourceId: string; sourceNodeId: string; url: string; ratio: number } =>
              !!item && !!item.url,
          )
        const linkedBySourceId = new Map(linkedImages.map((item) => [item.sourceId, item]))
        updateSnapshotNodeBase(
          mediaInputNode.id,
          (snapshotNode) => {
            const current = Array.isArray(snapshotNode.inputImages) ? snapshotNode.inputImages : []
            const nextImages = current
              .map((item) => {
                const sourceType = (item as { sourceType?: unknown })?.sourceType
                if (sourceType === 'external') return item
                const sourceId =
                  typeof (item as { sourceId?: unknown })?.sourceId === 'string'
                    ? (item as { sourceId: string }).sourceId
                    : ''
                return sourceId ? linkedBySourceId.get(sourceId) : null
              })
              .filter((item): item is { id: string; sourceType: 'node' | 'external'; sourceId: string; sourceNodeId?: string; url: string; ratio: number } => !!item)
            const nextNodeSourceIds = new Set(
              nextImages
                .filter((item) => item.sourceType === 'node')
                .map((item) => item.sourceId),
            )
            nextImages.push(
              ...linkedImages.filter((item) => !nextNodeSourceIds.has(item.sourceId)),
            )
            const limitedNextImages = nextImages.slice(0, 4)
            const same =
              current.length === limitedNextImages.length &&
              current.every(
                (item, idx) =>
                  (item as { id?: unknown })?.id === limitedNextImages[idx]?.id &&
                  (item as { sourceType?: unknown })?.sourceType === limitedNextImages[idx]?.sourceType &&
                  (item as { sourceId?: unknown })?.sourceId === limitedNextImages[idx]?.sourceId &&
                  item?.url === limitedNextImages[idx]?.url &&
                  item?.ratio === limitedNextImages[idx]?.ratio,
              )
            return same ? snapshotNode : { ...snapshotNode, inputImages: limitedNextImages }
          },
          { history: false, dirty: true },
        )
      })
  },
  { deep: true },
)

function onCanvasDragOver(event: DragEvent) {
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  canvasDrop.onCanvasDragOver(event)
}

async function onCanvasDrop(event: DragEvent) {
  await canvasDrop.onCanvasDrop(event)
}

function resolveMediaInputNodeLabel(nodeId: string) {
  const kind = (nodes.value as Node[]).find((node) => node.id === nodeId)?.data?.kind
  return kind === 'prompt' ? 'Prompt' : '节点'
}

async function onFlowMediaInputImageDrop(nodeId: string, event: DragEvent) {
  const currentInputCount = getMediaInputImages(nodeId).length
  const label = resolveMediaInputNodeLabel(nodeId)
  if (currentInputCount >= 4) {
    appToast.warning(`${label} 最多接入 4 张图片`)
    return
  }
  const file = event.dataTransfer?.files?.[0]
  if (!file) return
  try {
    const uploaded = await canvasDrop.uploadImageToBoard(file)
    updateSnapshotNodeBase(nodeId, (snapshotNode) => {
      const current = Array.isArray(snapshotNode.inputImages) ? snapshotNode.inputImages : []
      return {
        ...snapshotNode,
        inputImages: [
          ...current,
          {
            id: `external:${Date.now()}:${Math.random().toString(16).slice(2, 8)}`,
            sourceType: 'external',
            sourceId: uploaded.url,
            url: uploaded.url,
            ratio: uploaded.ratio,
          },
        ].slice(0, 4),
      }
    })
    markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
  } catch (error) {
    const msg = error instanceof Error ? error.message : '上传失败'
    appToast.warning(msg)
  }
}

function onFlowMediaInputImageOpen(nodeId: string) {
  const kind = (nodes.value as Node[]).find((node) => node.id === nodeId)?.data?.kind
  if (kind !== 'prompt') return
  const currentInputCount = getMediaInputImages(nodeId).length
  const label = resolveMediaInputNodeLabel(nodeId)
  if (currentInputCount >= 4) {
    appToast.warning(`${label} 最多接入 4 张图片`)
    return
  }
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = IMAGE_UPLOAD_ACCEPT
  input.onchange = () => {
    const file = input.files?.[0]
    if (!file) return
    const syntheticDrop = {
      dataTransfer: { files: [file] },
      clientX: window.innerWidth / 2,
      clientY: window.innerHeight / 2,
    } as unknown as DragEvent
    void onFlowMediaInputImageDrop(nodeId, syntheticDrop)
  }
  input.click()
}

function onFlowMediaInputImagePreview(image: { url: string; ratio: number }) {
  void openImageUrlPreview(image.url, image.ratio)
}

function onFlowMediaInputImageRemove(nodeId: string, imageId: string) {
  const currentInputImages = getMediaInputImages(nodeId)
  const removed = currentInputImages.find((item) => item.id === imageId)
  if (!removed) return
  flowTransactions.commitGraphMutation((currentNodes, currentEdges) => {
    const nextNodes = currentNodes.map((node) => {
      if (node.id !== nodeId || (node.data?.kind !== 'prompt' && node.data?.kind !== 'fusion')) return node
      const snapshotNode = node.data.snapshotNode
      if (!snapshotNode || typeof snapshotNode !== 'object') return node
      return {
        ...node,
        data: {
          ...node.data,
          snapshotNode: {
            ...(snapshotNode as Record<string, unknown>),
            inputImages: currentInputImages.filter((item) => item.id !== imageId),
          },
        },
      }
    })
    const nextEdges =
      removed.sourceType === 'node'
        ? currentEdges.filter(
            (edge) =>
              !(
                edge.source === removed.sourceId &&
                edge.target === nodeId &&
                isImageInputHandle(edge.targetHandle)
              ),
          )
        : currentEdges
    return { nodes: nextNodes, edges: nextEdges }
  })
  markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
}

function onFlowImageOpenPreview(nodeId: string) {
  const snapshot = getImageNodeSnapshot(nodeId)
  if (!snapshot) return
  void openNodeImagePreview(snapshot, 'latest-output')
}

function onFlowImageContextMenu(nodeId: string, event: MouseEvent) {
  event.preventDefault()
  cancelFlowExpandMode()
  selectFlowNodeForContextMenu(nodeId)
  activeImageActionNodeId.value = nodeId
  activeImagePromptNodeId.value = nodeId
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
}

function onFlowFusionContextMenu(nodeId: string, event: MouseEvent) {
  event.preventDefault()
  cancelFlowExpandMode()
  selectFlowNodeForContextMenu(nodeId)
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = nodeId
  hideStrengthControl()
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
    activeImageActionNodeId.value = null
    activeImagePromptNodeId.value = null
    activeFusionPromptNodeId.value = null
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

const isFlowCanvasLocked = computed(
  () => isFlowInteractionActive.value || !!flowMaskEditorNodeId.value,
)
const isMaskEditorChromeHidden = computed(() => !!flowMaskEditorNodeId.value)

watch(
  () => flowMaskEditorNodeId.value,
  (nextNodeId, prevNodeId) => {
    if (prevNodeId && !nextNodeId) restoreMaskEditorReturnViewport()
  },
)

function onFlowMaskEditorDone(payload: { prompt: string; outputCount: number }) {
  const nodeId = flowMaskEditorNodeId.value
  if (!nodeId) return
  const didCommitMask = commitFlowMaskEditorMode()
  const committed = getFlowMaskEditorCommittedData(nodeId)
  if (!didCommitMask || !committed?.executionMaskDataUrl) {
    appToast.warning('请先标记局部重绘区域')
    return
  }
  activeImagePromptNodeId.value = null
  activeImageActionNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  void runImageInpaintAction(nodeId, {
    prompt: payload.prompt,
    outputCount: payload.outputCount,
    maskDataUrl: committed.executionMaskDataUrl,
  })
}

function onFlowImageDirectAction(
  nodeId: string,
  action: ImageNodeAction,
) {
  if (action === IMAGE_NODE_ACTION.imageExtend) {
    cancelFlowMaskEditorMode()
    const entered = enterFlowExpandMode(nodeId)
    activeImageActionNodeId.value = null
    activeImagePromptNodeId.value = null
    activeFusionPromptNodeId.value = null
    hideStrengthControl()
    if (!entered) appToast.warning('当前图片不可扩图')
    return
  }
  if (action === IMAGE_NODE_ACTION.imageInpaint) {
    cancelFlowExpandMode()
    const entered = enterFlowMaskEditorMode(nodeId)
    activeImageActionNodeId.value = null
    activeImagePromptNodeId.value = null
    activeFusionPromptNodeId.value = null
    hideStrengthControl()
    if (!entered) appToast.warning('当前图片不可局部重绘')
    else {
      const viewport = getViewport()
      maskEditorReturnViewport = { x: viewport.x, y: viewport.y, zoom: viewport.zoom }
      maskEditorFocusRequestId += 1
      void focusImageStageForMaskEditor(nodeId, maskEditorFocusRequestId)
    }
    return
  }
  if (isImageStrengthAction(action) && suppressClickAction.value === action) {
    return
  }
  cancelFlowExpandMode()
  cancelFlowMaskEditorMode()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  void runImageDirectAction(
    nodeId,
    action,
    isImageStrengthAction(action) ? { strength: getActionStrength(action) } : undefined,
  )
}

function onFlowImagePromptGenerate(nodeId: string, prompt: string, outputCount: number) {
  cancelFlowExpandMode()
  cancelFlowMaskEditorMode()
  activeImagePromptNodeId.value = null
  activeImageActionNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  void runImagePromptGeneration(nodeId, prompt, outputCount)
}

function onFlowImageExpandSubmit(nodeId: string, prompt: string, outputCount: number) {
  if (flowExpandNodeId.value !== nodeId) return
  activeImagePromptNodeId.value = null
  activeImageActionNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  void commitFlowExpandMode(prompt, outputCount)
}

function onFlowFusionPromptGenerate(
  nodeId: string,
  prompt: string,
  outputCount: number,
  aspectRatio: string,
) {
  cancelFlowExpandMode()
  cancelFlowMaskEditorMode()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideStrengthControl()
  onFlowNodeParamUpdate(nodeId, { key: 'prompt', value: prompt })
  onFlowNodeParamUpdate(nodeId, { key: 'outputCount', value: outputCount })
  onFlowNodeParamUpdate(nodeId, { key: 'aspectRatio', value: aspectRatio })
  void runFusionPromptGeneration(nodeId, prompt, outputCount, aspectRatio)
}

function isImageInputHandle(handle: string | null | undefined) {
  return String(handle ?? '') === 'in:image'
}

function getMediaInputImages(nodeId: string): Array<{
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string
  url: string
  ratio: number
}> {
  const mediaInputNode = (nodes.value as Node[]).find((node) => node.id === nodeId)
  const snapshot = mediaInputNode?.data?.snapshotNode as
    | { inputImages?: Array<{ id?: unknown; sourceType?: unknown; sourceId?: unknown; sourceNodeId?: unknown; url?: unknown; ratio?: unknown }> }
    | undefined
  if (!Array.isArray(snapshot?.inputImages)) return []
  const normalized: Array<{
    id: string
    sourceType: 'node' | 'external'
    sourceId: string
    sourceNodeId?: string
    url: string
    ratio: number
  }> = []
  snapshot.inputImages.forEach((item) => {
    const id = typeof item.id === 'string' ? item.id : ''
    const sourceType = item.sourceType === 'node' || item.sourceType === 'external' ? item.sourceType : null
    const sourceId = typeof item.sourceId === 'string' ? item.sourceId : ''
    const sourceNodeId = typeof item.sourceNodeId === 'string' ? item.sourceNodeId : undefined
    const url = typeof item.url === 'string' ? item.url.trim() : ''
    const ratio = Number(item.ratio)
    if (!id || !sourceType || !sourceId || !url || !Number.isFinite(ratio) || ratio <= 0) return
    normalized.push({ id, sourceType, sourceId, sourceNodeId, url, ratio })
  })
  return normalized
}

function resolvePromptInputImageNodeIds(promptNodeId: string): string[] {
  const ordered = getMediaInputImages(promptNodeId)
    .filter((item) => item.sourceType === 'node')
    .map((item) => item.sourceId)
  const linked = new Set(
    (edges.value as Edge[])
      .filter((edge) => edge.target === promptNodeId && isImageInputHandle(edge.targetHandle))
      .map((edge) => edge.source),
  )
  return ordered.length
    ? ordered.filter((sourceNodeId) => linked.has(sourceNodeId))
    : Array.from(linked)
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

.floating-toolbar {
  position: absolute;
  left: 16px;
  top: 50%;
  transform: translateY(-50%);
  background: transparent;
  padding: 0;
  border-radius: 0;
  border: none;
  z-index: 100;
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

.floating-status {
  position: absolute;
  right: 24px;
  bottom: 24px;
  z-index: 100;
  pointer-events: auto;
}
.zoom-controls {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 9px;
  height: 33px;
  padding: 4px;
  border: var(--canvas-control-border-width) solid #d8ddea;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.94);
  color: #202124;
}
.zoom-btn {
  width: 23px;
  height: 23px;
  margin: 0;
  padding: 0;
  border-radius: 999px;
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--ib-control-icon);
  display: flex;
  align-items: center;
  justify-content: center;
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
}
.zoom-btn-icon {
  width: 16px;
  height: 16px;
  stroke-width: var(--canvas-control-icon-stroke);
}
.zoom-btn:hover {
  background: #f1f3f9;
  color: var(--ib-control-icon-hover);
}
.zoom-level {
  font-size: 13px;
  font-weight: 400;
  line-height: 1.2;
  color: #202124;
  min-width: 33px;
  text-align: center;
}
</style>

<style>
@keyframes flow-edge-dashdraw {
  from {
    stroke-dashoffset: 10;
  }
}
</style>
