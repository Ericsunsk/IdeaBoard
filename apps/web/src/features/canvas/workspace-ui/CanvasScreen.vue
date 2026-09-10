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
      @reload-board-catalog="reloadCatalog"
      @select-board="selectBoard"
      @export="exportSelectedImages"
      @navigate-back="navigateBack"
      @select-tool="setActiveTool"
      @toggle-library="canvasLibrary.commands.toggle"
      @library-focus-change="canvasLibrary.commands.setFocusWithin"
      @toggle-minimap="toggleCanvasMinimap"
      @zoom-out="onZoomOut"
      @zoom-in="onZoomIn"
    >
      <template #library>
        <CanvasLibraryFloatingIsland
          presentation="canvas"
          :open="canvasLibrary.view.open.value"
          :folder-facts="canvasLibrary.view.folderFacts"
          :mutation-publisher="canvasLibrary.view.mutationPublisher"
          @add-to-canvas="addLibraryAssetToCanvas"
          @close="canvasLibrary.commands.close"
          @focus-scope-change="canvasLibrary.commands.setFocusWithin"
        />
      </template>
    </CanvasChrome>

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
        :edges="renderEdges"
        v-bind="vueFlowProps"
        @nodes-change="onNodesChange"
        @edges-change="vueFlowSelectionBindings.onEdgesChange"
        @move-start="onFlowMoveStart"
        @move="onFlowMove"
        @move-end="onFlowMoveEnd"
        @viewport-change-end="onViewportChangeEnd"
        @node-click="onFlowNodeClick"
        @connect-start="connectionDropLifecycle.onConnectStart"
        @connect-end="onFlowConnectEnd"
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
            :connection-drop-target="connectionDropTargetNodeId === nodeProps.id"
            @node-intent="nodePresentationIntentRouter.dispatch"
            @node-mousedown="onFlowNodeMouseDown"
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
import '../components/styles/canvas-surface-skin.css'
import '../components/styles/canvas-vue-flow-adapter-skin.css'
import '../components/styles/canvas-style-contracts.css'
import { computed, defineAsyncComponent, onBeforeUnmount, ref, watch } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { uploadApi } from '../../../shared/api/upload'
import { USER_BOARDS_PATH } from '../../../shared/constants/routePaths'
import { PanOnScrollMode, useVueFlow, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useCanvasImageLightbox } from './useCanvasImageLightbox'
import { provideCanvasHandleProximityCoordinator } from '../vue-flow-adapter/useCanvasHandleProximity'
import { provideCanvasViewportContext } from '../vue-flow-adapter/useCanvasViewportContext'
import { CanvasGroupPresentation } from '../group-presentation'
import {
  createCanvasConnectionDropLifecycle,
  useCanvasVueFlowHandleGeometryAdapter,
  useVueFlowAdapter,
} from '../vue-flow-adapter'
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
} from '../workspace-chrome'
import { CANVAS_VIEWPORT_MAX_ZOOM, CANVAS_VIEWPORT_MIN_ZOOM } from '../utils/canvasViewportStorage'
import { useWorkspaceAccountControls, type WorkspaceNavigationAdapter } from '../../workspace'
import { useCanvasScreenImageToolUi } from './internal/useCanvasScreenImageToolUi'
import { useCanvasScreenSurfaceInteractions } from './internal/useCanvasScreenSurfaceInteractions'
import { createCanvasScreenNodePresentation } from './internal/createCanvasScreenNodePresentation'
import { useCanvasScreenLifecycle } from './internal/useCanvasScreenLifecycle'
import { useCanvasLibraryIsland } from './internal/useCanvasLibraryIsland'
import { loadLibraryFloatingIsland, materializeLibraryAssetForCanvas } from '../../library'
import type { CanvasLibraryImportDescriptor } from '../../library'

const props = defineProps<{
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
}>()

const { route, router, getUserIdentity, logoutAndRedirect } = props

const CanvasLibraryFloatingIsland = defineAsyncComponent(loadLibraryFloatingIsland)

function openRecharge() {
  accountControls.commands.openAccount('recharge')
}

const flowPaneAdapter = useVueFlowAdapter()
const { connectionStartHandle } = useVueFlow()
const handleProximityGeometry = useCanvasVueFlowHandleGeometryAdapter()
const handleProximitySurface = provideCanvasHandleProximityCoordinator(handleProximityGeometry)

const canvasWrapRef = ref<HTMLElement | null>(null)
const activeTool = ref<'select' | 'hand'>('select')
const isCanvasMinimapEnabled = ref(false)
const duplicateModifierPressed = ref(false)
const canvasLibrary = useCanvasLibraryIsland()
const imageToolUi = useCanvasScreenImageToolUi({ activeTool, canvasWrapRef })
const { openNodeImagePreview, openImageUrlPreview } = useCanvasImageLightbox()
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
  materializeLibraryAsset: materializeLibraryAssetForCanvas,
  openNodeImagePreview,
  openImageUrlPreview,
  imageWorkflowUi: imageToolUi.imageWorkflowUi,
  logoutAndRedirect,
  openRecharge,
})
const viewModel = workspace.viewModel
const commands = workspace.commands

const { nodes, renderEdges } = viewModel.flowPane
const { connectionAdapter } = commands.flowPane
const connectionDropTargetNodeId = ref<string | null>(null)
const connectionDropLifecycle = createCanvasConnectionDropLifecycle({
  document,
  connect: connectionAdapter.vueFlowBindings.onConnect,
  onTargetChange: (nodeId) => {
    connectionDropTargetNodeId.value = nodeId
  },
})

function onFlowConnectEnd(event: MouseEvent | TouchEvent | undefined) {
  connectionDropLifecycle.onConnectEnd(event)
}

watch(
  connectionStartHandle,
  (handle) => {
    handleProximitySurface.setSuspended(handle !== null)
    if (handle === null) connectionDropLifecycle.cancel()
  },
  { immediate: true, flush: 'sync' },
)
onBeforeUnmount(connectionDropLifecycle.dispose)

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

const canvasNavigationAdapter: WorkspaceNavigationAdapter = {
  navigate: onNavigateRequest,
  logout: onLogoutRequest,
}
const accountControls = useWorkspaceAccountControls({
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
watch(zoomLevel, handleProximitySurface.requestRefresh, { flush: 'post' })

const imageTools = {
  viewModel: viewModel.imageWorkflow.tools,
  commands: commands.imageWorkflow.tools,
}
const {
  viewModel: {
    locks: { isChromeHidden: isRetouchChromeHidden, isCanvasLocked: isFlowCanvasLocked },
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
  nodeInteractions: surface.nodeInteractions,
})
const {
  nodePresentationIntentRouter,
  nodePropsFor,
  isCanvasShortcutSuppressed: isNodePresentationShortcutSuppressed,
} = nodePresentation
const isCanvasShortcutSuppressed = computed(
  () => canvasLibrary.view.focusWithin.value || isNodePresentationShortcutSuppressed.value,
)
const onCanvasDrop = commands.graphEditing.onCanvasDrop
const onExportSelectedImages = commands.graphEditing.onExportSelectedImages

function addLibraryAssetToCanvas(asset: CanvasLibraryImportDescriptor) {
  const bounds = canvasWrapRef.value?.getBoundingClientRect()
  if (!bounds) return
  const center = flowPaneAdapter.screenToFlowCoordinate({
    x: bounds.left + bounds.width / 2,
    y: bounds.top + bounds.height / 2,
  })
  commands.graphEditing.libraryImport.add(asset, center)
}

function handleMiniMapDragMove(payload: { delta: Parameters<typeof onMiniMapDragMove>[0] }) {
  onMiniMapDragMove(payload.delta)
}

const chromeProps = computed(() => ({
  inert: isRetouchChromeHidden.value,
  isHidden: isRetouchChromeHidden.value,
  board: canvasChromeBoard.value,
  account: canvasChromeAccount.value,
  tools: {
    activeTool: activeTool.value,
    libraryMounted: canvasLibrary.view.mounted.value,
    libraryOpen: canvasLibrary.view.open.value,
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
    connectOnClick: false,
    defaultEdgeOptions: { animated: false, updatable: false },
    applyDefault: false,
    minZoom: CANVAS_VIEWPORT_MIN_ZOOM,
    maxZoom: CANVAS_VIEWPORT_MAX_ZOOM,
    zoomOnScroll: false,
    panOnScroll: !isCanvasLocked,
    zoomOnPinch: !isCanvasLocked,
    panOnScrollMode: PanOnScrollMode.Free,
    onlyRenderVisibleElements: false,
    ...connectionAdapter.vueFlowBindings,
  }
})

const minimapProps = computed(() => ({
  enabled: isCanvasMinimapEnabled.value,
  inert: isRetouchChromeHidden.value,
  isHidden: isRetouchChromeHidden.value,
}))

function updateBoardNameDraft(value: string) {
  boardNameDraft.value = value
}

function confirmBoardRename() {
  void onConfirmBoardRename()
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

<style scoped>
.canvas-layout {
  width: 100%;
  height: 100vh;
  position: relative;
  overflow: hidden;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-flow-wrap {
  --color-ib-page-dot: #c9ced8;
  position: absolute;
  inset: 0;
  z-index: 2;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}
</style>
