import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useFlowSelectionController } from './useFlowSelectionController'

const vueFlowMocks = vi.hoisted(() => ({
  selectedElements: [] as Array<{ id: string }>,
  removeSelectedElements: vi.fn(),
  addSelectedNodes: vi.fn(),
  findNode: vi.fn(),
}))

vi.mock('@vue-flow/core', () => ({
  useVueFlow: () => ({
    getSelectedElements: { value: vueFlowMocks.selectedElements },
    removeSelectedElements: vueFlowMocks.removeSelectedElements,
    addSelectedNodes: vueFlowMocks.addSelectedNodes,
    findNode: vueFlowMocks.findNode,
  }),
}))

function createHarness(activeTool = ref<string>('select')) {
  const dismissTransientCanvasUi = vi.fn()
  const deleteSelected = vi.fn()
  const controller = useFlowSelectionController({
    activeTool,
    dismissTransientCanvasUi,
    _deleteSelectedFlowElements: deleteSelected,
  })

  return {
    activeTool,
    controller,
    dismissTransientCanvasUi,
    deleteSelected,
  }
}

describe('useFlowSelectionController', () => {
  beforeEach(() => {
    vueFlowMocks.selectedElements = []
    vueFlowMocks.removeSelectedElements.mockReset()
    vueFlowMocks.addSelectedNodes.mockReset()
    vueFlowMocks.findNode.mockReset()
  })

  it('dismisses transient UI on pane click with board and context surfaces', () => {
    const harness = createHarness()

    harness.controller.onPaneClick()

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeExpand: true,
      closeBoardHeader: true,
      closeContextMenu: true,
    })
  })

  it('selects a clicked node and dismisses node panels without clearing selection directly', () => {
    const harness = createHarness()
    const node = { id: 'node-a' }
    vueFlowMocks.findNode.mockReturnValue(node)

    harness.controller.onFlowNodeMouseDown('node-a', new MouseEvent('mousedown', { button: 0 }))

    expect(vueFlowMocks.addSelectedNodes).toHaveBeenCalledWith([node])
    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ closeExpand: true })
  })

  it('does not select a node while the hand tool is active but still dismisses transient UI', () => {
    const activeTool = ref('hand')
    const harness = createHarness(activeTool)

    harness.controller.onFlowNodeMouseDown('node-a', new MouseEvent('mousedown', { button: 0 }))

    expect(vueFlowMocks.findNode).not.toHaveBeenCalled()
    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ closeExpand: true })
  })

  it('dismisses transient UI before deleting selected elements', () => {
    const harness = createHarness()

    harness.controller.deleteSelectedFlowElements()

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeExpand: true,
      closeContextMenu: true,
    })
    expect(harness.deleteSelected).toHaveBeenCalledTimes(1)
  })
})
