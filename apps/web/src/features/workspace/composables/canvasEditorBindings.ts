import type { ComputedRef, Ref } from 'vue'
import { useCanvasKeyboardShortcuts } from './useCanvasKeyboardShortcuts'
import { useCanvasPointerInteractions } from './useCanvasPointerInteractions'
import { useCanvasSurfaceInteractions } from './useCanvasSurfaceInteractions'
import type { UseCanvasEditorOptions } from './canvasEditorTypes'
import type { DraggableNode, NodeDragKind } from './useCanvasSelectionState'
import type { CanvasImageItem } from '../types/canvasNodes'

type PointerBindings = Parameters<typeof useCanvasPointerInteractions>[0]
type KeyboardBindings = Parameters<typeof useCanvasKeyboardShortcuts>[0]
type SurfaceBindings = Parameters<typeof useCanvasSurfaceInteractions>[0]

interface CanvasEditorUiStateBindings {
  isDuplicateModifierPressed: Ref<boolean>
  nodeContextMenu: Ref<{ visible: boolean; x: number; y: number; clientX: number; clientY: number }>
  isPanning: Ref<boolean>
  isSelecting: Ref<boolean>
  isDraggingItem: Ref<boolean>
  draggingNodeKeys: Ref<string[]>
  isResizingItem: Ref<boolean>
  isSpacePressed: Ref<boolean>
  selectionRect: Ref<{ x1: number; y1: number; x2: number; y2: number }>
}

interface CanvasSelectionBindings {
  selectedIds: Ref<number[]>
  selectedNodeKeys: Ref<string[]>
  activeImageActionId: Ref<number | null>
  activeImageNodeId: Ref<number | null>
  activePromptNodeId: Ref<number | null>
  activeModelNodeId: Ref<number | null>
  activeViewpointNodeId: Ref<number | null>
  activeMaskNodeId: Ref<number | null>
  activeColorNodeId: Ref<number | null>
  activeEditNodeId: Ref<number | null>
  activeFusionNodeId: Ref<number | null>
  activeEnhanceNodeId: Ref<number | null>
  activeWearNodeId: Ref<number | null>
  clearActiveNodes: () => void
  clearSelectedNodeKeys: () => void
  removeSelectedNodeKey: (kind: NodeDragKind, id: number) => void
  activateNodeByKind: (kind: NodeDragKind, id: number) => void
  isNodeActive: (kind: NodeDragKind, id: number) => boolean
  resolveNodeBySelectionKey: (key: string) => DraggableNode | null
  getNodeSelectionKey: (kind: NodeDragKind, id: number) => string
  clearCanvasQuickActionState: () => void
  hasActiveNodeSelection: ComputedRef<boolean>
  clearActiveNodeSelection: () => void
  clearSelectedIds: () => void
  deleteActiveNode: () => boolean
  deleteSelectedCanvasImages: () => void
}

export function createCanvasPointerBindings(
  options: UseCanvasEditorOptions,
  uiState: CanvasEditorUiStateBindings,
  selection: CanvasSelectionBindings,
): PointerBindings {
  return {
    messages: options.messages,
    imageNodes: options.imageNodes,
    promptNodes: options.promptNodes,
    modelNodes: options.modelNodes,
    viewpointNodes: options.viewpointNodes,
    maskNodes: options.maskNodes,
    colorNodes: options.colorNodes,
    editNodes: options.editNodes,
    fusionNodes: options.fusionNodes,
    enhanceNodes: options.enhanceNodes,
    wearNodes: options.wearNodes,
    activeTool: options.activeTool,
    showStyleStrengthControl: options.showStyleStrengthControl,
    canvasX: options.canvasX,
    canvasY: options.canvasY,
    zoomLevel: options.zoomLevel,
    canvasLayoutRef: options.canvasLayoutRef,
    selectedIds: selection.selectedIds,
    selectedNodeKeys: selection.selectedNodeKeys,
    activeImageActionId: selection.activeImageActionId,
    activeImageNodeId: selection.activeImageNodeId,
    activePromptNodeId: selection.activePromptNodeId,
    activeModelNodeId: selection.activeModelNodeId,
    activeViewpointNodeId: selection.activeViewpointNodeId,
    activeMaskNodeId: selection.activeMaskNodeId,
    activeColorNodeId: selection.activeColorNodeId,
    activeEditNodeId: selection.activeEditNodeId,
    activeFusionNodeId: selection.activeFusionNodeId,
    activeEnhanceNodeId: selection.activeEnhanceNodeId,
    activeWearNodeId: selection.activeWearNodeId,
    isPanning: uiState.isPanning,
    isSelecting: uiState.isSelecting,
    isDraggingItem: uiState.isDraggingItem,
    draggingNodeKeys: uiState.draggingNodeKeys,
    isResizingItem: uiState.isResizingItem,
    isSpacePressed: uiState.isSpacePressed,
    selectionRect: uiState.selectionRect,
    nodeContextMenu: uiState.nodeContextMenu,
    isBoardMenuOpen: options.isBoardMenuOpen,
    setBoardMenuOpen: options.setBoardMenuOpen,
    isBoardRenaming: options.isBoardRenaming,
    confirmBoardRename: options.confirmBoardRename,
    clearSelectedConnection: () => options.clearSelectedConnection?.(),
    clearSelectedNodeKeys: selection.clearSelectedNodeKeys,
    removeSelectedNodeKey: selection.removeSelectedNodeKey,
    clearActiveNodes: selection.clearActiveNodes,
    activateNodeByKind: selection.activateNodeByKind,
    isNodeActive: selection.isNodeActive,
    resolveNodeBySelectionKey: selection.resolveNodeBySelectionKey,
    getNodeSelectionKey: selection.getNodeSelectionKey,
    beginHistoryBatch: options.beginHistoryBatch,
    endHistoryBatch: options.endHistoryBatch,
    importCanvasImageToImageNodeAtPoint: options.importCanvasImageToImageNodeAtPoint,
    importCanvasImageToMaskNodeAtPoint: options.importCanvasImageToMaskNodeAtPoint,
    message: options.message,
  }
}

export function createCanvasSurfaceBindings(
  options: UseCanvasEditorOptions,
  selection: Pick<CanvasSelectionBindings, 'selectedIds' | 'activeImageActionId'>,
  closeNodeContextMenu: () => void,
): SurfaceBindings {
  return {
    messages: options.messages,
    selectedIds: selection.selectedIds,
    canvasX: options.canvasX,
    canvasY: options.canvasY,
    activeImageActionId: selection.activeImageActionId,
    nodeImageDragMime: options.nodeImageDragMime,
    closeNodeContextMenu,
    updateZoom: options.updateZoom,
    getCanvasPointFromClient: options.getCanvasPointFromClient,
    createCanvasImageAt: options.createCanvasImageAt,
    uploadImage: options.uploadImage,
  }
}

export function createCanvasKeyboardBindings(
  options: UseCanvasEditorOptions,
  uiState: Pick<CanvasEditorUiStateBindings, 'isDuplicateModifierPressed' | 'isSpacePressed' | 'nodeContextMenu'>,
  selection: Pick<
    CanvasSelectionBindings,
    | 'activeImageActionId'
    | 'clearCanvasQuickActionState'
    | 'hasActiveNodeSelection'
    | 'clearActiveNodeSelection'
    | 'selectedIds'
    | 'clearSelectedIds'
    | 'deleteActiveNode'
    | 'deleteSelectedCanvasImages'
  >,
  closeNodeContextMenu: () => void,
): KeyboardBindings {
  return {
    isLeaveConfirmVisible: options.isLeaveConfirmVisible,
    resolveLeaveConfirm: options.resolveLeaveConfirm,
    isNodeContextMenuVisible: () => uiState.nodeContextMenu.value.visible,
    closeNodeContextMenu,
    hasSelectedConnection: () => options.hasSelectedConnection?.() ?? false,
    clearSelectedConnection: () => options.clearSelectedConnection?.(),
    activeImageActionId: selection.activeImageActionId,
    showStyleStrengthControl: options.showStyleStrengthControl,
    clearCanvasQuickActionState: selection.clearCanvasQuickActionState,
    hasActiveNodeSelection: () => selection.hasActiveNodeSelection.value,
    clearActiveNodeSelection: selection.clearActiveNodeSelection,
    selectedIds: selection.selectedIds,
    clearSelectedIds: selection.clearSelectedIds,
    deleteSelectedConnection: () => options.deleteSelectedConnection?.() ?? false,
    deleteActiveNode: selection.deleteActiveNode,
    deleteSelectedCanvasImages: selection.deleteSelectedCanvasImages,
    isSpacePressed: uiState.isSpacePressed,
    isDuplicateModifierPressed: uiState.isDuplicateModifierPressed,
    undo: options.undo,
    redo: options.redo,
  }
}
