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
    <CanvasChrome
      :inert="isImageEditorChromeHidden"
      :is-hidden="isImageEditorChromeHidden"
      :board="canvasChromeBoard"
      :account="canvasChromeAccount"
      :tools="canvasChromeTools"
      :status="canvasChromeStatus"
      @update:board-name-draft="boardNameDraft = $event"
      @toggle-board-menu="toggleBoardMenu"
      @start-board-rename="startBoardRename"
      @cancel-board-rename="cancelBoardRename"
      @confirm-board-rename="onConfirmBoardRename"
      @select-board="onSelectBoard"
      @export="onExportSelectedImages"
      @navigate-back="onNavigateRequest(USER_BOARDS_PATH)"
      @select-tool="setActiveTool"
      @toggle-minimap="toggleCanvasMinimap"
      @zoom-out="onZoomOut"
      @zoom-in="onZoomIn"
    />

    <div
      ref="canvasWrapRef"
      class="canvas-flow-wrap ib-page-dot-bg"
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
        :apply-default="false"
        :min-zoom="CANVAS_VIEWPORT_MIN_ZOOM"
        :max-zoom="CANVAS_VIEWPORT_MAX_ZOOM"
        :zoom-on-scroll="false"
        :pan-on-scroll="!isFlowCanvasLocked"
        :zoom-on-pinch="!isFlowCanvasLocked"
        :pan-on-scroll-mode="PanOnScrollMode.Free"
        :only-render-visible-elements="true"
        :is-valid-connection="commands.flowPane.isValidConnection"
        @nodes-change="commands.flowPane.onNodesChange"
        @edges-change="commands.flowPane.onEdgesChange"
        @connect="commands.flowPane.onConnect"
        @connect-end="onFlowConnectEnd"
        @edge-update="commands.flowPane.onEdgeUpdate"
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
        <CanvasChromeMiniMap
          :enabled="isCanvasMinimapEnabled"
          :inert="isImageEditorChromeHidden"
          :is-hidden="isImageEditorChromeHidden"
          @center-requested="onMiniMapCenterRequested"
          @drag-start="onMiniMapDragStart"
          @drag-move="handleMiniMapDragMove"
          @drag-end="onMiniMapDragEnd"
        />
      </VueFlow>
      <CanvasOverlays
        :image-tool-session="imageTools"
        :node-context-menu="nodeContextMenu"
        :canvas-node-menu-items="canvasNodeMenuItems"
        @create-node="handleNodeContextMenuCreate"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../../../app/stores/user'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { uploadApi } from '../../../shared/api/upload'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useDismissableLayer } from '../../../shared/composables/useDismissableLayer'
import { ADMIN_ROOT_PATH, USER_BOARDS_PATH } from '../../../shared/constants/routePaths'
import { canAccessAdmin } from '../../../shared/auth/adminAccess'
import { createUserAccountMenuItems } from '../../../shared/components/userAccountMenuActions'
import { PanOnScrollMode, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowKeyboardShortcuts } from '../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../composables/useFlowLeaveGuard'
import { useFlowImageActionStrengthControl } from '../composables/useFlowImageActionStrengthControl'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import { useFlowSelectionController } from '../composables/useFlowSelectionController'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
import { provideCanvasViewportContext } from '../composables/useCanvasViewportContext'
import {
  ACTIVE_NODE_PANEL_KIND,
  useFlowActiveNodePanels,
} from '../composables/useFlowActiveNodePanels'
import { useFlowCanvasShellInteractions } from '../composables/useFlowCanvasShellInteractions'
import { useFlowExpandInteractionHandlers } from '../composables/useFlowExpandInteractionHandlers'
import {
  useFlowTransientUiDismissal,
  type DismissTransientCanvasUi,
} from '../composables/useFlowTransientUiDismissal'
import { useImageStageRegistry } from '../composables/useImageStageRegistry'
import { useVueFlowPaneAdapter } from '../flow-pane'
import { useCanvasWorkspaceRuntime } from '../workspace-runtime'
import { CANVAS_VIEWPORT_MAX_ZOOM, CANVAS_VIEWPORT_MIN_ZOOM } from '../utils/canvasViewportStorage'
import FlowCanvasNode from '../components/FlowCanvasNode.vue'
import {
  CanvasChrome,
  CanvasChromeMiniMap,
  type CanvasChromeAccountState,
  type CanvasChromeBoardState,
  type CanvasChromeStatusState,
  type CanvasChromeToolState,
} from '../components/chrome'
import CanvasOverlays from '../components/CanvasOverlays.vue'
import { openAccountManagementDialog } from '../../account-management/useAccountManagementDialogController'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const flowPaneAdapter = useVueFlowPaneAdapter()
const { screenToFlowCoordinate } = flowPaneAdapter

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
const { openNodeImagePreview, openImageUrlPreview } = useFlowImagePreviewLightbox()

const workspace = useCanvasWorkspaceRuntime({
  route,
  router,
  getUserIdentity: () => ({
    userId: Number(userStore.user?.id),
    userUid: `${userStore.user?.uid ?? ''}`.trim(),
  }),
  createNodeKind,
  flowPaneAdapter,
  uploadApi,
  sanitizeApiErrorMessage,
  openNodeImagePreview,
  openImageUrlPreview,
  logoutAndRedirect,
  startNodeDragInteraction: () => startNodeDragInteraction(),
  stopNodeDragInteraction: () => stopNodeDragInteraction(),
  refreshFlowHandleVisibility: () => refreshFlowHandleVisibility(),
  onNativeRemove: () => {
    dismissTransientCanvasUi({ closeExpand: true, closeContextMenu: true })
  },
})
const viewModel = workspace.viewModel
const commands = workspace.commands

const { nodes, edges } = viewModel.flowPane

type CanvasBenchmarkConnectDeleteTarget = {
  sourceNodeId: string
  targetNodeId: string
  sourceHandle: string
  targetHandle: string
  deleteNodeId: string
}

type CanvasBenchmarkWindow = Window & {
  __ideaboardCanvasBenchmarkActions?: {
    connectDelete: (target: CanvasBenchmarkConnectDeleteTarget) => void
  }
}

const isCanvasBenchmarkActionsEnabled = computed(() => route.query.canvasBenchmarkActions === '1')

if (import.meta.env.DEV && isCanvasBenchmarkActionsEnabled.value) {
  const benchmarkWindow = window as CanvasBenchmarkWindow
  benchmarkWindow.__ideaboardCanvasBenchmarkActions = {
    connectDelete: (target) => {
      commands.flowPane.onConnect({
        source: target.sourceNodeId,
        target: target.targetNodeId,
        sourceHandle: target.sourceHandle,
        targetHandle: target.targetHandle,
      })
      commands.flowPane.onNodesChange([{ id: target.deleteNodeId, type: 'remove' }])
    },
  }
}

onBeforeUnmount(() => {
  delete (window as CanvasBenchmarkWindow).__ideaboardCanvasBenchmarkActions
})

const {
  currentBoardId,
  isBoardMenuOpen,
  isRenamingBoard,
  boardNameDraft,
  currentBoardName,
  boardOptions,
} = viewModel.boardChrome
const {
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
} = commands.boardSession

const userMenuItems = computed(() =>
  createUserAccountMenuItems({
    canAccessAdmin: canAccessAdmin(userStore.user),
    hiddenKeys: ['home'],
    handlers: {
      profile: openAccountManagementDialog,
      admin: () => onNavigateRequest(ADMIN_ROOT_PATH),
      logout: () => {
        void onLogoutRequest()
      },
    },
  }),
)

const canvasChromeBoard = computed<CanvasChromeBoardState>(() => ({
  currentName: currentBoardName.value,
  options: boardOptions.value,
  currentId: currentBoardId.value,
  isMenuOpen: isBoardMenuOpen.value,
  isRenaming: isRenamingBoard.value,
  setNameInputRef: setBoardNameInputRef,
}))
const canvasChromeAccount = computed<CanvasChromeAccountState>(() => ({
  avatarUrl: userStore.user?.avatar ?? undefined,
  menuItems: userMenuItems.value,
}))
const canvasChromeTools = computed<CanvasChromeToolState>(() => ({
  activeTool: activeTool.value,
  minimapEnabled: isCanvasMinimapEnabled.value,
}))
const canvasChromeStatus = computed<CanvasChromeStatusState>(() => ({
  exportDisabled: !hasSelectedFlowNodes.value,
  zoomLevel: zoomLevel.value,
}))

const { history: flowHistory } = viewModel.persistence
const { flushAutosave, loadCanvasBoardAndProbe, reattachPendingImageResults } = commands.persistence

const { zoomLevel } = viewModel.viewportControls
const {
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
} = commands.viewportControls
provideCanvasViewportContext({ zoomLevel })

const {
  hasSelectedFlowNodes,
  isFlowInteractionActive,
  isFlowViewportPanInteractionActive,
  isFlowInteractionLocked,
  nodeContextMenu,
  canvasNodeMenuItems,
} = viewModel.graphEditing
const {
  scheduleFlowNodeInternalsRefresh,
  createNodeAtPosition,
  onExportSelectedImages,
  closeNodeContextMenu,
  openNodeContextMenu,
  onFlowMediaInputImageDrop,
  onFlowMediaInputImageOpen,
  onFlowMediaInputImagePreview,
  onFlowMediaInputImageRemove,
  onCanvasDrop,
  onCanvasDragOver: onFlowCanvasDropDragOver,
  onFlowImageOpenPreview,
  onFlowImageAppendToCanvas,
  beginFlowNodeDragDuplicateTracking,
  finishFlowNodeDragDuplicateTracking,
  getFlowNodeUiStateCached,
} = commands.graphEditing

function onFlowConnectEnd() {
  refreshFlowHandleVisibility()
}

function onFlowMove(payload: Parameters<typeof syncFlowMove>[0]) {
  syncFlowMove(payload)
  if (!payload.flowTransform || !imageTools.viewModel.chrome.value.isReady) return
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

const { runImageDirectAction } = commands.imageWorkflow

const {
  activeStrengthAction,
  consumeSuppressedStrengthClick,
  setStrengthRangeRef,
  setStrengthTriggerRef,
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
  onCancel: () => {
    clearActiveNodePanel()
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
} = commands.imageWorkflow.createCanvasExtendOverlay({
  activeTool: activeTool as Ref<string>,
  getImageStageElement,
})

const imageTools = commands.imageWorkflow.createImageTools({
  activeTool: activeTool as Ref<string>,
  canvasWrapRef,
  isCanvasInteractionLocked: isFlowInteractionLocked,
  getImageStageElement,
  clearActiveNodePanel,
  hideStrengthControl,
  isImageActionPanelVisible: (nodeId) =>
    isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeId),
  isImagePromptComposerVisible: (nodeId) =>
    isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, nodeId),
  strengthControl: {
    activeAction: activeStrengthAction,
    getValue: getActionStrength,
    setRangeRef: setStrengthRangeRef,
    setTriggerRef: setStrengthTriggerRef,
    onRangePointerDown: onStrengthRangePointerDown,
    onInput: onStrengthInput,
    onChange: onStrengthChange,
    onPointerUp: onStrengthPointerUp,
    onClick: onStrengthClick,
  },
})

const {
  viewModel: {
    active: { nodeId: flowImageEditorNodeId, maskNodeId: flowMaskEditorNodeId },
    locks: { isChromeHidden: isImageEditorChromeHidden, isCanvasLocked: isFlowCanvasLocked },
    nodeBindings: { stageFor: imageToolStageBindingFor, actionFor: imageToolActionBindingFor },
  },
  commands: {
    lifecycle: {
      enter: enterFlowImageEditorMode,
      cancel: cancelFlowMaskEditorMode,
      discardNodeEdits: discardFlowImageToolNodeEdits,
    },
    editor: {
      onCanvasWheel,
      focusAndShowChrome: focusImageEditorAndShowChrome,
      scheduleChromeAnchorUpdate: scheduleImageEditorChromeAnchorUpdate,
    },
    prompt: { runPromptAction: runFlowImageToolPromptAction },
  },
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
} = commands.nodeInteraction.createNodeMediaInputs({
  discardNodeEdits: discardFlowImageToolNodeEdits,
})

const { onFlowDeleteNode, onFlowPromptTextUpdate, onFlowNodeParamUpdate, onFlowImageReset } =
  commands.nodeInteraction.createNodeHandlers({
    dismissTransientCanvasUi,
    discardFlowImageToolNodeEdits,
    _onFlowImageReset: onFlowImageResetBase,
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

const { onFlowPromptGenerate } = commands.imageWorkflow.createPromptGenerationHandlers({
  cancelFlowExpandMode,
  cancelFlowMaskEditorMode,
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
    beginNodeDragTransaction: commands.flowPane.nodeDragTransactions.begin,
    commitNodeDragTransaction: commands.flowPane.nodeDragTransactions.commit,
    duplicateModifierPressed,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
    isNodeDragLocked: () => isFlowCanvasLocked.value,
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
} = commands.imageWorkflow.createImageActionHandlers({
  flowImageEditorNodeId,
  consumeSuppressedStrengthClick,
  openActiveNodePanel,
  clearActiveNodePanel,
  hideStrengthControl,
  selectFlowNodeForContextMenu,
  cancelFlowExpandMode,
  cancelFlowMaskEditorMode,
  enterFlowExpandMode,
  enterFlowImageEditorMode,
  focusImageEditorAndShowChrome,
  runImagePromptAction: runFlowImageToolPromptAction,
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

<style>
@import '../components/styles/canvas-vue-flow-adapter-skin.css';
@import '../components/styles/canvas-style-contracts.css';
</style>

<style scoped>
:global(html.canvas-overscroll-guard),
:global(body.canvas-overscroll-guard) {
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-layout {
  --canvas-control-border-width: 1.2px;
  --canvas-tool-button-gap: 7px;
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

.canvas-flow-wrap {
  position: absolute;
  inset: 0;
  z-index: 2;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}
</style>
