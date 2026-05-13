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
      :board-name-draft="boardNameDraft"
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
    />

    <aside class="floating-toolbar">
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
        :default-edge-options="{ animated: false, updatable: true }"
        :min-zoom="0.1"
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
            @generate-fusion="() => onFlowFusionGenerate(nodeProps.id)"
            @generate-wear="() => onFlowWearGenerate(nodeProps.id)"
            @generate-model="() => onFlowModelGenerate(nodeProps.id)"
            @generate-viewpoint="() => onFlowViewpointGenerate(nodeProps.id)"
            @image-stage-drop="(event) => onFlowImageStageDrop(nodeProps.id, event)"
            @image-open-file="() => onFlowImageOpenFile(nodeProps.id)"
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
            @image-prompt-generate="(prompt) => onFlowImagePromptGenerate(nodeProps.id, prompt)"
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

    <div class="floating-status">
      <div class="zoom-controls">
        <button type="button" class="zoom-btn" title="缩小" @click="onZoomOut">−</button>
        <span class="zoom-level">{{ zoomLevel }}%</span>
        <button type="button" class="zoom-btn" title="放大" @click="onZoomIn">+</button>
      </div>
      <button
        type="button"
        class="logo-btn"
        title="返回画布列表"
        @click="onNavigateRequest('/user/boards')"
      >
        <svg
          class="logo-btn-icon"
          width="18"
          height="18"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M9 7 5 11l4 4" />
          <path d="M5 11h8.5c3.2 0 5.5 2.3 5.5 5.4 0 1.1-.28 2.1-.78 2.9" />
        </svg>
      </button>
    </div>

    <div
      v-if="leaveConfirmVisible"
      class="leave-confirm-mask"
      @mousedown.stop.prevent
      @click.self="resolveLeaveConfirm('cancel')"
    >
      <div class="leave-confirm-card" @mousedown.stop @click.stop>
        <div class="leave-confirm-title">离开前是否保存？</div>
        <div class="leave-confirm-desc">当前画布有未保存更改。</div>
        <div class="leave-confirm-actions">
          <button type="button" class="leave-btn ghost" @click="resolveLeaveConfirm('cancel')">
            继续编辑
          </button>
          <button type="button" class="leave-btn danger" @click="resolveLeaveConfirm('discard')">
            不保存
          </button>
          <button type="button" class="leave-btn primary" @click="resolveLeaveConfirm('save')">
            保存并离开
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Ref } from 'vue'
import { useUserStore } from '../../../app/stores/user'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { uploadApi } from '../../../shared/api/upload'
import type { Edge, Node } from '@vue-flow/core'
import { useVueFlow, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowHistory } from '../composables/useFlowHistory'
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

const message = useMessage()
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
  removeSelectedElements,
  addSelectedNodes,
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
const leaveConfirmVisible = ref(false)
const activeTool = ref<'select' | 'hand' | 'text'>('select')

const createNodeKind = ref<FlowCreatableNodeKind>('prompt')

const FLOW_ENABLE_VISIBLE_ELEMENTS_CULLING =
  import.meta.env.VITE_WORKSPACE_FLOW_ENABLE_CULLING === 'true'

let pendingNodeInternalsRefresh = false
let queuedNodeInternalsIds: Set<string> | null = null
let leaveConfirmResolver: ((choice: 'save' | 'discard' | 'cancel') => void) | null = null
let leaveConfirmPromise: Promise<'save' | 'discard' | 'cancel'> | null = null
let skipNextLeaveConfirm = false
const activeImageActionNodeId = ref<string | null>(null)
const activeImagePromptNodeId = ref<string | null>(null)
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

function clearSelectedFlowElements(elements?: Parameters<typeof removeSelectedElements>[0]) {
  removeSelectedElements(elements)
}

function selectFlowNodes(nextNodes: Node[]) {
  addSelectedNodes(nextNodes as Parameters<typeof addSelectedNodes>[0])
}

function onPaneContextMenu(event: MouseEvent) {
  event.preventDefault()
  closeBoardHeaderEditing()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
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
  hideFlowStyleStrengthControl()
}

function requestLeaveConfirm(): Promise<'save' | 'discard' | 'cancel'> {
  if (leaveConfirmPromise) return leaveConfirmPromise
  leaveConfirmVisible.value = true
  leaveConfirmPromise = new Promise((resolve) => {
    leaveConfirmResolver = resolve
  })
  return leaveConfirmPromise
}

function resolveLeaveConfirm(choice: 'save' | 'discard' | 'cancel') {
  if (!leaveConfirmPromise) return
  leaveConfirmVisible.value = false
  const resolver = leaveConfirmResolver
  leaveConfirmResolver = null
  leaveConfirmPromise = null
  resolver?.(choice)
}

async function confirmFlowLeave(): Promise<boolean> {
  if (skipNextLeaveConfirm) {
    skipNextLeaveConfirm = false
    return true
  }
  if (!isDirty.value || isSavingBoard.value) return true
  const choice = await requestLeaveConfirm()
  if (choice === 'discard') return true
  if (choice === 'cancel') return false
  return !!(await saveCurrentBoardSnapshot())
}

function setActiveTool(tool: 'select' | 'hand' | 'text') {
  closeBoardHeaderEditing()
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  hideFlowStyleStrengthControl()
  activeTool.value = tool
  if (tool === 'text') {
    message.info('文本工具稍后接入')
  }
}

function onEscapeShortcut() {
  if (isBoardMenuOpen.value || isRenamingBoard.value) {
    closeBoardHeaderEditing()
    return true
  }
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
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
  if (isDirty.value && !(await confirmFlowLeave())) return
  skipNextLeaveConfirm = true
  await router.push(path)
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

type FlowHistorySnapshot = ReturnType<typeof flowHistory.capture>

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
    message.warning(msg)
  },
  onSuccess: (msg) => {
    message.success(msg)
  },
  onError: (msg) => {
    message.error(msg)
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
    warning: (msg: string) => message.warning(msg),
    success: (msg: string) => message.success(msg),
  },
  router,
  route,
  isDirty,
  confirmFlowLeave,
  skipNextLeaveConfirm: {
    get value() {
      return skipNextLeaveConfirm
    },
    set value(v: boolean) {
      skipNextLeaveConfirm = v
    },
  },
})

function commitFlowNodes(nextNodes: Node[]) {
  setNodes(nextNodes)
}

function commitFlowEdges(nextEdges: Edge[]) {
  setEdges(nextEdges)
}

function commitFlowEdgesAppend(nextEdges: Edge[]) {
  setEdges((currentEdges) => [...(currentEdges as Edge[]), ...nextEdges])
}

function commitFlowNodesAppend(nextNodes: Node[]) {
  setNodes((currentNodes) => [...(currentNodes as Node[]), ...nextNodes])
  scheduleFlowNodeInternalsRefresh(nextNodes.map((node) => node.id))
}

const flowNodeMutations = useFlowNodeMutations({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  createNodeKind,
  boardBaseSnapshot,
  captureHistory: flowHistory.capture,
  pushHistory: flowHistory.push,
  markDirty,
  refreshSnapshot,
  appendFlowNodes: (nextNodes) => {
    commitFlowNodesAppend(nextNodes)
  },
  commitNodes: (nextNodes) => {
    commitFlowNodes(nextNodes)
  },
  commitEdges: (nextEdges) => {
    commitFlowEdges(nextEdges)
  },
  onWarning: (text) => {
    message.warning(text)
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

const selectionExport = useFlowSelectionExport({ nodes, message })
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

function onCanvasDragOver(event: DragEvent) {
  activeImageActionNodeId.value = null
  activeImagePromptNodeId.value = null
  hideFlowStyleStrengthControl()
  canvasDrop.onCanvasDragOver(event)
}

async function onCanvasDrop(event: DragEvent) {
  await canvasDrop.onCanvasDrop(event)
}

function onFlowImageContextMenu(nodeId: string, event: MouseEvent) {
  event.preventDefault()
  selectFlowNodeForContextMenu(nodeId)
  activeImageActionNodeId.value = nodeId
  activeImagePromptNodeId.value = nodeId
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
  captureHistory: flowHistory.capture as () => FlowHistorySnapshot,
  onWarning: (text) => {
    message.warning(text)
  },
  onSuccess: (text) => {
    message.success(text)
  },
  onError: (text) => {
    message.error(text)
  },
  onOutputCommitted: (nodeId) => {
    markFlowExecutionNodeDirty(nodeId)
  },
})

const {
  isNodeDirectActionRunning,
  runImageDirectAction,
  runImagePromptGeneration,
} = useFlowImageDirectActions({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  captureHistory: flowHistory.capture as () => FlowHistorySnapshot,
  pushHistory: flowHistory.push,
  markDirty,
  refreshSnapshot,
  refreshNodeInternals: scheduleFlowNodeInternalsRefresh,
  onSuccess: (text) => {
    message.success(text)
  },
  onWarning: (text) => {
    message.warning(text)
  },
  commitNodes: (nextNodes) => {
    commitFlowNodes(nextNodes)
  },
  commitEdges: (nextEdges) => {
    commitFlowEdges(nextEdges)
  },
  appendEdges: commitFlowEdgesAppend,
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
  hideFlowStyleStrengthControl()
  void runImageDirectAction(
    nodeId,
    action,
    action === 'style-innovate' ? { strength: flowStyleInnovateStrength.value } : undefined,
  )
}

function onFlowImagePromptGenerate(nodeId: string, prompt: string) {
  activeImagePromptNodeId.value = null
  activeImageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  void runImagePromptGeneration(nodeId, prompt)
}

const {
  clearFlowSelection,
  selectFlowNodeForContextMenu,
  onPaneClick,
  onFlowNodeClick,
  onFlowNodeMouseDown,
  deleteSelectedFlowElements,
} = useFlowSelectionController({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  activeTool: activeTool as Ref<string>,
  activeImageActionNodeId,
  activeImagePromptNodeId,
  hideFlowStyleStrengthControl,
  closeBoardHeaderEditing,
  closeNodeContextMenu,
  _deleteSelectedFlowElements: () => {
    flowNodeMutations.deleteSelectedFlowElements()
  },
  clearSelectedElements: clearSelectedFlowElements,
  selectFlowNodes,
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
    }
  },
  captureFlowHistorySnapshot: flowHistory.capture as () => FlowHistorySnapshot,
  pushFlowHistorySnapshot: flowHistory.push,
  markDirty,
  refreshSnapshot,
  appendFlowNodes: (nextNodes) => {
    commitFlowNodesAppend(nextNodes)
  },
})

const { onNodeDragStart, onNodeDragStop } = useFlowNodeDragInteractions({
  flowHistory: {
    onNodeDragStart: flowHistory.onNodeDragStart,
    onNodeDragStop: flowHistory.onNodeDragStop,
  },
  duplicateModifierPressed,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
  markDirty,
  activeImageActionNodeId,
  activeImagePromptNodeId,
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
  onWarning: (msg) => message.warning(msg),
  onSuccess: (msg) => message.success(msg),
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
    void saveCurrentBoardSnapshot()
  },
  onUndo: flowHistory.undo,
  onRedo: flowHistory.redo,
  onDeleteSelected: deleteSelectedFlowElements,
  isLeaveConfirmVisible: () => leaveConfirmVisible.value,
  onCancelLeaveConfirm: () => {
    resolveLeaveConfirm('cancel')
  },
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
  isDirty,
  isSaving: isSavingBoard,
  confirmLeave: confirmFlowLeave,
})

onMounted(async () => {
  await loadInitialOrRouteBoard()
  refreshBoardMetas()
  refreshSnapshot()
})
</script>

<style scoped>
.canvas-layout {
  width: 100%;
  height: 100vh;
  background: #ffffff;
  position: relative;
  overflow: hidden;
}

.canvas-layout.interaction-lite :deep(.workflow-node),
.canvas-layout.interaction-lite :deep(.vue-flow__edge-path) {
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
  background: #fff;
  padding: 8px;
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid #e2e4e8;
  z-index: 100;
}
.tool-item {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #5f6368;
  cursor: pointer;
  border: none;
  background: transparent;
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
  stroke-width: 2;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge.selected .vue-flow__edge-path),
.workspace-flow-canvas-wrap :deep(.vue-flow__edge:focus .vue-flow__edge-path),
.workspace-flow-canvas-wrap :deep(.vue-flow__edge:focus-visible .vue-flow__edge-path) {
  stroke: #7b8798;
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
  display: flex;
  gap: 10px;
  align-items: center;
}
.zoom-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  background: #f1f3f9;
  border-radius: 12px;
  padding: 6px 8px;
  color: #202124;
}
.zoom-btn {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  border: none;
  background: transparent;
  cursor: pointer;
  color: #5f6368;
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
  background: #fff;
  color: #6757c8;
}
.zoom-level {
  font-size: 14px;
  font-weight: 500;
  line-height: 1.2;
  color: #202124;
  min-width: 44px;
  text-align: center;
}
.logo-btn {
  width: 40px;
  height: 40px;
  border-radius: 999px;
  border: 1px solid #e8eaed;
  background: rgba(255, 255, 255, 0.95);
  color: #5f6368;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition:
    background-color 0.2s ease,
    color 0.2s ease,
    transform 0.2s ease;
}
.logo-btn:hover {
  background: #fff;
  border-color: #d8ddea;
  color: #6757c8;
  transform: rotate(-90deg);
}
.logo-btn:active {
  transform: rotate(-84deg) scale(0.98);
}
.logo-btn:focus-visible {
  outline: 2px solid rgba(146, 120, 230, 0.36);
  outline-offset: 2px;
}
.logo-btn-icon {
  transition: transform 0.2s ease;
}

.leave-confirm-mask {
  position: absolute;
  inset: 0;
  z-index: 500;
  background: rgba(15, 23, 42, 0.16);
  display: flex;
  align-items: center;
  justify-content: center;
}

.leave-confirm-card {
  width: min(90vw, 340px);
  background: #fff;
  border-radius: 14px;
  border: 1px solid #e2e8f0;
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.18);
  padding: 18px 18px 14px;
}

.leave-confirm-title {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
}

.leave-confirm-desc {
  margin-top: 8px;
  font-size: 13px;
  color: #475569;
}

.leave-confirm-actions {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.leave-btn {
  border: 1px solid #dbe3ef;
  border-radius: 10px;
  padding: 7px 12px;
  font-size: 13px;
  cursor: pointer;
  background: #fff;
  color: #334155;
}

.leave-btn.ghost {
  background: #fff;
}

.leave-btn.danger {
  border-color: #fecaca;
  color: #b91c1c;
  background: #fff5f5;
}

.leave-btn.primary {
  border-color: #9278e6;
  background: #9278e6;
  color: #fff;
}
</style>
