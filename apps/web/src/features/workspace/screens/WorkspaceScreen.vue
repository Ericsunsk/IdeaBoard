<template>
  <div
    class="canvas-layout"
    :class="[`tool-${activeTool}`, { 'interaction-lite': isFlowInteractionActive }]"
    @mousedown.self="closeBoardHeaderEditing"
  >
    <HomeCanvasHeader
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

    <aside class="floating-toolbar" @wheel.prevent.stop>
      <div
        class="tool-item"
        :class="{ active: activeTool === 'select' }"
        title="Select (V)"
        @click="setActiveTool('select')"
      >
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
        >
          <path d="M3 3l7.07 16.97 2.51-7.39 7.39-2.51L3 3z" />
          <path d="M13 13l6 6" />
        </svg>
      </div>
      <div
        class="tool-item"
        :class="{ active: activeTool === 'hand' }"
        title="Hand (H)"
        @click="setActiveTool('hand')"
      >
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
        >
          <path d="M18 11V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v0" />
          <path d="M14 10V4a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v10" />
          <path d="M10 10.5V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v8" />
          <path
            d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"
          />
        </svg>
      </div>
      <div
        class="tool-item"
        :class="{ active: activeTool === 'text' }"
        title="Text (T)"
        @click="setActiveTool('text')"
      >
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.5"
        >
          <polyline points="4 7 4 4 20 4 20 7" />
          <line x1="9" y1="20" x2="15" y2="20" />
          <line x1="12" y1="4" x2="12" y2="20" />
        </svg>
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
        :pan-on-drag="activeTool === 'hand'"
        :nodes-draggable="activeTool !== 'hand'"
        :nodes-connectable="activeTool !== 'hand'"
        :elements-selectable="activeTool !== 'hand'"
        :select-nodes-on-drag="activeTool === 'select'"
        :selection-key-code="activeTool === 'select' ? 'Shift' : false"
        :multi-selection-key-code="activeTool === 'select' ? 'Shift' : null"
        :delete-key-code="null"
        :default-edge-options="{ animated: false, updatable: true }"
        :min-zoom="0.1"
        :zoom-on-scroll="false"
        :pan-on-scroll="true"
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
            :mask-source-image-url="getFlowMaskEffectiveSourceCached(nodeProps.id).url"
            :mask-source-image-ratio="getFlowMaskEffectiveSourceCached(nodeProps.id).ratio"
            :mask-has-connected-image-input="
              getFlowMaskEffectiveSourceCached(nodeProps.id).hasConnectedImageInput
            "
            :image-action-visible="activeImageActionNodeId === nodeProps.id"
            :image-prompt-composer-visible="activeImagePromptNodeId === nodeProps.id"
            :fusion-prompt-composer-visible="activeFusionPromptNodeId === nodeProps.id"
            :image-direct-generating="isNodeDirectActionRunning(nodeProps.id)"
            :show-style-strength-control="
              flowShowStyleStrengthControl && activeImageActionNodeId === nodeProps.id
            "
            :style-innovate-strength-pct="flowStyleInnovateStrength"
            :set-style-strength-track-ref="setFlowStyleStrengthTrackRef"
            @update-prompt-text="(value) => onFlowPromptTextUpdate(nodeProps.id, value)"
            @update-node-param="(payload) => onFlowNodeParamUpdate(nodeProps.id, payload)"
            @color-apply-preset="(value) => onFlowColorApplyPreset(nodeProps.id, value)"
            @color-remove="(index) => onFlowColorRemove(nodeProps.id, index)"
            @color-add="(value) => onFlowColorAdd(nodeProps.id, value)"
            @mask-toggle-invert="() => onFlowMaskToggleInvert(nodeProps.id)"
            @delete-node="() => onFlowDeleteNode(nodeProps.id)"
            @generate-edit="() => onFlowEditGenerate(nodeProps.id)"
            @generate-enhance="() => onFlowEnhanceGenerate(nodeProps.id)"
            @generate-color="() => onFlowColorGenerate(nodeProps.id)"
            @generate-wear="() => onFlowWearGenerate(nodeProps.id)"
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
            @mask-open-file="() => onFlowMaskOpenFile(nodeProps.id)"
            @mask-clear-image="() => onFlowMaskClearImage(nodeProps.id)"
            @mask-stage-drop="(event) => onFlowMaskStageDrop(nodeProps.id, event)"
            @mask-canvas-mousedown="(event) => onFlowMaskCanvasMouseDown(nodeProps.id, event)"
            @mask-clear-mask="() => onFlowMaskClearMask(nodeProps.id)"
            @mask-set-canvas-ref="(element) => onFlowMaskSetCanvasRef(nodeProps.id, element)"
            @node-mousedown="(event) => onFlowNodeMouseDown(nodeProps.id, event)"
            @request-internals-update="() => scheduleFlowNodeInternalsRefresh([nodeProps.id])"
            @image-contextmenu="(event) => onFlowImageContextMenu(nodeProps.id, event)"
            @image-prompt-generate="(prompt, outputCount) => onFlowImagePromptGenerate(nodeProps.id, prompt, outputCount)"
            @fusion-contextmenu="(event) => onFlowFusionContextMenu(nodeProps.id, event)"
            @fusion-prompt-generate="(prompt, outputCount) => onFlowFusionPromptGenerate(nodeProps.id, prompt, outputCount)"
            @prompt-generate="(prompt, outputCount, aspectRatio) => onFlowPromptGenerate(nodeProps.id, prompt, outputCount, aspectRatio)"
            @image-action="(action) => onFlowImageDirectAction(nodeProps.id, action)"
            @style-innovate-mousedown="onFlowStyleInnovateMouseDown"
            @style-strength-track-mousedown="onFlowStyleStrengthTrackMouseDown"
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

    <div class="floating-status" @wheel.prevent.stop>
      <div class="zoom-controls">
        <button type="button" class="zoom-btn" title="缩小" @click="onZoomOut">−</button>
        <span class="zoom-level">{{ zoomLevel }}%</span>
        <button type="button" class="zoom-btn" title="放大" @click="onZoomIn">+</button>
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
import { PanOnScrollMode, useVueFlow, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowHistory } from '../composables/useFlowHistory'
import { useFlowTransactions, type FlowHistorySnapshot } from '../composables/useFlowTransactions'
import { useFlowBoardPersistence } from '../composables/useFlowBoardPersistence'
import { useFlowKeyboardShortcuts } from '../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../composables/useFlowLeaveGuard'
import { useFlowMaskDrawing } from '../composables/useFlowMaskDrawing'
import { useFlowSelectionExport } from '../composables/useFlowSelectionExport'
import {
  useFlowNodeMutations,
  type FlowCreatableNodeKind,
} from '../composables/useFlowNodeMutations'
import { useFlowGenerationHandlers } from '../composables/useFlowGenerationHandlers'
import { useFlowImageDirectActions } from '../composables/useFlowImageDirectActions'
import { useFlowStyleInnovateControl } from '../composables/useFlowStyleInnovateControl'
import { useFlowImageNodeInteractions } from '../composables/useFlowImageNodeInteractions'
import { useFlowCanvasInteractionState } from '../composables/useFlowCanvasInteractionState'
import { useFlowNodeUiState } from '../composables/useFlowNodeUiState'
import { createEmptyCanvasBoardSnapshot } from '../utils/flowSnapshotAdapter'
import type { CanvasBoardSnapshot } from '../../workspace/types/canvasNodes'
import { useFlowNodeContextMenu } from '../composables/useFlowNodeContextMenu'
import { useFlowCanvasDrop } from '../composables/useFlowCanvasDrop'
import { useFlowBoardHeaderActions } from '../composables/useFlowBoardHeaderActions'
import { useFlowSelectionController } from '../composables/useFlowSelectionController'
import { useFlowNodeMediaInputs } from '../composables/useFlowNodeMediaInputs'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
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
  setNodes,
  setEdges,
} = useVueFlow()

const zoomLevel = ref(100)
let lastFlowZoom = 100

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

const nodes = ref<Node[]>([])
const edges = ref<Edge[]>([])
const canvasWrapRef = ref<HTMLElement | null>(null)
const boardBaseSnapshot = ref<CanvasBoardSnapshot>(createEmptyCanvasBoardSnapshot())

const createNodeOptions: readonly { key: string; label: string; value: FlowCreatableNodeKind }[] = [
  { key: 'prompt', label: 'Prompt', value: 'prompt' },
  { key: 'image', label: 'Image', value: 'image' },
  { key: 'edit', label: 'Edit', value: 'edit' },
  { key: 'enhance', label: 'Enhance', value: 'enhance' },
  { key: 'color', label: 'Color', value: 'color' },
  { key: 'fusion', label: 'Fusion', value: 'fusion' },
  { key: 'wear', label: 'Wear', value: 'wear' },
  { key: 'mask', label: 'Mask', value: 'mask' },
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
  closeBoardHeaderEditing()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideFlowStyleStrengthControl()
  clearFlowSelection()
  const flowPoint = screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
  const rect = canvasWrapRef.value?.getBoundingClientRect()
  const x = rect ? event.clientX - rect.left : event.clientX
  const y = rect ? event.clientY - rect.top : event.clientY
  openNodeContextMenu(x, y, flowPoint.x, flowPoint.y)
}

function handleNodeContextMenuCreate(kind: FlowCreatableNodeKind) {
  createNodeAtPosition(kind, nodeContextMenu.value.flowX, nodeContextMenu.value.flowY)
  closeNodeContextMenu()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideFlowStyleStrengthControl()
}

function setActiveTool(tool: 'select' | 'hand' | 'text') {
  closeBoardHeaderEditing()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideFlowStyleStrengthControl()
  activeTool.value = tool
  if (tool === 'text') {
    appToast.info('文本工具稍后接入')
  }
}

function onEscapeShortcut() {
  if (isBoardMenuOpen.value || isRenamingBoard.value) {
    closeBoardHeaderEditing()
    return true
  }
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideFlowStyleStrengthControl()
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
  onNodeDeleted: (nodeId, kind) => {
    if (kind === 'mask') {
      removeMaskRuntimeForNode(nodeId)
    }
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
  hideFlowStyleStrengthControl()
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
const _onFlowColorApplyPreset = flowNodeMutations.onFlowColorApplyPreset
const _onFlowColorRemove = flowNodeMutations.onFlowColorRemove
const _onFlowColorAdd = flowNodeMutations.onFlowColorAdd
const _onFlowMaskToggleInvert = flowNodeMutations.onFlowMaskToggleInvert
const markFlowExecutionNodeDirty = flowNodeMutations.markFlowExecutionNodeDirty

function onFlowColorApplyPreset(nodeId: string, value: string[]) {
  _onFlowColorApplyPreset(nodeId, value)
  markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
}

function onFlowColorRemove(nodeId: string, index: number) {
  _onFlowColorRemove(nodeId, index)
  markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
}

function onFlowColorAdd(nodeId: string, value: string) {
  _onFlowColorAdd(nodeId, value)
  markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
}

function onFlowMaskToggleInvert(nodeId: string) {
  _onFlowMaskToggleInvert(nodeId)
  markFlowExecutionNodeDirty(nodeId)
}

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
let clearMaskCanvasRef: (nodeId: string) => void = () => {}

const {
  getImageNodeSnapshot,
  getImageNodeDisplayPayload,
  getMaskNodeSnapshot,
  getFlowMaskEffectiveSourceCached,
  onFlowImageOpenFile,
  onFlowImageReset,
  onFlowImageStageDrop,
  onFlowMaskOpenFile,
  onFlowMaskClearImage,
  onFlowMaskClearMask,
  onFlowMaskStageDrop,
} = useFlowNodeMediaInputs({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  canvasDrop: {
    applyImageFileToImageNode: async (nodeId, file) => {
      if (!canvasDropRef) return
      await canvasDropRef.applyImageFileToImageNode(nodeId, file)
    },
    applyImageFileToMaskNode: async (nodeId, file) => {
      if (!canvasDropRef) return
      await canvasDropRef.applyImageFileToMaskNode(nodeId, file)
    },
  },
  updateSnapshotNode,
  markFlowExecutionNodeDirty,
  clearMaskCanvas: (nodeId) => {
    clearMaskCanvasRef(nodeId)
  },
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
  hideFlowStyleStrengthControl()
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
  selectFlowNodeForContextMenu(nodeId)
  activeImageActionNodeId.value = nodeId
  activeImagePromptNodeId.value = nodeId
  activeFusionPromptNodeId.value = null
  hideFlowStyleStrengthControl()
}

function onFlowFusionContextMenu(nodeId: string, event: MouseEvent) {
  event.preventDefault()
  selectFlowNodeForContextMenu(nodeId)
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = nodeId
  hideFlowStyleStrengthControl()
}

const {
  onFlowMaskSetCanvasRef,
  renderMaskNodeMask,
  onFlowMaskCanvasMouseDown,
  clearMaskCanvas,
  removeMaskRuntimeForNode,
} = useFlowMaskDrawing({
  getMaskNodeSnapshot,
  updateSnapshotNode,
  startCanvasInteraction: startFlowInteraction,
  stopCanvasInteraction: stopFlowInteraction,
})
clearMaskCanvasRef = clearMaskCanvas

const {
  onFlowEnhanceGenerate,
  onFlowEditGenerate,
  onFlowColorGenerate,
  onFlowFusionGenerate,
  onFlowWearGenerate,
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
  runImagePromptGeneration,
  runImagePromptGenerationFromSource,
  runTextPromptGeneration,
} = useFlowImageDirectActions({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  commitGraphMutation: flowTransactions.commitGraphMutation,
  commitNodeMutation: flowTransactions.commitNodeMutation,
  commitEdgeMutation: flowTransactions.commitEdgeMutation,
  commitDirty: flowTransactions.commitDirty,
  refreshNodeInternals: scheduleFlowNodeInternalsRefresh,
  onSuccess: (text) => {
    appToast.success(text)
  },
  onWarning: (text) => {
    appToast.warning(text)
  },
})

const {
  flowStyleInnovateStrength,
  flowShowStyleStrengthControl,
  flowSuppressStyleInnovateClick,
  setFlowStyleStrengthTrackRef,
  hideFlowStyleStrengthControl,
  onFlowStyleInnovateMouseDown,
  onFlowStyleStrengthTrackMouseDown,
} = useFlowStyleInnovateControl({
  onCommit: (strength) => {
    const targetNodeId = activeImageActionNodeId.value
    if (!targetNodeId) return
    activeImageActionNodeId.value = null
    activeImagePromptNodeId.value = null
    activeFusionPromptNodeId.value = null
    void runImageDirectAction(targetNodeId, 'style-innovate', { strength })
  },
})

function onFlowImageDirectAction(
  nodeId: string,
  action: 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design',
) {
  if (action === 'style-innovate' && flowSuppressStyleInnovateClick.value) {
    return
  }
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideFlowStyleStrengthControl()
  void runImageDirectAction(
    nodeId,
    action,
    action === 'style-innovate' ? { strength: flowStyleInnovateStrength.value } : undefined,
  )
}

function onFlowImagePromptGenerate(nodeId: string, prompt: string, outputCount: number) {
  activeImagePromptNodeId.value = null
  activeImageActionNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideFlowStyleStrengthControl()
  void runImagePromptGeneration(nodeId, prompt, outputCount)
}

function onFlowFusionPromptGenerate(nodeId: string, prompt: string, outputCount: number) {
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  activeFusionPromptNodeId.value = null
  hideFlowStyleStrengthControl()
  onFlowNodeParamUpdate(nodeId, { key: 'prompt', value: prompt })
  onFlowNodeParamUpdate(nodeId, { key: 'outputCount', value: outputCount })
  void onFlowFusionGenerate(nodeId)
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
  hideFlowStyleStrengthControl,
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
  hideFlowStyleStrengthControl,
})

const canvasDrop = useFlowCanvasDrop({
  currentBoardId,
  screenToFlowCoordinate,
  createNodeAtPosition,
  updateSnapshotNode,
  getImageNodeSnapshot,
  getMaskNodeSnapshot,
  uploadApi,
  sanitizeApiErrorMessage,
  renderMaskNodeMask,
  clearMaskCanvas,
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
.canvas-layout {
  --flow-edge-color: #b1b1b7;
  --flow-edge-active-color: #9278e6;
  --flow-edge-selected-color: #7b8798;
  --flow-edge-width: 2;
  --flow-edge-dasharray: 5;
  --flow-edge-running-duration: 0.5s;
  width: 100%;
  height: 100vh;
  background: #ffffff;
  position: relative;
  overflow: hidden;
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
  background: rgba(255, 255, 255, 0.94);
  padding: 4px;
  border-radius: 999px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid #d8ddea;
  z-index: 100;
}
.tool-item {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--ib-control-icon);
  cursor: pointer;
  border: none;
  background: transparent;
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
}
.tool-item:hover {
  background: var(--ib-control-bg-hover);
  color: var(--ib-control-icon-hover);
}
.tool-item.active {
  background: #f1f3f9;
  color: #9278e6;
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
  border: 1px solid #d8ddea;
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
  font-size: 18px;
  line-height: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
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
