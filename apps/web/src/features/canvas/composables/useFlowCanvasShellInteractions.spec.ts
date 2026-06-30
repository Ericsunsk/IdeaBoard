import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import { useFlowCanvasShellInteractions } from './useFlowCanvasShellInteractions'

function createCanvasElement() {
  const element = document.createElement('div')
  element.getBoundingClientRect = () => DOMRect.fromRect({ x: 10, y: 20, width: 400, height: 300 })
  return element
}

function createHarness(options: { expandActive?: boolean; boardHeaderActive?: boolean } = {}) {
  const canvasWrapRef = ref<HTMLElement | null>(createCanvasElement())
  const nodeContextMenu = ref({ visible: false, x: 0, y: 0, flowX: 0, flowY: 0 })
  const activeTool = ref<'select' | 'hand'>('select')
  const dismissTransientCanvasUi = vi.fn()
  const clearFlowSelection = vi.fn(() => false)
  const openNodeContextMenu = vi.fn()
  const createNodeAtPosition = vi.fn()
  const onCanvasDropDragOver = vi.fn()

  const shell = useFlowCanvasShellInteractions({
    canvasWrapRef,
    nodeContextMenu,
    activeTool,
    screenToFlowCoordinate: ({ x, y }) => ({ x: x + 100, y: y + 200 }),
    openNodeContextMenu,
    createNodeAtPosition,
    dismissTransientCanvasUi,
    clearFlowSelection,
    onCanvasDropDragOver,
    isFlowExpandActive: () => options.expandActive ?? false,
    isBoardHeaderEditingActive: () => options.boardHeaderActive ?? false,
  })

  return {
    shell,
    nodeContextMenu,
    activeTool,
    dismissTransientCanvasUi,
    clearFlowSelection,
    openNodeContextMenu,
    createNodeAtPosition,
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
      closeExpand: true,
      closeBoardHeader: true,
    })
    expect(harness.clearFlowSelection).toHaveBeenCalledTimes(1)
    expect(harness.openNodeContextMenu).toHaveBeenCalledWith(32, 44, 142, 264)
  })

  it('creates a node at the stored menu coordinates and closes the menu', () => {
    const harness = createHarness()
    harness.nodeContextMenu.value = { visible: true, x: 10, y: 20, flowX: 120, flowY: 180 }

    harness.shell.handleNodeContextMenuCreate('prompt' as CanvasCreatableNodeKind)

    expect(harness.createNodeAtPosition).toHaveBeenCalledWith('prompt', 120, 180)
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeExpand: true,
      closeContextMenu: true,
    })
  })

  it('keeps Escape priority for expand and board header before general dismissal', () => {
    const expandHarness = createHarness({ expandActive: true })
    expect(expandHarness.shell.onEscapeShortcut()).toBe(true)
    expect(expandHarness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeExpand: true,
      clearNodePanel: false,
      hideStrength: false,
    })
    expect(expandHarness.clearFlowSelection).not.toHaveBeenCalled()

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

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith()
    expect(harness.clearFlowSelection).toHaveBeenCalledTimes(1)
  })

  it('clears selection when switching tools', () => {
    const harness = createHarness()

    harness.shell.setActiveTool('hand')

    expect(harness.activeTool.value).toBe('hand')
    expect(harness.clearFlowSelection).toHaveBeenCalledTimes(1)
  })
})
