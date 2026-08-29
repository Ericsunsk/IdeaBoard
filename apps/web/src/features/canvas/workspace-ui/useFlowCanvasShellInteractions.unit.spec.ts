import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useFlowCanvasShellInteractions } from './useFlowCanvasShellInteractions'

function createCanvasElement() {
  const element = document.createElement('div')
  element.getBoundingClientRect = () => DOMRect.fromRect({ x: 10, y: 20, width: 400, height: 300 })
  return element
}

function createHarness(
  options: {
    outpaintActive?: boolean
    boardHeaderActive?: boolean
    screenToFlowCoordinate?: (point: { x: number; y: number }) => { x: number; y: number }
  } = {},
) {
  const canvasWrapRef = ref<HTMLElement | null>(createCanvasElement())
  const activeTool = ref<'select' | 'hand'>('select')
  const dismissTransientCanvasUi = vi.fn()
  const clearFlowSelection = vi.fn(() => false)
  const openNodeContextMenu = vi.fn()
  const onCanvasDropDragOver = vi.fn()

  const shell = useFlowCanvasShellInteractions({
    canvasWrapRef,
    activeTool,
    screenToFlowCoordinate:
      options.screenToFlowCoordinate ?? (({ x, y }) => ({ x: x + 100, y: y + 200 })),
    openNodeContextMenu,
    dismissTransientCanvasUi,
    clearFlowSelection,
    onCanvasDropDragOver,
    isFlowOutpaintActive: () => options.outpaintActive ?? false,
    isBoardHeaderEditingActive: () => options.boardHeaderActive ?? false,
  })

  return {
    shell,
    activeTool,
    dismissTransientCanvasUi,
    clearFlowSelection,
    openNodeContextMenu,
    onCanvasDropDragOver,
  }
}

describe('useFlowCanvasShellInteractions', () => {
  it('dismisses transient UI before opening the pane context menu', () => {
    const harness = createHarness()
    const event = new MouseEvent('contextmenu', { clientX: 42, clientY: 64, cancelable: true })

    harness.shell.onPaneContextMenu(event)

    expect(event.defaultPrevented).toBe(true)
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeOutpaint: true,
      closeBoardHeader: true,
    })
    expect(harness.clearFlowSelection).toHaveBeenCalledTimes(1)
    expect(harness.openNodeContextMenu).toHaveBeenCalledWith({
      x: 32,
      y: 44,
      flowX: 142,
      flowY: 264,
    })
  })

  it('keeps the surface position independent from the current flow viewport', () => {
    const harness = createHarness({
      screenToFlowCoordinate: () => ({ x: -85, y: 439 }),
    })
    const event = new MouseEvent('contextmenu', { clientX: 42, clientY: 64, cancelable: true })

    harness.shell.onPaneContextMenu(event)

    expect(harness.openNodeContextMenu).toHaveBeenCalledWith({
      x: 32,
      y: 44,
      flowX: -85,
      flowY: 439,
    })
  })

  it('keeps Escape priority for outpaint and board header before general dismissal', () => {
    const outpaintHarness = createHarness({ outpaintActive: true })
    expect(outpaintHarness.shell.onEscapeShortcut()).toBe(true)
    expect(outpaintHarness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeOutpaint: true,
      clearNodePanel: false,
      hideStrength: false,
    })
    expect(outpaintHarness.clearFlowSelection).not.toHaveBeenCalled()

    const headerHarness = createHarness({ boardHeaderActive: true })
    expect(headerHarness.shell.onEscapeShortcut()).toBe(true)
    expect(headerHarness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeBoardHeader: true,
      clearNodePanel: false,
      hideStrength: false,
    })
    expect(headerHarness.clearFlowSelection).not.toHaveBeenCalled()
  })

  it('dismisses transient UI and returns the selection clear result on plain Escape', () => {
    const harness = createHarness()
    harness.clearFlowSelection.mockReturnValue(true)

    expect(harness.shell.onEscapeShortcut()).toBe(true)

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ closeContextMenu: true })
    expect(harness.clearFlowSelection).toHaveBeenCalledTimes(1)
  })

  it('clears selection when switching tools', () => {
    const harness = createHarness()

    harness.shell.setActiveTool('hand')

    expect(harness.activeTool.value).toBe('hand')
    expect(harness.clearFlowSelection).toHaveBeenCalledTimes(1)
  })
})
