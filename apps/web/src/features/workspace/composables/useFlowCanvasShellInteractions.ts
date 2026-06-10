import type { Ref } from 'vue'
import type { FlowCreatableNodeKind } from './useFlowNodeMutations'
import type { DismissTransientWorkspaceUi } from './useFlowTransientUiDismissal'

type FlowTool = 'select' | 'hand' | 'text'

type UseFlowCanvasShellInteractionsOptions = {
  canvasWrapRef: Ref<HTMLElement | null>
  nodeContextMenu: Ref<{ visible: boolean; x: number; y: number; flowX: number; flowY: number }>
  activeTool: Ref<FlowTool>
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  openNodeContextMenu: (x: number, y: number, flowX: number, flowY: number) => void
  createNodeAtPosition: (kind: FlowCreatableNodeKind, flowX: number, flowY: number) => void
  dismissTransientWorkspaceUi: DismissTransientWorkspaceUi
  clearFlowSelection: () => boolean
  toastInfo: (message: string) => void
  onCanvasDropDragOver: (event: DragEvent) => void
  isFlowExpandActive: () => boolean
  isBoardHeaderEditingActive: () => boolean
}

export function useFlowCanvasShellInteractions(options: UseFlowCanvasShellInteractionsOptions) {
  function onPaneContextMenu(event: MouseEvent) {
    event.preventDefault()
    options.dismissTransientWorkspaceUi({ closeExpand: true, closeBoardHeader: true })
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
    options.dismissTransientWorkspaceUi({ closeExpand: true, closeContextMenu: true })
  }

  function setActiveTool(tool: FlowTool) {
    options.dismissTransientWorkspaceUi({ closeExpand: true, closeBoardHeader: true })
    if (tool === 'text') {
      options.toastInfo('文本工具稍后接入')
      return
    }
    options.activeTool.value = tool
  }

  function onEscapeShortcut() {
    if (options.isFlowExpandActive()) {
      options.dismissTransientWorkspaceUi({
        closeExpand: true,
        clearNodePanels: false,
        hideStrength: false,
      })
      return true
    }
    if (options.isBoardHeaderEditingActive()) {
      options.dismissTransientWorkspaceUi({
        closeBoardHeader: true,
        clearNodePanels: false,
        hideStrength: false,
      })
      return true
    }
    options.dismissTransientWorkspaceUi()
    return options.clearFlowSelection()
  }

  function onCanvasDragOver(event: DragEvent) {
    options.dismissTransientWorkspaceUi()
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
