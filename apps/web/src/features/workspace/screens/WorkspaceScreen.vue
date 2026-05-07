<template>
  <div class="canvas-layout" :class="`tool-${activeTool}`" @mousedown.self="closeBoardHeaderEditing">
    <div class="canvas-grid" :style="canvasGridStyle"></div>

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
        fit-view-on-init
        :pan-on-drag="activeTool === 'hand'"
        :nodes-draggable="activeTool !== 'hand'"
        :nodes-connectable="activeTool !== 'hand'"
        :elements-selectable="activeTool !== 'hand'"
        :select-nodes-on-drag="activeTool === 'select'"
        :selection-key-code="activeTool === 'select' ? 'Shift' : false"
        :multi-selection-key-code="activeTool === 'select' ? 'Shift' : null"
        :default-edge-options="{ animated: false }"
        :is-valid-connection="isValidConnection"
        @connect="onConnect"
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
            @message-contextmenu="(event) => onFlowMessageContextMenu(nodeProps.id, event)"
            @message-action="(action) => onFlowMessageDirectAction(nodeProps.id, action)"
            @style-innovate-mousedown="onFlowStyleInnovateMouseDown"
            @style-strength-track-mousedown="onFlowStyleStrengthTrackMouseDown"
            @request-internals-update="() => scheduleFlowNodeInternalsRefresh([nodeProps.id])"
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
      :selected-preview-items="[]"
      :user-prompt="flowPromptText"
      :prompt-placeholder="flowPromptPlaceholder"
      :has-direct-prompt="false"
      :direct-generating="false"
      :disabled="true"
      @toggle-settings="flowPromptShowSettings = !flowPromptShowSettings"
      @select-ratio="flowPromptRatio = $event"
      @select-quality="flowPromptQuality = $event"
      @update:user-prompt="flowPromptText = $event"
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
import { computed, nextTick, onMounted, ref, watch, type Ref } from 'vue'
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
import { loadImageSizeFromUrl, useFlowGenerationHandlers } from '../composables/useFlowGenerationHandlers'
import { useFlowMessageDirectActions } from '../composables/useFlowMessageDirectActions'
import { useFlowStyleInnovateControl } from '../composables/useFlowStyleInnovateControl'
import { useFlowMessageImageInteractions } from '../composables/useFlowMessageImageInteractions'
import { useFlowNodeUiState } from '../composables/useFlowNodeUiState'
import { parseHandlePort, resolveSourceImageFromNode } from '../services/flowTaskExecutorUtils'
import type { FlowNodeData } from '../types/flowTypes'
import { createEmptyCanvasBoardSnapshot, type FlowAdapterEdge, type FlowAdapterNode, flowToCanvasBoardSnapshot } from '../utils/flowSnapshotAdapter'
import { buildFlowEdgeClassName, resolveFlowEdgeToneFromHandles } from '../utils/flowEdgeVisual'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { createCanvasImageItemAt } from '../../workspace/utils/canvasNodeFactory'
import type {
  CanvasBoardSnapshot,
  CanvasImageItem,
  CanvasConnectionNodeType,
  ImageNodeItem,
  MaskNodeItem,
} from '../../workspace/types/canvasNodes'
import HomeCanvasHeader from '../../workspace/components/home/HomeCanvasHeader.vue'
import HomeBottomGenerateBar from '../../workspace/components/home/HomeBottomGenerateBar.vue'
import { useRouter } from 'vue-router'
import {
  isCanvasBoardNameTaken,
  listCanvasBoards,
  normalizeCanvasBoardName,
  renameCanvasBoard,
} from '../../boards/lib/canvasBoards'
import {
  GENERATE_QUALITY_OPTIONS,
  PATTERN_GENERATE_RATIO_OPTIONS,
  type GenerateQualityOption,
  type PatternGenerateRatioOption,
} from '../../workspace/components/nodes/nodeOptionConfig'

const message = useMessage()
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { screenToFlowCoordinate, updateNodeInternals, zoomIn, zoomOut, getViewport, setViewport } = useVueFlow()
const isDev = import.meta.env.DEV

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
const isBoardMenuOpen = ref(false)
const isRenamingBoard = ref(false)
const boardNameDraft = ref('')
const boardNameInputRef = ref<HTMLInputElement | null>(null)
const boardMetas = ref(listCanvasBoards())
const flowPromptShowSettings = ref(false)
const flowPromptText = ref('')
const flowPromptRatio = ref<PatternGenerateRatioOption>('1:1')
const flowPromptQuality = ref<GenerateQualityOption>('1k')
const createNodeKind = ref<FlowCreatableNodeKind>('prompt')
const FLOW_NODE_KIND_DRAG_MIME = 'application/x-ideaboard-flow-node-kind'
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
const zoomLevel = ref(100)
const viewportOffsetX = ref(0)
const viewportOffsetY = ref(0)
const nodeContextMenu = ref({ visible: false, x: 0, y: 0, flowX: 0, flowY: 0 })
const activeMessageActionNodeId = ref<string | null>(null)

const canvasNodeMenuItems = [
  { key: 'prompt', label: 'Prompt' },
  { key: 'image', label: 'Image' },
  { key: 'edit', label: 'Edit' },
  { key: 'enhance', label: 'Enhance' },
  { key: 'color', label: 'Color' },
  { key: 'fusion', label: 'Fusion' },
  { key: 'wear', label: 'Wear' },
  { key: 'mask', label: 'Mask' },
  { key: 'model', label: 'Model' },
  { key: 'viewpoint', label: 'Viewpoint' },
] as const

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
const currentBoardName = computed(() => {
  const boardId = `${currentBoardId.value ?? ''}`.trim()
  if (!boardId) return 'Untitled Canvas'
  return boardMetas.value.find(board => board.id === boardId)?.name ?? boardId
})
const boardOptions = computed(() =>
  boardMetas.value.map(board => ({ id: board.id, name: board.name })),
)
const flowPromptPlaceholder = computed(() => {
  return activeTool.value === 'text'
    ? '文本工具稍后接入，请在节点内执行生成'
    : '请在节点内执行生成'
})
const canvasGridStyle = computed(() => ({
  '--zoom': `${Math.max(0.5, zoomLevel.value / 100)}`,
  '--offset-x': `${viewportOffsetX.value}px`,
  '--offset-y': `${viewportOffsetY.value}px`,
}))

function setBoardNameInputRef(el: HTMLInputElement | null) {
  boardNameInputRef.value = el
}

function refreshBoardMetas() {
  boardMetas.value = listCanvasBoards()
}

function toggleBoardMenu() {
  if (!isBoardMenuOpen.value) refreshBoardMetas()
  isBoardMenuOpen.value = !isBoardMenuOpen.value
}

function startBoardRename() {
  if (!currentBoardId.value) return
  isRenamingBoard.value = true
  isBoardMenuOpen.value = false
  boardNameDraft.value = currentBoardName.value
  void nextTick(() => {
    boardNameInputRef.value?.focus()
    boardNameInputRef.value?.select()
  })
}

function cancelBoardRename() {
  isRenamingBoard.value = false
  boardNameDraft.value = ''
}

function closeBoardHeaderEditing() {
  isBoardMenuOpen.value = false
  cancelBoardRename()
}

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

function closeNodeContextMenu() {
  nodeContextMenu.value.visible = false
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
  nodeContextMenu.value = { visible: true, x, y, flowX: flowPoint.x, flowY: flowPoint.y }
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
  const flowNodes = nodes.value as Array<{ id: string; selected?: boolean } & Record<string, unknown>>
  const flowEdges = edges.value as Array<{ selected?: boolean } & Record<string, unknown>>
  if (isShiftToggle) {
    nodes.value = flowNodes.map((node) => (
      node.id === targetNodeId
        ? { ...node, selected: !node.selected }
        : node
    )) as unknown as Node[]
    return
  }
  nodes.value = flowNodes.map((node) => (
    node.id === targetNodeId
      ? (node.selected ? node : { ...node, selected: true })
      : (node.selected ? { ...node, selected: false } : node)
  )) as unknown as Node[]
  edges.value = flowEdges.map((edge) => (
    edge.selected ? { ...edge, selected: false } : edge
  )) as unknown as Edge[]
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

function onConfirmBoardRename() {
  const boardId = `${currentBoardId.value ?? ''}`.trim()
  if (!boardId) return
  const normalized = normalizeCanvasBoardName(boardNameDraft.value)
  isRenamingBoard.value = false
  boardNameDraft.value = ''
  if (!normalized) {
    message.warning('请输入画布名称')
    return
  }
  if (isCanvasBoardNameTaken(boardId, normalized)) {
    message.warning('名称已存在')
    return
  }
  if (!renameCanvasBoard(boardId, normalized)) {
    message.warning('重命名失败')
    return
  }
  refreshBoardMetas()
  message.success('已重命名画布')
}

function onSelectBoard(boardId: string) {
  if (!boardId) return
  refreshBoardMetas()
  isBoardMenuOpen.value = false
  if (boardId === currentBoardId.value) return
  void navigateToBoard(boardId)
}

async function navigateToBoard(boardId: string) {
  if (isDirty.value && !(await confirmFlowLeave())) return
  skipNextLeaveConfirm = true
  await router.push({ path: '/user/workspace', query: { ...route.query, boardId } })
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

function onZoomIn() {
  zoomIn()
}

function onZoomOut() {
  zoomOut()
}

function onFlowMove(payload: { flowTransform?: { x?: number; y?: number; zoom?: number } }) {
  closeBoardHeaderEditing()
  const transform = payload.flowTransform
  if (!transform) return
  if (Number.isFinite(transform.zoom)) {
    zoomLevel.value = Math.max(10, Math.round((transform.zoom as number) * 100))
  }
  if (Number.isFinite(transform.x)) {
    viewportOffsetX.value = transform.x as number
  }
  if (Number.isFinite(transform.y)) {
    viewportOffsetY.value = transform.y as number
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
})

const currentBoardId = boardPersistence.currentBoardId
const canSaveBoard = boardPersistence.canSaveBoard
const loadInitialOrRouteBoard = boardPersistence.loadInitialOrRouteBoard
const saveCurrentBoardSnapshot = boardPersistence.saveCurrentBoardSnapshot
const importEmptyBoardSnapshot = boardPersistence.importEmptyBoardSnapshot
const importBoardSnapshot = boardPersistence.importBoardSnapshot

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
    nodes.value = [...nodes.value, ...nextNodes]
    scheduleFlowNodeInternalsRefresh(nextNodes.map(node => node.id))
  },
  onWarning: (text) => {
    message.warning(text)
  },
})

const isValidConnection = flowNodeMutations.isValidConnection
const onConnect = flowNodeMutations.onConnect
const onEdgeUpdate = flowNodeMutations.onEdgeUpdate
const createNodeAtPosition = flowNodeMutations.createNodeAtPosition
const onFlowDeleteNode = flowNodeMutations.onFlowDeleteNode
const updateSnapshotNodeBase = flowNodeMutations.updateSnapshotNode
const deleteSelectedFlowElements = flowNodeMutations.deleteSelectedFlowElements
const onFlowPromptTextUpdate = flowNodeMutations.onFlowPromptTextUpdate
const onFlowNodeParamUpdate = flowNodeMutations.onFlowNodeParamUpdate
const onFlowColorApplyPreset = flowNodeMutations.onFlowColorApplyPreset
const onFlowColorRemove = flowNodeMutations.onFlowColorRemove
const onFlowColorAdd = flowNodeMutations.onFlowColorAdd
const onFlowMaskToggleInvert = flowNodeMutations.onFlowMaskToggleInvert

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

function onNodeDragStart() {
  activeMessageActionNodeId.value = null
  hideFlowStyleStrengthControl()
  flowHistory.onNodeDragStart()
}

function onNodeDragStop() {
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
  const rawUrl = event.dataTransfer?.getData(FLOW_NODE_IMAGE_DRAG_MIME)
  if (!rawUrl) return null
  const url = rawUrl.trim()
  if (!url) return null
  const ratioRaw = event.dataTransfer?.getData(`${FLOW_NODE_IMAGE_DRAG_MIME}-ratio`) || ''
  const ratio = Number(ratioRaw)
  return {
    url,
    ratio: Number.isFinite(ratio) && ratio > 0 ? ratio : 1,
  }
}

async function loadImageRatioFromFile(file: File): Promise<number | null> {
  const localUrl = URL.createObjectURL(file)
  try {
    const size = await loadImageSizeFromUrl(localUrl)
    if (!size || !size.width || !size.height) return null
    return size.height / size.width
  } finally {
    URL.revokeObjectURL(localUrl)
  }
}

async function uploadImageToBoard(file: File): Promise<{ url: string; ratio: number }> {
  if (!file.type.startsWith('image/')) {
    throw new Error('请上传图片文件')
  }
  const boardId = `${currentBoardId.value ?? ''}`.trim()
  if (!boardId) {
    throw new Error('缺少 boardId，无法上传图片')
  }
  const ratio = await loadImageRatioFromFile(file)
  const uploadRes = await uploadApi.upload(file, boardId)
  if (uploadRes.code !== 0 || !uploadRes.data?.url) {
    throw new Error(sanitizeApiErrorMessage(uploadRes.msg, '上传失败'))
  }
  return {
    url: uploadRes.data.url,
    ratio: ratio && ratio > 0 ? ratio : 1,
  }
}

async function applyImageFileToImageNode(nodeId: string, file: File): Promise<void> {
  const snapshotNode = getImageNodeSnapshot(nodeId)
  if (!snapshotNode) {
    message.warning('Image 节点不存在')
    return
  }
  try {
    const uploaded = await uploadImageToBoard(file)
    updateSnapshotNode(nodeId, prev => ({
      ...prev,
      sourceImageUrl: uploaded.url,
      sourceImageRatio: uploaded.ratio,
      outputImages: [],
    }))
    message.success('Image 节点输入已更新')
  } catch (error) {
    const msg = error instanceof Error ? error.message : '上传失败'
    message.warning(msg)
  }
}

async function loadImageRatioFromUrl(url: string): Promise<number | null> {
  const size = await loadImageSizeFromUrl(url)
  if (!size || !size.width || !size.height) return null
  return size.height / size.width
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
}

function onFlowImageStageDrop(nodeId: string, event: DragEvent) {
  event.preventDefault()
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
  const snapshotNode = getMaskNodeSnapshot(nodeId)
  if (!snapshotNode) {
    message.warning('Mask 节点不存在')
    return
  }
  try {
    const uploaded = await uploadImageToBoard(file)
    updateSnapshotNode(nodeId, prev => ({
      ...prev,
      sourceImageUrl: uploaded.url,
      sourceImageRatio: uploaded.ratio,
      executionMaskDataUrl: '',
      maskDataUrl: '',
    }))
    void nextTick(() => renderMaskNodeMask(nodeId))
    message.success('Mask 节点输入已更新')
  } catch (error) {
    const msg = error instanceof Error ? error.message : '上传失败'
    message.warning(msg)
  }
}

function onFlowMaskClearImage(nodeId: string) {
  updateSnapshotNode(nodeId, prev => ({
    ...prev,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    executionMaskDataUrl: '',
    maskDataUrl: '',
  }))
  clearMaskCanvas(nodeId)
}

function onFlowMaskClearMask(nodeId: string) {
  updateSnapshotNode(nodeId, prev => ({
    ...prev,
    executionMaskDataUrl: '',
    maskDataUrl: '',
  }))
  clearMaskCanvas(nodeId)
}

async function applyMaskSourceByUrl(nodeId: string, url: string, ratio?: number) {
  const nextRatio = Number.isFinite(ratio) && Number(ratio) > 0
    ? Number(ratio)
    : (await loadImageRatioFromUrl(url)) || 1
  updateSnapshotNode(nodeId, prev => ({
    ...prev,
    sourceImageUrl: url,
    sourceImageRatio: nextRatio,
    executionMaskDataUrl: '',
    maskDataUrl: '',
  }))
  void nextTick(() => renderMaskNodeMask(nodeId))
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
  if (!event.dataTransfer) return
  event.dataTransfer.dropEffect = 'copy'
}

async function onCanvasDrop(event: DragEvent) {
  const point = screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
  const nodeKindRaw = event.dataTransfer?.getData(FLOW_NODE_KIND_DRAG_MIME) || ''
  const nodeKind = nodeKindRaw.trim() as FlowCreatableNodeKind
  if (nodeKind && createNodeOptions.some(item => item.value === nodeKind)) {
    createNodeAtPosition(nodeKind, point.x, point.y)
    return
  }

  const file = event.dataTransfer?.files?.[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    message.warning('请上传图片文件')
    return
  }
  try {
    const uploaded = await uploadImageToBoard(file)
    const messageNode = createCanvasImageItemAt({ x: point.x, y: point.y }, uploaded.url, uploaded.ratio)
    appendMessageFlowNode(messageNode)
  } catch (error) {
    const msg = error instanceof Error ? error.message : '上传失败'
    message.warning(msg)
  }
}

function appendMessageFlowNode(item: CanvasImageItem) {
  const historyBefore = flowHistory.capture()
  const data: FlowNodeData = {
    kind: 'message',
    title: `Message #${item.id}`,
    summary: 'Canvas message image',
    inputPorts: [],
    outputPorts: [],
    snapshotNode: item,
  }
  const nextNode: FlowAdapterNode = {
    id: `message-${item.id}`,
    type: 'canvas',
    dragHandle: resolveFlowNodeDragHandle('message'),
    position: { x: item.x, y: item.y },
    data,
  }
  ;(nodes.value as unknown as FlowAdapterNode[]).push(nextNode)
  flowHistory.push(historyBefore)
  markDirty()
  refreshSnapshot()
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
} = useFlowMaskDrawing({
  getMaskNodeSnapshot,
  updateSnapshotNode,
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
  onFlowMessageImageDragStart,
  onFlowMessageResizeMouseDown,
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
    nodes.value = [...nodes.value, ...nextNodes]
    scheduleFlowNodeInternalsRefresh(nextNodes.map(node => node.id))
  },
  startCanvasInteraction: () => undefined,
  stopCanvasInteraction: () => undefined,
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
  await loadInitialOrRouteBoard()
  ensureFlowEdgeVisuals()
  refreshBoardMetas()
  refreshSnapshot()
})
</script>

<style scoped>
.canvas-layout { width: 100%; height: 100vh; background: #ffffff; position: relative; overflow: hidden; }
.canvas-grid { position: absolute; inset: 0; background-image: radial-gradient(circle, #e2e4e8 calc(1.5px * var(--zoom, 1)), transparent calc(1.5px * var(--zoom, 1))); background-size: calc(24px * var(--zoom, 1)) calc(24px * var(--zoom, 1)); background-position: calc(50% + var(--offset-x, 0px)) calc(50% + var(--offset-y, 0px)); z-index: 0; opacity: 1; }

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
  width: 18px;
  height: 18px;
  min-width: 18px;
  min-height: 18px;
  border: 0;
  border-radius: 999px;
  background: transparent !important;
  box-shadow: none;
  opacity: 0;
  cursor: crosshair;
}

.workspace-flow-canvas-wrap :deep(.vue-flow__handle.connecting),
.workspace-flow-canvas-wrap :deep(.vue-flow__handle.connectionindicator) {
  background: transparent !important;
  border-color: transparent !important;
  box-shadow: none !important;
  opacity: 0 !important;
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
