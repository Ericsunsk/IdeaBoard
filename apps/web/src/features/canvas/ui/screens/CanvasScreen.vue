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
      @reload-catalog="reloadCatalog"
      @restore-archived-board="restoreArchivedBoard"
      @select-board="selectBoard"
      @share="openSharePanel"
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
          ref="canvasLibraryIslandRef"
          presentation="canvas"
          :open="canvasLibrary.view.isOpen.value && !isRetouchChromeHidden"
          :workspace-id="canvasWorkspaceId"
          :drop-targets="libraryDropTargets"
          :folder-facts="canvasLibrary.view.folderFacts"
          :mutation-publisher="canvasLibrary.view.mutationPublisher"
          :save-module="canvasLibrary.view.saveModule"
          @add-to-canvas="addLibraryAssetToCanvas"
          @close="canvasLibrary.commands.close"
          @focus-scope-change="canvasLibrary.commands.setFocusWithin"
        />
      </template>
      <template #presence>
        <PresenceRoster :peers="presence.remotePeers.value" :local-user="presenceUser" />
      </template>
    </CanvasChrome>

    <div
      ref="canvasWrapRef"
      class="canvas-flow-wrap page-dot-bg"
      :class="{ 'is-single-node-selection': isSingleNodeSelection }"
      :style="nodeUiStyle"
      @dragover.prevent="onCanvasDragOver"
      @drop.prevent="onCanvasDrop"
      @pointerdown.capture="onCanvasPointerDownCapture"
      @mousedown.capture="onCanvasMouseDownCapture"
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
        @node-drag-start="onGuardedNodeDragStart"
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
            :is-connection-drop-target="connectionDropTargetNodeId === nodeProps.id"
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
        :is-visible="nodeContextMenu.isVisible"
        :x="nodeContextMenu.x"
        :y="nodeContextMenu.y"
        :items="canvasNodeMenuItems"
        @create="handleNodeContextMenuCreate"
      />
      <ShareDialog :panel="sharePanel" :ensure-draft-saved="ensureCanvasDraftSaved" />
    </div>
  </div>
</template>

<script setup lang="ts">
import '../styles/canvas-surface-skin.css'
import '../styles/canvas-vue-flow-adapter-skin.css'
import '../styles/canvas-style-contracts.css'
import { computed, defineAsyncComponent, onBeforeUnmount, ref, watch } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { uploadApi } from '../../../../api/upload.ts'
import { USER_BOARDS_PATH } from '../../../workspace/routePaths.ts'
import {
  PanOnScrollMode,
  useVueFlow,
  VueFlow,
  type FlowProps,
  type NodeDragEvent,
} from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useCanvasImageLightbox } from './useCanvasImageLightbox'
import { CanvasGroupPresentation } from '../nodes/groups'
import {
  createCanvasConnectionDropLifecycle,
  provideCanvasHandleProximityCoordinator,
  provideCanvasNodeUiStateContext,
  provideCanvasViewportContext,
  resolveCanvasNodeUiScale,
  useCanvasVueFlowHandleGeometryAdapter,
  useVueFlowAdapter,
} from '../../session/flow-adapter'
import {
  createCanvasGraphEditingBrowserAdapter,
  createCanvasGraphInteractionBrowserAdapter,
} from '../../session/graph-editing'
import { useCanvasWorkspaceRuntime } from '../../session/workspace'
import { FlowCanvasNode } from '../nodes'
import { CanvasNodeContextMenu } from '../overlays'
import { ImageToolPresentation } from '../../tools/image-tools'
import {
  CanvasChrome,
  CanvasChromeMiniMap,
  type CanvasChromeAccountState,
  type CanvasChromeBoardState,
} from '../chrome/components'
import {
  CANVAS_VIEWPORT_MAX_ZOOM,
  CANVAS_VIEWPORT_MIN_ZOOM,
} from '../../session/browser/canvasViewportStorage.ts'
import {
  useWorkspaceAccountControls,
  useWorkspaceSwitcher,
  type WorkspaceNavigationAdapter,
} from '../../../workspace'
import { getWorkspaceSelectionStore } from '../../../workspace'
import {
  observeOpenBoardAccessLoss,
  ShareDialog,
  subscribeBoardAccessLost,
  useSharePanel,
} from '../../../board/ports/canvasSession'
import { useCanvasScreenImageToolUi } from './internal/useCanvasScreenImageToolUi'
import { useCanvasScreenSurfaceInteractions } from './internal/useCanvasScreenSurfaceInteractions'
import type { CanvasScreenLibrarySaveBridge } from './internal/useCanvasScreenSurfaceInteractions'
import { createCanvasScreenNodePresentation } from './internal/createCanvasScreenNodePresentation'
import { useCanvasScreenLifecycle } from './internal/useCanvasScreenLifecycle'
import PresenceRoster from './presence/PresenceRoster.vue'
import { useCanvasPresenceSession, type PresenceUser } from '../../session/collaboration'
import { useCanvasCollaborationSession } from '../../session/collaboration'
import { useSoftLocks } from './presence/useSoftLocks'
import { useCanvasLibraryIsland } from './internal/useCanvasLibraryIsland'
import { useCanvasLibraryDrop } from './internal/useCanvasLibraryDrop'
import {
  loadLibraryFloatingIsland,
  materializeLibraryAssetForCanvas,
  requireWorkspaceId,
  type LibraryDropHit,
} from '../../../library'
import type { CanvasLibraryImportDescriptor } from '../../../library'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { i18n } from '../../../../core/i18n'
import { useUserStore } from '../../../auth'

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
const selectedTool = ref<'select' | 'hand'>('select')
const isSpacePressed = ref(false)
const activeTool = computed({
  get: () => (isSpacePressed.value ? 'hand' : selectedTool.value),
  set: (tool: 'select' | 'hand') => {
    selectedTool.value = tool
  },
})
const isCanvasMinimapEnabled = ref(false)
const isDuplicateModifierPressed = ref(false)
const canvasLibrary = useCanvasLibraryIsland()
const toast = useAppToast()
// Workspace id comes from the shared selection store (primed from persisted
// selection); no workspace query is issued from the Canvas screen.
const workspaceSelectionStore = getWorkspaceSelectionStore()
workspaceSelectionStore.getSelectedId()
const canvasWorkspaceId = workspaceSelectionStore.selectedId

function requireCanvasWorkspaceId(): string {
  return requireWorkspaceId(canvasWorkspaceId.value)
}
const userStore = useUserStore()
const presenceUser = computed<PresenceUser | null>(() => {
  const sessionUser = userStore.user
  const id = sessionUser?.public_id?.trim() || getUserIdentity().userUid.trim()
  if (!id) return null
  const name = sessionUser?.display_name?.trim() || id.replace(/^usr_/, '') || id
  const avatarUrl = sessionUser?.avatar?.trim()
  return {
    id,
    name,
    ...(avatarUrl ? { avatarUrl } : {}),
  }
})
const selectedElementIds = computed(() =>
  flowPaneAdapter.getSelectedElements.value.map((element) => element.id),
)
// Viewers (and archived boards) never write: empty selection broadcasts,
// no soft-lock claims, no Y.Doc mutations.
const collaboration = useCanvasCollaborationSession({
  createTransport: (context) =>
    useCanvasPresenceSession({
      ...context,
      workspaceId: canvasWorkspaceId,
      localUser: presenceUser,
      selectedIds: selectedElementIds,
    }),
})
const presence = collaboration.transport
const canWritePresence = collaboration.canWrite
const isCollaborationReadOnly = collaboration.isReadOnly

const imageToolUi = useCanvasScreenImageToolUi({ activeTool, canvasWrapRef })
const { openNodeImagePreview, openImageUrlPreview } = useCanvasImageLightbox()
const graphEditingBrowser = createCanvasGraphEditingBrowserAdapter()
const graphInteractionBrowser = createCanvasGraphInteractionBrowserAdapter()

const workspace = useCanvasWorkspaceRuntime({
  route,
  router,
  getUserIdentity,
  getWorkspaceId: requireCanvasWorkspaceId,
  flowPaneAdapter,
  uploadApi,
  graphEditingBrowser,
  graphInteractionBrowser,
  materializeLibraryAsset: (assetId) =>
    materializeLibraryAssetForCanvas(requireCanvasWorkspaceId(), assetId),
  openNodeImagePreview,
  openImageUrlPreview,
  imageWorkflowUi: imageToolUi.imageWorkflowUi,
  logoutAndRedirect,
  openRecharge,
  collaboration,
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
    if (handle === null) {
      connectionDropLifecycle.cancel()
      collaboration.activity.end('connect')
    } else collaboration.activity.begin('connect')
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
  isBoardArchived,
  isBoardReadOnly,
  isRestoringArchivedBoard,
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
  restoreArchivedBoard: onRestoreArchivedBoard,
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
  shouldShowHome: false,
})
// Avatar dropdown workspace row (same `useWorkspaceSwitcher` view-model as
// the workspace/admin headers): list + switch with current highlight, plus
// the gear settings dialog reused from `UserAccountMenu`.
const workspaceSwitcher = useWorkspaceSwitcher({
  beforeWorkspaceChange: commands.persistence.saveBeforeScopeChange,
})

// Share panel (issue #938): the canvas opens the single Share boundary
// over the current board. Invites require the local draft to exist and
// the autosave to flush first; the command flow re-validates the server
// board before every write.
const sharePanel = useSharePanel()

// Confirmed access loss (issue #941): the open board revalidates on
// directed/roster signals, SSE gaps, and hidden-tab recovery; a proven
// loss closes Share, purges protected state, toasts once, and replaces
// the route with an authorized Gallery (no history entry back to the dead
// canvas). Unconfirmed errors preserve the draft and never navigate.
const stopAccessLossObservation = observeOpenBoardAccessLoss({
  getBoardId: () => currentBoardId.value,
})
const stopAccessLossSubscription = subscribeBoardAccessLost((boardId) => {
  if (!boardId || currentBoardId.value !== boardId) return
  sharePanel.close()
  toast.error(i18n.global.t('canvas.session.boardAccessLost'))
  void router.replace(USER_BOARDS_PATH)
})
onBeforeUnmount(() => {
  stopAccessLossObservation()
  stopAccessLossSubscription()
})

function openSharePanel() {
  const boardId = currentBoardId.value
  if (!boardId) return
  sharePanel.open(boardId, currentBoardName.value)
}

async function ensureCanvasDraftSaved(boardId: string): Promise<boolean> {
  if (currentBoardId.value !== boardId) return false
  await commands.persistence.flushAutosave()
  return currentBoardId.value === boardId
}

const canvasChromeBoard = computed<CanvasChromeBoardState>(() => ({
  currentName: currentBoardName.value,
  options: boardOptions.value,
  currentId: currentBoardId.value,
  isMenuOpen: isBoardMenuOpen.value,
  isRenaming: isRenamingBoard.value,
  isRenamePending: isRenamePending.value,
  catalogRecovery: catalogRecovery.value,
  archived: {
    isArchived: isBoardArchived.value,
    isRestoring: isRestoringArchivedBoard.value,
  },
  isReadOnly: isBoardReadOnly.value,
  setNameInputRef: setBoardNameInputRef,
}))
const canvasChromeAccount = computed<CanvasChromeAccountState>(() => ({
  avatarUrl: accountControls.viewModel.avatarUrl.value,
  userEmail: accountControls.viewModel.userEmail.value,
  balance: accountControls.viewModel.balance.value,
  menuItems: accountControls.viewModel.menuItems.value,
  workspace: workspaceSwitcher.menuProps.value,
}))
const { zoomLevel, imageQuality } = viewModel.viewportControls
const nodeUiStyle = computed(() => ({
  '--canvas-node-ui-scale': String(resolveCanvasNodeUiScale(zoomLevel.value)),
}))
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
provideCanvasViewportContext({ zoomLevel, imageQuality })
provideCanvasNodeUiStateContext({
  getFlowNodeUiState: (...args) => commands.graphEditing.getFlowNodeUiStateCached(...args),
})
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
const libraryDropTargets = useCanvasLibraryDrop({
  element: canvasWrapRef,
  context: () => [canvasWorkspaceId.value, currentBoardId.value, isFlowCanvasLocked.value],
  canReceive: () => !isFlowCanvasLocked.value,
  screenToFlowCoordinate: flowPaneAdapter.screenToFlowCoordinate,
  add: (asset, point) => commands.graphEditing.libraryImport.add(asset, point),
})
const { isFlowInteractionActive, isFlowViewportPanInteractionActive } = viewModel.graphEditing
// Soft locks + Y.Doc binding (TapNow §3§5): a single-node selection claims
// a purely client-side lock (smaller deviceId wins, viewers abstain).
// Locks carry no node visualization; locked nodes are only excluded from
// drag cohorts. The binding observes the
// room's `nodes`/`edges` maps with the local device origin. Canonical graph
// commits write only their changed ids; remote snapshots replace the graph
// projection without generating another local commit.
// Vue Flow mounts `.vue-flow__nodesselection-rect` for a single selected
// node as well (`!userSelectionActive && bbox`), so pure CSS cannot tell
// single from multi. The adapter skin restores the multi-node group ring
// on `::before`; this flag suppresses exactly the single-node case.
const isSingleNodeSelection = computed(
  () =>
    flowPaneAdapter.getSelectedElements.value.filter((element) => {
      const candidate = element as { source?: unknown; target?: unknown }
      return !(typeof candidate.source === 'string' && typeof candidate.target === 'string')
    }).length === 1,
)
const softLocks = useSoftLocks({
  peers: presence.remotePeers,
  selectedIds: selectedElementIds,
  deviceId: presence.localClientId,
  canWrite: canWritePresence,
})
type CanvasLibraryIslandInstance = {
  setExternalDragHover: (hit: LibraryDropHit | null) => void
  getSelectedFolderKey: () => string
  isFolderKnown: (key: string) => boolean
} | null
const canvasLibraryIslandRef = ref<CanvasLibraryIslandInstance>(null)
const surface = useCanvasScreenSurfaceInteractions({
  workspace,
  activity: collaboration.activity,
  flowPaneAdapter,
  canvasWrapRef,
  activeTool,
  isCanvasMinimapEnabled,
  isDuplicateModifierPressed,
  activePanels: imageToolUi.activePanels,
  librarySave: {
    saveModule: canvasLibrary.view.saveModule,
    isIslandOpen: () => canvasLibrary.view.isOpen.value,
    setIslandHover: (hit) => canvasLibraryIslandRef.value?.setExternalDragHover(hit),
    getSelectedFolderKey: () => canvasLibraryIslandRef.value?.getSelectedFolderKey() ?? 'all',
    isFolderKnown: (key) => canvasLibraryIslandRef.value?.isFolderKnown(key) ?? false,
    notifyError: (message) => toast.error(message),
  } satisfies CanvasScreenLibrarySaveBridge,
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
    setActiveTool,
    onCanvasDragOver,
    onEscapeShortcut,
  },
} = surface
const {
  onCanvasContextMenuCapture,
  onCanvasPointerDownCapture,
  onCanvasMouseDownCapture,
  onPaneContextMenu,
} = surface
const nodePresentation = createCanvasScreenNodePresentation({
  workspace,
  activePanels: imageToolUi.activePanels,
  imageStages: imageToolUi.imageStages,
  isDuplicateModifierPressed,
  nodeInteractions: surface.nodeInteractions,
})
const {
  nodePresentationIntentRouter,
  nodePropsFor,
  isCanvasShortcutSuppressed: isNodePresentationShortcutSuppressed,
} = nodePresentation
const isCanvasShortcutSuppressed = computed(
  () => canvasLibrary.view.isFocusWithin.value || isNodePresentationShortcutSuppressed.value,
)
const onCanvasDrop = commands.graphEditing.onCanvasDrop
const onExportSelectedImages = commands.graphEditing.onExportSelectedImages

// Soft-lock drag guard: nodes locked by another editor leave the drag cohort.
// GraphSession commits final coordinates on drop and publishes that commit.
function onGuardedNodeDragStart(payload: NodeDragEvent) {
  const editable = softLocks.filterEditable(payload.nodes.map((node) => node.id))
  if (editable.length === 0) return
  if (editable.length === payload.nodes.length) {
    onNodeDragStart(payload)
    return
  }
  onNodeDragStart({
    ...payload,
    nodes: payload.nodes.filter((node) => editable.includes(node.id)),
  })
}

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
  isInert: isRetouchChromeHidden.value,
  isHidden: isRetouchChromeHidden.value,
  board: canvasChromeBoard.value,
  account: canvasChromeAccount.value,
  tools: {
    activeTool: activeTool.value,
    isLibraryMounted: canvasLibrary.view.isMounted.value,
    isLibraryOpen: canvasLibrary.view.isOpen.value,
    isMinimapEnabled: isCanvasMinimapEnabled.value,
  },
  status: {
    isExportDisabled: !viewModel.graphEditing.hasSelectedFlowNodes.value,
    zoomLevel: viewModel.viewportControls.zoomLevel.value,
  },
}))

const isEditLocked = computed(
  () =>
    isFlowCanvasLocked.value ||
    isBoardArchived.value ||
    isBoardReadOnly.value ||
    isCollaborationReadOnly.value,
)

const vueFlowProps = computed(() => {
  const isSelectTool = activeTool.value === 'select'
  const isHandTool = activeTool.value === 'hand'
  // Edit lock covers every non-writable state: flow tool lock, archived,
  // and viewer read-only (server capabilities). Any residual programmatic
  // edit still funnels through the save path, where 409 `board_archived` /
  // 403 `board_forbidden` re-asserts the banner without toast spam.
  // Camera lock is separate: viewer (read-only) never locks navigation so
  // the canvas stays explorable; only flow lock and archived lock pan/zoom.
  const isCameraLocked = isFlowCanvasLocked.value || isBoardArchived.value

  return {
    panOnDrag: isCameraLocked ? false : isHandTool ? [0, 1, 2] : [1, 2],
    nodesDraggable: !isEditLocked.value && !isHandTool,
    nodesConnectable: !isEditLocked.value && !isHandTool,
    elementsSelectable: !isEditLocked.value && !isHandTool,
    selectNodesOnDrag: !isEditLocked.value && isSelectTool,
    selectionKeyCode: !isEditLocked.value && isSelectTool,
    multiSelectionKeyCode: !isEditLocked.value && isSelectTool ? 'Shift' : null,
    deleteKeyCode:
      isEditLocked.value || isCanvasShortcutSuppressed.value ? null : ['Backspace', 'Delete'],
    edgesUpdatable: false,
    connectOnClick: false,
    defaultEdgeOptions: {
      animated: false,
      updatable: false,
      class: (edge) => (edge.sourceNode.selected ? 'is-source-selected' : undefined),
    },
    applyDefault: false,
    minZoom: CANVAS_VIEWPORT_MIN_ZOOM,
    maxZoom: CANVAS_VIEWPORT_MAX_ZOOM,
    zoomOnScroll: false,
    panOnScroll: !isCameraLocked,
    panActivationKeyCode: null,
    panOnScrollSpeed: 1.2,
    autoPanSpeed: 15,
    zoomOnDoubleClick: false,
    zoomOnPinch: !isCameraLocked,
    panOnScrollMode: PanOnScrollMode.Free,
    onlyRenderVisibleElements: false,
    ...connectionAdapter.vueFlowBindings,
  } satisfies Partial<FlowProps>
})

const minimapProps = computed(() => ({
  isEnabled: isCanvasMinimapEnabled.value,
  isInert: isRetouchChromeHidden.value,
  isHidden: isRetouchChromeHidden.value,
}))

function updateBoardNameDraft(value: string) {
  boardNameDraft.value = value
}

function confirmBoardRename() {
  void onConfirmBoardRename()
}

function restoreArchivedBoard() {
  void onRestoreArchivedBoard()
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
  isDuplicateModifierPressed,
  keyboard: {
    onDeleteInlineSelection,
    isShortcutSuppressed: () => isCanvasShortcutSuppressed.value,
    onEscape: onEscapeShortcut,
    onPanViewport,
    setActiveTool,
    onSpacePressedChange: (isPressed) => {
      isSpacePressed.value = isPressed
    },
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
  --color-page-dot: var(--color-canvas-dot);
  position: absolute;
  inset: 0;
  z-index: 2;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}
</style>
