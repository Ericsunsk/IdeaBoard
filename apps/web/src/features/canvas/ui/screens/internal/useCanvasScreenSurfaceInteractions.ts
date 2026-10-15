import { computed, type ComputedRef, type Ref } from 'vue'
import type { NodeChange, NodeDragEvent, NodeMouseEvent } from '@vue-flow/core'
import type { CanvasCreatableNodeKind } from '../../../../board/ports/canvas'
import { useFlowNodeDragInteractions } from '../useFlowNodeDragInteractions'
import { useFlowCanvasShellInteractions } from '../useFlowCanvasShellInteractions'
import { useFlowTransientUiDismissal } from '../useFlowTransientUiDismissal'
import { useCanvasCustomLayerOverlay } from '../../overlays'
import {
  useVueFlowSelectionAdapter,
  type CanvasVueFlowAdapter,
} from '../../../session/flow-adapter'
import { useCanvasGroupPresentationInteraction } from '../../nodes/groups'
import type { CanvasWorkspaceRuntime } from '../../../session/workspace'
import { useCanvasNodeContextMenu } from '../useCanvasNodeContextMenu'
import type { useFlowActiveNodePanels } from '../useFlowActiveNodePanels'

type ActivePanels = ReturnType<typeof useFlowActiveNodePanels>

type CanvasScreenSurfaceInteractionOptions = {
  workspace: CanvasWorkspaceRuntime
  flowPaneAdapter: CanvasVueFlowAdapter
  canvasWrapRef: Ref<HTMLElement | null>
  activeTool: Ref<'select' | 'hand'>
  isCanvasMinimapEnabled: Ref<boolean>
  isDuplicateModifierPressed: Ref<boolean>
  activePanels: ActivePanels
}

function useScreenTransientUi(
  options: CanvasScreenSurfaceInteractionOptions,
  contextMenu: ReturnType<typeof useCanvasNodeContextMenu>,
) {
  const { commands } = options.workspace
  const { hideStrengthControl, cancelOutpaint } = commands.imageWorkflow
  const hasCanvasDismissableLayer = computed(
    () =>
      contextMenu.nodeContextMenu.value.isVisible ||
      options.activePanels.activeNodePanel.value !== null,
  )
  const { dismissTransientCanvasUi } = useFlowTransientUiDismissal({
    cancelFlowOutpaintMode: cancelOutpaint,
    closeBoardHeaderEditing: commands.boardSession.closeBoardHeaderEditing,
    closeNodeContextMenu: contextMenu.close,
    clearActiveNodePanel: options.activePanels.clearActiveNodePanel,
    hideStrengthControl,
  })

  useCanvasCustomLayerOverlay({
    isVisible: hasCanvasDismissableLayer,
    ignore: ['.node-tool-surface', '.canvas-node-context-menu', '[data-inpaint]', '[data-retouch]'],
    ownerDocument: () =>
      options.canvasWrapRef.value?.ownerDocument ??
      (typeof document === 'undefined' ? null : document),
    onDismiss: () => dismissTransientCanvasUi({ shouldCloseContextMenu: true }),
  })

  return dismissTransientCanvasUi
}

function useScreenGrouping(
  options: CanvasScreenSurfaceInteractionOptions,
  selection: ReturnType<typeof useVueFlowSelectionAdapter>,
  dismissTransientCanvasUi: ReturnType<typeof useScreenTransientUi>,
  isCanvasGroupInteractionEnabled: ComputedRef<boolean>,
) {
  const { viewModel, commands } = options.workspace
  const { flowPaneAdapter } = options
  function performSelectionGroupAction() {
    const action = viewModel.graphEditing.grouping.selectionGroupAction.value
    if (action?.kind === 'group') {
      commands.graphEditing.grouping.groupSelectedNodes()
      return
    }
    if (action?.kind === 'ungroup') commands.graphEditing.grouping.ungroupSelectedGroup()
  }

  const groupPresentation = useCanvasGroupPresentationInteraction({
    canvas: {
      host: options.canvasWrapRef,
      nodes: viewModel.flowPane.nodes,
      viewport: flowPaneAdapter.viewport,
      isInteractionEnabled: isCanvasGroupInteractionEnabled,
    },
    grouping: {
      frames: viewModel.graphEditing.grouping.groupFrames,
      action: viewModel.graphEditing.grouping.selectionGroupAction,
      selectAtPoint: commands.graphEditing.grouping.selectGroupAtPoint,
      drag: commands.graphEditing.grouping.groupDrag,
      resize: commands.graphEditing.grouping.groupResize,
      onAction: performSelectionGroupAction,
    },
    pane: {
      toFlowPoint: flowPaneAdapter.screenToFlowCoordinate,
      onPointerDownCapture: selection.paneSelectionBindings.onPointerDownCapture,
      dismiss: dismissTransientCanvasUi,
    },
  })

  return groupPresentation
}

function useScreenNodeDragInteractions(
  options: CanvasScreenSurfaceInteractionOptions,
  dismissTransientCanvasUi: ReturnType<typeof useScreenTransientUi>,
) {
  const { commands, viewModel } = options.workspace
  const tools = viewModel.imageWorkflow.tools
  const dragInteractions = useFlowNodeDragInteractions({
    beginNodeDragTransaction: commands.flowPane.nodeDragTransactions.begin,
    commitNodeDragTransaction: commands.flowPane.nodeDragTransactions.commit,
    isDuplicateModifierPressed: options.isDuplicateModifierPressed,
    beginFlowNodeDragDuplicateTracking: commands.graphEditing.beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking: commands.graphEditing.finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
    isNodeDragLocked: () => tools.locks.isCanvasLocked.value || tools.retouch.isApplying.value,
  })

  function onNodeDragStart(payload: NodeDragEvent) {
    commands.viewportControls.onNodeDragStart()
    try {
      dragInteractions.onNodeDragStart(payload.nodes.map((node) => node.id))
    } catch (error) {
      commands.viewportControls.onNodeDragStop()
      throw error
    }
  }

  function onNodeDragStop() {
    try {
      dragInteractions.onNodeDragStop()
    } finally {
      commands.viewportControls.onNodeDragStop()
    }
  }

  return { onNodeDragStart, onNodeDragStop }
}

function createScreenBackgroundNavigation(
  isPanEnabled: ComputedRef<boolean>,
  groupPresentation: ReturnType<typeof useScreenGrouping>,
  shell: ReturnType<typeof useFlowCanvasShellInteractions>,
) {
  function isBackgroundContextMenu(event: MouseEvent) {
    return (
      event.target instanceof Element &&
      (event.target.classList.contains('vue-flow__pane') ||
        event.target.classList.contains('canvas-group-frame-body'))
    )
  }

  function onPaneContextMenu(event: MouseEvent) {
    if (!isBackgroundContextMenu(event)) return
    if (groupPresentation.onCanvasContextMenuCapture(event)) return
    shell.onPaneContextMenu(event)
  }

  function onCanvasContextMenuCapture(event: MouseEvent) {
    if (isPanEnabled.value && isBackgroundContextMenu(event)) {
      // Vue Flow resolves right click versus pan on release. Keyboard and
      // Ctrl-click menus have no pan gesture and use the same routing.
      event.preventDefault()
      if (event.button !== 2 || event.ctrlKey) onPaneContextMenu(event)
      return
    }
    groupPresentation.onCanvasContextMenuCapture(event)
  }

  function onCanvasMouseDownCapture(event: MouseEvent) {
    // Ctrl-click belongs to the context menu even while Space is held.
    // Vue Flow also admits middle-button graph drags before checking panOnDrag.
    if (
      (event.ctrlKey && isBackgroundContextMenu(event)) ||
      (event.button === 1 && !isPanEnabled.value)
    ) {
      event.preventDefault()
      event.stopPropagation()
    }
  }

  return { onCanvasContextMenuCapture, onCanvasMouseDownCapture, onPaneContextMenu }
}

export function useCanvasScreenSurfaceInteractions(options: CanvasScreenSurfaceInteractionOptions) {
  const { workspace, flowPaneAdapter } = options
  const { viewModel, commands } = workspace
  const contextMenu = useCanvasNodeContextMenu()
  const imageTools = {
    viewModel: viewModel.imageWorkflow.tools,
    commands: commands.imageWorkflow.tools,
  }
  const { nodeId: flowOutpaintNodeId } = viewModel.imageWorkflow.outpaint
  const {
    viewModel: {
      active: { nodeId: flowRetouchNodeId, maskNodeId: flowInpaintNodeId },
      locks: { isCanvasLocked: isFlowCanvasLocked },
    },
    commands: {
      lifecycle: { discardNodeEdits: discardFlowImageToolNodeEdits },
      editor: { scheduleChromeAnchorUpdate: scheduleRetouchChromeAnchorUpdate },
    },
  } = imageTools

  const dismissTransientCanvasUi = useScreenTransientUi(options, contextMenu)

  const nodeInteractions = commands.graphEditing.createNodeInteractions({
    dismissTransientCanvasUi,
    discardNodeEdits: discardFlowImageToolNodeEdits,
  })

  const selection = useVueFlowSelectionAdapter({
    activeTool: options.activeTool,
    dismissTransientCanvasUi,
    onEdgesChange: commands.flowPane.onEdgesChange,
    onSelectionChanged: commands.flowPane.reconcileRendererSelection,
  })

  const isCanvasGroupInteractionEnabled = computed(
    () =>
      options.activeTool.value === 'select' &&
      !isFlowCanvasLocked.value &&
      !flowRetouchNodeId.value &&
      !flowInpaintNodeId.value &&
      !flowOutpaintNodeId.value,
  )

  function onFlowNodeClick(payload: NodeMouseEvent) {
    commands.graphEditing.grouping.handleNodeClick(payload.node.id)
    selection.commands.onFlowNodeClick()
  }

  function onDeleteInlineSelection() {
    if (!isCanvasGroupInteractionEnabled.value) return false
    const didDelete = commands.graphEditing.grouping.deleteSelectedGroup()
    if (didDelete) {
      dismissTransientCanvasUi({ shouldCloseOutpaint: true, shouldCloseContextMenu: true })
    }
    return didDelete
  }

  const groupPresentation = useScreenGrouping(
    options,
    selection,
    dismissTransientCanvasUi,
    isCanvasGroupInteractionEnabled,
  )

  const { onNodeDragStart, onNodeDragStop } = useScreenNodeDragInteractions(
    options,
    dismissTransientCanvasUi,
  )

  function onFlowMove(payload: Parameters<typeof commands.viewportControls.onFlowMove>[0]) {
    commands.viewportControls.onFlowMove(payload)
    if (!payload.flowTransform || !imageTools.viewModel.chrome.value.isReady) return
    scheduleRetouchChromeAnchorUpdate()
  }

  function toggleCanvasMinimapBase() {
    options.isCanvasMinimapEnabled.value = !options.isCanvasMinimapEnabled.value
  }

  const shell = useFlowCanvasShellInteractions({
    canvasWrapRef: options.canvasWrapRef,
    activeTool: options.activeTool,
    screenToFlowCoordinate: flowPaneAdapter.screenToFlowCoordinate,
    openNodeContextMenu: contextMenu.open,
    dismissTransientCanvasUi,
    clearFlowSelection: selection.commands.clearSelection,
    onCanvasDropDragOver: commands.graphEditing.onCanvasDragOver,
    isFlowOutpaintActive: () => !!flowOutpaintNodeId.value,
    isBoardHeaderEditingActive: () =>
      viewModel.boardChrome.isBoardMenuOpen.value || viewModel.boardChrome.isRenamingBoard.value,
  })

  const isPanEnabled = computed(
    () => !isFlowCanvasLocked.value && !viewModel.boardChrome.isBoardArchived.value,
  )

  const navigation = createScreenBackgroundNavigation(isPanEnabled, groupPresentation, shell)

  const closePolicyHandlers = groupPresentation.bindClosePolicyHandlers({
    onCanvasWheel: imageTools.commands.editor.onCanvasWheel,
    onFlowMoveStart: commands.viewportControls.onFlowMoveStart,
    onMiniMapDragStart: commands.viewportControls.onMiniMapDragStart,
    onNodeDragStart,
    onCanvasPaneClick: () => {
      commands.graphEditing.grouping.handlePaneClick()
      selection.commands.onPaneClick()
    },
    toggleCanvasMinimap: toggleCanvasMinimapBase,
    setActiveTool: shell.setActiveTool,
    onCanvasDragOver: shell.onCanvasDragOver,
  })

  function onNodesChange(changes: NodeChange[]) {
    if (changes.some((change) => change.type === 'remove')) {
      dismissTransientCanvasUi({ shouldCloseOutpaint: true, shouldCloseContextMenu: true })
    }
    commands.flowPane.onNodesChange(changes)
  }

  function handleNodeContextMenuCreate(kind: CanvasCreatableNodeKind) {
    contextMenu.createNode(kind, commands.graphEditing.createNodeAtPosition)
    dismissTransientCanvasUi({ shouldCloseOutpaint: true })
  }

  return {
    nodeContextMenu: contextMenu.nodeContextMenu,
    canvasNodeMenuItems: contextMenu.canvasNodeMenuItems,
    nodeInteractions,
    vueFlowSelectionBindings: selection.vueFlowSelectionBindings,
    onFlowNodeMouseDown: selection.commands.onFlowNodeMouseDown,
    onCanvasPointerDownCapture: groupPresentation.onCanvasPointerDownCapture,
    ...navigation,
    groupPresentation,
    onFlowNodeClick,
    onDeleteInlineSelection,
    onNodeDragStop,
    onFlowMove,
    onNodesChange,
    handleNodeContextMenuCreate,
    shell: {
      ...closePolicyHandlers,
      onEscapeShortcut: shell.onEscapeShortcut,
    },
  }
}
