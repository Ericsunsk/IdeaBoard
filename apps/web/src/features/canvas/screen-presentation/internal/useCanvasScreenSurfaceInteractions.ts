import { computed, type Ref } from 'vue'
import type { NodeChange, NodeMouseEvent } from '@vue-flow/core'
import type { CanvasCreatableNodeKind } from '../../../canvas-board/domain'
import { useFlowNodeDragInteractions } from '../useFlowNodeDragInteractions'
import { useFlowCanvasShellInteractions } from '../useFlowCanvasShellInteractions'
import { useFlowTransientUiDismissal } from '../useFlowTransientUiDismissal'
import { useCanvasCustomLayerOverlay } from '../../overlay-lifecycle'
import { useVueFlowSelectionAdapter, type CanvasVueFlowAdapter } from '../../vue-flow-adapter'
import { useCanvasGroupPresentationInteraction } from '../../group-presentation'
import type { CanvasWorkspaceRuntime } from '../../workspace-runtime'
import { useCanvasNodeContextMenu } from '../useCanvasNodeContextMenu'
import type { useFlowActiveNodePanels } from '../useFlowActiveNodePanels'

type ActivePanels = ReturnType<typeof useFlowActiveNodePanels>

export function useCanvasScreenSurfaceInteractions(options: {
  workspace: CanvasWorkspaceRuntime
  flowPaneAdapter: CanvasVueFlowAdapter
  canvasWrapRef: Ref<HTMLElement | null>
  activeTool: Ref<'select' | 'hand'>
  isCanvasMinimapEnabled: Ref<boolean>
  duplicateModifierPressed: Ref<boolean>
  activePanels: ActivePanels
}) {
  const { workspace, flowPaneAdapter } = options
  const { viewModel, commands } = workspace
  const contextMenu = useCanvasNodeContextMenu()
  const imageTools = {
    viewModel: viewModel.imageWorkflow.tools,
    commands: commands.imageWorkflow.tools,
  }
  const { hideStrengthControl, cancelCanvasExtend } = commands.imageWorkflow
  const { nodeId: flowExpandNodeId } = viewModel.imageWorkflow.canvasExtend
  const {
    viewModel: {
      active: { nodeId: flowImageEditorNodeId, maskNodeId: flowMaskEditorNodeId },
      locks: { isCanvasLocked: isFlowCanvasLocked },
    },
    commands: {
      lifecycle: { discardNodeEdits: discardFlowImageToolNodeEdits },
      editor: { scheduleChromeAnchorUpdate: scheduleImageEditorChromeAnchorUpdate },
    },
  } = imageTools

  const hasCanvasDismissableLayer = computed(
    () =>
      contextMenu.nodeContextMenu.value.visible ||
      options.activePanels.activeNodePanel.value !== null,
  )
  const { dismissTransientCanvasUi } = useFlowTransientUiDismissal({
    cancelFlowExpandMode: cancelCanvasExtend,
    closeBoardHeaderEditing: commands.boardSession.closeBoardHeaderEditing,
    closeNodeContextMenu: contextMenu.close,
    clearActiveNodePanel: options.activePanels.clearActiveNodePanel,
    hideStrengthControl,
  })

  useCanvasCustomLayerOverlay({
    visible: hasCanvasDismissableLayer,
    ignore: [
      '.node-tool-surface',
      '.canvas-node-context-menu',
      '[data-image-mask-editor]',
      '[data-image-editor]',
    ],
    ownerDocument: () =>
      options.canvasWrapRef.value?.ownerDocument ??
      (typeof document === 'undefined' ? null : document),
    onDismiss: () => dismissTransientCanvasUi({ closeContextMenu: true }),
  })

  const nodeInteractions = commands.graphEditing.createNodeInteractions({
    dismissTransientCanvasUi,
    discardNodeEdits: discardFlowImageToolNodeEdits,
  })

  const selection = useVueFlowSelectionAdapter({
    activeTool: options.activeTool,
    dismissTransientCanvasUi,
    onEdgesChange: commands.flowPane.onEdgesChange,
  })

  const isCanvasGroupInteractionEnabled = computed(
    () =>
      options.activeTool.value === 'select' &&
      !isFlowCanvasLocked.value &&
      !flowImageEditorNodeId.value &&
      !flowMaskEditorNodeId.value &&
      !flowExpandNodeId.value,
  )

  function onFlowNodeClick(payload: NodeMouseEvent) {
    commands.graphEditing.grouping.handleNodeClick({
      node: payload.node,
      selectNode: selection.commands.onFlowNodeClick,
    })
  }

  function onDeleteInlineSelection() {
    if (!isCanvasGroupInteractionEnabled.value) return false
    const didDelete = commands.graphEditing.grouping.deleteSelectedGroup()
    if (didDelete) {
      dismissTransientCanvasUi({ closeExpand: true, closeContextMenu: true })
    }
    return didDelete
  }

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

  const dragInteractions = useFlowNodeDragInteractions({
    beginNodeDragTransaction: commands.flowPane.nodeDragTransactions.begin,
    commitNodeDragTransaction: commands.flowPane.nodeDragTransactions.commit,
    duplicateModifierPressed: options.duplicateModifierPressed,
    beginFlowNodeDragDuplicateTracking: commands.graphEditing.beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking: commands.graphEditing.finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
    isNodeDragLocked: () => isFlowCanvasLocked.value,
  })

  function onNodeDragStart() {
    commands.viewportControls.onNodeDragStart()
    try {
      dragInteractions.onNodeDragStart()
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

  function onFlowMove(payload: Parameters<typeof commands.viewportControls.onFlowMove>[0]) {
    commands.viewportControls.onFlowMove(payload)
    if (!payload.flowTransform || !imageTools.viewModel.chrome.value.isReady) return
    scheduleImageEditorChromeAnchorUpdate()
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
    isFlowExpandActive: () => !!flowExpandNodeId.value,
    isBoardHeaderEditingActive: () =>
      viewModel.boardChrome.isBoardMenuOpen.value || viewModel.boardChrome.isRenamingBoard.value,
  })

  const closePolicyHandlers = groupPresentation.bindClosePolicyHandlers({
    onCanvasWheel: imageTools.commands.editor.onCanvasWheel,
    onFlowMoveStart: commands.viewportControls.onFlowMoveStart,
    onMiniMapDragStart: commands.viewportControls.onMiniMapDragStart,
    onNodeDragStart,
    onCanvasPaneClick: () =>
      commands.graphEditing.grouping.handlePaneClick(selection.commands.onPaneClick),
    toggleCanvasMinimap: toggleCanvasMinimapBase,
    onPaneContextMenu: shell.onPaneContextMenu,
    setActiveTool: shell.setActiveTool,
    onCanvasDragOver: shell.onCanvasDragOver,
  })

  function onNodesChange(changes: NodeChange[]) {
    if (changes.some((change) => change.type === 'remove')) {
      dismissTransientCanvasUi({ closeExpand: true, closeContextMenu: true })
    }
    commands.flowPane.onNodesChange(changes)
  }

  function handleNodeContextMenuCreate(kind: CanvasCreatableNodeKind) {
    contextMenu.createNode(kind, commands.graphEditing.createNodeAtPosition)
    dismissTransientCanvasUi({ closeExpand: true })
  }

  return {
    nodeContextMenu: contextMenu.nodeContextMenu,
    canvasNodeMenuItems: contextMenu.canvasNodeMenuItems,
    nodeInteractions,
    vueFlowSelectionBindings: selection.vueFlowSelectionBindings,
    onFlowNodeMouseDown: selection.commands.onFlowNodeMouseDown,
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
