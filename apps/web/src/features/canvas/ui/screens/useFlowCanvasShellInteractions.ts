import type { Ref } from 'vue'
import type { DismissTransientCanvasUi } from './useFlowTransientUiDismissal'

type FlowTool = 'select' | 'hand'

type UseFlowCanvasShellInteractionsOptions = {
  canvasWrapRef: Ref<HTMLElement | null>
  activeTool: Ref<FlowTool>
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  openNodeContextMenu: (snapshot: { x: number; y: number; flowX: number; flowY: number }) => void
  dismissTransientCanvasUi: DismissTransientCanvasUi
  clearFlowSelection: () => boolean
  onCanvasDropDragOver: (event: DragEvent) => void
  isFlowOutpaintActive: () => boolean
  isBoardHeaderEditingActive: () => boolean
}

export function useFlowCanvasShellInteractions(options: UseFlowCanvasShellInteractionsOptions) {
  function onPaneContextMenu(event: MouseEvent) {
    event.preventDefault()
    options.dismissTransientCanvasUi({ shouldCloseOutpaint: true, shouldCloseBoardHeader: true })
    options.clearFlowSelection()
    const flowPoint = options.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const rect = options.canvasWrapRef.value?.getBoundingClientRect()
    const x = rect ? event.clientX - rect.left : event.clientX
    const y = rect ? event.clientY - rect.top : event.clientY
    options.openNodeContextMenu({ x, y, flowX: flowPoint.x, flowY: flowPoint.y })
  }

  function setActiveTool(tool: FlowTool) {
    options.dismissTransientCanvasUi({ shouldCloseOutpaint: true, shouldCloseBoardHeader: true })
    options.clearFlowSelection()
    options.activeTool.value = tool
  }

  function onEscapeShortcut() {
    if (options.isFlowOutpaintActive()) {
      options.dismissTransientCanvasUi({
        shouldCloseOutpaint: true,
        shouldClearNodePanel: false,
        shouldHideStrength: false,
      })
      return true
    }
    if (options.isBoardHeaderEditingActive()) {
      options.dismissTransientCanvasUi({
        shouldCloseBoardHeader: true,
        shouldClearNodePanel: false,
        shouldHideStrength: false,
      })
      return true
    }
    options.dismissTransientCanvasUi({ shouldCloseContextMenu: true })
    return options.clearFlowSelection()
  }

  function onCanvasDragOver(event: DragEvent) {
    options.dismissTransientCanvasUi()
    options.onCanvasDropDragOver(event)
  }

  return {
    onPaneContextMenu,
    setActiveTool,
    onEscapeShortcut,
    onCanvasDragOver,
  }
}
