<template>
  <CanvasScreenPresentation :presentation="canvasScreenPresentation" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../../../app/stores/user'
import { uploadApi } from '../../../shared/api/upload'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { USER_BOARDS_PATH } from '../../../shared/constants/routePaths'
import type { NodeMouseEvent } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { useFlowKeyboardShortcuts } from '../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../composables/useFlowLeaveGuard'
import { useFlowImageActionStrengthControl } from '../composables/useFlowImageActionStrengthControl'
import type { CanvasConnectionPort, CanvasCreatableNodeKind } from '../../canvas-board/domain'
import type { CanvasGraphNodeId } from '../graph-runtime'
import { useFlowNodeDragInteractions } from '../composables/useFlowNodeDragInteractions'
import { useFlowImagePreviewLightbox } from '../composables/useFlowImagePreviewLightbox'
import { provideCanvasViewportContext } from '../composables/useCanvasViewportContext'
import {
  ACTIVE_NODE_PANEL_KIND,
  useFlowActiveNodePanels,
} from '../composables/useFlowActiveNodePanels'
import { useFlowCanvasShellInteractions } from '../composables/useFlowCanvasShellInteractions'
import { useCanvasGroupPresentationInteraction } from '../group-presentation'
import { useFlowExpandInteractionHandlers } from '../composables/useFlowExpandInteractionHandlers'
import {
  useFlowTransientUiDismissal,
  type DismissTransientCanvasUi,
} from '../composables/useFlowTransientUiDismissal'
import { useImageStageRegistry } from '../composables/useImageStageRegistry'
import { useFlowPaneSelectionAdapter, useVueFlowPaneAdapter } from '../flow-pane'
import { useCanvasWorkspaceRuntime } from '../workspace-runtime'
import {
  createFusionNodePresentationResolver,
  createImageNodePresentationResolver,
} from '../node-presentation'
import { CanvasScreenPresentation } from '../screen-presentation'
import { useCanvasCustomLayerOverlay } from '../overlay-lifecycle'
import type { CanvasChromeAccountState, CanvasChromeBoardState } from '../components/chrome'
import {
  useUserAccountControls,
  type UserShellNavigationAdapter,
} from '../../user-shell/useUserAccountControls'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })

function openRecharge() {
  accountControls.commands.openAccount('recharge')
}

const flowPaneAdapter = useVueFlowPaneAdapter()
const { screenToFlowCoordinate, viewport: flowViewport } = flowPaneAdapter

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
  openNodeImagePreview,
  openImageUrlPreview,
  logoutAndRedirect,
  startNodeDragInteraction: () => startNodeDragInteraction(),
  stopNodeDragInteraction: () => stopNodeDragInteraction(),
  refreshFlowHandleVisibility: () => refreshFlowHandleVisibility(),
  onNativeRemove: () => {
    dismissTransientCanvasUi({ closeExpand: true, closeContextMenu: true })
  },
  openRecharge,
})
const viewModel = workspace.viewModel
const commands = workspace.commands
const isGenerationAvailable = computed(() => commands.imageWorkflow.isGenerationAvailable())

const { nodes } = viewModel.flowPane
const { connectionAdapter } = commands.flowPane

type CanvasBenchmarkConnectDeleteTarget = {
  sourceNodeId: CanvasGraphNodeId
  targetNodeId: CanvasGraphNodeId
  sourcePort: CanvasConnectionPort
  targetPort: CanvasConnectionPort
  deleteNodeId: CanvasGraphNodeId
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
      connectionAdapter.connectIntent({
        sourceNodeId: target.sourceNodeId,
        targetNodeId: target.targetNodeId,
        sourcePort: target.sourcePort,
        targetPort: target.targetPort,
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
const { history: flowHistory } = viewModel.persistence

const { flushAutosave, loadCanvasBoardAndProbe, reattachPendingImageResults } = commands.persistence

const { zoomLevel } = viewModel.viewportControls
const {
  onZoomIn,
  onZoomOut,
  onPanViewport,
  onFlowMoveStart: onFlowMoveStartBase,
  onFlowMove: syncFlowMove,
  onFlowMoveEnd,
  onViewportChangeEnd,
  onMiniMapCenterRequested,
  onMiniMapDragStart: onMiniMapDragStartBase,
  onMiniMapDragMove,
  onMiniMapDragEnd,
  onNodeDragStart: onNodeDragStartBase,
  onNodeDragStop,
} = commands.viewportControls
provideCanvasViewportContext({ zoomLevel })

const {
  isFlowInteractionActive,
  isFlowViewportPanInteractionActive,
  isFlowInteractionLocked,
  nodeContextMenu,
  canvasNodeMenuItems,
} = viewModel.graphEditing
const { groupFrames, selectionGroupAction } = viewModel.graphEditing.grouping
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
const {
  selectGroupAtPoint,
  handlePaneClick,
  handleNodeClick,
  groupSelectedNodes,
  ungroupSelectedGroup,
  deleteSelectedGroup,
  groupDrag,
  groupResize,
} = commands.graphEditing.grouping

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
      onCanvasWheel: onCanvasWheelBase,
      focusAndShowChrome: focusImageEditorAndShowChrome,
      scheduleChromeAnchorUpdate: scheduleImageEditorChromeAnchorUpdate,
    },
    prompt: { runPromptAction: runFlowImageToolPromptAction },
  },
} = imageTools

const imagePresentationFor = createImageNodePresentationResolver({
  actionBindingFor: imageToolActionBindingFor,
  stageBindingFor: imageToolStageBindingFor,
}).forNode

const fusionPresentationFor = createFusionNodePresentationResolver({
  promptComposerVisibleFor: (nodeId) =>
    isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.fusionPrompt, nodeId),
}).forNode

const isCanvasShortcutSuppressed = computed(
  () => !!flowImageEditorNodeId.value || !!flowMaskEditorNodeId.value,
)
const isCanvasGroupInteractionEnabled = computed(
  () =>
    activeTool.value === 'select' &&
    !isFlowCanvasLocked.value &&
    !flowImageEditorNodeId.value &&
    !flowMaskEditorNodeId.value &&
    !flowExpandNodeId.value,
)

function onFlowNodeClick(payload: NodeMouseEvent) {
  handleNodeClick({
    node: payload.node,
    selectNode: () => selectionCommands.onFlowNodeClick(),
  })
}

function onDeleteInlineSelection() {
  if (!isCanvasGroupInteractionEnabled.value) return false
  return deleteSelectedGroup()
}

function performSelectionGroupAction() {
  const action = selectionGroupAction.value
  if (action?.kind === 'group') {
    groupSelectedNodes()
    return
  }
  if (action?.kind === 'ungroup') {
    ungroupSelectedGroup()
  }
}

function toggleCanvasMinimapBase() {
  isCanvasMinimapEnabled.value = !isCanvasMinimapEnabled.value
}

dismissTransientCanvasUi = useFlowTransientUiDismissal({
  cancelFlowExpandMode,
  closeBoardHeaderEditing,
  closeNodeContextMenu,
  clearActiveNodePanel,
  hideStrengthControl,
}).dismissTransientCanvasUi

useCanvasCustomLayerOverlay({
  visible: hasCanvasDismissableLayer,
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
  paneSelectionBindings,
  vueFlowSelectionBindings,
  commands: selectionCommands,
} = useFlowPaneSelectionAdapter({
  activeTool: activeTool as Ref<string>,
  dismissTransientCanvasUi,
  onEdgesChange: (changes) => commands.flowPane.onEdgesChange(changes),
})

const groupPresentation = useCanvasGroupPresentationInteraction({
  canvas: {
    host: canvasWrapRef,
    nodes,
    viewport: flowViewport,
    isInteractionEnabled: isCanvasGroupInteractionEnabled,
  },
  grouping: {
    frames: groupFrames,
    action: selectionGroupAction,
    selectAtPoint: selectGroupAtPoint,
    drag: groupDrag,
    resize: groupResize,
    onAction: performSelectionGroupAction,
  },
  pane: {
    toFlowPoint: screenToFlowCoordinate,
    onPointerDownCapture: paneSelectionBindings.onPointerDownCapture,
    dismiss: (options) => dismissTransientCanvasUi(options),
  },
})
const { bindClosePolicyHandlers, onCanvasContextMenuCapture, onCanvasPointerDownCapture } =
  groupPresentation

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
  onPaneContextMenu: onPaneContextMenuBase,
  handleNodeContextMenuCreate,
  setActiveTool: setActiveToolBase,
  onEscapeShortcut,
  onCanvasDragOver: onCanvasDragOverBase,
} = useFlowCanvasShellInteractions({
  canvasWrapRef,
  nodeContextMenu,
  activeTool,
  screenToFlowCoordinate,
  openNodeContextMenu,
  createNodeAtPosition,
  dismissTransientCanvasUi,
  clearFlowSelection: selectionCommands.clearSelection,
  onCanvasDropDragOver: onFlowCanvasDropDragOver,
  isFlowExpandActive: () => !!flowExpandNodeId.value,
  isBoardHeaderEditingActive: () => isBoardMenuOpen.value || isRenamingBoard.value,
})

const {
  onCanvasWheel,
  onFlowMoveStart,
  onMiniMapDragStart,
  onNodeDragStart,
  onCanvasPaneClick,
  toggleCanvasMinimap,
  onPaneContextMenu,
  setActiveTool,
  onCanvasDragOver,
} = bindClosePolicyHandlers({
  onCanvasWheel: onCanvasWheelBase,
  onFlowMoveStart: onFlowMoveStartBase,
  onMiniMapDragStart: onMiniMapDragStartBase,
  onNodeDragStart: onNodeDragStartBase,
  onCanvasPaneClick: () => handlePaneClick(selectionCommands.onPaneClick),
  toggleCanvasMinimap: toggleCanvasMinimapBase,
  onPaneContextMenu: onPaneContextMenuBase,
  setActiveTool: setActiveToolBase,
  onCanvasDragOver: onCanvasDragOverBase,
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
  selectFlowNodeForContextMenu: selectionCommands.selectNodeForContextMenu,
  cancelFlowExpandMode,
  cancelFlowMaskEditorMode,
  enterFlowExpandMode,
  enterFlowImageEditorMode,
  focusImageEditorAndShowChrome,
  runImagePromptAction: runFlowImageToolPromptAction,
  getActionStrength,
})

const canvasScreenPresentation = {
  workspace,
  chrome: {
    board: canvasChromeBoard,
    account: canvasChromeAccount,
    activeTool,
    minimapEnabled: isCanvasMinimapEnabled,
    isHidden: isImageEditorChromeHidden,
    closeBoardHeaderEditing,
    commands: {
      updateBoardNameDraft: (value: string) => {
        boardNameDraft.value = value
      },
      toggleBoardMenu,
      startBoardRename,
      cancelBoardRename,
      confirmBoardRename: () => void onConfirmBoardRename(),
      reloadBoardCatalog: () => void reloadCatalog(),
      selectBoard: (boardId: string) => void onSelectBoard(boardId),
      exportSelectedImages: () => void onExportSelectedImages(),
      navigateBack: () => canvasNavigationAdapter.navigate(USER_BOARDS_PATH),
      selectTool: setActiveTool,
      toggleMinimap: toggleCanvasMinimap,
      zoomOut: onZoomOut,
      zoomIn: onZoomIn,
    },
  },
  canvas: {
    containerRef: canvasWrapRef,
    isInteractionActive: isFlowInteractionActive,
    isViewportPanActive: isFlowViewportPanInteractionActive,
    isLocked: isFlowCanvasLocked,
    isShortcutSuppressed: isCanvasShortcutSuppressed,
    connectionBindings: connectionAdapter.vueFlowBindings,
    surface: {
      onDragOver: onCanvasDragOver,
      onDrop: onCanvasDrop,
      onPointerDownCapture: onCanvasPointerDownCapture,
      onContextMenuCapture: onCanvasContextMenuCapture,
      onWheel: onCanvasWheel,
    },
    flow: {
      onNodesChange: commands.flowPane.onNodesChange,
      onEdgesChange: vueFlowSelectionBindings.onEdgesChange,
      onConnectEnd: onFlowConnectEnd,
      onMoveStart: onFlowMoveStart,
      onMove: onFlowMove,
      onMoveEnd: onFlowMoveEnd,
      onViewportChangeEnd,
      onNodeClick: onFlowNodeClick,
      onNodeDragStart,
      onNodeDragStop,
      onSelectionStart: vueFlowSelectionBindings.onSelectionStart,
      onSelectionEnd: vueFlowSelectionBindings.onSelectionEnd,
      onPaneClick: onCanvasPaneClick,
      onPaneContextMenu,
    },
    group: groupPresentation,
    node: {
      copyMode: duplicateModifierPressed,
      getUiState: getFlowNodeUiStateCached,
      imagePresentationFor,
      fusionPresentationFor,
      generationAvailable: isGenerationAvailable,
      setImageStageRef,
      clearImageStageRef,
      expand: {
        nodeId: flowExpandNodeId,
        overlayStyle: flowExpandOverlayStyle,
        handleRadius: flowExpandHandleRadius,
        visibilityRefreshToken: flowHandleVisibilityRefreshToken,
      },
      commands: {
        updatePromptText: onFlowPromptTextUpdate,
        updateNodeParameter: onFlowNodeParamUpdate,
        deleteNode: onFlowDeleteNode,
        dropMediaInput: onFlowMediaInputImageDrop,
        openMediaInput: onFlowMediaInputImageOpen,
        previewMediaInput: onFlowMediaInputImagePreview,
        removeMediaInput: onFlowMediaInputImageRemove,
        generateFromPromptNode: onFlowPromptGenerate,
        dropImageStage: onFlowImageStageDrop,
        openImageFile: onFlowImageOpenFile,
        previewImage: onFlowImageOpenPreview,
        appendImageToCanvas: onFlowImageAppendToCanvas,
        resetImage: onFlowImageReset,
        openImageContextMenu: onFlowImageContextMenu,
        generateFromImagePrompt: onFlowImagePromptGenerate,
        submitCanvasExtend: onFlowImageExpandSubmit,
        runImageAction: onFlowImageDirectAction,
        beginCanvasExtendHandle: onFlowExpandHandlePointerDown,
        moveCanvasExtendHandle: onFlowExpandHandlePointerMove,
        endCanvasExtendHandle: onFlowExpandHandlePointerUp,
        beginCanvasExtendFrame: onFlowExpandFramePointerDown,
        openFusionContextMenu: onFlowFusionContextMenu,
        generateFromFusion: onFlowFusionPromptGenerate,
      },
      listeners: {
        nodeMousedown: selectionCommands.onFlowNodeMouseDown,
        requestInternalsUpdate: onFlowNodeInternalsUpdate,
      },
    },
    minimap: {
      enabled: isCanvasMinimapEnabled,
      isHidden: isImageEditorChromeHidden,
      commands: {
        centerRequested: onMiniMapCenterRequested,
        dragStart: onMiniMapDragStart,
        dragMove: handleMiniMapDragMove,
        dragEnd: onMiniMapDragEnd,
      },
    },
    overlays: {
      imageToolSession: imageTools,
      nodeContextMenu,
      canvasNodeMenuItems,
      createNode: handleNodeContextMenuCreate,
    },
  },
}

watch(
  () => route.query.boardId,
  async () => {
    await loadCanvasBoardAndProbe()
    void reattachPendingImageResults()
  },
)

useFlowKeyboardShortcuts({
  onSave: () => {
    void flushAutosave({ silent: false })
  },
  onUndo: flowHistory.undo,
  onRedo: flowHistory.redo,
  onDeleteInlineSelection,
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
})
</script>
