<template>
  <div class="canvas-layout" :class="[`tool-${activeTool}`, { 'interaction-lite': isFlowInteractionActive }]" @mousedown.self="closeBoardHeaderEditing">


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
      <div class="tool-item" :class="{ active: activeTool === 'select' }" title="Select (V)" @click="setActiveTool('select')">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 3l7.07 16.97 2.51-7.39 7.39-2.51L3 3z"/><path d="M13 13l6 6"/></svg>
      </div>
      <div class="tool-item" :class="{ active: activeTool === 'hand' }" title="Hand (H)" @click="setActiveTool('hand')">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 11V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v0"/><path d="M14 10V4a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v10"/><path d="M10 10.5V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v8"/><path d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/></svg>
      </div>
      <div class="tool-item" :class="{ active: activeTool === 'text' }" title="Text (T)" @click="setActiveTool('text')">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="4 7 4 4 20 4 20 7"/><line x1="9" y1="20" x2="15" y2="20"/><line x1="12" y1="4" x2="12" y2="20"/></svg>
      </div>
      <div class="toolbar-divider"></div>
      <div class="tool-item" title="图·文生图" @click="onNavigateRequest('/user/picture')">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M3 14l4-4 5 5 5-5 4 4"/></svg>
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
        :only-render-visible-elements="FLOW_ENABLE_VISIBLE_ELEMENTS_CULLING"
        :is-valid-connection="isValidConnection"
        :connection-line-style="flowConnectionLineStyle"
        @connect="onConnect"
        @connect-start="onFlowConnectStart"
        @connect-end="onFlowConnectEnd"
        @edge-update="onEdgeUpdate"
        @edge-update-start="onFlowEdgeUpdateStart"
        @edge-update-end="onFlowEdgeUpdateEnd"
        @move-start="closeBoardHeaderEditing"
        @move="onFlowMove"
        @node-click="onFlowNodeClick"
        @node-drag-start="onNodeDragStart"
        @node-drag-stop="onNodeDragStop"
        @nodes-initialized="onFlowNodesInitialized"
        @pane-click="onPaneClick"
        @pane-context-menu="onPaneContextMenu"
      >
        <template #node-canvas="nodeProps">
          <FlowCanvasNode
            :data="nodeProps.data"
            :selected="!!nodeProps.selected"
            :dragging="isDraggingNode && !!nodeProps.dragging"
            :copy-mode="isAltPressed"
            :ui-state="getFlowNodeUiStateCached(nodeProps.id)"
            :mask-source-image-url="getFlowMaskEffectiveSourceCached(nodeProps.id).url"
            :mask-source-image-ratio="getFlowMaskEffectiveSourceCached(nodeProps.id).ratio"
            :mask-has-connected-image-input="getFlowMaskEffectiveSourceCached(nodeProps.id).hasConnectedImageInput"
            :message-action-visible="activeMessageActionNodeId === nodeProps.id"
            :message-direct-generating="isNodeDirectActionRunning(nodeProps.id)"
            :show-style-strength-control="flowShowStyleStrengthControl && activeMessageActionNodeId === nodeProps.id"
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
            @image-display-dragstart="(event) => onFlowImageDisplayDragStart(nodeProps.id, event)"
            @image-reset="() => onFlowImageReset(nodeProps.id)"
            @message-image-dragstart="(event) => onFlowMessageImageDragStart(nodeProps.id, event)"
            @message-resize-mousedown="(payload) => onFlowMessageResizeMouseDown(nodeProps.id, payload)"
            @mask-open-file="() => onFlowMaskOpenFile(nodeProps.id)"
            @mask-clear-image="() => onFlowMaskClearImage(nodeProps.id)"
            @mask-stage-drop="(event) => onFlowMaskStageDrop(nodeProps.id, event)"
            @mask-canvas-mousedown="(event) => onFlowMaskCanvasMouseDown(nodeProps.id, event)"
            @mask-clear-mask="() => onFlowMaskClearMask(nodeProps.id)"
            @mask-set-canvas-ref="(element) => onFlowMaskSetCanvasRef(nodeProps.id, element)"
            @node-mousedown="(event) => onFlowNodeMouseDown(nodeProps.id, event)"
            @request-internals-update="() => scheduleFlowNodeInternalsRefresh([nodeProps.id])"
            @message-contextmenu="(event) => onFlowMessageContextMenu(nodeProps.id, event)"
            @message-action="(action) => onFlowMessageDirectAction(nodeProps.id, action)"
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

    <HomeBottomGenerateBar
      class="workspace-flow-bottom-bar"
      :show-settings="flowPromptShowSettings"
      :ratio-options="flowPromptRatioOptions"
      :quality-options="flowPromptQualityOptions"
      :selected-ratio="flowPromptRatio"
      :selected-quality="flowPromptQuality"
      :selected-preview-items="selectedPreviewItems"
      :user-prompt="flowPromptText"
      :prompt-placeholder="flowPromptPlaceholder"
      :has-direct-prompt="flowPromptText.trim().length > 0"
      :direct-generating="isDirectGenerating"
      @toggle-settings="flowPromptShowSettings = !flowPromptShowSettings"
      @select-ratio="flowPromptRatio = $event"
      @select-quality="flowPromptQuality = $event"
      @update:user-prompt="flowPromptText = $event"
      @generate="onFlowGenerate"
      @clear-selected="onClearSelected"
      @remove-selected="onRemoveSelectedPreviewItem"
      @focus-selected="onFocusSelectedPreviewItem"
    />

    <div class="floating-status">
      <div class="zoom-controls">
        <button class="zoom-btn" @click="onZoomOut">−</button>
        <span class="zoom-level">{{ zoomLevel }}%</span>
        <button class="zoom-btn" @click="onZoomIn">+</button>
      </div>
      <button class="logo-btn" title="返回画布列表" @click="onNavigateRequest('/user/boards')">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="16" y1="12" x2="6" y2="12" />
          <polyline points="10 8 6 12 10 16" />
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
          <button type="button" class="leave-btn ghost" @click="resolveLeaveConfirm('cancel')">继续编辑</button>
          <button type="button" class="leave-btn danger" @click="resolveLeaveConfirm('discard')">不保存</button>
          <button type="button" class="leave-btn primary" @click="resolveLeaveConfirm('save')">保存并离开</button>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useUserStore } from '../../../app/stores/user'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { uploadApi } from '../../../shared/api'
import type { Edge, Node } from '@vue-flow/core'
import { useVueFlow, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import FlowCanvasNode from '../components/FlowCanvasNode.vue'
import WorkspaceNodeContextMenu from '../components/WorkspaceNodeContextMenu.vue'
import { useFlowHistory } from '../composables/useFlowHistory'
import { useFlowBoardPersistence } from '../composables/useFlowBoardPersistence'
import { useFlowKeyboardShortcuts } from '../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../composables/useFlowLeaveGuard'
import { useFlowMaskDrawing } from '../composables/useFlowMaskDrawing'
import { useFlowSelectionExport } from '../composables/useFlowSelectionExport'
import { useFlowNodeMutations, type FlowCreatableNodeKind } from '../composables/useFlowNodeMutations'
import { useFlowGenerationHandlers } from '../composables/useFlowGenerationHandlers'
import { useFlowMessageDirectActions } from '../composables/useFlowMessageDirectActions'
import { useFlowStyleInnovateControl } from '../composables/useFlowStyleInnovateControl'
import { useFlowMessageImageInteractions } from '../composables/useFlowMessageImageInteractions'
import { useFlowCanvasInteractionState } from '../composables/useFlowCanvasInteractionState'
import { useFlowBoardViewport } from '../composables/useFlowBoardViewport'
import { useFlowNodeUiState } from '../composables/useFlowNodeUiState'
import type { FlowNodeData } from '../types/flowTypes'
import { createEmptyCanvasBoardSnapshot } from '../utils/flowSnapshotAdapter'
import type { CanvasBoardSnapshot } from '../../workspace/types/canvasNodes'
import HomeCanvasHeader from '../../workspace/components/home/HomeCanvasHeader.vue'
import HomeBottomGenerateBar from '../../workspace/components/home/HomeBottomGenerateBar.vue'
import { useRouter } from 'vue-router'
import {
  GENERATE_QUALITY_OPTIONS,
  PATTERN_GENERATE_RATIO_OPTIONS,
  type GenerateQualityOption,
  type PatternGenerateRatioOption,
} from '../../workspace/components/nodes/nodeOptionConfig'
import { useFlowNodeContextMenu } from '../composables/useFlowNodeContextMenu'
import { useFlowSelectionPreviewBar } from '../composables/useFlowSelectionPreviewBar'
import { useFlowCanvasDrop } from '../composables/useFlowCanvasDrop'
import { useFlowBoardHeaderActions } from '../composables/useFlowBoardHeaderActions'
import { useFlowConnectionVisuals } from '../composables/useFlowConnectionVisuals'
import { useFlowSelectionController } from '../composables/useFlowSelectionController'
import { useFlowNodeMediaInputs } from '../composables/useFlowNodeMediaInputs'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'

const message = useMessage()
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { screenToFlowCoordinate, updateNodeInternals, setNodes, setEdges } = useVueFlow()
const {
  zoomLevel,
  onZoomIn,
  onZoomOut,
  getViewport,
  setViewport,
  applyFlowTransform,
} = useFlowBoardViewport()

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
const flowPromptShowSettings = ref(false)
const flowPromptText = ref('')
const flowPromptRatio = ref<PatternGenerateRatioOption>('1:1')
const flowPromptQuality = ref<GenerateQualityOption>('1k')

const createNodeKind = ref<FlowCreatableNodeKind>('prompt')

const FLOW_ENABLE_VISIBLE_ELEMENTS_CULLING = import.meta.env.VITE_WORKSPACE_FLOW_ENABLE_CULLING === 'true'

const FLOW_NODE_IMAGE_DRAG_MIME = 'application/x-canvas-node-image'
let pendingNodeInternalsRefresh = false
let queuedNodeInternalsIds: Set<string> | null = null
let leaveConfirmResolver: ((choice: 'save' | 'discard' | 'cancel') => void) | null = null
let leaveConfirmPromise: Promise<'save' | 'discard' | 'cancel'> | null = null
let skipNextLeaveConfirm = false
const activeMessageActionNodeId = ref<string | null>(null)

function scheduleFlowNodeInternalsRefresh(nodeIds?: string[]) {
  const hasCanvasNodes = (nodes.value as Array<{ type?: string }>).some(node => node.type === 'canvas')
  if (!hasCanvasNodes) return
  if (nodeIds && nodeIds.length > 0) {
    if (!queuedNodeInternalsIds) queuedNodeInternalsIds = new Set()
    nodeIds.forEach(id => queuedNodeInternalsIds?.add(id))
  }
  if (pendingNodeInternalsRefresh) return
  pendingNodeInternalsRefresh = true
  void nextTick(() => {
    const ids = (queuedNodeInternalsIds && queuedNodeInternalsIds.size > 0)
      ? Array.from(queuedNodeInternalsIds)
      : nodes.value.map(node => node.id)
    queuedNodeInternalsIds = null
    if (ids.length === 0) {
      pendingNodeInternalsRefresh = false
      return
    }
    requestAnimationFrame(() => {
      updateNodeInternals(ids)
      requestAnimationFrame(() => {
        updateNodeInternals(ids)
        pendingNodeInternalsRefresh = false
      })
    })
  })
}

function onFlowNodesInitialized() {
  scheduleFlowNodeInternalsRefresh()
}

const flowPromptRatioOptions = PATTERN_GENERATE_RATIO_OPTIONS
const flowPromptQualityOptions = GENERATE_QUALITY_OPTIONS
const flowPromptPlaceholder = computed(() => {
  return activeTool.value === 'text'
    ? '文本工具稍后接入，请在节点内执行生成'
    : '请在节点内执行生成'
})
function onPaneContextMenu(event: MouseEvent) {
  event.preventDefault()
  closeBoardHeaderEditing()
  activeMessageActionNodeId.value = null
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
  activeMessageActionNodeId.value = null
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
  activeMessageActionNodeId.value = null
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
  activeMessageActionNodeId.value = null
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
  const zoomChanged = applyFlowTransform(transform)
  if (zoomChanged) {
    scheduleFlowNodeInternalsRefresh()
  }
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
    syncFlowAfterGraphRestore()
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
  onGraphCommitted: syncFlowAfterGraphRestore,
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
  message: { warning: (msg: string) => message.warning(msg), success: (msg: string) => message.success(msg) },
  router,
  route,
  isDirty,
  confirmFlowLeave,
  skipNextLeaveConfirm: { get value() { return skipNextLeaveConfirm }, set value(v: boolean) { skipNextLeaveConfirm = v } },
})

function commitFlowNodes(nextNodes: Node[]) {
  nodes.value = nextNodes
  setNodes(nextNodes)
}

function commitFlowEdges(nextEdges: Edge[]) {
  edges.value = nextEdges
  setEdges(nextEdges)
}

function commitFlowNodesAppend(nextNodes: Node[]) {
  const combined = [...(nodes.value as Node[]), ...nextNodes]
  nodes.value = combined
  setNodes(combined)
  void nextTick(() => {
    requestAnimationFrame(() => {
      updateNodeInternals(nextNodes.map(n => n.id))
      requestAnimationFrame(() => {
        updateNodeInternals(nextNodes.map(n => n.id))
      })
    })
  })
}

function syncFlowAfterGraphRestore() {
  scheduleFlowNodeInternalsRefresh()
  void nextTick(() => {
    ensureFlowEdgeVisuals()
  })
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
  activeMessageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  closeNodeContextMenu()
  _onFlowDeleteNode(nodeId)
  void nextTick(() => {
    ensureFlowEdgeVisuals()
  })
}
const updateSnapshotNodeBase = flowNodeMutations.updateSnapshotNode
const _deleteSelectedFlowElements = flowNodeMutations.deleteSelectedFlowElements
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

const {
  nodeContextMenu,
  canvasNodeMenuItems,
  closeNodeContextMenu,
  openNodeContextMenu,
} = useFlowNodeContextMenu()

const {
  flowConnectionLineStyle,
  onFlowConnectStart,
  onFlowConnectEnd,
  onFlowEdgeUpdateStart,
  onFlowEdgeUpdateEnd,
  ensureFlowEdgeVisuals,
} = useFlowConnectionVisuals({
  edges: edges as Ref<Edge[]>,
  commitEdges: (nextEdges) => {
    commitFlowEdges(nextEdges)
  },
  startFlowInteraction: startFlowInteraction,
  stopFlowInteraction: stopFlowInteraction,
})

let canvasDropRef: ReturnType<typeof useFlowCanvasDrop> | null = null
let clearMaskCanvasRef: ((nodeId: string) => void) = () => {}

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
    readNodeImageDropPayload: (event) => canvasDropRef?.readNodeImageDropPayload(event) ?? null,
    applyImageFileToImageNode: async (nodeId, file) => {
      if (!canvasDropRef) return
      await canvasDropRef.applyImageFileToImageNode(nodeId, file)
    },
    applyImageFileToMaskNode: async (nodeId, file) => {
      if (!canvasDropRef) return
      await canvasDropRef.applyImageFileToMaskNode(nodeId, file)
    },
    applyMaskSourceByUrl: async (nodeId, url, ratio) => {
      if (!canvasDropRef) return
      await canvasDropRef.applyMaskSourceByUrl(nodeId, url, ratio)
    },
  },
  updateSnapshotNode,
  markFlowExecutionNodeDirty,
  clearMaskCanvas: (nodeId) => {
    clearMaskCanvasRef(nodeId)
  },
})

function onCanvasDragOver(event: DragEvent) {
  activeMessageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  canvasDrop.onCanvasDragOver(event)
}

async function onCanvasDrop(event: DragEvent) {
  await canvasDrop.onCanvasDrop(event)
}

function onFlowMessageContextMenu(nodeId: string, event: MouseEvent) {
  applyFlowNodeSelection(nodeId, event)
  activeMessageActionNodeId.value = nodeId
}

const {
  onFlowMaskSetCanvasRef,
  renderMaskNodeMask,
  onFlowMaskCanvasMouseDown,
  stopMaskPainting,
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
  isDirectGenerating,
  isNodeDirectActionRunning,
  runPatternGenerate,
  runMessageDirectAction,
} = useFlowMessageDirectActions({
  nodes: nodes as Ref<Node[]>,
  captureHistory: flowHistory.capture as () => FlowHistorySnapshot,
  pushHistory: flowHistory.push,
  markDirty,
  refreshSnapshot,
  onSuccess: (text) => {
    message.success(text)
  },
  onWarning: (text) => {
    message.warning(text)
  },
  updateSnapshotNodeSilently: (nodeId, patcher) => {
    updateSnapshotNodeBase(nodeId, patcher, { history: false })
  },
  commitNodes: (nextNodes) => {
    commitFlowNodes(nextNodes)
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
    const targetNodeId = activeMessageActionNodeId.value
    if (!targetNodeId) return
    activeMessageActionNodeId.value = null
    void runMessageDirectAction(targetNodeId, 'style-innovate', { strength })
  },
})

function onFlowMessageDirectAction(
  nodeId: string,
  action: 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design',
) {
  if (action === 'style-innovate' && flowSuppressStyleInnovateClick.value) {
    return
  }
  activeMessageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  void runMessageDirectAction(nodeId, action, action === 'style-innovate' ? { strength: flowStyleInnovateStrength.value } : undefined)
}

const { clearFlowSelection, onPaneClick, onFlowNodeClick, onFlowNodeMouseDown, applyFlowNodeSelection, deleteSelectedFlowElements } = useFlowSelectionController({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  activeTool: activeTool as Ref<string>,
  activeMessageActionNodeId,
  hideFlowStyleStrengthControl,
  closeBoardHeaderEditing,
  closeNodeContextMenu,
  _deleteSelectedFlowElements: () => {
    _deleteSelectedFlowElements()
    void nextTick(() => {
      ensureFlowEdgeVisuals()
    })
  },
})

const {
  selectedPreviewItems,
  onClearSelected,
  onRemoveSelectedPreviewItem,
  onFocusSelectedPreviewItem,
  onFlowGenerate,
} = useFlowSelectionPreviewBar({
  nodes: nodes as Ref<Node[]>,
  flowPromptText,
  flowPromptRatio,
  flowPromptQuality,
  clearFlowSelection,
  getViewport,
  setViewport,
  get runPatternGenerate() { return runPatternGenerate },
})

const {
  onFlowImageAppendToCanvas,
  onFlowImageDisplayDragStart,
  appendMessageFlowNode,
  onFlowMessageImageDragStart,
  onFlowMessageResizeMouseDown,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
} = useFlowMessageImageInteractions({
  nodes: nodes as Ref<Node[]>,
  FLOW_NODE_IMAGE_DRAG_MIME,
  MIN_MESSAGE_IMAGE_WIDTH: 120,
  MIN_MESSAGE_IMAGE_HEIGHT: 72,
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
  getViewport,
  updateSnapshotNode,
  captureFlowHistorySnapshot: flowHistory.capture as () => FlowHistorySnapshot,
  pushFlowHistorySnapshot: flowHistory.push,
  markDirty,
  refreshSnapshot,
  appendFlowNodes: (nextNodes) => {
    commitFlowNodesAppend(nextNodes)
  },
  startCanvasInteraction: startFlowInteraction,
  stopCanvasInteraction: stopFlowInteraction,
})

const { isAltPressed, isDraggingNode, onNodeDragStart, onNodeDragStop } = useFlowNodeDragInteractions({
  flowHistory: {
    onNodeDragStart: flowHistory.onNodeDragStart,
    onNodeDragStop: flowHistory.onNodeDragStop,
  },
  startFlowInteraction,
  stopFlowInteraction,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
  ensureFlowEdgeVisuals,
  markDirty,
  activeMessageActionNodeId,
  hideFlowStyleStrengthControl,
})

const canvasDrop = useFlowCanvasDrop({
  currentBoardId,
  screenToFlowCoordinate,
  createNodeAtPosition,
  appendMessageFlowNode,
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
  () => {
    const ids = (nodes.value as Array<{ id: string }>).map(node => node.id)
    return ids.join(',')
  },
  () => {
    scheduleFlowNodeInternalsRefresh()
  },
)

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
})

useFlowLeaveGuard({
  isDirty,
  isSaving: isSavingBoard,
  confirmLeave: confirmFlowLeave,
})

onMounted(async () => {
  await loadInitialOrRouteBoard()
  ensureFlowEdgeVisuals()
  refreshBoardMetas()
  refreshSnapshot()
})
</script>

<style scoped>
.canvas-layout { width: 100%; height: 100vh; background: #ffffff; position: relative; overflow: hidden; }

.canvas-layout.interaction-lite :deep(.workflow-node),
.canvas-layout.interaction-lite :deep(.flow-canvas-node__message),
.canvas-layout.interaction-lite :deep(.vue-flow__edge-path),
.canvas-layout.interaction-lite :deep(.workspace-connection-line path) {
  transition: none !important;
}

.canvas-layout.interaction-lite :deep(.workflow-node *),
.canvas-layout.interaction-lite :deep(.flow-canvas-node__message *) {
  transition-duration: 0s !important;
}

.canvas-layout.interaction-lite :deep(.vue-flow__edge.selected .vue-flow__edge-path),
.canvas-layout.interaction-lite :deep(.workspace-flow-edge.is-selected .vue-flow__edge-path) {
  filter: none !important;
}
.workspace-flow-canvas-wrap :deep(.vue-flow__viewport) {
  background-image: radial-gradient(circle, #e2e4e8 1.5px, transparent 1.5px);
  background-size: 24px 24px;
  background-position: 0 0;
}

.floating-toolbar { position: absolute; left: 16px; top: 50%; transform: translateY(-50%); background: #fff; padding: 8px; border-radius: 16px; display: flex; flex-direction: column; gap: 8px; border: 1px solid #e2e4e8; z-index: 100; }
.tool-item { width: 40px; height: 40px; border-radius: 10px; display: flex; align-items: center; justify-content: center; color: #5f6368; cursor: pointer; border: none; background: transparent; }
.tool-item.active { background: #f1f3f9; color: #9278e6; }
.toolbar-divider { width: 24px; height: 1px; background: #f1f3f9; margin: 4px auto; }

.workspace-flow-canvas-wrap { position: absolute; inset: 0; z-index: 2; }

.workspace-flow-canvas-wrap :deep(.vue-flow) {
  background: transparent;
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

.workspace-flow-canvas-wrap :deep(.vue-flow__handle) {
  width: 9px;
  height: 9px;
  min-width: 9px;
  min-height: 9px;
  border: 0;
  border-radius: 999px;
  box-shadow: none;
  opacity: 1;
  cursor: crosshair;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__handle.connecting),
.workspace-flow-canvas-wrap :deep(.vue-flow__handle.connectionindicator) {
  opacity: 1 !important;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge.tone-image .vue-flow__edge-path) {
  stroke: #5b8def;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge.tone-mask .vue-flow__edge-path) {
  stroke: #34c38f;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge.tone-prompt .vue-flow__edge-path) {
  stroke: #f59e0b;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge .vue-flow__edge-path) {
  stroke-width: 1.8;
  opacity: 0.78;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__edge.selected .vue-flow__edge-path) {
  opacity: 0.98;
  stroke-width: 2.2;
}

.workspace-flow-canvas-wrap :deep(.workspace-flow-edge) {
  pointer-events: visibleStroke;
}

.workspace-flow-canvas-wrap :deep(.workspace-flow-edge .vue-flow__edge-path) {
  transition: opacity 0.18s ease, stroke-width 0.18s ease;
}

.workspace-flow-canvas-wrap :deep(.workspace-flow-edge.is-selected .vue-flow__edge-path) {
  opacity: 0.98;
  stroke-width: 2.2;
}

.workspace-flow-canvas-wrap :deep(.workspace-connection-line path) {
  stroke-width: 2;
  opacity: 0.9;
}

.workspace-flow-bottom-bar {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  bottom: 24px;
  z-index: 100;
}

.floating-status { position: absolute; right: 24px; bottom: 24px; z-index: 100; display: flex; gap: 10px; align-items: center; }
.zoom-controls { display: flex; align-items: center; gap: 8px; background: rgba(255,255,255,.9); backdrop-filter: blur(8px); border: 1px solid #e8eaed; border-radius: 999px; padding: 6px 10px; box-shadow: 0 6px 18px rgba(0,0,0,.06); }
.zoom-btn { width: 28px; height: 28px; border-radius: 8px; border: 1px solid #eef1f6; background: #fff; cursor: pointer; color: #5f6368; font-size: 18px; line-height: 1; display: flex; align-items: center; justify-content: center; }
.zoom-level { font-size: 13px; font-weight: 600; color: #5f6368; min-width: 40px; text-align: center; }
.logo-btn { width: 40px; height: 40px; border-radius: 999px; border: 1px solid #e8eaed; background: rgba(255,255,255,.95); color: #5f6368; display: flex; align-items: center; justify-content: center; cursor: pointer; box-shadow: 0 6px 18px rgba(0,0,0,.08); }

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
