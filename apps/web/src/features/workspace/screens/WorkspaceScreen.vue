<template>
  <div 
    class="canvas-layout" 
    :class="{ 'copy-mode': isDuplicateModifierPressed && activeTool === 'select' }"
    ref="canvasLayoutRef"
    :style="canvasStyle" 
    @mousedown="onCanvasMouseDown"
    @contextmenu="onCanvasContextMenu"
    @dragover.prevent="isDraggingFile = true"
    @dragleave="isDraggingFile = false"
    @drop="handleDrop"
  >
    <!-- ① 无限画布背景层 -->
    <div class="canvas-grid" :class="{ 'grid-dragging': isDraggingFile }"></div>

    <!-- ② 顶部悬浮栏 -->
    <HomeCanvasHeader
      :current-board-name="currentBoardName"
      :board-options="boardOptions"
      :current-board-id="currentBoardId"
      :is-board-menu-open="isBoardMenuOpen"
      :is-renaming-board="isRenamingBoard"
      :board-name-draft="boardNameDraft"
      :set-board-name-input-ref="setBoardNameInputRef"
      :export-disabled="selectedIds.length === 0"
      :avatar-url="userStore.user?.avatar"
      @update:board-name-draft="boardNameDraft = $event"
      @toggle-board-menu="toggleBoardMenu"
      @start-board-rename="startBoardRename"
      @cancel-board-rename="cancelBoardRename"
      @confirm-board-rename="confirmBoardRename"
      @select-board="handleBoardSelect"
      @export="handleExport"
      @open-center="navigateAway('/user/center')"
    />

    <!-- 框选视觉矩形 -->
    <div 
      v-if="isSelecting" 
      class="selection-marquee"
      :style="{
        left: Math.min(selectionRect.x1, selectionRect.x2) + 'px',
        top: Math.min(selectionRect.y1, selectionRect.y2) + 'px',
        width: Math.abs(selectionRect.x2 - selectionRect.x1) + 'px',
        height: Math.abs(selectionRect.y2 - selectionRect.y1) + 'px'
      }"
    ></div>

    <!-- ③ 左侧工具栏 -->
    <aside class="floating-toolbar">
      <div class="tool-item" :class="{ active: activeTool === 'select' }" title="Select (V)" @click="activeTool = 'select'">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 3l7.07 16.97 2.51-7.39 7.39-2.51L3 3z"/><path d="M13 13l6 6"/></svg>
      </div>
      <div class="tool-item" :class="{ active: activeTool === 'hand' }" title="Hand (H)" @click="activeTool = 'hand'">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 11V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v0"/><path d="M14 10V4a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v10"/><path d="M10 10.5V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v8"/><path d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/></svg>
      </div>
      <div class="tool-item" :class="{ active: activeTool === 'text' }" title="Text (T)" @click="activeTool = 'text'">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="4 7 4 4 20 4 20 7"/><line x1="9" y1="20" x2="15" y2="20"/><line x1="12" y1="4" x2="12" y2="20"/></svg>
      </div>
      <div class="toolbar-divider"></div>
      <!-- 业务入口 -->
      <div 
        class="tool-item" 
        v-for="t in tools" 
        :key="t.title" 
        :class="{ active: route.path === t.path }" 
        :title="t.title" 
        @click="navigateAway(t.path)"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" v-html="t.icon"></svg>
      </div>
    </aside>

    <!-- ④ 底部指令栏 -->
    <HomeBottomGenerateBar
      :show-settings="showSettings"
      :ratio-options="patternGenerateRatioOptions"
      :quality-options="patternGenerateQualityOptions"
      :selected-ratio="selectedRatio"
      :selected-quality="selectedQuality"
      :selected-preview-items="selectedPreviewItems"
      :user-prompt="userPrompt"
      :prompt-placeholder="promptPlaceholder"
      :has-direct-prompt="hasDirectPrompt"
      :direct-generating="directGenerating"
      @toggle-settings="showSettings = !showSettings"
      @select-ratio="selectedRatio = $event"
      @select-quality="selectedQuality = $event"
      @clear-selected="clearSelectedIds"
      @focus-selected="focusSelectedItem"
      @remove-selected="removeSelectedId"
      @update:user-prompt="userPrompt = $event"
      @generate="handleGenerate"
    />

    <!-- ⑤ 右下角控件 -->
    <div class="floating-status">
      <div class="zoom-controls">
        <button class="zoom-btn" @click="updateZoom(-10)">−</button>
        <span class="zoom-level">{{ zoomLevel }}%</span>
        <button class="zoom-btn" @click="updateZoom(10)">+</button>
      </div>
      <button class="logo-btn" title="返回画布列表" @click="navigateAway('/user/boards')">
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

    <div
      v-if="nodeContextMenu.visible"
      class="node-context-menu"
      :style="{ left: `${nodeContextMenu.x}px`, top: `${nodeContextMenu.y}px` }"
      @mousedown.stop
      @click.stop
    >
      <button
        v-for="item in canvasNodeMenuItems"
        :key="item.key"
        type="button"
        class="node-context-item"
        @click="handleNodeMenuAction(item.actionKey)"
      >
        {{ item.label }}
      </button>
    </div>

    <HomeCanvasConnectionLayer
      :rendered-connections="renderedConnections"
      :selected-connection-id="selectedConnectionId"
      :hidden-connection-id="reconnectingConnectionId"
      :draft-connection-path="draftConnectionPath"
      :draft-connection-kind="draftConnectionKind"
      :draft-connection-tone="draftConnectionTone"
      @path-mousedown="onConnectionPathMouseDown"
    />

    <!-- ⑥ 内容展示区 -->
    <div class="canvas-content">
      <HomeCanvasNodesBasic
        :image-nodes="imageNodes"
        :prompt-nodes="promptNodes"
        :copy-mode="nodeCopyMode"
        :is-node-selected="isNodeSelected"
        :is-node-dragging="isNodeDragging"
        :get-image-node-ui-state="getImageNodeUiState"
        :get-prompt-node-ui-state="getPromptNodeUiState"
        :get-image-node-stage-height="getImageNodeStageHeight"
        :get-resolved-image-node-display="getResolvedImageNodeDisplay"
        @image-node-mousedown="(event, node) => onImageNodeMouseDown(event, node)"
        @image-port-mousedown="onImageNodePortMouseDown"
        @image-delete="deleteImageNode"
        @image-stage-drop="(event, node) => onImageNodeStageDrop(event, node)"
        @image-open-file="openImageNodeFilePicker"
        @image-append-to-canvas="appendImageNodeDisplayToCanvas"
        @image-reset="resetImageNode"
        @image-display-dragstart="(event, node) => onImageNodeDisplayDragStart(event, node)"
        @prompt-node-mousedown="(event, node) => onPromptNodeMouseDown(event, node)"
        @prompt-port-mousedown="onPromptNodePortMouseDown"
        @prompt-delete="deletePromptNode"
        @prompt-focus="activatePromptNode"
        @prompt-update-text="onPromptNodeTextUpdate"
      />

      <HomeCanvasNodesStructured
        :model-nodes="modelNodes"
        :viewpoint-nodes="viewpointNodes"
        :mask-nodes="maskNodes"
        :copy-mode="nodeCopyMode"
        :is-node-selected="isNodeSelected"
        :is-node-dragging="isNodeDragging"
        :get-model-node-ui-state="getModelNodeUiState"
        :get-viewpoint-node-ui-state="getViewpointNodeUiState"
        :get-mask-node-ui-state="getMaskNodeUiState"
        :get-node-connection-error-ports="getNodeConnectionErrorPorts"
        :get-mask-node-stage-height="getMaskNodeStageHeight"
        :get-mask-canvas-width="getMaskCanvasWidth"
        :get-mask-node-effective-source="getMaskNodeEffectiveSource"
        :model-background-options="modelBackgroundOptions"
        @model-node-mousedown="(event, node) => onModelNodeMouseDown(event, node)"
        @model-port-mousedown="onModelNodePortMouseDown"
        @model-delete="deleteModelNode"
        @model-update-angle-yaw="onModelNodeUpdateAngleYaw"
        @model-update-angle-framing="onModelNodeUpdateAngleFraming"
        @model-update-model-strength="onModelNodeUpdateModelStrength"
        @model-update-pose-strength="onModelNodeUpdatePoseStrength"
        @model-select-background="onModelNodeSelectBackground"
        @model-generate="handleModelNodeGenerate"
        @viewpoint-node-mousedown="(event, node) => onViewpointNodeMouseDown(event, node)"
        @viewpoint-port-mousedown="onViewpointNodePortMouseDown"
        @viewpoint-delete="deleteViewpointNode"
        @viewpoint-update-category="(node, category) => setViewpointNodeCategory(node, category)"
        @viewpoint-select="(node, key) => setViewpointSelection(node, key)"
        @viewpoint-update-lighting="onViewpointNodeUpdateLighting"
        @viewpoint-generate="handleViewpointNodeGenerate"
        @mask-node-mousedown="(event, node) => onMaskNodeMouseDown(event, node)"
        @mask-port-mousedown="onMaskNodePortMouseDown"
        @mask-delete="deleteMaskNode"
        @mask-open-file-picker="openMaskNodeFilePicker"
        @mask-clear-image="clearMaskNodeImage"
        @mask-stage-drop="(event, node) => onMaskStageDrop(event, node)"
        @mask-canvas-mousedown="onMaskNodeCanvasMouseDownBridge"
        @mask-set-draw-mode="onMaskNodeSetDrawMode"
        @mask-update-brush-size="onMaskNodeUpdateBrushSize"
        @mask-update-refine-amount="onMaskNodeUpdateRefineAmount"
        @mask-toggle-invert="onMaskNodeToggleInvert"
        @mask-clear-mask="clearMaskNodeMask"
        @mask-set-canvas-ref="setMaskCanvasRef"
      />

      <HomeCanvasNodesExecution
        :edit-nodes="editNodes"
        :fusion-nodes="fusionNodes"
        :enhance-nodes="enhanceNodes"
        :wear-nodes="wearNodes"
        :color-nodes="colorNodes"
        :copy-mode="nodeCopyMode"
        :is-node-selected="isNodeSelected"
        :is-node-dragging="isNodeDragging"
        :get-edit-node-ui-state="getEditNodeUiState"
        :get-fusion-node-ui-state="getFusionNodeUiState"
        :get-enhance-node-ui-state="getEnhanceNodeUiState"
        :get-wear-node-ui-state="getWearNodeUiState"
        :get-color-node-ui-state="getColorNodeUiState"
        :get-node-connection-error-ports="getNodeConnectionErrorPorts"
        @edit-node-mousedown="(event, node) => onEditNodeMouseDown(event, node)"
        @edit-delete="deleteEditNode"
        @edit-update-mode="onEditNodeUpdateMode"
        @edit-generate="handleEditNodeGenerate"
        @edit-port-mousedown="onEditNodePortMouseDown"
        @fusion-node-mousedown="(event, node) => onFusionNodeMouseDown(event, node)"
        @fusion-delete="deleteFusionNode"
        @fusion-update-blend-ratio="onFusionNodeUpdateBlendRatio"
        @fusion-generate="handleFusionNodeGenerate"
        @fusion-port-mousedown="onFusionNodePortMouseDown"
        @enhance-node-mousedown="(event, node) => onEnhanceNodeMouseDown(event, node)"
        @enhance-delete="deleteEnhanceNode"
        @enhance-update-mode="onEnhanceNodeUpdateMode"
        @enhance-update-upscale-mode="onEnhanceNodeUpdateUpscaleMode"
        @enhance-update-upscale-factor="onEnhanceNodeUpdateUpscaleFactor"
        @enhance-update-output-format="onEnhanceNodeUpdateOutputFormat"
        @enhance-update-output-quality="onEnhanceNodeUpdateOutputQuality"
        @enhance-update-extend-top="onEnhanceNodeUpdateExtendTop"
        @enhance-update-extend-right="onEnhanceNodeUpdateExtendRight"
        @enhance-update-extend-bottom="onEnhanceNodeUpdateExtendBottom"
        @enhance-update-extend-left="onEnhanceNodeUpdateExtendLeft"
        @enhance-generate="handleEnhanceNodeGenerate"
        @enhance-port-mousedown="onEnhanceNodePortMouseDown"
        @wear-node-mousedown="(event, node) => onWearNodeMouseDown(event, node)"
        @wear-delete="deleteWearNode"
        @wear-update-tile-mode="onWearNodeUpdateTileMode"
        @wear-update-rotation="onWearNodeUpdateRotation"
        @wear-update-scale="onWearNodeUpdateScale"
        @wear-update-opacity="onWearNodeUpdateOpacity"
        @wear-generate="handleWearNodeGenerate"
        @wear-port-mousedown="onWearNodePortMouseDown"
        @color-node-mousedown="(event, node) => onColorNodeMouseDown(event, node)"
        @color-delete="deleteColorNode"
        @color-apply-preset="onColorNodeApplyPreset"
        @color-remove-color="(node, index) => removeColorNodeColor(node, index)"
        @color-add-color="(node, value) => addColorNodeColor(node, value)"
        @color-strength-input="onColorNodeStrengthInput"
        @color-generate="handleColorNodeGenerate"
        @color-port-mousedown="onColorNodePortMouseDown"
      />

      <HomeCanvasImageItem
        v-for="msg in messages"
        :key="msg.id"
        :item="msg"
        :is-active="selectedIds.includes(msg.id)"
        :is-pending="isCanvasImagePending(msg)"
        :show-action-menu="activeImageActionId === msg.id && !isCanvasImagePending(msg)"
        :show-style-strength-control="showStyleStrengthControl"
        :style-strength-menu-style="showStyleStrengthControl ? getStyleStrengthMenuStyle(msg) : undefined"
        :direct-generating="directGenerating"
        :display-height="getImageDisplayHeight(msg)"
        :style-innovate-strength-pct="styleInnovateStrengthPct"
        :show-resize-handles="selectedIds.length === 1 && selectedIds[0] === msg.id && !isCanvasImagePending(msg)"
        :set-image-ref="(el) => setCanvasImageRef(msg.id, el)"
        :set-style-strength-track-ref="setStyleStrengthTrackRef"
        @item-mousedown="onItemMouseDown($event, msg)"
        @item-contextmenu="onItemContextMenu($event, msg)"
        @image-load="onImageLoad(msg, $event)"
        @style-innovate-mousedown="onStyleInnovateMouseDown"
        @style-innovate-click="onStyleInnovateClick(msg)"
        @action-remove-bg="runCanvasAction('remove-bg', msg)"
        @action-to-vector="runCanvasAction('to-vector', msg)"
        @action-sketch-to-design="runCanvasAction('sketch-to-design', msg)"
        @style-strength-track-mousedown="onStyleStrengthTrackMouseDown"
        @resize-mousedown="onResizeMouseDown($event.event, msg, $event.direction)"
      />
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import type { ComponentPublicInstance, Ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useMessage } from 'naive-ui'
import { taskApi, uploadApi, vectorizeApi } from '../../../shared/api/index'
import { useUserStore } from '../../../app/stores/user'
import { getAuthStorageUserScope } from '../../../shared/utils/auth'
import {
  collectCanvasBoardImageUrls,
  getCanvasBoardState,
  isImageUrlReferencedInOtherCanvasBoardStates,
} from '../../boards/lib/canvasBoardState'
import { CANVAS_NODE_MENU_ITEMS, type CanvasNodeCreateActionKey } from '../config/canvasNodeMenu'
import { useCanvasConnections } from '../composables/useCanvasConnections'
import { useCanvasViewport } from '../composables/useCanvasViewport'
import { useCanvasBoard } from '../composables/useCanvasBoard'
import { useCanvasEditor } from '../composables/useCanvasEditor'
import { useCanvasHistory } from '../composables/useCanvasHistory'
import { useCanvasNodeConnections } from '../composables/useCanvasNodeConnections'
import { useCanvasNodeUiState } from '../composables/useCanvasNodeUiState'
import { useCanvasDirectActions } from '../composables/useCanvasDirectActions'
import { useGenerationPipeline } from '../composables/useGenerationPipeline'
import { useImageNode } from '../composables/useImageNode'
import { clampMaskRefineAmount, useMaskNode } from '../composables/useMaskNode'
import { useModelNode } from '../composables/useModelNode'
import { useNodeExecutors } from '../composables/useNodeExecutors'
import { useViewpointNode } from '../composables/useViewpointNode'
import HomeCanvasConnectionLayer from '../components/home/HomeCanvasConnectionLayer.vue'
import HomeCanvasImageItem from '../components/home/HomeCanvasImageItem.vue'
import HomeBottomGenerateBar from '../components/home/HomeBottomGenerateBar.vue'
import HomeCanvasHeader from '../components/home/HomeCanvasHeader.vue'
import HomeCanvasNodesBasic from '../components/home/HomeCanvasNodesBasic.vue'
import HomeCanvasNodesExecution from '../components/home/HomeCanvasNodesExecution.vue'
import HomeCanvasNodesStructured from '../components/home/HomeCanvasNodesStructured.vue'
import {
  describeViewpointOption,
  GENERATE_QUALITY_OPTIONS,
  PATTERN_GENERATE_RATIO_OPTIONS,
  MODEL_BACKGROUND_OPTIONS,
  VIEWPOINT_LIGHTING_LABELS,
  type GenerateQualityOption,
  type PatternGenerateRatioOption,
  normalizeViewpointSelectionByCategory,
  VIEWPOINT_OPTIONS_BY_CATEGORY,
} from '../components/nodes/nodeOptionConfig'
import { normalizeHexColor } from '../components/nodes/colorPopoverUtils'
import {
  enhanceModeSupportsPrompt,
  enhanceModeSupportsMask,
  editModeSupportsPrompt,
  editModeSupportsSourceImage,
} from '../config/canvasNodeConnectionContract'
import { isCanvasConnectionAllowed } from '../config/canvasConnections'
import {
  createCanvasImageItemAt,
  createEnhanceNodeAt,
  createEditNodeAt,
  createFusionNodeAt,
  createImageNodeAt,
  createMaskNodeAt,
  createModelNodeAt,
  createColorNodeAt,
  createPromptNodeAt,
  createWearNodeAt,
  createViewpointNodeAt,
} from '../utils/canvasNodeFactory'
import {
  DEFAULT_COLOR_STRENGTH,
  ENHANCE_EXTEND_PERCENT_DEFAULT,
  ENHANCE_EXTEND_PERCENT_MAX,
  ENHANCE_EXTEND_PERCENT_MIN,
  clampWearOpacity,
  clampWearRotation,
  clampWearScale,
  clampModelPoseStrength,
  clampModelReferenceStrength,
  isCanvasExecutionOutputNodeType,
  isCanvasImagePending,
  isColorNodePlaceholderState,
  MAX_COLOR_NODE_SWATCHES,
  normalizeWearTileMode,
  normalizeColorNodeColors,
  resolveWorkflowNodeOutputStatus,
} from '../types/canvasNodes'
import type {
  CanvasBoardSnapshot,
  CanvasConnection,
  CanvasExecutionOutputNodeType,
  CanvasExecutionOutputNodeItem,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasImageItem,
  CanvasNodeItem,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelAngleFraming,
  ModelAngleSpec,
  ModelNodeItem,
  PromptNodeItem,
  WearNodeItem,
  ViewpointNodeItem,
  WorkflowNodeOutputStatus,
} from '../types/canvasNodes'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const userStore = useUserStore()
const defaultCanvasName = 'Untitled'
const USER_DOWNLOAD_ENDPOINT = '/api/user/download'

const patternGenerateRatioOptions = PATTERN_GENERATE_RATIO_OPTIONS
const patternGenerateQualityOptions = GENERATE_QUALITY_OPTIONS
const modelBackgroundOptions = MODEL_BACKGROUND_OPTIONS
const viewpointLightingLabels = VIEWPOINT_LIGHTING_LABELS

const nodeImageDragMime = 'application/x-ideaboard-image-url'

// 状态
const userPrompt = ref('')
const nodeGenerating = ref(false)
const directGenerating = ref(false)
const messages = ref<CanvasImageItem[]>([])
const {
  zoomLevel,
  canvasX,
  canvasY,
  canvasLayoutRef,
  canvasStyle,
  updateZoom,
  getCanvasPointFromClient,
  setViewport,
  resetViewport,
  createViewportSnapshot,
} = useCanvasViewport()
const activeTool = ref('select')
const isDraggingFile = ref(false)
const showSettings = ref(false)
const imageNodes = ref<ImageNodeItem[]>([])
const promptNodes = ref<PromptNodeItem[]>([])
const modelNodes = ref<ModelNodeItem[]>([])
const viewpointNodes = ref<ViewpointNodeItem[]>([])
const maskNodes = ref<MaskNodeItem[]>([])
const colorNodes = ref<ColorNodeItem[]>([])
const enhanceNodes = ref<EnhanceNodeItem[]>([])
const editNodes = ref<EditNodeItem[]>([])
const fusionNodes = ref<FusionNodeItem[]>([])
const wearNodes = ref<WearNodeItem[]>([])
const RECENT_HISTORY_RECYCLE_GUARD_STEPS = 10
const PENDING_RECYCLE_STORAGE_KEY_PREFIX = 'ideaboard.canvas.pendingUploadRecycle.v1'

interface PendingRecycleUploadEntry {
  url: string
  userId: number
  boardId: string
}

const pendingRecycleEntries = ref<PendingRecycleUploadEntry[]>(readPendingRecycleEntries())
let pendingRecycleScanInFlight = false
let pendingRecycleScanQueued = false
let pendingRecycleScanTimer: number | null = null

function getPendingRecycleStorageKey(): string | null {
  const scope = getAuthStorageUserScope()
  return scope ? `${PENDING_RECYCLE_STORAGE_KEY_PREFIX}.${scope}` : null
}

function normalizePendingRecycleEntry(entry: unknown): PendingRecycleUploadEntry | null {
  if (!entry || typeof entry !== 'object') return null
  const rawUrl = typeof (entry as { url?: unknown }).url === 'string' ? (entry as { url: string }).url.trim() : ''
  const rawBoardId = typeof (entry as { boardId?: unknown }).boardId === 'string' ? (entry as { boardId: string }).boardId.trim() : ''
  const rawUserId = Number((entry as { userId?: unknown }).userId)
  if (!rawUrl || !rawBoardId || !Number.isFinite(rawUserId) || rawUserId <= 0) return null
  return {
    url: rawUrl,
    userId: Math.floor(rawUserId),
    boardId: rawBoardId,
  }
}

function dedupePendingRecycleEntries(entries: PendingRecycleUploadEntry[]): PendingRecycleUploadEntry[] {
  const entryMap = new Map<string, PendingRecycleUploadEntry>()
  entries.forEach((entry) => {
    const normalized = normalizePendingRecycleEntry(entry)
    if (!normalized) return
    entryMap.set(normalized.url, normalized)
  })
  return Array.from(entryMap.values())
}

function readPendingRecycleEntries(): PendingRecycleUploadEntry[] {
  if (typeof window === 'undefined') return []
  const storageKey = getPendingRecycleStorageKey()
  if (!storageKey) return []
  const raw = localStorage.getItem(storageKey)
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed)
      ? dedupePendingRecycleEntries(parsed as PendingRecycleUploadEntry[])
      : []
  } catch {
    return []
  }
}

function writePendingRecycleEntries(entries: PendingRecycleUploadEntry[]) {
  if (typeof window === 'undefined') return
  const storageKey = getPendingRecycleStorageKey()
  if (!storageKey) return
  if (!entries.length) {
    localStorage.removeItem(storageKey)
    return
  }
  localStorage.setItem(storageKey, JSON.stringify(entries))
}

function setPendingRecycleEntries(entries: PendingRecycleUploadEntry[]) {
  const nextEntries = dedupePendingRecycleEntries(entries)
  pendingRecycleEntries.value = nextEntries
  writePendingRecycleEntries(nextEntries)
}

function clearPendingRecycleScanTimer() {
  if (pendingRecycleScanTimer === null) return
  window.clearTimeout(pendingRecycleScanTimer)
  pendingRecycleScanTimer = null
}

function parseCanvasImagePath(url: string): string | null {
  const raw = url.trim()
  if (!raw) return null
  if (raw.startsWith('/')) return raw
  try {
    const parsed = new URL(raw, window.location.origin)
    return parsed.pathname || null
  } catch {
    return null
  }
}

function encodeWorkspacePathSegment(segment: string): string {
  return encodeURIComponent(segment).replace(
    /[!'()*]/g,
    (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`,
  )
}

function uploadWorkspaceImage(file: File) {
  const boardId = `${currentBoardId.value ?? ''}`.trim()
  if (!boardId) {
    throw new Error('缺少画布ID')
  }
  return uploadApi.upload(file, boardId)
}

interface PersistedImageUrlReference {
  path: string
  getUrl: () => string
  setUrl: (url: string) => void
}

function resolveAbsoluteCanvasUrl(rawUrl: string): URL | null {
  if (typeof window === 'undefined') return null
  try {
    return new URL(rawUrl, window.location.origin)
  } catch {
    return null
  }
}

function isStableWorkspaceUploadUrl(rawUrl: string): boolean {
  if (typeof window === 'undefined') return true
  const normalized = rawUrl.trim()
  if (!normalized) return true
  if (normalized.startsWith('/uploads/')) return true
  const parsed = resolveAbsoluteCanvasUrl(normalized)
  if (!parsed) return false
  return parsed.origin === window.location.origin && parsed.pathname.startsWith('/uploads/')
}

function getProxyTargetUrl(rawUrl: string): string | null {
  const parsed = resolveAbsoluteCanvasUrl(rawUrl.trim())
  if (!parsed) return null
  if (parsed.origin !== window.location.origin || !parsed.pathname.startsWith(USER_DOWNLOAD_ENDPOINT)) {
    return null
  }
  const target = parsed.searchParams.get('url')
  return target?.trim() || null
}

function resolveImageFetchRequestUrl(rawUrl: string): string {
  const normalized = rawUrl.trim()
  const proxyTarget = getProxyTargetUrl(normalized)
  if (proxyTarget) {
    return `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(proxyTarget)}`
  }
  const parsed = resolveAbsoluteCanvasUrl(normalized)
  if (parsed && (parsed.protocol === 'http:' || parsed.protocol === 'https:')) {
    if (parsed.origin !== window.location.origin) {
      return `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(parsed.href)}`
    }
    return parsed.href
  }
  return normalized
}

function inferFileExtensionFromMimeType(mimeType: string): string {
  const normalized = mimeType.toLowerCase()
  if (normalized.includes('png')) return 'png'
  if (normalized.includes('webp')) return 'webp'
  if (normalized.includes('gif')) return 'gif'
  if (normalized.includes('bmp')) return 'bmp'
  if (normalized.includes('jpeg') || normalized.includes('jpg')) return 'jpg'
  if (normalized.includes('svg')) return 'svg'
  return 'jpg'
}

function toUploadFilename(rawUrl: string, mimeType: string): string {
  const parsed = resolveAbsoluteCanvasUrl(rawUrl.trim())
  const pathname = parsed?.pathname || rawUrl
  const leaf = pathname.split('/').filter(Boolean).pop() || ''
  const safeLeaf = leaf.replace(/[^A-Za-z0-9._-]/g, '')
  const hasExtension = /\.[A-Za-z0-9]{2,8}$/.test(safeLeaf)
  if (hasExtension) return safeLeaf
  return `workspace-image-${Date.now()}.${inferFileExtensionFromMimeType(mimeType)}`
}

async function fetchAndUploadStableWorkspaceUrl(rawUrl: string): Promise<string> {
  const requestUrl = resolveImageFetchRequestUrl(rawUrl)
  let response: Response
  try {
    response = await fetch(requestUrl, { credentials: 'include' })
  } catch (error) {
    throw new Error(`下载图片失败：${error instanceof Error ? error.message : '网络异常'}`)
  }
  if (!response.ok) {
    throw new Error(`下载图片失败：HTTP ${response.status}`)
  }
  const blob = await response.blob()
  if (!blob.size) {
    throw new Error('下载图片失败：响应为空')
  }
  const uploadFile = new File([blob], toUploadFilename(rawUrl, blob.type || 'image/jpeg'), {
    type: blob.type || 'image/jpeg',
  })
  let uploadResult: Awaited<ReturnType<typeof uploadWorkspaceImage>>
  try {
    uploadResult = await uploadWorkspaceImage(uploadFile)
  } catch (error) {
    throw new Error(`上传图片失败：${error instanceof Error ? error.message : '未知错误'}`)
  }
  const stableUrl = `${uploadResult?.data?.url ?? ''}`.trim()
  if (!stableUrl) {
    throw new Error('上传图片失败：服务端未返回可用链接')
  }
  if (!isStableWorkspaceUploadUrl(stableUrl)) {
    throw new Error('上传图片失败：返回链接不是 workspace uploads 地址')
  }
  return stableUrl
}

function appendOutputImageReferences(
  target: PersistedImageUrlReference[],
  prefix: string,
  nodes: Array<{ outputImages: Array<{ url: string }> }>,
) {
  nodes.forEach((node, nodeIndex) => {
    node.outputImages.forEach((output, outputIndex) => {
      target.push({
        path: `${prefix}[${nodeIndex}].outputImages[${outputIndex}].url`,
        getUrl: () => output.url,
        setUrl: (url) => {
          output.url = url
        },
      })
    })
  })
}

function collectPersistedImageUrlReferences(): PersistedImageUrlReference[] {
  const references: PersistedImageUrlReference[] = []
  messages.value.forEach((item, index) => {
    references.push({
      path: `messages[${index}].url`,
      getUrl: () => item.url,
      setUrl: (url) => {
        item.url = url
      },
    })
  })
  imageNodes.value.forEach((node, index) => {
    references.push({
      path: `imageNodes[${index}].sourceImageUrl`,
      getUrl: () => node.sourceImageUrl,
      setUrl: (url) => {
        node.sourceImageUrl = url
      },
    })
  })
  maskNodes.value.forEach((node, index) => {
    references.push({
      path: `maskNodes[${index}].sourceImageUrl`,
      getUrl: () => node.sourceImageUrl,
      setUrl: (url) => {
        node.sourceImageUrl = url
      },
    })
  })
  appendOutputImageReferences(references, 'imageNodes', imageNodes.value)
  appendOutputImageReferences(references, 'modelNodes', modelNodes.value)
  appendOutputImageReferences(references, 'viewpointNodes', viewpointNodes.value)
  appendOutputImageReferences(references, 'colorNodes', colorNodes.value)
  appendOutputImageReferences(references, 'enhanceNodes', enhanceNodes.value)
  appendOutputImageReferences(references, 'editNodes', editNodes.value)
  appendOutputImageReferences(references, 'fusionNodes', fusionNodes.value)
  appendOutputImageReferences(references, 'wearNodes', wearNodes.value)
  return references
}

async function stabilizePersistedImageUrlsBeforeSave() {
  const normalizeAndUploadTasks = new Map<string, Promise<string>>()
  const references = collectPersistedImageUrlReferences()
  for (const reference of references) {
    const original = reference.getUrl()
    const normalized = original.trim()
    if (!normalized) continue
    if (normalized !== original) {
      reference.setUrl(normalized)
    }
    if (isStableWorkspaceUploadUrl(normalized)) {
      continue
    }
    let uploadTask = normalizeAndUploadTasks.get(normalized)
    if (!uploadTask) {
      uploadTask = fetchAndUploadStableWorkspaceUrl(normalized)
      normalizeAndUploadTasks.set(normalized, uploadTask)
    }
    try {
      const stableUrl = await uploadTask
      reference.setUrl(stableUrl)
    } catch (error) {
      const reason = error instanceof Error && error.message ? error.message : '未知错误'
      throw new Error(`保存失败：${reference.path} 使用了不稳定图片链接，${reason}`)
    }
  }
}

let pendingSaveRecycleEntriesBuffer: PendingRecycleUploadEntry[] = []

function resolveCanvasWorkspaceUploadMeta(url: string): CanvasImageItem['uploadMeta'] | undefined {
  const path = parseCanvasImagePath(url)
  if (!path) return undefined
  const boardId = `${currentBoardId.value ?? ''}`.trim()
  if (!boardId) return undefined
  const encodedBoardId = encodeWorkspacePathSegment(boardId)
  const segments = path.split('/').filter(Boolean)
  if (segments.length < 6) return undefined
  let matched = false
  for (let i = 0; i + 4 < segments.length; i += 1) {
    if (segments[i] !== 'uploads' || segments[i + 1] !== 'workspace') continue
    if (segments[i + 3] !== encodedBoardId || segments[i + 4] !== 'content') continue
    matched = true
    break
  }
  if (!matched) return undefined
  return {
    kind: 'workspace-local-drop-upload',
    userId: Number(userStore.user?.id),
  }
}

function resolvePendingRecycleEntryForUrl(
  url: string,
  boardIdOverride?: string,
): PendingRecycleUploadEntry | null {
  const normalizedUrl = url.trim()
  if (!normalizedUrl || !isStableWorkspaceUploadUrl(normalizedUrl)) return null
  const currentUserID = Number(userStore.user?.id)
  const boardId = `${boardIdOverride ?? currentBoardId.value ?? ''}`.trim()
  if (!Number.isFinite(currentUserID) || currentUserID <= 0 || !boardId) return null
  const path = parseCanvasImagePath(normalizedUrl)
  const encodedBoardId = encodeWorkspacePathSegment(boardId)
  const segments = path?.split('/').filter(Boolean) ?? []
  let matched = false
  for (let i = 0; i + 4 < segments.length; i += 1) {
    if (segments[i] !== 'uploads' || segments[i + 1] !== 'workspace') continue
    if (segments[i + 3] !== encodedBoardId || segments[i + 4] !== 'content') continue
    matched = true
    break
  }
  if (!matched) return null
  return {
    url: normalizedUrl,
    userId: currentUserID,
    boardId,
  }
}

function queueWorkspaceUploadUrlsForRecycle(
  urls: Iterable<string>,
  boardIdOverride?: string,
) {
  const nextEntryMap = new Map(
    pendingRecycleEntries.value.map(entry => [entry.url, entry] as const),
  )
  for (const url of urls) {
    const entry = resolvePendingRecycleEntryForUrl(url, boardIdOverride)
    if (!entry) continue
    nextEntryMap.set(entry.url, entry)
  }
  setPendingRecycleEntries(Array.from(nextEntryMap.values()))
}

function queueDeletedCanvasUploadsForRecycle(deletedItems: CanvasImageItem[]) {
  if (!deletedItems.length) return
  queueWorkspaceUploadUrlsForRecycle(
    deletedItems.map(item => item.url),
  )
}

async function flushPendingRecycleUploads() {
  if (pendingRecycleScanInFlight) {
    pendingRecycleScanQueued = true
    return
  }
  clearPendingRecycleScanTimer()
  if (pendingRecycleEntries.value.length === 0) return
  pendingRecycleScanInFlight = true
  try {
    const activeBoardId = currentBoardId.value?.trim() || ''
    const activeSnapshotUrls = collectCanvasBoardImageUrls(createBoardSnapshotBase())
    const recycleTargets: PendingRecycleUploadEntry[] = []
    const retainedEntries: PendingRecycleUploadEntry[] = []

    pendingRecycleEntries.value.forEach((entry) => {
      const normalized = normalizePendingRecycleEntry(entry)
      if (!normalized) return
      const { url } = normalized

      if (activeSnapshotUrls.has(url.trim())) {
        return
      }

      if (hasHistoryImageUrlReference(url, {
        recentUndoSteps: RECENT_HISTORY_RECYCLE_GUARD_STEPS,
        includeRedo: true,
        includeCurrent: false,
      })) {
        retainedEntries.push(normalized)
        return
      }

      const excludeBoardId = normalized.boardId === activeBoardId ? activeBoardId : undefined
      if (isImageUrlReferencedInOtherCanvasBoardStates(url, excludeBoardId)) {
        retainedEntries.push(normalized)
        return
      }

      recycleTargets.push(normalized)
    })

    setPendingRecycleEntries(retainedEntries)
    if (!recycleTargets.length) return

    const results = await Promise.allSettled(recycleTargets.map(entry => uploadApi.recycle(entry.url, entry.boardId)))
    const failedCount = results.filter(result => result.status === 'rejected').length
    if (failedCount > 0) {
      message.warning(`有 ${failedCount} 张图片回收失败，已保留文件`)
      results.forEach((result, index) => {
        if (result.status === 'rejected') {
          retainedEntries.push(recycleTargets[index])
        }
      })
      setPendingRecycleEntries(retainedEntries)
    }
  } finally {
    pendingRecycleScanInFlight = false
    if (pendingRecycleScanQueued) {
      pendingRecycleScanQueued = false
      if (pendingRecycleEntries.value.length > 0) {
        void flushPendingRecycleUploads()
      }
    }
  }
}

async function schedulePendingRecycleScan() {
  if (pendingRecycleEntries.value.length === 0) return
  if (pendingRecycleScanTimer !== null) return
  pendingRecycleScanTimer = window.setTimeout(() => {
    pendingRecycleScanTimer = null
    void flushPendingRecycleUploads()
  }, 120)
}
const runningExecutionNodeKeys = ref<Set<string>>(new Set())
const executionNodeLastRunFailedMap = ref<Record<string, boolean>>({})
const executionNodeWatchSources = [
  modelNodes,
  viewpointNodes,
  colorNodes,
  enhanceNodes,
  editNodes,
  fusionNodes,
  wearNodes,
] as const
const connections = ref<CanvasConnection[]>([])
const processedMaskPreviewUrls = ref<Record<number, string>>({})
const styleInnovateDefaultStrength = 50
const styleInnovateStrength = ref(styleInnovateDefaultStrength)
const styleStrengthTrackRef = ref<HTMLElement | HTMLElement[] | null>(null)
const showStyleStrengthControl = ref(false)
const styleInnovateStrengthPct = computed(() => styleInnovateStrength.value)
const suppressStyleInnovateClick = ref(false)
const canvasImageRefs = new Map<number, HTMLImageElement>()
const styleStrengthMenuGap = 16
let styleStrengthLongPressTimer: number | null = null
let styleStrengthHideTimer: number | null = null
let styleStrengthDragging = false
let styleStrengthPointerY = 0

const tools = [
  { title: '图・文生图', path: '/user/picture', icon: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 14l4-4 5 5 5-5 4 4"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M19 3l-1 2-2 1 2 1 1 2 1-2 2-1-2-1-1-2z" fill="currentColor" stroke="none"/>' }
]

const canvasNodeMenuItems = CANVAS_NODE_MENU_ITEMS

const processedMaskPreviewSignatures = new Map<number, string>()
const {
  selectedConnectionId,
  renderedConnections,
  draftConnectionPath,
  draftConnectionTone,
  draftConnectionKind,
  reconnectingConnectionId,
  hasSelectedConnection,
  clearSelectedConnection,
  selectConnection,
  deleteSelectedConnection,
  findIncomingConnection,
  getIncomingConnections,
  hasIncomingPortConnection,
  getOutgoingConnections,
  hasOutgoingPortConnection,
  removeConnectionsForNode,
  removeIncomingConnectionsForPort,
  clearNodeConnectionError,
  clearAllNodeConnectionErrors,
  getNodeConnectionErrorPorts,
  triggerNodeConnectionError,
  clearConnectionDraft,
  beginConnectionReconnect,
  refreshRenderedConnections,
  scheduleRenderedConnectionsRefresh,
  startConnectionDrag,
  addGlobalConnectionListeners,
  removeGlobalConnectionListeners,
  disposeConnectionRuntime,
} = useCanvasConnections({
  connections,
  canvasLayoutRef,
  canvasX,
  canvasY,
  zoomLevel,
  lookupNodeByType: getCanvasConnectionNodeByType,
  isConnectionAllowed: (from, to) => isCanvasConnectionAllowed(from, to, getCanvasConnectionNodeByType),
  onConnectionNotAllowed: () => {
    message.warning('当前这两个端口不能连接')
  },
  onInvalidDirection: () => {
    message.warning('连线只能从输出口接到输入口')
  },
})

const {
  selectedIds,
  activeImageActionId,
  isDuplicateModifierPressed,
  activeImageNodeId,
  activePromptNodeId,
  activeModelNodeId,
  activeViewpointNodeId,
  activeMaskNodeId,
  activeColorNodeId,
  activeEditNodeId,
  activeFusionNodeId,
  activeEnhanceNodeId,
  activeWearNodeId,
  nodeContextMenu,
  isSelecting,
  selectionRect,
  activateImageNode,
  activatePromptNode,
  activateModelNode,
  activateViewpointNode,
  activateMaskNode,
  activateColorNode,
  activateEditNode,
  activateFusionNode,
  activateEnhanceNode,
  activateWearNode,
  isNodeSelected,
  isNodeDragging,
  closeNodeContextMenu,
  resetEditorUiState,
  onCanvasContextMenu,
  onCanvasMouseDown,
  onItemMouseDown,
  onItemContextMenu,
  onImageNodeMouseDown,
  onPromptNodeMouseDown,
  onModelNodeMouseDown,
  onViewpointNodeMouseDown,
  onMaskNodeMouseDown,
  onEditNodeMouseDown,
  onFusionNodeMouseDown,
  onEnhanceNodeMouseDown,
  onWearNodeMouseDown,
  onColorNodeMouseDown,
  onResizeMouseDown,
  readNodeImageDropPayload,
  handleDrop: handleCanvasDropCore,
  clearSelectedIds,
  removeSelectedNodeKey,
  removeSelectedId,
  focusSelectedItem,
} = useCanvasEditor({
  messages,
  imageNodes,
  promptNodes,
  modelNodes,
  viewpointNodes,
  maskNodes,
  colorNodes,
  enhanceNodes,
  editNodes,
  fusionNodes,
  wearNodes,
  activeTool,
  showStyleStrengthControl,
  canvasX,
  canvasY,
  zoomLevel,
  canvasLayoutRef,
  isBoardMenuOpen: () => isBoardMenuOpen.value,
  setBoardMenuOpen: (visible) => {
    isBoardMenuOpen.value = visible
  },
  isBoardRenaming: () => isRenamingBoard.value,
  isLeaveConfirmVisible: () => leaveConfirmVisible.value,
  confirmBoardRename: () => confirmBoardRename(),
  saveBoard: () => persistBoardStateNow(),
  resolveLeaveConfirm: (decision) => resolveLeaveConfirm(decision),
  updateZoom,
  getCanvasPointFromClient,
  createCanvasImageAt: createCanvasImageItemAt,
  uploadImage: uploadWorkspaceImage,
  resolveCanvasImageUploadMeta: (url) => resolveCanvasWorkspaceUploadMeta(url),
  importCanvasImageToImageNodeAtPoint: (clientX, clientY, item) => importCanvasImageToImageNodeAtPoint(clientX, clientY, item),
  importCanvasImageToMaskNodeAtPoint: (clientX, clientY, item) => importCanvasImageToMaskNodeAtPoint(clientX, clientY, item),
  deleteImageNode,
  deletePromptNode,
  deleteModelNode,
  deleteViewpointNode,
  deleteMaskNode,
  deleteColorNode,
  deleteEnhanceNode,
  deleteEditNode,
  deleteFusionNode,
  deleteWearNode,
  onDeleteCanvasImages: (deletedItems) => {
    queueDeletedCanvasUploadsForRecycle(deletedItems)
  },
  hasSelectedConnection,
  clearSelectedConnection,
  deleteSelectedConnection,
  beginHistoryBatch: () => beginCanvasHistoryBatch(),
  endHistoryBatch: (commit) => endCanvasHistoryBatch(commit),
  undo: () => undoCanvasSnapshot(),
  redo: () => redoCanvasSnapshot(),
  message,
  nodeImageDragMime,
})

const {
  getMaskCanvasWidth,
  getMaskStageHeight,
  setMaskCanvasRef,
  renderMaskNodeMask,
  onMaskCanvasMouseDown,
  dataUrlToFile,
  buildFullMaskDataUrl,
  refineExecutionMaskDataUrl,
  buildPreviewMaskDataUrlFromExecutionMask,
  openMaskNodeFilePicker,
  clearMaskNodeImage,
  clearMaskNodeMask,
  onMaskStageDrop: handleMaskStageDrop,
  importCanvasImageToMaskNodeAtPoint,
  removeMaskRuntimeFor,
  resetMaskRuntime,
} = useMaskNode({
  maskNodes,
  activeImageActionId,
  showStyleStrengthControl,
  message,
  uploadImage: uploadWorkspaceImage,
  loadImageRatioFromFile,
  loadImageRatioFromUrl,
  loadImageSizeFromUrl,
  activateMaskNode,
  closeNodeContextMenu,
  readNodeImageDropPayload,
})

const {
  getImageNodePreferredImage,
  openImageNodeFilePicker,
  resetImageNode,
  onImageNodeStageDrop: handleImageNodeStageDrop,
  importCanvasImageToImageNodeAtPoint,
} = useImageNode({
  imageNodes,
  message,
  uploadImage: uploadWorkspaceImage,
  loadImageRatioFromFile,
  loadImageRatioFromUrl,
  readNodeImageDropPayload,
  hasIncomingImageConnection: (nodeId) => hasIncomingPortConnection('image', nodeId, 'image'),
})

const {
  updateModelAngleYaw,
  updateModelAngleFraming,
  clampModelAngleYaw,
} = useModelNode()

const {
  setViewpointSelection,
} = useViewpointNode()

const {
  stopTaskPolling: stopNodeTaskPolling,
  resolvePreferredImageOutputNode,
  appendGeneratedImages,
  submitTaskAndPoll,
} = useGenerationPipeline({
  generating: nodeGenerating,
  messages,
  selectedIds,
  imageNodes,
  activeImageNodeId,
  message,
  loadImageRatioFromUrl,
  createCanvasImageAt: createCanvasImageItemAt,
  submitTask: taskApi.submit,
  getTask: taskApi.get,
})

const {
  stopTaskPolling: stopDirectTaskPolling,
  submitTaskAndPoll: submitDirectTaskAndPoll,
  createPendingGeneratedImages,
  replacePendingGeneratedImages,
  removePendingGeneratedImages,
} = useGenerationPipeline({
  generating: directGenerating,
  messages,
  selectedIds,
  imageNodes,
  activeImageNodeId,
  message,
  loadImageRatioFromUrl,
  createCanvasImageAt: createCanvasImageItemAt,
  submitTask: taskApi.submit,
  getTask: taskApi.get,
})

function getExecutionOutputNodeByType(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
) {
  const node = getCanvasConnectionNodeByType(nodeType, nodeId)
  if (!node || !('outputStatus' in node)) return null
  return node as CanvasExecutionOutputNodeItem
}

function setWorkflowNodeOutputStatus(
  node: { outputImages: { url: string }[]; outputStatus?: WorkflowNodeOutputStatus },
  status?: WorkflowNodeOutputStatus,
) {
  node.outputStatus = status ?? (node.outputImages.length > 0 ? 'ready' : 'idle')
}

function cloneNodeOutputImages(outputs: { url: string; ratio: number }[]) {
  return outputs.map(output => ({ ...output }))
}

function cloneModelAngleSpec(angleSpec: ModelAngleSpec) {
  return { ...angleSpec }
}

function toNormalizedColorNode(node: ColorNodeItem): ColorNodeItem {
  const colors = normalizeColorNodeColors(node.colors.map(color => normalizeHexColor(color)))
  return {
    ...node,
    outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
    colors,
    strength: Math.max(0, Math.min(100, Math.round(node.strength ?? DEFAULT_COLOR_STRENGTH))),
    outputImages: cloneNodeOutputImages(node.outputImages),
  }
}

function markExecutionNodeDirty(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
  visited = new Set<string>(),
) {
  const visitKey = `${nodeType}:${nodeId}`
  if (visited.has(visitKey)) return
  visited.add(visitKey)

  const node = getExecutionOutputNodeByType(nodeType, nodeId)
  if (!node) return
  setWorkflowNodeOutputStatus(node, node.outputImages.length > 0 ? 'stale' : 'idle')
  setExecutionNodeLastRunFailed(nodeType, nodeId, false)

  getOutgoingConnections(nodeType, nodeId, 'image').forEach((connection) => {
    if (isCanvasExecutionOutputNodeType(connection.toNodeType)) {
      markExecutionNodeDirty(connection.toNodeType, connection.toNodeId, visited)
      return
    }
    if (connection.toNodeType === 'mask') {
      markConnectedExecutionTargetsDirty('mask', connection.toNodeId, visited)
    }
  })
}

function markConnectedExecutionTargetsDirty(
  fromNodeType: CanvasConnectionNodeType,
  fromNodeId: number,
  visited = new Set<string>(),
) {
  const outgoingPort =
    fromNodeType === 'prompt'
      ? 'prompt'
      : fromNodeType === 'mask'
        ? 'mask'
        : 'image'
  getOutgoingConnections(fromNodeType, fromNodeId, outgoingPort).forEach((connection) => {
    if (isCanvasExecutionOutputNodeType(connection.toNodeType)) {
      markExecutionNodeDirty(connection.toNodeType, connection.toNodeId, visited)
      return
    }
    if (connection.toNodeType === 'mask') {
      markConnectedExecutionTargetsDirty('mask', connection.toNodeId, visited)
    }
  })
}

function appendWorkflowNodeOutputs(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
  urls: string[],
) {
  if (urls.length === 0) return
  const node = getExecutionOutputNodeByType(nodeType, nodeId)
  if (!node) return
  setWorkflowNodeOutputStatus(node, 'ready')
  const startIndex = node.outputImages.length
  urls.forEach((url) => {
    node.outputImages.push({ url, ratio: 1 })
  })
  urls.forEach((url, offset) => {
    const outputIndex = startIndex + offset
    void loadImageRatioFromUrl(url).then((ratio) => {
      if (!ratio) return
      const targetNode = getExecutionOutputNodeByType(nodeType, nodeId)
      if (!targetNode) return
      const output = targetNode.outputImages[outputIndex]
      if (!output || output.url !== url) return
      output.ratio = ratio
    })
  })
  setExecutionNodeLastRunFailed(nodeType, nodeId, false)
}

function getExecutionNodeStateKey(nodeType: CanvasExecutionOutputNodeType, nodeId: number): string {
  return `${nodeType}:${nodeId}`
}

function setExecutionNodeRunning(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
  running: boolean,
) {
  const key = getExecutionNodeStateKey(nodeType, nodeId)
  if (running) {
    if (runningExecutionNodeKeys.value.has(key)) return
    const nextKeys = new Set(runningExecutionNodeKeys.value)
    nextKeys.add(key)
    runningExecutionNodeKeys.value = nextKeys
    return
  }
  if (!runningExecutionNodeKeys.value.has(key)) return
  const nextKeys = new Set(runningExecutionNodeKeys.value)
  nextKeys.delete(key)
  runningExecutionNodeKeys.value = nextKeys
}

function isExecutionNodeRunning(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
): boolean {
  return runningExecutionNodeKeys.value.has(getExecutionNodeStateKey(nodeType, nodeId))
}

function setExecutionNodeLastRunFailed(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
  failed: boolean,
) {
  const key = getExecutionNodeStateKey(nodeType, nodeId)
  if (!failed) {
    if (executionNodeLastRunFailedMap.value[key] == null) return
    const nextMap = { ...executionNodeLastRunFailedMap.value }
    delete nextMap[key]
    executionNodeLastRunFailedMap.value = nextMap
    return
  }
  executionNodeLastRunFailedMap.value = {
    ...executionNodeLastRunFailedMap.value,
    [key]: true,
  }
}

function isExecutionNodeLastRunFailed(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
): boolean {
  return !!executionNodeLastRunFailedMap.value[getExecutionNodeStateKey(nodeType, nodeId)]
}

function getViewpointLabel(key: string): string {
  return describeViewpointOption(key)
}

function getViewpointLightingLabel(key: ViewpointNodeItem['lighting']): string {
  return viewpointLightingLabels[key] || key
}

function loadImageRatioFromUrl(url: string): Promise<number | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      if (!img.naturalWidth || !img.naturalHeight) {
        resolve(null)
        return
      }
      resolve(img.naturalHeight / img.naturalWidth)
    }
    img.onerror = () => resolve(null)
    img.src = url
  })
}

function loadImageSizeFromUrl(url: string): Promise<{ width: number; height: number } | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      if (!img.naturalWidth || !img.naturalHeight) {
        resolve(null)
        return
      }
      resolve({ width: img.naturalWidth, height: img.naturalHeight })
    }
    img.onerror = () => resolve(null)
    img.src = url
  })
}

function loadImageRatioFromFile(file: File): Promise<number | null> {
  return new Promise((resolve) => {
    const localUrl = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      const ratio = img.naturalWidth && img.naturalHeight
        ? img.naturalHeight / img.naturalWidth
        : null
      URL.revokeObjectURL(localUrl)
      resolve(ratio)
    }
    img.onerror = () => {
      URL.revokeObjectURL(localUrl)
      resolve(null)
    }
    img.src = localUrl
  })
}

function resetEditorState() {
  messages.value = []
  resetEditorUiState()
  clearAllNodeConnectionErrors()
  resetMaskRuntime()
  imageNodes.value = []
  promptNodes.value = []
  modelNodes.value = []
  viewpointNodes.value = []
  maskNodes.value = []
  colorNodes.value = []
  enhanceNodes.value = []
  editNodes.value = []
  fusionNodes.value = []
  wearNodes.value = []
  runningExecutionNodeKeys.value = new Set()
  executionNodeLastRunFailedMap.value = {}
  connections.value = []
  processedMaskPreviewUrls.value = {}
  processedMaskPreviewSignatures.clear()
  clearConnectionDraft()
  userPrompt.value = ''
  resetViewport()
}

let suspendWorkflowCacheReactions = false
let previousPromptSignatureMap: Record<string, string> = {}
let previousImageSourceSignatureMap: Record<string, string> = {}
let previousMaskSignatureMap: Record<string, string> = {}
let previousExecutionInputSignatureMap: Record<string, string> = {}
let previousExecutionOutputSignatureMap: Record<string, string> = {}
let previousConnectionTargetSignatureMap: Record<string, string> = {}

function buildPromptSignatureMap(): Record<string, string> {
  return promptNodes.value.reduce<Record<string, string>>((acc, node) => {
    acc[String(node.id)] = node.text
    return acc
  }, {})
}

function buildImageSourceSignatureMap(): Record<string, string> {
  return imageNodes.value.reduce<Record<string, string>>((acc, node) => {
    const latestOutput = node.outputImages[node.outputImages.length - 1]
    acc[String(node.id)] = [
      node.sourceImageUrl,
      String(node.sourceImageRatio || 1),
      latestOutput?.url || '',
      String(latestOutput?.ratio || 1),
      String(node.outputImages.length),
    ].join('|')
    return acc
  }, {})
}

function buildMaskSignatureMap(): Record<string, string> {
  return maskNodes.value.reduce<Record<string, string>>((acc, node) => {
    acc[String(node.id)] = [
      node.sourceImageUrl,
      String(node.sourceImageRatio || 1),
      node.executionMaskDataUrl || '',
      node.maskDataUrl,
      String(clampMaskRefineAmount(node.refineAmount ?? 0)),
      node.invertMask ? '1' : '0',
    ].join('|')
    return acc
  }, {})
}

function buildExecutionInputSignatureMap(): Record<string, string> {
  const result: Record<string, string> = {}

  modelNodes.value.forEach((node) => {
    result[`model:${node.id}`] = [
      `${node.angleSpec.yaw}:${node.angleSpec.framing}`,
      String(node.modelStrength),
      String(node.poseStrength),
      node.background,
    ].join('|')
  })

  viewpointNodes.value.forEach((node) => {
    result[`viewpoint:${node.id}`] = [
      node.selectedViewpoint,
      node.lighting,
    ].join('|')
  })

  colorNodes.value.forEach((node) => {
    result[`color:${node.id}`] = [
      node.colors.join(','),
      String(node.strength),
    ].join('|')
  })

  editNodes.value.forEach((node) => {
    result[`edit:${node.id}`] = node.mode
  })

  fusionNodes.value.forEach((node) => {
    result[`fusion:${node.id}`] = String(node.blendRatio)
  })

  enhanceNodes.value.forEach((node) => {
    result[`enhance:${node.id}`] = [
      node.mode,
      node.upscaleMode,
      String(node.upscaleFactor),
      node.outputFormat,
      String(node.outputQuality),
      String(node.extendTop),
      String(node.extendRight),
      String(node.extendBottom),
      String(node.extendLeft),
    ].join('|')
  })

  wearNodes.value.forEach((node) => {
    result[`wear:${node.id}`] = [
      node.tileMode,
      String(node.rotation),
      String(node.scale),
      String(node.opacity),
    ].join('|')
  })

  return result
}

function buildExecutionOutputSignatureMap(): Record<string, string> {
  const result: Record<string, string> = {}
  const assign = (nodeType: CanvasExecutionOutputNodeType, nodes: { id: number; outputImages: { url: string; ratio: number }[] }[]) => {
    nodes.forEach((node) => {
      const latestOutput = node.outputImages[node.outputImages.length - 1]
      result[`${nodeType}:${node.id}`] = [
        latestOutput?.url || '',
        String(latestOutput?.ratio || 1),
        String(node.outputImages.length),
      ].join('|')
    })
  }

  assign('model', modelNodes.value)
  assign('viewpoint', viewpointNodes.value)
  assign('color', colorNodes.value)
  assign('edit', editNodes.value)
  assign('fusion', fusionNodes.value)
  assign('enhance', enhanceNodes.value)
  assign('wear', wearNodes.value)
  return result
}

function buildConnectionTargetSignatureMap(): Record<string, string> {
  const grouped: Record<string, string[]> = {}
  connections.value.forEach((connection) => {
    const targetKey = `${connection.toNodeType}:${connection.toNodeId}`
    if (!grouped[targetKey]) {
      grouped[targetKey] = []
    }
    grouped[targetKey].push(`${connection.fromNodeType}:${connection.fromNodeId}:${connection.fromPort}->${connection.toPort}`)
  })
  return Object.entries(grouped).reduce<Record<string, string>>((acc, [key, values]) => {
    acc[key] = values.sort().join('|')
    return acc
  }, {})
}

function primeWorkflowCacheSignatureMaps() {
  previousPromptSignatureMap = buildPromptSignatureMap()
  previousImageSourceSignatureMap = buildImageSourceSignatureMap()
  previousMaskSignatureMap = buildMaskSignatureMap()
  previousExecutionInputSignatureMap = buildExecutionInputSignatureMap()
  previousExecutionOutputSignatureMap = buildExecutionOutputSignatureMap()
  previousConnectionTargetSignatureMap = buildConnectionTargetSignatureMap()
}

function forEachChangedSignature(
  previousMap: Record<string, string>,
  nextMap: Record<string, string>,
  onChanged: (key: string) => void,
) {
  const keys = new Set([...Object.keys(previousMap), ...Object.keys(nextMap)])
  keys.forEach((key) => {
    if (previousMap[key] !== nextMap[key]) {
      onChanged(key)
    }
  })
}

let resetHistoryBaseline: ((snapshot?: CanvasBoardSnapshot | null) => void) | null = null

async function applyBoardSnapshot(state: CanvasBoardSnapshot | null) {
  suspendWorkflowCacheReactions = true
  try {
    if (!state) {
      resetEditorState()
      await nextTick()
      primeWorkflowCacheSignatureMaps()
      resetHistoryBaseline?.(createBoardSnapshotBase())
      return
    }

    messages.value = state.messages.map(item => ({ ...item }))
    resetEditorUiState()
    clearAllNodeConnectionErrors()
    runningExecutionNodeKeys.value = new Set()
    executionNodeLastRunFailedMap.value = {}

    imageNodes.value = state.imageNodes.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))

    promptNodes.value = state.promptNodes.map(node => ({ ...node }))

    modelNodes.value = state.modelNodes.map(node => ({
      ...node,
      modelStrength: clampModelReferenceStrength(node.modelStrength),
      poseStrength: clampModelPoseStrength(node.poseStrength),
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      angleSpec: cloneModelAngleSpec(node.angleSpec),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    viewpointNodes.value = state.viewpointNodes.map(node => ({
      ...node,
      selectedViewpoint: normalizeViewpointSelectionByCategory(node.activeCategory, node.selectedViewpoint),
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    maskNodes.value = state.maskNodes.map(node => ({
      ...node,
      refineAmount: clampMaskRefineAmount(node.refineAmount ?? 0),
      invertMask: !!node.invertMask,
    }))
    enhanceNodes.value = state.enhanceNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputQuality: Math.max(30, Math.min(100, Math.round(node.outputQuality ?? 95))),
      extendTop: clampEnhanceExtendPercent(node.extendTop ?? ENHANCE_EXTEND_PERCENT_DEFAULT),
      extendRight: clampEnhanceExtendPercent(node.extendRight ?? ENHANCE_EXTEND_PERCENT_DEFAULT),
      extendBottom: clampEnhanceExtendPercent(node.extendBottom ?? ENHANCE_EXTEND_PERCENT_DEFAULT),
      extendLeft: clampEnhanceExtendPercent(node.extendLeft ?? ENHANCE_EXTEND_PERCENT_DEFAULT),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    editNodes.value = state.editNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    fusionNodes.value = state.fusionNodes.map(node => ({
      ...node,
      blendRatio: Math.max(0, Math.min(100, Math.round(node.blendRatio ?? 50))),
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    wearNodes.value = state.wearNodes.map(node => ({
      ...node,
      tileMode: normalizeWearTileMode(node.tileMode),
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      rotation: clampWearRotation(node.rotation),
      scale: clampWearScale(node.scale),
      opacity: clampWearOpacity(node.opacity),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    colorNodes.value = state.colorNodes.map(toNormalizedColorNode)
    connections.value = state.connections.map(connection => ({ ...connection }))
    clearSelectedConnection()
    clearConnectionDraft()
    setViewport(state.viewport)

    await nextTick()
    maskNodes.value.forEach(node => renderMaskNodeMask(node))
    scheduleRenderedConnectionsRefresh()
    primeWorkflowCacheSignatureMaps()
    resetHistoryBaseline?.(createBoardSnapshotBase())
  } finally {
    suspendWorkflowCacheReactions = false
  }
}

function createBoardSnapshotBase(): CanvasBoardSnapshot {
  return {
    messages: messages.value.map(item => ({ ...item })),
    imageNodes: imageNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    promptNodes: promptNodes.value.map(node => ({ ...node })),
    modelNodes: modelNodes.value.map(node => ({
      ...node,
      angleSpec: cloneModelAngleSpec(node.angleSpec),
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    viewpointNodes: viewpointNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    maskNodes: maskNodes.value.map(node => ({ ...node })),
    colorNodes: colorNodes.value.map(toNormalizedColorNode),
    enhanceNodes: enhanceNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    editNodes: editNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    fusionNodes: fusionNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    wearNodes: wearNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    connections: connections.value.map(connection => ({ ...connection })),
    viewport: createViewportSnapshot(),
  }
}

function collectRemovedWorkspaceUploadEntries(
  previousSnapshot: CanvasBoardSnapshot | null | undefined,
  nextSnapshot: CanvasBoardSnapshot | null | undefined,
  boardId: string,
): PendingRecycleUploadEntry[] {
  const previousUrls = collectCanvasBoardImageUrls(previousSnapshot)
  const nextUrls = collectCanvasBoardImageUrls(nextSnapshot)
  const removedEntries: PendingRecycleUploadEntry[] = []

  previousUrls.forEach((url) => {
    if (nextUrls.has(url)) return
    const entry = resolvePendingRecycleEntryForUrl(url, boardId)
    if (entry) {
      removedEntries.push(entry)
    }
  })

  return removedEntries
}

async function prepareBoardSnapshotBeforePersist() {
  const boardId = `${currentBoardId.value ?? ''}`.trim()
  const previousSnapshot = boardId ? getCanvasBoardState(boardId) : null
  await stabilizePersistedImageUrlsBeforeSave()
  pendingSaveRecycleEntriesBuffer = boardId
    ? collectRemovedWorkspaceUploadEntries(previousSnapshot, createBoardSnapshotBase(), boardId)
    : []
}

async function handleBoardPersisted() {
  if (pendingSaveRecycleEntriesBuffer.length > 0) {
    setPendingRecycleEntries([
      ...pendingRecycleEntries.value,
      ...pendingSaveRecycleEntriesBuffer,
    ])
    pendingSaveRecycleEntriesBuffer = []
  }
  await schedulePendingRecycleScan()
}

const {
  notifySnapshotChanged,
  beginBatch: beginCanvasHistoryBatch,
  endBatch: endCanvasHistoryBatch,
  resetHistoryBaseline: syncCanvasHistoryBaseline,
  undo: undoCanvasSnapshot,
  redo: redoCanvasSnapshot,
  hasImageUrlReference: hasHistoryImageUrlReference,
} = useCanvasHistory({
  createSnapshot: createBoardSnapshotBase,
  applySnapshot: applyBoardSnapshot,
  maxEntries: RECENT_HISTORY_RECYCLE_GUARD_STEPS,
})

resetHistoryBaseline = syncCanvasHistoryBaseline

const {
  currentBoardId,
  currentBoardName,
  boardOptions,
  isBoardMenuOpen,
  isRenamingBoard,
  boardNameDraft,
  boardNameInputRef,
  isBoardDirty,
  leaveConfirmVisible,
  toggleBoardMenu,
  startBoardRename,
  cancelBoardRename,
  confirmBoardRename,
  handleBoardSelect,
  resolveLeaveConfirm,
  confirmSaveBeforeLeave,
  persistBoardStateNow,
  markBoardDirty,
} = useCanvasBoard({
  defaultBoardName: defaultCanvasName,
  message,
  createSnapshot: createBoardSnapshotBase,
  applySnapshot: applyBoardSnapshot,
  prepareSnapshotBeforePersist: prepareBoardSnapshotBeforePersist,
  onPersisted: handleBoardPersisted,
})

primeWorkflowCacheSignatureMaps()

function setBoardNameInputRef(element: HTMLInputElement | null) {
  boardNameInputRef.value = element
}

async function navigateAway(path: string) {
  if (path === route.path) return
  const canLeave = await confirmSaveBeforeLeave()
  if (!canLeave) return
  void router.push(path)
}

watch(
  promptNodes,
  () => {
    const nextMap = buildPromptSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousPromptSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousPromptSignatureMap, nextMap, (key) => {
      const nodeId = Number(key)
      if (Number.isFinite(nodeId)) {
        markConnectedExecutionTargetsDirty('prompt', nodeId)
      }
    })
    previousPromptSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  imageNodes,
  () => {
    const nextMap = buildImageSourceSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousImageSourceSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousImageSourceSignatureMap, nextMap, (key) => {
      const nodeId = Number(key)
      if (Number.isFinite(nodeId)) {
        markConnectedExecutionTargetsDirty('image', nodeId)
      }
    })
    previousImageSourceSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  maskNodes,
  () => {
    const nextMap = buildMaskSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousMaskSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousMaskSignatureMap, nextMap, (key) => {
      const nodeId = Number(key)
      if (Number.isFinite(nodeId)) {
        markConnectedExecutionTargetsDirty('mask', nodeId)
      }
    })
    previousMaskSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  executionNodeWatchSources,
  () => {
    const nextMap = buildExecutionInputSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousExecutionInputSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousExecutionInputSignatureMap, nextMap, (key) => {
      const [nodeType, rawNodeId] = key.split(':')
      const nodeId = Number(rawNodeId)
      if (isCanvasExecutionOutputNodeType(nodeType) && Number.isFinite(nodeId)) {
        markExecutionNodeDirty(nodeType, nodeId)
      }
    })
    previousExecutionInputSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  executionNodeWatchSources,
  () => {
    const nextMap = buildExecutionOutputSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousExecutionOutputSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousExecutionOutputSignatureMap, nextMap, (key) => {
      const [nodeType, rawNodeId] = key.split(':')
      const nodeId = Number(rawNodeId)
      if (!isCanvasExecutionOutputNodeType(nodeType) || !Number.isFinite(nodeId)) return
      const node = getExecutionOutputNodeByType(nodeType, nodeId)
      if (!node) return
      setWorkflowNodeOutputStatus(node)
      markConnectedExecutionTargetsDirty(nodeType, nodeId)
    })
    previousExecutionOutputSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  connections,
  () => {
    const nextMap = buildConnectionTargetSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousConnectionTargetSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousConnectionTargetSignatureMap, nextMap, (key) => {
      const [nodeType, rawNodeId] = key.split(':')
      const nodeId = Number(rawNodeId)
      if (!Number.isFinite(nodeId)) return
      if (isCanvasExecutionOutputNodeType(nodeType)) {
        markExecutionNodeDirty(nodeType, nodeId)
        return
      }
      if (nodeType === 'mask') {
        markConnectedExecutionTargetsDirty('mask', nodeId)
      }
    })
    previousConnectionTargetSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  [messages, imageNodes, promptNodes, modelNodes, viewpointNodes, maskNodes, colorNodes, enhanceNodes, editNodes, fusionNodes, wearNodes, connections],
  () => {
    markBoardDirty()
    notifySnapshotChanged()
  },
  { deep: true }
)

watch(
  () => userStore.user?.id,
  () => {
    setPendingRecycleEntries(readPendingRecycleEntries())
  },
  { immediate: true },
)

watch(
  [connections, imageNodes, promptNodes, modelNodes, viewpointNodes, maskNodes, colorNodes, enhanceNodes, editNodes, fusionNodes, wearNodes, canvasX, canvasY, zoomLevel],
  () => {
    refreshRenderedConnections()
  },
  { deep: true, flush: 'post' }
)

watch(
  maskNodes,
  (nodes) => {
    const nextIds = new Set(nodes.map(node => node.id))
    Object.keys(processedMaskPreviewUrls.value).forEach((rawId) => {
      const id = Number(rawId)
      if (!nextIds.has(id)) {
        clearProcessedMaskPreview(id)
      }
    })
    nodes.forEach((node) => {
      void refreshProcessedMaskPreview(node)
    })
  },
  { deep: true, immediate: true }
)

function deletePromptNode(id: number) {
  deleteNodeFromCollection(promptNodes, 'prompt', id, () => {
    if (activePromptNodeId.value === id) {
      activePromptNodeId.value = null
    }
  })
}

function deleteModelNode(id: number) {
  deleteNodeFromCollection(modelNodes, 'model', id, () => {
    if (activeModelNodeId.value === id) {
      activeModelNodeId.value = null
    }
  })
}

function deleteViewpointNode(id: number) {
  deleteNodeFromCollection(viewpointNodes, 'viewpoint', id, () => {
    if (activeViewpointNodeId.value === id) {
      activeViewpointNodeId.value = null
    }
  })
}

function deleteMaskNode(id: number) {
  deleteNodeFromCollection(maskNodes, 'mask', id, () => {
    removeMaskRuntimeFor(id)
    clearProcessedMaskPreview(id)
    if (activeMaskNodeId.value === id) {
      activeMaskNodeId.value = null
    }
  })
}

function deleteImageNode(id: number) {
  deleteNodeFromCollection(imageNodes, 'image', id, () => {
    if (activeImageNodeId.value === id) {
      activeImageNodeId.value = null
    }
  })
}

function deleteColorNode(id: number) {
  deleteNodeFromCollection(colorNodes, 'color', id, () => {
    if (activeColorNodeId.value === id) {
      activeColorNodeId.value = null
    }
  })
}

function deleteEnhanceNode(id: number) {
  deleteNodeFromCollection(enhanceNodes, 'enhance', id, () => {
    if (activeEnhanceNodeId.value === id) {
      activeEnhanceNodeId.value = null
    }
  })
}

function deleteEditNode(id: number) {
  deleteNodeFromCollection(editNodes, 'edit', id, () => {
    if (activeEditNodeId.value === id) {
      activeEditNodeId.value = null
    }
  })
}

function deleteFusionNode(id: number) {
  deleteNodeFromCollection(fusionNodes, 'fusion', id, () => {
    if (activeFusionNodeId.value === id) {
      activeFusionNodeId.value = null
    }
  })
}

function deleteWearNode(id: number) {
  deleteNodeFromCollection(wearNodes, 'wear', id, () => {
    if (activeWearNodeId.value === id) {
      activeWearNodeId.value = null
    }
  })
}

function createNodeFromContextMenu<T extends { id: number }>(
  createAt: (point: { x: number; y: number }) => T,
  append: (node: T) => void,
  activate: (id: number) => void,
  afterCreate?: (node: T) => void,
) {
  const { clientX, clientY } = nodeContextMenu.value
  const point = getCanvasPointFromClient(clientX, clientY)
  const newNode = createAt(point)
  append(newNode)
  activate(newNode.id)
  afterCreate?.(newNode)
  closeNodeContextMenu()
}

function collectWorkspaceUploadUrlsFromNodeLike(
  node: { sourceImageUrl?: string; outputImages?: Array<{ url: string }> },
): string[] {
  const urls = new Set<string>()
  const sourceImageUrl = `${node.sourceImageUrl ?? ''}`.trim()
  if (sourceImageUrl) {
    urls.add(sourceImageUrl)
  }
  node.outputImages?.forEach((output) => {
    const outputUrl = `${output.url ?? ''}`.trim()
    if (outputUrl) {
      urls.add(outputUrl)
    }
  })
  return Array.from(urls)
}

function deleteNodeFromCollection<T extends { id: number }>(
  collection: Ref<T[]>,
  nodeType: CanvasConnectionNodeType,
  id: number,
  afterDelete?: () => void,
) {
  const removedNode = collection.value.find(node => node.id === id) as
    | (T & { sourceImageUrl?: string; outputImages?: Array<{ url: string }> })
    | undefined
  if (removedNode) {
    queueWorkspaceUploadUrlsForRecycle(collectWorkspaceUploadUrlsFromNodeLike(removedNode))
  }
  collection.value = collection.value.filter(node => node.id !== id)
  clearNodeConnectionError(nodeType, id)
  if (isCanvasExecutionOutputNodeType(nodeType)) {
    setExecutionNodeLastRunFailed(nodeType, id, false)
  }
  removeConnectionsForNode(nodeType, id)
  removeSelectedNodeKey(nodeType, id)
  afterDelete?.()
}

function buildNodeCreateHandler<T extends { id: number }>(
  createAt: (point: { x: number; y: number }) => T,
  collection: Ref<T[]>,
  activate: (id: number) => void,
  afterCreate?: (node: T) => void,
) {
  return () => createNodeFromContextMenu(
    createAt,
    (newNode) => collection.value.push(newNode),
    activate,
    afterCreate,
  )
}

const nodeContextMenuHandlers: Record<CanvasNodeCreateActionKey, () => void> = {
  createImageNode: buildNodeCreateHandler(createImageNodeAt, imageNodes, activateImageNode),
  createPromptNode: buildNodeCreateHandler(createPromptNodeAt, promptNodes, activatePromptNode),
  createModelNode: buildNodeCreateHandler(createModelNodeAt, modelNodes, activateModelNode),
  createViewpointNode: buildNodeCreateHandler(createViewpointNodeAt, viewpointNodes, activateViewpointNode),
  createMaskNode: buildNodeCreateHandler(createMaskNodeAt, maskNodes, activateMaskNode),
  createEditNode: buildNodeCreateHandler(createEditNodeAt, editNodes, activateEditNode),
  createFusionNode: buildNodeCreateHandler(createFusionNodeAt, fusionNodes, activateFusionNode),
  createEnhanceNode: buildNodeCreateHandler(createEnhanceNodeAt, enhanceNodes, activateEnhanceNode),
  createWearNode: buildNodeCreateHandler(createWearNodeAt, wearNodes, activateWearNode),
  createColorNode: buildNodeCreateHandler(createColorNodeAt, colorNodes, activateColorNode),
}

function handleNodeMenuAction(actionKey: CanvasNodeCreateActionKey) {
  nodeContextMenuHandlers[actionKey]?.()
}

function handleConnectionPortMouseDown(
  event: MouseEvent,
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  port: CanvasConnectionPort,
) {
  clearSelectedIds()
  clearSelectedConnection()
  closeNodeContextMenu()
  const target = event.currentTarget as HTMLElement | null
  const direction = target?.dataset.portDirection
  if (direction === 'input') {
    const existingConnection = findIncomingConnection(nodeType, nodeId, port)
    if (!existingConnection) return
    beginConnectionReconnect(existingConnection.id)
    return
  }
  startConnectionDrag(event, nodeType, nodeId, port)
}

function onConnectionPathMouseDown(event: MouseEvent, connectionId: number) {
  event.preventDefault()
  event.stopPropagation()
  clearSelectedIds()
  closeNodeContextMenu()
  clearSelectedConnection()
  selectConnection(connectionId)
}

function onMaskStageDrop(e: DragEvent, node: MaskNodeItem) {
  isDraggingFile.value = false
  handleMaskStageDrop(e, node)
}

function onImageNodeStageDrop(e: DragEvent, node: ImageNodeItem) {
  isDraggingFile.value = false
  handleImageNodeStageDrop(e, node)
}

async function handleDrop(e: DragEvent) {
  isDraggingFile.value = false
  await handleCanvasDropCore(e)
}

function clearStyleStrengthLongPressTimer() {
  if (!styleStrengthLongPressTimer) return
  clearTimeout(styleStrengthLongPressTimer)
  styleStrengthLongPressTimer = null
}

function clearStyleStrengthHideTimer() {
  if (!styleStrengthHideTimer) return
  clearTimeout(styleStrengthHideTimer)
  styleStrengthHideTimer = null
}

function removeStyleStrengthGlobalListeners() {
  window.removeEventListener('mousemove', onStyleInnovateMouseMove, true)
  window.removeEventListener('mouseup', onStyleInnovateMouseUp, true)
}

function addStyleStrengthGlobalListeners() {
  window.addEventListener('mousemove', onStyleInnovateMouseMove, true)
  window.addEventListener('mouseup', onStyleInnovateMouseUp, true)
}

function getStyleStrengthTrackEl(): HTMLElement | null {
  if (Array.isArray(styleStrengthTrackRef.value)) {
    return styleStrengthTrackRef.value[0] ?? null
  }
  return styleStrengthTrackRef.value
}

function setStyleStrengthTrackRef(el: Element | ComponentPublicInstance | null) {
  const targetEl =
    el instanceof HTMLElement
      ? el
      : el && '$el' in el && (el as ComponentPublicInstance).$el instanceof HTMLElement
        ? (el as ComponentPublicInstance).$el
        : null
  styleStrengthTrackRef.value = targetEl
}

function setCanvasImageRef(id: number, el: Element | ComponentPublicInstance | null) {
  const imageEl =
    el instanceof HTMLImageElement
      ? el
      : el && '$el' in el && (el as ComponentPublicInstance).$el instanceof HTMLImageElement
        ? (el as ComponentPublicInstance).$el
        : null

  if (imageEl) {
    canvasImageRefs.set(id, imageEl)
    return
  }
  canvasImageRefs.delete(id)
}

function getStyleStrengthMenuStyle(item: CanvasImageItem): { top: string; left: string } {
  const imageEl = canvasImageRefs.get(item.id)
  if (!imageEl) {
    return { top: '8px', left: `calc(100% + ${styleStrengthMenuGap}px)` }
  }
  const top = Math.max(0, imageEl.offsetTop)
  const left = Math.max(0, imageEl.offsetLeft + imageEl.clientWidth + styleStrengthMenuGap)
  return {
    top: `${top}px`,
    left: `${left}px`,
  }
}

function getActiveCanvasImageItem(): CanvasImageItem | null {
  const id = activeImageActionId.value ?? selectedIds.value[0] ?? null
  if (id === null) return null
  return messages.value.find(item => item.id === id) ?? null
}

function updateStyleStrengthFromClientY(clientY: number) {
  const track = getStyleStrengthTrackEl()
  if (!track) return
  const rect = track.getBoundingClientRect()
  if (rect.height <= 0) return
  const ratio = 1 - (clientY - rect.top) / rect.height
  const clamped = Math.max(0, Math.min(1, ratio))
  styleInnovateStrength.value = Math.max(0, Math.min(100, Math.round(clamped * 100)))
}

function beginStyleStrengthDragging(clientY: number) {
  showStyleStrengthControl.value = true
  styleStrengthDragging = true
  styleStrengthPointerY = clientY
  addStyleStrengthGlobalListeners()
  void nextTick(() => updateStyleStrengthFromClientY(styleStrengthPointerY))
}

function onStyleInnovateMouseDown(e: MouseEvent) {
  if (directGenerating.value) return
  clearStyleStrengthHideTimer()
  clearStyleStrengthLongPressTimer()
  suppressStyleInnovateClick.value = false
  styleStrengthDragging = false
  styleStrengthPointerY = e.clientY

  styleStrengthLongPressTimer = window.setTimeout(() => {
    suppressStyleInnovateClick.value = true
    beginStyleStrengthDragging(styleStrengthPointerY)
  }, 260)

  addStyleStrengthGlobalListeners()
}

function onStyleInnovateMouseMove(e: MouseEvent) {
  styleStrengthPointerY = e.clientY
  if (!styleStrengthDragging || !showStyleStrengthControl.value) return
  updateStyleStrengthFromClientY(e.clientY)
}

function onStyleInnovateMouseUp() {
  clearStyleStrengthLongPressTimer()
  removeStyleStrengthGlobalListeners()
  if (!styleStrengthDragging) return
  styleStrengthDragging = false
  clearStyleStrengthHideTimer()
  showStyleStrengthControl.value = false

  const target = getActiveCanvasImageItem()
  if (!target) return
  void runCanvasAction('style-innovate', target)
}

function onStyleStrengthTrackMouseDown(e: MouseEvent) {
  clearStyleStrengthHideTimer()
  beginStyleStrengthDragging(e.clientY)
}

function onStyleInnovateClick(item: CanvasImageItem) {
  if (suppressStyleInnovateClick.value) {
    suppressStyleInnovateClick.value = false
    return
  }
  void runCanvasAction('style-innovate', item)
}

const selectedPreviewItems = computed(() => {
  const messageMap = new Map(messages.value.map(item => [item.id, item]))
  return selectedIds.value
    .map(id => messageMap.get(id))
    .filter((item): item is CanvasImageItem => !!item && item.type === 'image' && !!item.url)
    .map(item => ({ id: item.id, url: item.url }))
})
const promptPlaceholder = computed(() => 'Pattern generation prompt')
const hasDirectPrompt = computed(() => !!userPrompt.value.trim())
const selectedRatio = ref<PatternGenerateRatioOption>('3:4')
const selectedQuality = ref<GenerateQualityOption>('1k')

function getImageNodeConnectionOutput(node: ImageNodeItem | null): { url: string; ratio: number } | null {
  if (!node) return null
  return getImageNodePreferredImage(node)
}

function clearProcessedMaskPreview(id: number) {
  if (!(id in processedMaskPreviewUrls.value)) {
    processedMaskPreviewSignatures.delete(id)
    return
  }
  const next = { ...processedMaskPreviewUrls.value }
  delete next[id]
  processedMaskPreviewUrls.value = next
  processedMaskPreviewSignatures.delete(id)
}

function getMaskPreviewSignature(node: MaskNodeItem): string {
  return `${getMaskExecutionDataUrl(node)}|${clampMaskRefineAmount(node.refineAmount ?? 0)}|${node.invertMask ? 1 : 0}`
}

function getMaskExecutionDataUrl(node: MaskNodeItem): string {
  return node.executionMaskDataUrl || node.maskDataUrl || ''
}

async function refreshProcessedMaskPreview(node: MaskNodeItem) {
  const baseExecutionMaskDataUrl = getMaskExecutionDataUrl(node)
  if (!baseExecutionMaskDataUrl) {
    clearProcessedMaskPreview(node.id)
    return
  }

  const refineAmount = clampMaskRefineAmount(node.refineAmount ?? 0)
  const signature = getMaskPreviewSignature(node)
  processedMaskPreviewSignatures.set(node.id, signature)

  if (refineAmount === 0 && !node.invertMask) {
    const previewMaskDataUrl = await buildPreviewMaskDataUrlFromExecutionMask(baseExecutionMaskDataUrl)
    if (processedMaskPreviewSignatures.get(node.id) !== signature) return
    processedMaskPreviewUrls.value = {
      ...processedMaskPreviewUrls.value,
      [node.id]: previewMaskDataUrl || node.maskDataUrl || baseExecutionMaskDataUrl,
    }
    return
  }

  const refinedExecutionMaskDataUrl = await refineExecutionMaskDataUrl(
    baseExecutionMaskDataUrl,
    refineAmount,
    !!node.invertMask,
  )
  if (processedMaskPreviewSignatures.get(node.id) !== signature) return
  const refinedPreviewMaskDataUrl = await buildPreviewMaskDataUrlFromExecutionMask(refinedExecutionMaskDataUrl)
  if (processedMaskPreviewSignatures.get(node.id) !== signature) return
  processedMaskPreviewUrls.value = {
    ...processedMaskPreviewUrls.value,
    [node.id]: refinedPreviewMaskDataUrl || node.maskDataUrl || baseExecutionMaskDataUrl,
  }
}

function getMaskNodeProcessedOutputUrl(node: MaskNodeItem): string {
  const baseExecutionMaskDataUrl = getMaskExecutionDataUrl(node)
  if (!baseExecutionMaskDataUrl) return ''
  return processedMaskPreviewUrls.value[node.id] || node.maskDataUrl || baseExecutionMaskDataUrl
}

function getImageNodeConnectedInput(node: ImageNodeItem): { url: string; ratio: number; mode: 'mask' | 'linked-output' | 'node-output' } | null {
  const connection = findIncomingConnection('image', node.id, 'image')
  if (!connection) return null
  if (connection.fromNodeType === 'mask') {
    const sourceNode = maskNodes.value.find(item => item.id === connection.fromNodeId) ?? null
    if (!sourceNode || !getMaskExecutionDataUrl(sourceNode)) return null
    const source = getMaskNodeEffectiveSource(sourceNode)
    return {
      url: getMaskNodeProcessedOutputUrl(sourceNode),
      ratio: source.ratio,
      mode: 'mask',
    }
  }
  const resolvedImage = getConnectedImageInput('image', node.id, 'image')
  if (!resolvedImage) return null
  return {
    ...resolvedImage,
    mode: connection.fromNodeType === 'image' ? 'linked-output' : 'node-output',
  }
}

function getResolvedImageNodeDisplay(node: ImageNodeItem): { image: { url: string; ratio: number } | null; mode: 'source' | 'node-output' | 'linked-output' | 'mask' | 'pending' } {
  if (hasIncomingPortConnection('image', node.id, 'image')) {
    const connectedInput = getImageNodeConnectedInput(node)
    if (!connectedInput) {
      return {
        image: null,
        mode: 'pending',
      }
    }
    return {
      image: {
        url: connectedInput.url,
        ratio: connectedInput.ratio,
      },
      mode: connectedInput.mode,
    }
  }

  const preferredImage = getImageNodePreferredImage(node)
  if (node.outputImages.length > 0 && preferredImage) {
    return {
      image: preferredImage,
      mode: 'node-output',
    }
  }

  return {
    image: preferredImage,
    mode: 'source',
  }
}

function getImageNodeStageHeight(node: ImageNodeItem): number {
  const displayImage = getResolvedImageNodeDisplay(node).image
  if (!displayImage) return 160
  const stageWidth = Math.max(120, node.width - 24)
  const ratio = Number.isFinite(displayImage.ratio) && displayImage.ratio > 0
    ? displayImage.ratio
    : 1
  const dynamicHeight = Math.round(stageWidth * ratio)
  return Math.max(110, dynamicHeight)
}

function appendImageNodeDisplayToCanvas(node: ImageNodeItem) {
  const display = getResolvedImageNodeDisplay(node).image
  if (!display) return
  const item = createCanvasImageItemAt(
    { x: node.x - 360, y: node.y + 18 },
    display.url,
    display.ratio,
  )
  messages.value.push(item)
  selectedIds.value = [item.id]
}

function onImageNodeDisplayDragStart(event: DragEvent, node: ImageNodeItem) {
  const display = getResolvedImageNodeDisplay(node).image
  if (!display || !event.dataTransfer) return
  event.dataTransfer.effectAllowed = 'copy'
  event.dataTransfer.setData(nodeImageDragMime, display.url)
  event.dataTransfer.setData(`${nodeImageDragMime}-ratio`, String(display.ratio || 1))
}

function getMaskNodeConnectedImageInput(node: MaskNodeItem): { url: string; ratio: number } | null {
  return getConnectedImageInput('mask', node.id, 'image')
}

function getMaskNodeEffectiveSource(node: MaskNodeItem): { url: string; ratio: number; connected: boolean } {
  const connectedImage = getMaskNodeConnectedImageInput(node)
  if (connectedImage) {
    return {
      url: connectedImage.url,
      ratio: connectedImage.ratio,
      connected: true,
    }
  }
  return {
    url: node.sourceImageUrl,
    ratio: node.sourceImageRatio,
    connected: false,
  }
}

function setViewpointNodeCategory(node: ViewpointNodeItem, category: ViewpointNodeItem['activeCategory']) {
  node.activeCategory = category
  node.selectedViewpoint = normalizeViewpointSelectionByCategory(category, node.selectedViewpoint)
}

function getCanvasConnectionNodeByType(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
): CanvasNodeItem | null {
  switch (nodeType) {
    case 'prompt':
      return promptNodes.value.find(item => item.id === nodeId) ?? null
    case 'image':
      return imageNodes.value.find(item => item.id === nodeId) ?? null
    case 'mask':
      return maskNodes.value.find(item => item.id === nodeId) ?? null
    case 'color':
      return colorNodes.value.find(item => item.id === nodeId) ?? null
    case 'model':
      return modelNodes.value.find(item => item.id === nodeId) ?? null
    case 'viewpoint':
      return viewpointNodes.value.find(item => item.id === nodeId) ?? null
    case 'edit':
      return editNodes.value.find(item => item.id === nodeId) ?? null
    case 'fusion':
      return fusionNodes.value.find(item => item.id === nodeId) ?? null
    case 'enhance':
      return enhanceNodes.value.find(item => item.id === nodeId) ?? null
    case 'wear':
      return wearNodes.value.find(item => item.id === nodeId) ?? null
    default:
      return null
  }
}

function resolvePreferredOutputNodeId(connectedOutputNodeId: number | null): number | null {
  return connectedOutputNodeId ?? resolvePreferredImageOutputNode()?.id ?? null
}

function buildPlacementSource(url: string, x: number, y: number): CanvasImageItem {
  return {
    id: -Date.now(),
    type: 'image',
    url,
    x,
    y,
    width: 300,
  }
}

function warnMissingNodeInputs(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  missingPorts: string[],
  formatMessage: (state: { disconnectedPorts: string[]; pendingPorts: string[] }) => string,
): boolean {
  if (missingPorts.length === 0) return false
  const { disconnectedPorts, pendingPorts } = getNodeRequiredPortIssues(nodeType, nodeId, missingPorts)
  const unresolvedPorts = missingPorts.filter(
    port => !disconnectedPorts.includes(port) && !pendingPorts.includes(port),
  )
  const mergedDisconnectedPorts = disconnectedPorts.concat(unresolvedPorts)
  triggerNodeConnectionError(nodeType, nodeId, missingPorts)
  message.warning(formatMessage({ disconnectedPorts: mergedDisconnectedPorts, pendingPorts }))
  return true
}

function setEnhanceNodeMode(node: EnhanceNodeItem, mode: EnhanceNodeItem['mode']) {
  node.mode = mode
  if (!enhanceModeSupportsPrompt(mode)) {
    removeIncomingConnectionsForPort('enhance', node.id, 'prompt')
  }
  if (!enhanceModeSupportsMask(mode)) {
    removeIncomingConnectionsForPort('enhance', node.id, 'mask')
  }
}

function setEditNodeMode(node: EditNodeItem, mode: EditNodeItem['mode']) {
  node.mode = mode
  if (!editModeSupportsPrompt(mode)) {
    removeIncomingConnectionsForPort('edit', node.id, 'prompt')
  }
  if (!editModeSupportsSourceImage(mode)) {
    removeIncomingConnectionsForPort('edit', node.id, 'imageB')
  }
}

function clampEnhanceExtendPercent(value: number): number {
  return Math.max(ENHANCE_EXTEND_PERCENT_MIN, Math.min(ENHANCE_EXTEND_PERCENT_MAX, Math.round(value)))
}

function getMaskNodeStageHeight(node: MaskNodeItem): number {
  const source = getMaskNodeEffectiveSource(node)
  return getMaskStageHeight({
    ...node,
    sourceImageUrl: source.url,
    sourceImageRatio: source.ratio,
  })
}

function removeColorNodeColor(node: ColorNodeItem, index: number) {
  if (index < 0 || index >= node.colors.length) return
  node.colors.splice(index, 1)
}

function addColorNodeColor(node: ColorNodeItem, rawColor: string) {
  if (node.colors.length >= MAX_COLOR_NODE_SWATCHES) {
    message.warning('最多保留 8 种颜色')
    return
  }
  const nextColor = normalizeHexColor(rawColor)
  if (node.colors.includes(nextColor)) return
  node.colors.push(nextColor)
}

const {
  getNodeConnectionStatus,
  getConnectedImageInput,
  getConnectedPromptInput,
  getConnectedOutputImageTarget,
  resolveConnectedPlacedImageInput,
  resolveConnectedPlacedMaskInput,
  getMissingRequiredNodeInputs,
} = useCanvasNodeConnections({
  imageNodes,
  promptNodes,
  maskNodes,
  getIncomingConnections,
  hasIncomingPortConnection,
  hasOutgoingPortConnection,
  findIncomingConnection,
  getOutgoingConnections,
  getImageNodePreferredImage,
  lookupNodeByType: getCanvasConnectionNodeByType,
  refineExecutionMaskDataUrl,
})

const {
  getNodeRequiredPortIssues,
  getNodeUiState,
} = useCanvasNodeUiState({
  getNodeConnectionStatus,
  lookupNodeByType: getCanvasConnectionNodeByType,
  isExecutionNodeRunning,
  isNodeLastRunFailed: isExecutionNodeLastRunFailed,
})

function resolveWorkflowPromptText(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  fallback = '',
): string {
  return getConnectedPromptInput(nodeType, nodeId, 'prompt') || fallback
}

const nodeCopyMode = computed(() => isDuplicateModifierPressed.value && activeTool.value === 'select')

const getImageNodeUiState = (node: ImageNodeItem) => getNodeUiState('image', node)
const getPromptNodeUiState = (node: PromptNodeItem) => getNodeUiState('prompt', node)
const getModelNodeUiState = (node: ModelNodeItem) => getNodeUiState('model', node)
const getViewpointNodeUiState = (node: ViewpointNodeItem) => getNodeUiState('viewpoint', node)
const getMaskNodeUiState = (node: MaskNodeItem) => getNodeUiState('mask', node)
const getEditNodeUiState = (node: EditNodeItem) => getNodeUiState('edit', node)
const getFusionNodeUiState = (node: FusionNodeItem) => getNodeUiState('fusion', node)
const getEnhanceNodeUiState = (node: EnhanceNodeItem) => getNodeUiState('enhance', node)
const getWearNodeUiState = (node: WearNodeItem) => getNodeUiState('wear', node)
const getColorNodeUiState = (node: ColorNodeItem) => getNodeUiState('color', node)

const onImageNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'image', nodeId, port)
const onPromptNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'prompt', nodeId, port)
const onModelNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'model', nodeId, port)
const onViewpointNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'viewpoint', nodeId, port)
const onMaskNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'mask', nodeId, port)
const onEditNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'edit', nodeId, port)
const onFusionNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'fusion', nodeId, port)
const onEnhanceNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'enhance', nodeId, port)
const onWearNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'wear', nodeId, port)
const onColorNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'color', nodeId, port)

function onPromptNodeTextUpdate(node: PromptNodeItem, value: string) {
  node.text = value
}

function onModelNodeSelectBackground(node: ModelNodeItem, key: string) {
  node.background = key
}

function onModelNodeUpdateAngleYaw(node: ModelNodeItem, value: number) {
  updateModelAngleYaw(node, clampModelAngleYaw(value))
}

function onModelNodeUpdateAngleFraming(node: ModelNodeItem, value: ModelAngleFraming) {
  updateModelAngleFraming(node, value)
}

function onModelNodeUpdateModelStrength(node: ModelNodeItem, value: number) {
  node.modelStrength = clampModelReferenceStrength(value)
}

function onModelNodeUpdatePoseStrength(node: ModelNodeItem, value: number) {
  node.poseStrength = clampModelPoseStrength(value)
}

function onViewpointNodeUpdateLighting(node: ViewpointNodeItem, value: ViewpointNodeItem['lighting']) {
  node.lighting = value
}

function onMaskNodeCanvasMouseDownBridge(event: MouseEvent, node: MaskNodeItem) {
  onMaskCanvasMouseDown(event, node)
}

function onMaskNodeSetDrawMode(node: MaskNodeItem, value: 'paint' | 'erase') {
  node.drawMode = value
}

function onMaskNodeUpdateBrushSize(node: MaskNodeItem, value: number) {
  node.brushSize = value
}

function onMaskNodeUpdateRefineAmount(node: MaskNodeItem, value: number) {
  node.refineAmount = clampMaskRefineAmount(value)
}

function onMaskNodeToggleInvert(node: MaskNodeItem) {
  node.invertMask = !node.invertMask
}

function onEditNodeUpdateMode(node: EditNodeItem, value: EditNodeItem['mode']) {
  setEditNodeMode(node, value)
}

function onFusionNodeUpdateBlendRatio(node: FusionNodeItem, value: number) {
  node.blendRatio = Math.max(0, Math.min(100, Math.round(value)))
}

function onEnhanceNodeUpdateMode(node: EnhanceNodeItem, value: EnhanceNodeItem['mode']) {
  setEnhanceNodeMode(node, value)
}

function onEnhanceNodeUpdateUpscaleMode(node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleMode']) {
  node.upscaleMode = value
}

function onEnhanceNodeUpdateUpscaleFactor(node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleFactor']) {
  node.upscaleFactor = value
}

function onEnhanceNodeUpdateOutputFormat(node: EnhanceNodeItem, value: EnhanceNodeItem['outputFormat']) {
  node.outputFormat = value
}

function onEnhanceNodeUpdateOutputQuality(node: EnhanceNodeItem, value: number) {
  node.outputQuality = Math.max(30, Math.min(100, Math.round(value)))
}

function onEnhanceNodeUpdateExtendTop(node: EnhanceNodeItem, value: number) {
  node.extendTop = clampEnhanceExtendPercent(value)
}

function onEnhanceNodeUpdateExtendRight(node: EnhanceNodeItem, value: number) {
  node.extendRight = clampEnhanceExtendPercent(value)
}

function onEnhanceNodeUpdateExtendBottom(node: EnhanceNodeItem, value: number) {
  node.extendBottom = clampEnhanceExtendPercent(value)
}

function onEnhanceNodeUpdateExtendLeft(node: EnhanceNodeItem, value: number) {
  node.extendLeft = clampEnhanceExtendPercent(value)
}

function onWearNodeUpdateTileMode(node: WearNodeItem, value: WearNodeItem['tileMode']) {
  node.tileMode = normalizeWearTileMode(value)
}

function onWearNodeUpdateRotation(node: WearNodeItem, value: number) {
  node.rotation = clampWearRotation(value)
}

function onWearNodeUpdateScale(node: WearNodeItem, value: number) {
  node.scale = clampWearScale(value)
}

function onWearNodeUpdateOpacity(node: WearNodeItem, value: number) {
  node.opacity = clampWearOpacity(value)
}

function onColorNodeApplyPreset(node: ColorNodeItem, value: string[]) {
  const baseColors = isColorNodePlaceholderState(node) ? [] : node.colors
  node.colors = normalizeColorNodeColors([...baseColors, ...value])
}

function onColorNodeStrengthInput(node: ColorNodeItem, value: number) {
  node.strength = Math.max(0, Math.min(100, Math.round(value)))
}

const {
  handleModelNodeGenerate,
  handleViewpointNodeGenerate,
  handleEditNodeGenerate,
  handleFusionNodeGenerate,
  handleEnhanceNodeGenerate,
  handleWearNodeGenerate,
  handleColorNodeGenerate,
} = useNodeExecutors({
  message,
  messages,
  selectedIds,
  imageNodes,
  maskNodes,
  activeImageNodeId,
  activeMaskNodeId,
  getViewpointLabel,
  getViewpointLightingLabel,
  getNodeConnectionStatus,
  triggerNodeConnectionError,
  resolveNodePromptText: resolveWorkflowPromptText,
  getConnectedOutputImageTarget,
  resolvePreferredOutputNodeId,
  buildPlacementSource,
  getMissingRequiredNodeInputs,
  warnMissingNodeInputs,
  submitTaskAndPoll,
  appendGeneratedImages,
  appendWorkflowNodeOutputs,
  normalizeHexColor,
  getImageNodePreferredImage,
  resolveConnectedPlacedImageInput,
  resolveConnectedPlacedMaskInput,
  buildFullMaskDataUrl,
  refineExecutionMaskDataUrl,
  dataUrlToFile,
  uploadImage: uploadWorkspaceImage,
  loadImageSizeFromUrl,
  setExecutionNodeRunning,
  setExecutionNodeLastRunFailed,
  isExecutionNodeRunning,
})

const {
  runCanvasAction,
  handleGenerate,
  handleExport,
} = useCanvasDirectActions({
  generating: directGenerating,
  selectedIds,
  activeImageActionId,
  showStyleStrengthControl,
  hasDirectPrompt,
  showSettings,
  selectedRatio,
  selectedQuality,
  userPrompt,
  messages,
  styleInnovateStrength,
  loadImageSizeFromUrl,
  message,
  submitTaskAndPoll: submitDirectTaskAndPoll,
  appendGeneratedImages,
  createPendingGeneratedImages,
  replacePendingGeneratedImages,
  removePendingGeneratedImages,
  vectorizeImage: vectorizeApi.vectorize,
})

function onImageLoad(msg: any, event: Event) {
  const target = event.target as HTMLImageElement | null
  if (!target || !target.naturalWidth || !target.naturalHeight) return
  msg.height_ratio = target.naturalHeight / target.naturalWidth
}

function getImageDisplayHeight(item: CanvasImageItem): number {
  const imageEl = canvasImageRefs.get(item.id)
  if (imageEl && imageEl.clientHeight > 0) {
    return Math.round(imageEl.clientHeight)
  }
  const width = item.width || 300
  const ratio = item.height_ratio || 1
  return Math.max(72, Math.round(width * ratio))
}

onMounted(async () => {
  if (!userStore.isLoggedIn) {
    addGlobalConnectionListeners()
    nextTick(() => {
      scheduleRenderedConnectionsRefresh()
    })
    return
  }
  addGlobalConnectionListeners()
  nextTick(() => {
    scheduleRenderedConnectionsRefresh()
  })
  void schedulePendingRecycleScan()
})

onUnmounted(() => {
  clearPendingRecycleScanTimer()
  void flushPendingRecycleUploads()
  stopNodeTaskPolling()
  stopDirectTaskPolling()
  clearStyleStrengthLongPressTimer()
  clearStyleStrengthHideTimer()
  removeStyleStrengthGlobalListeners()
  removeGlobalConnectionListeners()
  disposeConnectionRuntime()
})
</script>

<style scoped>
.canvas-layout { width: 100%; height: 100%; background: #ffffff; position: relative; overflow: hidden; }
.canvas-layout.copy-mode .canvas-item {
  cursor: copy;
}
.canvas-layout.copy-mode :deep(.workflow-node.copy-mode .workflow-node-header) {
  cursor: copy !important;
}
.canvas-grid { position: absolute; inset: 0; background-image: radial-gradient(circle, #e2e4e8 calc(1.5px * var(--zoom, 1)), transparent calc(1.5px * var(--zoom, 1))); background-size: calc(24px * var(--zoom, 1)) calc(24px * var(--zoom, 1)); background-position: calc(50% + var(--offset-x, 0px)) calc(50% + var(--offset-y, 0px)); z-index: 0; opacity: var(--grid-opacity, 1); }
.canvas-grid.grid-dragging { background: rgba(146, 120, 230, 0.05); }

.selection-marquee {
  position: absolute;
  background: rgba(146, 120, 230, 0.1);
  border: 1.5px solid #9278e6;
  border-radius: 4px;
  z-index: 1000;
  pointer-events: none;
}

.leave-confirm-mask {
  position: absolute;
  inset: 0;
  z-index: 220;
  background: rgba(17, 24, 39, 0.24);
  display: flex;
  align-items: center;
  justify-content: center;
}

.leave-confirm-card {
  width: min(420px, calc(100% - 28px));
  border-radius: 12px;
  border: 1px solid #d9deea;
  background: #fff;
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.16);
  padding: 16px;
}

.leave-confirm-title {
  font-size: 15px;
  line-height: 1.3;
  font-weight: 600;
  color: #2a3142;
}

.leave-confirm-desc {
  margin-top: 6px;
  font-size: 13px;
  line-height: 1.45;
  color: #6b7488;
}

.leave-confirm-actions {
  margin-top: 14px;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.leave-btn {
  height: 34px;
  border-radius: 8px;
  border: 1px solid #d5dbe8;
  background: #fff;
  color: #4f576a;
  font-size: 13px;
  font-weight: 500;
  padding: 0 12px;
  cursor: pointer;
  transition: border-color 0.15s ease, background-color 0.15s ease, color 0.15s ease;
}

.leave-btn:hover {
  border-color: #c2cae0;
}

.leave-btn.primary {
  border-color: #9278e6;
  background: #9278e6;
  color: #fff;
}

.leave-btn.primary:hover {
  background: #8166df;
  border-color: #8166df;
}

.leave-btn.danger {
  border-color: #e7ced4;
  color: #bf5a74;
  background: #fff;
}

.leave-btn.danger:hover {
  border-color: #ddb3bf;
  background: #fff6f8;
}

.node-context-menu {
  position: absolute;
  z-index: 160;
  width: 160px;
  min-width: 160px;
  max-width: 160px;
  padding: 6px;
  border-radius: 10px;
  border: 1px solid #d8ddea;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
  box-shadow: 0 10px 30px rgba(25, 30, 45, 0.12);
}

.node-context-item {
  width: 100%;
  height: 34px;
  border: none;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  padding: 0 10px;
  font-size: 13px;
  font-weight: 600;
  color: #4b5565;
  cursor: pointer;
}

.node-context-item:hover {
  background: rgba(146, 120, 230, 0.12);
  color: #6757c8;
}

.floating-toolbar { position: absolute; left: 16px; top: 50%; transform: translateY(-50%); background: #fff; padding: 8px; border-radius: 16px; display: flex; flex-direction: column; gap: 8px; border: 1px solid #e2e4e8; z-index: 100; }
.tool-item { width: 40px; height: 40px; border-radius: 10px; display: flex; align-items: center; justify-content: center; color: #5f6368; cursor: pointer; }
.tool-item.active { background: #f1f3f9; color: #9278e6; }
.toolbar-divider { width: 24px; height: 1px; background: #f1f3f9; margin: 4px auto; }

.floating-status { position: absolute; bottom: 24px; right: 24px; display: flex; gap: 16px; z-index: 100; }
.zoom-controls { background: #f1f3f9; padding: 4px; border-radius: 12px; display: flex; align-items: center; gap: 8px; }
.zoom-btn { 
  width: 28px; 
  height: 28px; 
  border: none; 
  background: transparent; 
  cursor: pointer; 
  color: #5f6368;
  font-size: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  transition: all 0.2s;
}
.zoom-btn:hover { background: #fff; color: #202124; }
.zoom-level { 
  font-size: 13px; 
  font-weight: 600;
  color: #5f6368;
  min-width: 40px; 
  text-align: center; 
}
.logo-btn { 
  width: 40px; 
  height: 40px; 
  border-radius: 50%; 
  background: #fff; 
  border: 1px solid #e2e4e8; 
  cursor: pointer; 
  display: flex; 
  align-items: center; 
  justify-content: center; 
  transition: all 0.3s ease;
  color: #5f6368;
}

.logo-btn:hover {
  background: #f8fafc;
  border-color: #9278e6;
  color: #9278e6;
}

.logo-btn:active {
  transform: scale(0.9);
}

.logo-btn svg {
  transition: transform 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.logo-btn:hover svg {
  transform: rotate(360deg);
}

.canvas-content { position: absolute; inset: 0; z-index: 2; pointer-events: none; transform: translate(var(--offset-x, 0px), var(--offset-y, 0px)) scale(var(--zoom, 1)); transform-origin: center center; }
:deep(.workflow-node) {
  position: absolute;
  z-index: 12;
  border: 1.5px solid #e2e8f0;
  border-radius: 14px;
  background: #ffffff;
  overflow: visible;
  transition: border-color 0.2s ease;
  will-change: transform;
  pointer-events: auto;
}

:deep(.workflow-node .workflow-node-header) {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
}

:deep(.workflow-node.active) {
  border-color: #9278e6;
}

:deep(.workflow-node.dragging) {
  transition: none;
}

</style>
