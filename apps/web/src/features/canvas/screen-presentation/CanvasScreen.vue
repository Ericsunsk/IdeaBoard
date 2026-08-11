<template>
  <div
    class="canvas-layout canvas-surface-skin"
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
      v-bind="chromeProps"
      @update:board-name-draft="updateBoardNameDraft"
      @toggle-board-menu="toggleBoardMenu"
      @start-board-rename="startBoardRename"
      @cancel-board-rename="cancelBoardRename"
      @confirm-board-rename="confirmBoardRename"
      @reload-board-catalog="reloadBoardCatalog"
      @select-board="selectBoard"
      @export="exportSelectedImages"
      @navigate-back="navigateBack"
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
      @pointerdown.capture="onCanvasPointerDownCapture"
      @contextmenu.capture="onCanvasContextMenuCapture"
      @wheel="onCanvasWheel"
    >
      <VueFlow
        :nodes="nodes"
        :edges="edges"
        v-bind="vueFlowProps"
        @nodes-change="onNodesChange"
        @edges-change="vueFlowSelectionBindings.onEdgesChange"
        @connect-end="onFlowConnectEnd"
        @move-start="onFlowMoveStart"
        @move="onFlowMove"
        @move-end="onFlowMoveEnd"
        @viewport-change-end="onViewportChangeEnd"
        @node-click="onFlowNodeClick"
        @node-drag-start="onNodeDragStart"
        @node-drag-stop="onNodeDragStop"
        @selection-start="vueFlowSelectionBindings.onSelectionStart"
        @selection-end="vueFlowSelectionBindings.onSelectionEnd"
        @pane-click="onCanvasPaneClick"
        @pane-context-menu="onPaneContextMenu"
      >
        <template #zoom-pane>
          <CanvasGroupPresentation
            :presentation="groupPresentation"
            :action-target="canvasWrapRef"
          />
        </template>
        <template #node-canvas="nodeProps">
          <FlowCanvasNode
            v-bind="nodePropsFor(nodeProps)"
            @node-intent="nodePresentationIntentRouter.dispatch"
            @node-mousedown="onFlowNodeMouseDown"
            @request-internals-update="onFlowNodeInternalsUpdate"
          />
        </template>
        <CanvasChromeMiniMap
          v-bind="minimapProps"
          @center-requested="onMiniMapCenterRequested"
          @drag-start="onMiniMapDragStart"
          @drag-move="handleMiniMapDragMove"
          @drag-end="onMiniMapDragEnd"
        />
      </VueFlow>
      <ImageToolPresentation :image-tools="imageTools" />
      <CanvasNodeContextMenu
        :visible="nodeContextMenu.visible"
        :x="nodeContextMenu.x"
        :y="nodeContextMenu.y"
        :items="canvasNodeMenuItems"
        @create="handleNodeContextMenuCreate"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { uploadApi } from '../../../shared/api/upload'
import { USER_BOARDS_PATH } from '../../../shared/constants/routePaths'
import { PanOnScrollMode, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
import { provideCanvasViewportContext } from '../composables/useCanvasViewportContext'
import { CanvasGroupPresentation } from '../group-presentation'
import { useVueFlowPaneAdapter } from '../flow-pane'
import {
  createCanvasGraphEditingBrowserAdapter,
  createCanvasGraphInteractionBrowserAdapter,
} from '../graph-editing'
import { useCanvasWorkspaceRuntime } from '../workspace-runtime'
import FlowCanvasNode from '../components/FlowCanvasNode.vue'
import CanvasNodeContextMenu from '../components/CanvasNodeContextMenu.vue'
import { ImageToolPresentation } from '../image-tools'
import {
  CanvasChrome,
  CanvasChromeMiniMap,
  type CanvasChromeAccountState,
  type CanvasChromeBoardState,
} from '../components/chrome'
import { CANVAS_VIEWPORT_MAX_ZOOM, CANVAS_VIEWPORT_MIN_ZOOM } from '../utils/canvasViewportStorage'
import {
  useUserAccountControls,
  type UserShellNavigationAdapter,
} from '../../user-shell/useUserAccountControls'
import { useCanvasScreenImageToolUi } from './internal/useCanvasScreenImageToolUi'
import { useCanvasScreenSurfaceInteractions } from './internal/useCanvasScreenSurfaceInteractions'
import { createCanvasScreenNodePresentation } from './internal/createCanvasScreenNodePresentation'
import { useCanvasScreenLifecycle } from './internal/useCanvasScreenLifecycle'

const props = defineProps<{
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
}>()

const { route, router, getUserIdentity, logoutAndRedirect } = props

function openRecharge() {
  accountControls.commands.openAccount('recharge')
}

const flowPaneAdapter = useVueFlowPaneAdapter()

const canvasWrapRef = ref<HTMLElement | null>(null)
const activeTool = ref<'select' | 'hand'>('select')
const isCanvasMinimapEnabled = ref(false)
const duplicateModifierPressed = ref(false)
const imageToolUi = useCanvasScreenImageToolUi({ activeTool, canvasWrapRef })
const { openNodeImagePreview, openImageUrlPreview } = useFlowImagePreviewLightbox()
const graphEditingBrowser = createCanvasGraphEditingBrowserAdapter()
const graphInteractionBrowser = createCanvasGraphInteractionBrowserAdapter()

const workspace = useCanvasWorkspaceRuntime({
  route,
  router,
  getUserIdentity,
  flowPaneAdapter,
  uploadApi,
  graphEditingBrowser,
  graphInteractionBrowser,
  openNodeImagePreview,
  openImageUrlPreview,
  imageWorkflowUi: imageToolUi.imageWorkflowUi,
  logoutAndRedirect,
  openRecharge,
})
const viewModel = workspace.viewModel
const commands = workspace.commands

const { nodes, edges } = viewModel.flowPane
const { connectionAdapter } = commands.flowPane

const {
  currentBoardId,
  isBoardMenuOpen,
  isRenamingBoard,
  boardNameDraft,
  isRenamePending,
  catalogRecovery,
  currentBoardName,
  boardOptions,
} = viewModel.boardChrome
const {
  setBoardNameInputRef,
  toggleBoardMenu,
  startBoardRename,
  cancelBoardRename,
  closeBoardHeaderEditing,
  onConfirmBoardRename,
  reloadCatalog,
  onSelectBoard,
  onNavigateRequest,
  onLogoutRequest,
} = commands.boardSession

const canvasNavigationAdapter: UserShellNavigationAdapter = {
  navigate: onNavigateRequest,
  logout: onLogoutRequest,
}
const accountControls = useUserAccountControls({
  navigation: canvasNavigationAdapter,
  showHome: false,
})

const canvasChromeBoard = computed<CanvasChromeBoardState>(() => ({
  currentName: currentBoardName.value,
  options: boardOptions.value,
  currentId: currentBoardId.value,
  isMenuOpen: isBoardMenuOpen.value,
  isRenaming: isRenamingBoard.value,
  isRenamePending: isRenamePending.value,
  catalogRecovery: catalogRecovery.value,
  setNameInputRef: setBoardNameInputRef,
}))
const canvasChromeAccount = computed<CanvasChromeAccountState>(() => ({
  avatarUrl: accountControls.viewModel.avatarUrl.value,
  balance: accountControls.viewModel.balance.value,
  menuItems: accountControls.viewModel.menuItems.value,
}))
const { zoomLevel } = viewModel.viewportControls
const {
  onZoomIn,
  onZoomOut,
  onPanViewport,
  onFlowMoveEnd,
  onViewportChangeEnd,
  onMiniMapCenterRequested,
  onMiniMapDragMove,
  onMiniMapDragEnd,
} = commands.viewportControls
provideCanvasViewportContext({ zoomLevel })

const imageTools = {
  viewModel: viewModel.imageWorkflow.tools,
  commands: commands.imageWorkflow.tools,
}
const {
  viewModel: {
    locks: { isChromeHidden: isImageEditorChromeHidden, isCanvasLocked: isFlowCanvasLocked },
  },
} = imageTools
const { isFlowInteractionActive, isFlowViewportPanInteractionActive } = viewModel.graphEditing
const surface = useCanvasScreenSurfaceInteractions({
  workspace,
  flowPaneAdapter,
  canvasWrapRef,
  activeTool,
  isCanvasMinimapEnabled,
  duplicateModifierPressed,
  activePanels: imageToolUi.activePanels,
})
const {
  nodeContextMenu,
  canvasNodeMenuItems,
  vueFlowSelectionBindings,
  onFlowNodeMouseDown,
  groupPresentation,
  onFlowNodeClick,
  onDeleteInlineSelection,
  onNodeDragStop,
  onFlowConnectEnd,
  onFlowMove,
  onNodesChange,
  handleNodeContextMenuCreate,
  shell: {
    onCanvasWheel,
    onFlowMoveStart,
    onMiniMapDragStart,
    onNodeDragStart,
    onCanvasPaneClick,
    toggleCanvasMinimap,
    onPaneContextMenu,
    setActiveTool,
    onCanvasDragOver,
    onEscapeShortcut,
  },
} = surface
const { onCanvasContextMenuCapture, onCanvasPointerDownCapture } = groupPresentation
const nodePresentation = createCanvasScreenNodePresentation({
  workspace,
  activePanels: imageToolUi.activePanels,
  imageStages: imageToolUi.imageStages,
  duplicateModifierPressed,
  handleVisibilityRefreshToken: surface.flowHandleVisibilityRefreshToken,
  nodeInteractions: surface.nodeInteractions,
})
const {
  nodePresentationIntentRouter,
  nodePropsFor,
  onFlowNodeInternalsUpdate,
  isCanvasShortcutSuppressed,
} = nodePresentation
const onCanvasDrop = commands.graphEditing.onCanvasDrop
const onExportSelectedImages = commands.graphEditing.onExportSelectedImages

function handleMiniMapDragMove(payload: { delta: Parameters<typeof onMiniMapDragMove>[0] }) {
  onMiniMapDragMove(payload.delta)
}

const chromeProps = computed(() => ({
  inert: isImageEditorChromeHidden.value,
  isHidden: isImageEditorChromeHidden.value,
  board: canvasChromeBoard.value,
  account: canvasChromeAccount.value,
  tools: {
    activeTool: activeTool.value,
    minimapEnabled: isCanvasMinimapEnabled.value,
  },
  status: {
    exportDisabled: !viewModel.graphEditing.hasSelectedFlowNodes.value,
    zoomLevel: viewModel.viewportControls.zoomLevel.value,
  },
}))

const vueFlowProps = computed(() => {
  const isSelectTool = activeTool.value === 'select'
  const isHandTool = activeTool.value === 'hand'
  const isCanvasLocked = isFlowCanvasLocked.value

  return {
    panOnDrag: !isCanvasLocked && isHandTool,
    nodesDraggable: !isCanvasLocked && !isHandTool,
    nodesConnectable: !isCanvasLocked && !isHandTool,
    elementsSelectable: !isCanvasLocked && !isHandTool,
    selectNodesOnDrag: !isCanvasLocked && isSelectTool,
    selectionKeyCode: !isCanvasLocked && isSelectTool,
    multiSelectionKeyCode: !isCanvasLocked && isSelectTool ? 'Shift' : null,
    deleteKeyCode:
      isCanvasLocked || isCanvasShortcutSuppressed.value ? null : ['Backspace', 'Delete'],
    edgesUpdatable: false,
    defaultEdgeOptions: { animated: false, updatable: false },
    applyDefault: false,
    minZoom: CANVAS_VIEWPORT_MIN_ZOOM,
    maxZoom: CANVAS_VIEWPORT_MAX_ZOOM,
    zoomOnScroll: false,
    panOnScroll: !isCanvasLocked,
    zoomOnPinch: !isCanvasLocked,
    panOnScrollMode: PanOnScrollMode.Free,
    onlyRenderVisibleElements: true,
    ...connectionAdapter.vueFlowBindings,
  }
})

const minimapProps = computed(() => ({
  enabled: isCanvasMinimapEnabled.value,
  inert: isImageEditorChromeHidden.value,
  isHidden: isImageEditorChromeHidden.value,
}))

function updateBoardNameDraft(value: string) {
  boardNameDraft.value = value
}

function confirmBoardRename() {
  void onConfirmBoardRename()
}

function reloadBoardCatalog() {
  void reloadCatalog()
}

function selectBoard(boardId: string) {
  void onSelectBoard(boardId)
}

function exportSelectedImages() {
  void onExportSelectedImages()
}

function navigateBack() {
  canvasNavigationAdapter.navigate(USER_BOARDS_PATH)
}

useCanvasScreenLifecycle({
  route,
  persistence: commands.persistence,
  history: viewModel.persistence.history,
  duplicateModifierPressed,
  keyboard: {
    onDeleteInlineSelection,
    isShortcutSuppressed: () => isCanvasShortcutSuppressed.value,
    onEscape: onEscapeShortcut,
    onPanViewport,
    setActiveTool,
  },
  benchmark: {
    connectIntent: connectionAdapter.connectIntent,
    onNodesChange: commands.flowPane.onNodesChange,
  },
})
</script>

<style>
@import '../components/styles/canvas-surface-skin.css';
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
  width: 100%;
  height: 100vh;
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
