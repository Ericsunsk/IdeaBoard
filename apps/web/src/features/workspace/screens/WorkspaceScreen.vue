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
      <button v-if="isDev" type="button" class="tool-item debug-toggle" title="Debug" @click="debugPanelOpen = !debugPanelOpen">D</button>
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

    <n-collapse-transition :show="isDev && debugPanelOpen">
      <n-card class="workspace-flow-json" size="small" title="Flow Debug">
        <div class="workspace-flow-debug-toolbar">
          <span class="workspace-flow-save-state" :class="`is-${saveStateTag}`">{{ saveStateText }}</span>
          <n-button size="tiny" :loading="isSavingBoard" :disabled="!canSaveBoard" @click="saveCurrentBoardSnapshot">保存</n-button>
          <n-select v-model:value="createNodeKind" size="small" class="workspace-flow-node-kind" :options="createNodeOptions" />
          <n-button size="tiny" @click="onAddNode">添加节点</n-button>
          <n-button size="tiny" @click="importEmptyBoardSnapshot">导入空白</n-button>
          <n-button size="tiny" :loading="isLoadingBoardSnapshot" @click="importBoardSnapshot">导入画布快照</n-button>
          <n-button size="tiny" :type="snapshotView === 'flow' ? 'primary' : 'default'" @click="setSnapshotView('flow')">Flow JSON</n-button>
          <n-button size="tiny" :type="snapshotView === 'board' ? 'primary' : 'default'" @click="setSnapshotView('board')">Board JSON</n-button>
          <n-button size="tiny" @click="refreshSnapshot">刷新</n-button>
          <n-button size="tiny" @click="copyJson">复制</n-button>
        </div>
        <pre>{{ snapshotText }}</pre>
      </n-card>
    </n-collapse-transition>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import { NButton, NCard, NCollapseTransition, NSelect, useMessage } from 'naive-ui'
import type { SelectMixedOption } from 'naive-ui/es/select/src/interface'
import { useUserStore } from '../../../app/stores/user'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { uploadApi } from '../../../shared/api'
import type { Edge, Node, NodeMouseEvent } from '@vue-flow/core'
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
import { parseHandlePort, resolveSourceImageFromNode } from '../services/flowTaskExecutorUtils'
import type { FlowNodeData } from '../types/flowTypes'
import { createEmptyCanvasBoardSnapshot, type FlowAdapterEdge, type FlowAdapterNode, flowToCanvasBoardSnapshot } from '../utils/flowSnapshotAdapter'
import { buildFlowEdgeClassName, resolveFlowEdgeToneFromHandles } from '../utils/flowEdgeVisual'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import type {
  CanvasBoardSnapshot,
  CanvasConnectionNodeType,
  ImageNodeItem,
  MaskNodeItem,
} from '../../workspace/types/canvasNodes'
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

const message = useMessage()
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { screenToFlowCoordinate, updateNodeInternals, setNodes, setEdges } = useVueFlow()
const isDev = import.meta.env.DEV

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
const snapshotText = ref('')
const snapshotView = ref<'flow' | 'board'>('flow')
const boardBaseSnapshot = ref<CanvasBoardSnapshot>(createEmptyCanvasBoardSnapshot())
const debugPanelOpen = ref(false)
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
const connectingSourceHandle = ref<string | null>(null)

const TONE_COLOR_MAP: Record<string, string> = {
  image: '#5b8def',
  mask: '#34c38f',
  prompt: '#f59e0b',
}

const flowConnectionLineStyle = computed(() => {
  const tone = resolveFlowEdgeToneFromHandles(connectingSourceHandle.value, null)
  return {
    stroke: TONE_COLOR_MAP[tone] || '#9278e6',
    strokeWidth: '2',
    opacity: '0.9',
  }
})

function onFlowConnectStart(event: { handleId?: string | null }) {
  connectingSourceHandle.value = event.handleId ?? null
  startFlowInteraction('connect')
}

function onFlowConnectEnd() {
  connectingSourceHandle.value = null
  stopFlowInteraction('connect')
}

function onFlowEdgeUpdateStart() {
  startFlowInteraction('edge-update')
}

function onFlowEdgeUpdateEnd() {
  connectingSourceHandle.value = null
  stopFlowInteraction('edge-update')
}

const createNodeKind = ref<FlowCreatableNodeKind>('prompt')

const FLOW_ENABLE_VISIBLE_ELEMENTS_CULLING = import.meta.env.VITE_WORKSPACE_FLOW_ENABLE_CULLING === 'true'

const FLOW_NODE_IMAGE_DRAG_MIME = 'application/x-canvas-node-image'
let pendingNodeInternalsRefresh = false
let queuedNodeInternalsIds: Set<string> | null = null
let leaveConfirmResolver: ((choice: 'save' | 'discard' | 'cancel') => void) | null = null
let leaveConfirmPromise: Promise<'save' | 'discard' | 'cancel'> | null = null
let skipNextLeaveConfirm = false
let lastManualNodeSelection:
  | {
    nodeId: string
    timestamp: number
  }
  | null = null
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
function clearFlowSelection(): boolean {
  let hasSelection = false
  const nextNodes = (nodes.value as Array<Node & { selected?: boolean }>).map((node) => {
    if (node.selected) {
      hasSelection = true
      return { ...node, selected: false }
    }
    return node
  })
  const nextEdges = (edges.value as Array<Edge & { selected?: boolean }>).map((edge) => {
    if (edge.selected) {
      hasSelection = true
      return { ...edge, selected: false }
    }
    return edge
  })
  if (!hasSelection) return false
  nodes.value = nextNodes
  edges.value = nextEdges
  return true
}

function onPaneClick() {
  closeBoardHeaderEditing()
  closeNodeContextMenu()
  activeMessageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  clearFlowSelection()
}

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

function onFlowNodeClick(payload: NodeMouseEvent) {
  if (activeTool.value === 'hand') return
  activeMessageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  const targetNodeId = payload.node.id
  const event = payload.event
  if (shouldSkipNodeClickAfterManualSelection(targetNodeId)) return
  applyFlowNodeSelection(targetNodeId, event instanceof MouseEvent ? event : null)
}

function applyFlowNodeSelection(targetNodeId: string, event: MouseEvent | null) {
  if (activeTool.value === 'hand') return
  const isShiftToggle = !!event?.shiftKey

  if (!isShiftToggle) {
    // Single selection: unselect others, select target
    let changed = false
    for (const node of (nodes.value as Array<{ id: string; selected?: boolean }>)) {
      const shouldBeSelected = node.id === targetNodeId
      if (node.selected !== shouldBeSelected) {
        node.selected = shouldBeSelected
        changed = true
      }
    }
    for (const edge of (edges.value as Array<{ selected?: boolean }>)) {
      if (edge.selected) {
        edge.selected = false
        changed = true
      }
    }
  } else {
    // Multi selection toggle
    for (const node of (nodes.value as Array<{ id: string; selected?: boolean }>)) {
      if (node.id === targetNodeId) {
        node.selected = !node.selected
        break
      }
    }
  }
}

function onFlowNodeMouseDown(nodeId: string, event: MouseEvent) {
  if (event.button === 0) {
    activeMessageActionNodeId.value = null
    hideFlowStyleStrengthControl()
  }
  applyFlowNodeSelection(nodeId, event)
  lastManualNodeSelection = {
    nodeId,
    timestamp: Date.now(),
  }
}

function shouldSkipNodeClickAfterManualSelection(nodeId: string): boolean {
  if (!lastManualNodeSelection) return false
  const isSameNode = lastManualNodeSelection.nodeId === nodeId
  const isRecent = Date.now() - lastManualNodeSelection.timestamp < 400
  if (!isSameNode || !isRecent) {
    lastManualNodeSelection = null
    return false
  }
  lastManualNodeSelection = null
  return true
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

function buildFlowEdgeClass(edge: { selected?: boolean; sourceHandle?: string | null; targetHandle?: string | null }): string {
  const tone = resolveFlowEdgeToneFromHandles(edge.sourceHandle, edge.targetHandle)
  return buildFlowEdgeClassName(tone, !!edge.selected)
}

function ensureFlowEdgeVisuals() {
  const source = edges.value as Array<Edge & { data?: { tone?: string } }>
  const withVisuals = source.map((edge): Edge => {
    const nextClass = buildFlowEdgeClass(edge)
    const tone = resolveFlowEdgeToneFromHandles(edge.sourceHandle, edge.targetHandle)
    return {
      ...edge,
      animated: false,
      class: nextClass,
      data: {
        ...(edge.data ?? {}),
        tone,
      },
    }
  })
  const same = source
    .map(edge => `${edge.id}|${edge.class ?? ''}|${edge.animated ? 1 : 0}|${edge.data?.tone ?? ''}`)
    .join(';')
    === withVisuals
      .map(edge => `${edge.id}|${edge.class ?? ''}|${edge.animated ? 1 : 0}|${edge.data?.tone ?? ''}`)
      .join(';')
  if (!same) {
    edges.value = withVisuals
  }
}

const flowMaskEffectiveSourceCache = computed(() => {
  const cache: Record<string, { url: string; ratio: number; hasConnectedImageInput: boolean }> = {}
  const flowNodes = nodes.value as Array<{ id: string; data?: { kind?: string } }>
  for (const node of flowNodes) {
    if (node.data?.kind !== 'mask') continue
    cache[node.id] = getFlowMaskEffectiveSource(node.id)
  }
  return cache
})

const createNodeOptions: SelectMixedOption[] = [
  { label: 'Prompt', value: 'prompt' },
  { label: 'Image', value: 'image' },
  { label: 'Edit', value: 'edit' },
  { label: 'Enhance', value: 'enhance' },
  { label: 'Color', value: 'color' },
  { label: 'Fusion', value: 'fusion' },
  { label: 'Wear', value: 'wear' },
  { label: 'Mask', value: 'mask' },
  { label: 'Model', value: 'model' },
  { label: 'Viewpoint', value: 'viewpoint' },
]

function canCreateNodeKind(kind: string): kind is FlowCreatableNodeKind {
  return createNodeOptions.some(item => item.value === kind)
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
})

type FlowHistorySnapshot = ReturnType<typeof flowHistory.capture>

function refreshSnapshot() {
  if (snapshotView.value === 'flow') {
    snapshotText.value = JSON.stringify(
      {
        nodes: nodes.value,
        edges: edges.value,
      },
      null,
      2,
    )
    return
  }
  const boardSnapshot = flowToCanvasBoardSnapshot({
    nodes: nodes.value as unknown as FlowAdapterNode[],
    edges: edges.value as unknown as FlowAdapterEdge[],
  }, boardBaseSnapshot.value)
  snapshotText.value = JSON.stringify(boardSnapshot, null, 2)
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
})

const currentBoardId = boardPersistence.currentBoardId
const canSaveBoard = boardPersistence.canSaveBoard
const loadInitialOrRouteBoard = boardPersistence.loadInitialOrRouteBoard
const saveCurrentBoardSnapshot = boardPersistence.saveCurrentBoardSnapshot
const importEmptyBoardSnapshot = boardPersistence.importEmptyBoardSnapshot
const importBoardSnapshot = boardPersistence.importBoardSnapshot

const {
  isBoardMenuOpen,
  isRenamingBoard,
  boardNameDraft,
  boardNameInputRef,
  boardMetas,
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
  navigateToBoard,
} = useFlowBoardHeaderActions({
  currentBoardId,
  message: { warning: (msg: string) => message.warning(msg), success: (msg: string) => message.success(msg) },
  router,
  route,
  isDirty,
  confirmFlowLeave,
  skipNextLeaveConfirm: { get value() { return skipNextLeaveConfirm }, set value(v: boolean) { skipNextLeaveConfirm = v } },
})

const saveStateTag = computed<'dirty' | 'saved' | 'failed'>(() => {
  if (!canSaveBoard.value) return 'failed'
  if (saveStatus.value === 'saved') return 'saved'
  if (saveStatus.value === 'failed') return 'failed'
  return isDirty.value ? 'dirty' : 'saved'
})

const saveStateText = computed(() => {
  if (!canSaveBoard.value) {
    return isLoadingBoardSnapshot.value ? '未启用保存（加载中）' : '未启用保存'
  }
  if (saveStatus.value === 'saved' && !isDirty.value) return '已保存'
  if (saveStatus.value === 'failed') return '保存失败'
  return '未保存修改'
})

function copyJson() {
  void navigator.clipboard.writeText(snapshotText.value || '')
  message.success('已复制 JSON')
}

function onAddNode() {
  const viewport = getViewport()
  const x = 320 - viewport.x / (viewport.zoom || 1)
  const y = 180 - viewport.y / (viewport.zoom || 1)
  createNodeAtPosition(createNodeKind.value, x, y)
}

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
function deleteSelectedFlowElements() {
  activeMessageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  closeNodeContextMenu()
  _deleteSelectedFlowElements()
  void nextTick(() => {
    ensureFlowEdgeVisuals()
  })
}
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

const isAltPressed = ref(false)
const isDraggingNode = ref(false)

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

function onWindowKeyDown(event: KeyboardEvent) {
  if (event.key === 'Alt') {
    isAltPressed.value = true
  }
}

function onWindowKeyUp(event: KeyboardEvent) {
  if (event.key === 'Alt') {
    isAltPressed.value = false
  }
}

function onNodeDragStart() {
  isDraggingNode.value = true
  startFlowInteraction('node-drag')
  activeMessageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  flowHistory.onNodeDragStart()
  beginFlowNodeDragDuplicateTracking(isAltPressed.value)
}

function onNodeDragStop() {
  isDraggingNode.value = false
  stopFlowInteraction('node-drag')
  finishFlowNodeDragDuplicateTracking(isAltPressed.value)
  flowHistory.onNodeDragStop(() => {
    markDirty()
  })
  ensureFlowEdgeVisuals()
}

function getImageNodeSnapshot(nodeId: string): ImageNodeItem | null {
  const node = (nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>).find(item => item.id === nodeId)
  if (!node || node.data?.kind !== 'image' || !node.data.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
  return node.data.snapshotNode as ImageNodeItem
}

function getImageNodeDisplayPayload(nodeId: string): { url: string; ratio: number } | null {
  const snapshot = getImageNodeSnapshot(nodeId)
  if (!snapshot) return null
  const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
  const latestOutput = outputs.length > 0 ? outputs[outputs.length - 1] : null
  const url = `${latestOutput?.url || snapshot.sourceImageUrl || ''}`.trim()
  if (!url) return null
  const outputRatio = Number(latestOutput?.ratio)
  const sourceRatio = Number(snapshot.sourceImageRatio)
  return {
    url,
    ratio: Number.isFinite(outputRatio) && outputRatio > 0
      ? outputRatio
      : (Number.isFinite(sourceRatio) && sourceRatio > 0 ? sourceRatio : 1),
  }
}

function getMaskNodeSnapshot(nodeId: string): MaskNodeItem | null {
  const node = (nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>).find(item => item.id === nodeId)
  if (!node || node.data?.kind !== 'mask' || !node.data.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
  return node.data.snapshotNode as MaskNodeItem
}

function resolveFlowImageInputForNode(
  targetNodeId: string,
  port: 'image' | 'mask' | 'prompt',
): { sourceKind: CanvasConnectionNodeType; snapshotNode: Record<string, unknown> } | null {
  const incomingEdge = (edges.value as Array<{ source: string; sourceHandle?: string | null; target: string; targetHandle?: string | null }>)
    .find(edge => edge.target === targetNodeId && parseHandlePort(edge.targetHandle) === port)
  if (!incomingEdge) return null
  const sourceNode = (nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>)
    .find(node => node.id === incomingEdge.source)
  if (!sourceNode || !sourceNode.data?.snapshotNode || typeof sourceNode.data.snapshotNode !== 'object') return null
  const sourceKind = sourceNode.data.kind
  if (
    sourceKind !== 'image' && sourceKind !== 'edit' && sourceKind !== 'enhance'
    && sourceKind !== 'color' && sourceKind !== 'fusion' && sourceKind !== 'wear'
    && sourceKind !== 'model' && sourceKind !== 'viewpoint'
  ) return null
  const sourceType = sourceKind as CanvasConnectionNodeType
  return {
    sourceKind: sourceType,
    snapshotNode: sourceNode.data.snapshotNode as Record<string, unknown>,
  }
}

function resolveFlowSourceImageRatio(sourceKind: CanvasConnectionNodeType, snapshotNode: Record<string, unknown>): number {
  if (sourceKind === 'image') {
    const sourceImageRatio = Number(snapshotNode.sourceImageRatio)
    if (Number.isFinite(sourceImageRatio) && sourceImageRatio > 0) return sourceImageRatio
  }
  const outputImages = Array.isArray(snapshotNode.outputImages) ? snapshotNode.outputImages as Array<{ ratio?: unknown }> : []
  const latestOutput = outputImages.length > 0 ? outputImages[outputImages.length - 1] : null
  const outputRatio = Number(latestOutput?.ratio)
  if (Number.isFinite(outputRatio) && outputRatio > 0) return outputRatio
  return 1
}

function getFlowMaskEffectiveSource(nodeId: string): { url: string; ratio: number; hasConnectedImageInput: boolean } {
  const maskNode = getMaskNodeSnapshot(nodeId)
  if (!maskNode) return { url: '', ratio: 1, hasConnectedImageInput: false }
  const imageInput = resolveFlowImageInputForNode(nodeId, 'image')
  if (imageInput) {
    const url = resolveSourceImageFromNode(imageInput.sourceKind, imageInput.snapshotNode).trim()
    if (url) {
      return {
        url,
        ratio: resolveFlowSourceImageRatio(imageInput.sourceKind, imageInput.snapshotNode),
        hasConnectedImageInput: true,
      }
    }
  }
  return {
    url: (maskNode.sourceImageUrl || '').trim(),
    ratio: maskNode.sourceImageRatio > 0 ? maskNode.sourceImageRatio : 1,
    hasConnectedImageInput: false,
  }
}

function getFlowMaskEffectiveSourceCached(nodeId: string): { url: string; ratio: number; hasConnectedImageInput: boolean } {
  return flowMaskEffectiveSourceCache.value[nodeId] || { url: '', ratio: 1, hasConnectedImageInput: false }
}

function readNodeImageDropPayload(event: DragEvent): { url: string; ratio: number } | null {
  return canvasDrop.readNodeImageDropPayload(event)
}

async function applyImageFileToImageNode(nodeId: string, file: File): Promise<void> {
  await canvasDrop.applyImageFileToImageNode(nodeId, file)
}

function onFlowImageOpenFile(nodeId: string) {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.onchange = () => {
    const file = input.files?.[0]
    if (!file) return
    void applyImageFileToImageNode(nodeId, file)
  }
  input.click()
}

function onFlowImageReset(nodeId: string) {
  updateSnapshotNode(nodeId, prev => ({
    ...prev,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputImages: [],
  }))
  markFlowExecutionNodeDirty(nodeId)
}

function onFlowImageStageDrop(nodeId: string, event: DragEvent) {
  event.preventDefault()
  const payload = readNodeImageDropPayload(event)
  if (payload) {
    updateSnapshotNode(nodeId, prev => ({
      ...prev,
      sourceImageUrl: payload.url,
      sourceImageRatio: payload.ratio,
      outputImages: [],
    }))
    markFlowExecutionNodeDirty(nodeId)
    return
  }
  const file = event.dataTransfer?.files?.[0]
  if (!file) return
  void applyImageFileToImageNode(nodeId, file)
}

function onFlowMaskOpenFile(nodeId: string) {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.onchange = () => {
    const file = input.files?.[0]
    if (!file) return
    void applyImageFileToMaskNode(nodeId, file)
  }
  input.click()
}

async function applyImageFileToMaskNode(nodeId: string, file: File): Promise<void> {
  await canvasDrop.applyImageFileToMaskNode(nodeId, file)
}

function onFlowMaskClearImage(nodeId: string) {
  updateSnapshotNode(nodeId, prev => ({
    ...prev,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    executionMaskDataUrl: '',
    maskDataUrl: '',
  }))
  markFlowExecutionNodeDirty(nodeId)
  clearMaskCanvas(nodeId)
}

function onFlowMaskClearMask(nodeId: string) {
  updateSnapshotNode(nodeId, prev => ({
    ...prev,
    executionMaskDataUrl: '',
    maskDataUrl: '',
  }))
  markFlowExecutionNodeDirty(nodeId)
  clearMaskCanvas(nodeId)
}

async function applyMaskSourceByUrl(nodeId: string, url: string, ratio?: number) {
  await canvasDrop.applyMaskSourceByUrl(nodeId, url, ratio)
}
function onFlowMaskStageDrop(nodeId: string, event: DragEvent) {
  event.preventDefault()
  const payload = readNodeImageDropPayload(event)
  if (payload) {
    void applyMaskSourceByUrl(nodeId, payload.url, payload.ratio)
    return
  }
  const file = event.dataTransfer?.files?.[0]
  if (!file) return
  void applyImageFileToMaskNode(nodeId, file)
}

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

const { getFlowNodeUiStateCached } = useFlowNodeUiState({
  nodes: nodes as Ref<Node[]>,
  edges: edges as Ref<Edge[]>,
  isFlowExecutionNodeRunning,
})

function setSnapshotView(view: 'flow' | 'board') {
  snapshotView.value = view
  refreshSnapshot()
}

watch(
  () => (edges.value as Array<Edge & { selected?: boolean }>).map(edge => `${edge.id}:${edge.selected ? 1 : 0}:${edge.sourceHandle ?? ''}:${edge.targetHandle ?? ''}`).join('|'),
  () => {
    ensureFlowEdgeVisuals()
  },
)

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
  window.addEventListener('keydown', onWindowKeyDown)
  window.addEventListener('keyup', onWindowKeyUp)
  await loadInitialOrRouteBoard()
  ensureFlowEdgeVisuals()
  refreshBoardMetas()
  refreshSnapshot()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onWindowKeyDown)
  window.removeEventListener('keyup', onWindowKeyUp)
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
.debug-toggle { cursor: pointer; font-size: 13px; font-weight: 700; }

.workspace-flow-node-kind {
  width: 132px;
}

.workspace-flow-save-state {
  border-radius: 999px;
  padding: 2px 10px;
  border: 1px solid #d4dbe7;
  background: #f6f8fc;
  font-size: 12px;
  color: #516074;
}

.workspace-flow-save-state.is-dirty {
  border-color: #f2c97d;
  background: #fff7e8;
  color: #9f6a00;
}

.workspace-flow-save-state.is-saved {
  border-color: #9fddb4;
  background: #ecfbf1;
  color: #15753a;
}

.workspace-flow-save-state.is-failed {
  border-color: #efb0b0;
  background: #fff1f1;
  color: #b12828;
}

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

.workspace-flow-json {
  position: absolute;
  left: 72px;
  top: 24px;
  z-index: 200;
  width: min(720px, calc(100vw - 96px));
}

.workspace-flow-debug-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
  align-items: center;
}

.workspace-flow-json pre {
  margin: 0;
  max-height: 280px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.4;
}

@media (max-width: 900px) {
  .workspace-flow-json {
    left: 12px;
    right: 12px;
    width: auto;
  }
}
</style>
