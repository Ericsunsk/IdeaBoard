import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useFlowSelectionController } from './useFlowSelectionController'

type TestFlowItem = {
  id: string
  selected?: boolean
}

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

function createHarness(
  activeTool = ref<string>('select'),
) {
  const dismissTransientCanvasUi = vi.fn()
  const controller = useFlowSelectionController({
    activeTool,
    dismissTransientCanvasUi,
  })

  return {
    activeTool,
    controller,
    dismissTransientCanvasUi,
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

  it('leaves selection cleanup to Vue Flow when nothing is selected', () => {
    const harness = createHarness()

    expect(harness.controller.clearFlowSelection()).toBe(false)

    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
  })

  it('lets Vue Flow own regular node selection while dismissing node panels', () => {
    const harness = createHarness()
    const node = { id: 'node-a' }
    vueFlowMocks.findNode.mockReturnValue(node)

    harness.controller.onFlowNodeMouseDown('node-a', new MouseEvent('mousedown', { button: 0 }))

    expect(vueFlowMocks.findNode).not.toHaveBeenCalled()
    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ closeExpand: true })
  })

  it('clears existing Vue Flow selection before selecting a context-menu target', () => {
    const harness = createHarness()
    const previousNode: TestFlowItem = { id: 'node-a' }
    const mountedNode: TestFlowItem = { id: 'node-b' }
    vueFlowMocks.selectedElements.push(previousNode)
    vueFlowMocks.findNode.mockReturnValue(mountedNode)

    harness.controller.selectFlowNodeForContextMenu('node-b')

    expect(vueFlowMocks.removeSelectedElements).toHaveBeenCalledWith([previousNode])
    expect(vueFlowMocks.addSelectedNodes).toHaveBeenCalledWith([mountedNode])
  })

  it('selects a context-menu target through Vue Flow', () => {
    const harness = createHarness()
    const mountedNode: TestFlowItem = { id: 'node-b' }
    vueFlowMocks.findNode.mockReturnValue(mountedNode)

    harness.controller.selectFlowNodeForContextMenu('node-b')

    expect(vueFlowMocks.addSelectedNodes).toHaveBeenCalledWith([mountedNode])
  })

  it('keeps native selection stable when the context-menu target is already selected', () => {
    const harness = createHarness()
    const mountedNode: TestFlowItem = { id: 'node-a' }
    vueFlowMocks.selectedElements.push(mountedNode)
    vueFlowMocks.findNode.mockReturnValue(mountedNode)

    harness.controller.selectFlowNodeForContextMenu('node-a')

    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
  })

  it('does not select a node while the hand tool is active but still dismisses transient UI', () => {
    const activeTool = ref('hand')
    const harness = createHarness(activeTool)

    harness.controller.onFlowNodeMouseDown('node-a', new MouseEvent('mousedown', { button: 0 }))

    expect(vueFlowMocks.findNode).not.toHaveBeenCalled()
    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ closeExpand: true })
  })
})
