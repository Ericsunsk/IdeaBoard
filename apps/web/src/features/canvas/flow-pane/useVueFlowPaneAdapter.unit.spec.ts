import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useVueFlowPaneAdapter } from './useVueFlowPaneAdapter'

const vueFlow = vi.hoisted(() => ({
  selectedElements: { value: [] as Array<{ id: string }> },
  findNode: vi.fn(),
  removeSelectedElements: vi.fn(),
  addSelectedNodes: vi.fn(),
}))

vi.mock('@vue-flow/core', () => ({
  useVueFlow: () => ({
    screenToFlowCoordinate: vi.fn(),
    updateNodeInternals: vi.fn(),
    zoomIn: vi.fn(),
    zoomOut: vi.fn(),
    viewport: ref({ x: 0, y: 0, zoom: 1 }),
    panBy: vi.fn(),
    getViewport: vi.fn(),
    setViewport: vi.fn(),
    setCenter: vi.fn(),
    getSelectedElements: vueFlow.selectedElements,
    removeSelectedElements: vueFlow.removeSelectedElements,
    addSelectedNodes: vueFlow.addSelectedNodes,
    findNode: vueFlow.findNode,
    setNodes: vi.fn(),
    setEdges: vi.fn(),
    addNodes: vi.fn(),
    addEdges: vi.fn(),
    removeNodes: vi.fn(),
    removeEdges: vi.fn(),
    updateNode: vi.fn(),
    applyNodeChanges: vi.fn(),
    applyEdgeChanges: vi.fn(),
  }),
}))

describe('useVueFlowPaneAdapter', () => {
  beforeEach(() => {
    vueFlow.selectedElements.value = []
    vueFlow.findNode.mockReset()
    vueFlow.removeSelectedElements.mockReset()
    vueFlow.addSelectedNodes.mockReset()
  })

  it('selects exactly one node through the Flow Pane seam', () => {
    const previousNode = { id: 'node-a' }
    const targetNode = { id: 'node-b' }
    vueFlow.selectedElements.value = [previousNode]
    vueFlow.findNode.mockReturnValue(targetNode)

    const adapter = useVueFlowPaneAdapter()

    expect(adapter.selectOnlyNode('node-b')).toBe(true)
    expect(vueFlow.removeSelectedElements).toHaveBeenCalledWith([previousNode])
    expect(vueFlow.addSelectedNodes).toHaveBeenCalledWith([targetNode])
  })

  it('does not disturb selection when the requested node is absent', () => {
    const previousNode = { id: 'node-a' }
    vueFlow.selectedElements.value = [previousNode]
    vueFlow.findNode.mockReturnValue(undefined)

    const adapter = useVueFlowPaneAdapter()

    expect(adapter.selectOnlyNode('missing-node')).toBe(false)
    expect(vueFlow.removeSelectedElements).not.toHaveBeenCalled()
    expect(vueFlow.addSelectedNodes).not.toHaveBeenCalled()
  })
})
