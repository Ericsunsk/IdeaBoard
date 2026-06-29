import type { Ref } from 'vue'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import type { DismissTransientCanvasUi } from './useFlowTransientUiDismissal'

type FlowTool = 'select' | 'hand' | 'text'

type UseFlowCanvasShellInteractionsOptions = {
  canvasWrapRef: Ref<HTMLElement | null>
  nodeContextMenu: Ref<{ visible: boolean; x: number; y: number; flowX: number; flowY: number }>
  activeTool: Ref<FlowTool>
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  openNodeContextMenu: (x: number, y: number, flowX: number, flowY: number) => void
  createNodeAtPosition: (kind: CanvasCreatableNodeKind, flowX: number, flowY: number) => void
  dismissTransientCanvasUi: DismissTransientCanvasUi
  clearFlowSelection: () => boolean
  toastInfo: (message: string) => void
  onCanvasDropDragOver: (event: DragEvent) => void
  isFlowExpandActive: () => boolean
  isBoardHeaderEditingActive: () => boolean
}

export function useFlowCanvasShellInteractions(options: UseFlowCanvasShellInteractionsOptions) {
  function onPaneContextMenu(event: MouseEvent) {
    event.preventDefault()
    options.dismissTransientCanvasUi({ closeExpand: true, closeBoardHeader: true })
    options.clearFlowSelection()
    const flowPoint = options.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const rect = options.canvasWrapRef.value?.getBoundingClientRect()
    const x = rect ? event.clientX - rect.left : event.clientX
    const y = rect ? event.clientY - rect.top : event.clientY
    options.openNodeContextMenu(x, y, flowPoint.x, flowPoint.y)
  }

  function handleNodeContextMenuCreate(kind: CanvasCreatableNodeKind) {
    options.createNodeAtPosition(
      kind,
      options.nodeContextMenu.value.flowX,
      options.nodeContextMenu.value.flowY,
    )
    options.dismissTransientCanvasUi({ closeExpand: true, closeContextMenu: true })
  }

  function setActiveTool(tool: FlowTool) {
    options.dismissTransientCanvasUi({ closeExpand: true, closeBoardHeader: true })
    if (tool === 'text') {
      options.toastInfo('文本工具稍后接入')
      return
    }
    options.activeTool.value = tool
  }

  function onEscapeShortcut() {
    if (options.isFlowExpandActive()) {
      options.dismissTransientCanvasUi({
        closeExpand: true,
        clearNodePanel: false,
        hideStrength: false,
      })
      return true
    }
    if (options.isBoardHeaderEditingActive()) {
      options.dismissTransientCanvasUi({
        closeBoardHeader: true,
        clearNodePanel: false,
        hideStrength: false,
      })
      return true
    }
    options.dismissTransientCanvasUi()
    return options.clearFlowSelection()
  }

  function onCanvasDragOver(event: DragEvent) {
    options.dismissTransientCanvasUi()
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
