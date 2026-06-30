import { beforeEach, describe, expect, it, vi } from 'vitest'
import { type Ref, ref } from 'vue'
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
  graph: { fullNodes?: TestFlowItem[]; fullEdges?: TestFlowItem[] } = {},
) {
  const dismissTransientCanvasUi = vi.fn()
  const deleteSelected = vi.fn()
  const fullNodes = ref(graph.fullNodes ?? []) as Ref<TestFlowItem[]>
  const fullEdges = ref(graph.fullEdges ?? []) as Ref<TestFlowItem[]>
  const controller = useFlowSelectionController({
    activeTool,
    fullNodes,
    fullEdges,
    dismissTransientCanvasUi,
    _deleteSelectedFlowElements: deleteSelected,
  })

  return {
    activeTool,
    controller,
    fullNodes,
    fullEdges,
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

  it('clears selected full graph nodes that are not mounted', () => {
    const harness = createHarness(ref('select'), {
      fullNodes: [
        { id: 'node-a', selected: true },
        { id: 'node-b', selected: false },
      ],
    })

    expect(harness.controller.clearFlowSelection()).toBe(true)

    expect(harness.fullNodes.value.find((node) => node.id === 'node-a')?.selected).toBe(false)
    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
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

  it('clears hidden full graph selection before selecting a non-shift clicked node', () => {
    const harness = createHarness(ref('select'), {
      fullNodes: [
        { id: 'node-a', selected: true },
        { id: 'node-b', selected: false },
      ],
    })
    const mountedNode = harness.fullNodes.value[1]
    vueFlowMocks.findNode.mockReturnValue(mountedNode)

    harness.controller.onFlowNodeMouseDown(
      'node-b',
      new MouseEvent('mousedown', { button: 0 }),
    )

    expect(harness.fullNodes.value.find((node) => node.id === 'node-a')?.selected).toBe(false)
    expect(vueFlowMocks.addSelectedNodes).toHaveBeenCalledWith([mountedNode])
  })

  it('writes the newly selected node back to the full graph', () => {
    const harness = createHarness(ref('select'), {
      fullNodes: [
        { id: 'node-a', selected: false },
        { id: 'node-b', selected: false },
      ],
    })
    const mountedNode = harness.fullNodes.value[1]
    vueFlowMocks.findNode.mockReturnValue(mountedNode)

    harness.controller.onFlowNodeMouseDown(
      'node-b',
      new MouseEvent('mousedown', { button: 0 }),
    )

    expect(harness.fullNodes.value.find((node) => node.id === 'node-b')?.selected).toBe(true)
  })

  it('repairs full graph selection when Vue Flow already has the node selected', () => {
    const harness = createHarness(ref('select'), {
      fullNodes: [{ id: 'node-a', selected: false }],
    })
    const mountedNode = harness.fullNodes.value[0]
    vueFlowMocks.selectedElements.push(mountedNode)
    vueFlowMocks.findNode.mockReturnValue(mountedNode)

    harness.controller.onFlowNodeMouseDown('node-a', new MouseEvent('mousedown', { button: 0 }))

    expect(harness.fullNodes.value.find((node) => node.id === 'node-a')?.selected).toBe(true)
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
