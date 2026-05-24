import type { Ref } from 'vue'
import type { FlowCreatableNodeKind } from './useFlowNodeMutations'

type FlowTool = 'select' | 'hand' | 'text'

type UseFlowCanvasShellInteractionsOptions = {
  canvasWrapRef: Ref<HTMLElement | null>
  nodeContextMenu: Ref<{ visible: boolean; x: number; y: number; flowX: number; flowY: number }>
  activeTool: Ref<FlowTool>
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  openNodeContextMenu: (x: number, y: number, flowX: number, flowY: number) => void
  closeNodeContextMenu: () => void
  createNodeAtPosition: (kind: FlowCreatableNodeKind, flowX: number, flowY: number) => void
  cancelFlowExpandMode: () => void
  closeBoardHeaderEditing: () => void
  clearActiveNodePanels: () => void
  hideStrengthControl: () => void
  clearFlowSelection: () => boolean
  toastInfo: (message: string) => void
  onCanvasDropDragOver: (event: DragEvent) => void
  isFlowExpandActive: () => boolean
  isBoardHeaderEditingActive: () => boolean
}

export function useFlowCanvasShellInteractions(options: UseFlowCanvasShellInteractionsOptions) {
  function onPaneContextMenu(event: MouseEvent) {
    event.preventDefault()
    options.cancelFlowExpandMode()
    options.closeBoardHeaderEditing()
    options.clearActiveNodePanels()
    options.hideStrengthControl()
    options.clearFlowSelection()
    const flowPoint = options.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const rect = options.canvasWrapRef.value?.getBoundingClientRect()
    const x = rect ? event.clientX - rect.left : event.clientX
    const y = rect ? event.clientY - rect.top : event.clientY
    options.openNodeContextMenu(x, y, flowPoint.x, flowPoint.y)
  }

  function handleNodeContextMenuCreate(kind: FlowCreatableNodeKind) {
    options.createNodeAtPosition(
      kind,
      options.nodeContextMenu.value.flowX,
      options.nodeContextMenu.value.flowY,
    )
    options.cancelFlowExpandMode()
    options.closeNodeContextMenu()
    options.clearActiveNodePanels()
    options.hideStrengthControl()
  }

  function setActiveTool(tool: FlowTool) {
    options.cancelFlowExpandMode()
    options.closeBoardHeaderEditing()
    options.clearActiveNodePanels()
    options.hideStrengthControl()
    options.activeTool.value = tool
    if (tool === 'text') {
      options.toastInfo('文本工具稍后接入')
    }
  }

  function onEscapeShortcut() {
    if (options.isFlowExpandActive()) {
      options.cancelFlowExpandMode()
      return true
    }
    if (options.isBoardHeaderEditingActive()) {
      options.closeBoardHeaderEditing()
      return true
    }
    options.clearActiveNodePanels()
    options.hideStrengthControl()
    return options.clearFlowSelection()
  }

  function onCanvasDragOver(event: DragEvent) {
    options.clearActiveNodePanels()
    options.hideStrengthControl()
    options.onCanvasDropDragOver(event)
  }

  return {
    onPaneContextMenu,
    handleNodeContextMenuCreate,
    setActiveTool,
    onEscapeShortcut,
    onCanvasDragOver,
  }
}
