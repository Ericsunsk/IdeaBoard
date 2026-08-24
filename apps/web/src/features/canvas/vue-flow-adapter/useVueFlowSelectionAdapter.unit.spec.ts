import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { EdgeChange } from '@vue-flow/core'
import { useVueFlowSelectionAdapter } from './useVueFlowSelectionAdapter'

type TestFlowItem = {
  id: string
  selected?: boolean
  source?: string
  target?: string
}

const vueFlowMocks = vi.hoisted(() => ({
  selectedElements: [] as Array<{ id: string }>,
  nodesSelectionActive: { value: false },
  removeSelectedElements: vi.fn(),
  addSelectedNodes: vi.fn(),
  findNode: vi.fn(),
}))

vi.mock('@vue-flow/core', () => ({
  useVueFlow: () => ({
    getSelectedElements: { value: vueFlowMocks.selectedElements },
    nodesSelectionActive: vueFlowMocks.nodesSelectionActive,
    removeSelectedElements: vueFlowMocks.removeSelectedElements,
    addSelectedNodes: vueFlowMocks.addSelectedNodes,
    findNode: vueFlowMocks.findNode,
  }),
}))

function createHarness(activeTool = ref<string>('select')) {
  const dismissTransientCanvasUi = vi.fn()
  const onEdgesChange = vi.fn()
  const adapter = useVueFlowSelectionAdapter({
    activeTool,
    dismissTransientCanvasUi,
    onEdgesChange,
  })

  return {
    activeTool,
    adapter,
    dismissTransientCanvasUi,
    onEdgesChange,
  }
}

function setSelectedElements(...elements: TestFlowItem[]) {
  vueFlowMocks.selectedElements.splice(0, vueFlowMocks.selectedElements.length, ...elements)
}

function createPanePointerEvent(options: MouseEventInit = {}) {
  const pane = document.createElement('div')
  pane.className = 'vue-flow__pane'
  const event = new MouseEvent('pointerdown', {
    button: 0,
    clientX: 10,
    clientY: 10,
    ...options,
  }) as PointerEvent
  pane.dispatchEvent(event)
  return event
}

describe('useVueFlowSelectionAdapter', () => {
  beforeEach(() => {
    vueFlowMocks.selectedElements = []
    vueFlowMocks.nodesSelectionActive.value = false
    vueFlowMocks.removeSelectedElements.mockReset()
    vueFlowMocks.addSelectedNodes.mockReset()
    vueFlowMocks.findNode.mockReset()
  })

  it('dismisses transient UI on pane click with board and context surfaces', () => {
    const harness = createHarness()

    harness.adapter.commands.onPaneClick()

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeExpand: true,
      closeBoardHeader: true,
      closeContextMenu: true,
    })
  })

  it('leaves selection cleanup to Vue Flow when nothing is selected', () => {
    const harness = createHarness()

    expect(harness.adapter.commands.clearSelection()).toBe(false)

    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
  })

  it('lets Vue Flow own regular node selection while dismissing node panels', () => {
    const harness = createHarness()
    const node = { id: 'node-a' }
    vueFlowMocks.findNode.mockReturnValue(node)

    harness.adapter.commands.onFlowNodeMouseDown(
      'node-a',
      new MouseEvent('mousedown', { button: 0 }),
    )

    expect(vueFlowMocks.findNode).not.toHaveBeenCalled()
    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ closeExpand: true })
  })

  it('does not select a node while the hand tool is active but still dismisses transient UI', () => {
    const activeTool = ref('hand')
    const harness = createHarness(activeTool)

    harness.adapter.commands.onFlowNodeMouseDown(
      'node-a',
      new MouseEvent('mousedown', { button: 0 }),
    )

    expect(vueFlowMocks.findNode).not.toHaveBeenCalled()
    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ closeExpand: true })
  })

  it('removes edge selection after marquee selection', () => {
    const harness = createHarness()
    const selectedNode: TestFlowItem = { id: 'node-a' }
    const selectedEdge: TestFlowItem = { id: 'edge-a', source: 'node-a', target: 'node-b' }
    setSelectedElements(selectedNode, selectedEdge)

    harness.adapter.vueFlowSelectionBindings.onSelectionEnd(
      new MouseEvent('mouseup', { clientX: 30, clientY: 30 }),
    )

    expect(vueFlowMocks.removeSelectedElements).toHaveBeenCalledWith([selectedEdge])
    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
  })

  it('keeps a marquee-selected node selected without showing a group frame', () => {
    const harness = createHarness()
    const selectedNode: TestFlowItem = { id: 'node-a' }
    setSelectedElements(selectedNode)
    vueFlowMocks.nodesSelectionActive.value = true

    harness.adapter.vueFlowSelectionBindings.onSelectionEnd(
      new MouseEvent('mouseup', { clientX: 30, clientY: 30 }),
    )

    expect(vueFlowMocks.nodesSelectionActive.value).toBe(false)
    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
  })

  it('shows the group frame when marquee selection contains multiple nodes', () => {
    const harness = createHarness()
    setSelectedElements({ id: 'node-a' }, { id: 'node-b' })

    harness.adapter.vueFlowSelectionBindings.onSelectionEnd(
      new MouseEvent('mouseup', { clientX: 30, clientY: 30 }),
    )

    expect(vueFlowMocks.nodesSelectionActive.value).toBe(true)
  })

  it('restores previous nodes outside a Shift marquee', () => {
    const harness = createHarness()
    const previousNode: TestFlowItem = { id: 'node-a' }
    const marqueeNode: TestFlowItem = { id: 'node-b' }
    setSelectedElements(previousNode)
    vueFlowMocks.findNode.mockImplementation((nodeId: string) =>
      nodeId === previousNode.id ? previousNode : undefined,
    )

    harness.adapter.paneSelectionBindings.onPointerDownCapture(
      createPanePointerEvent({ shiftKey: true }),
    )
    setSelectedElements(marqueeNode)
    harness.adapter.vueFlowSelectionBindings.onSelectionEnd(
      new MouseEvent('mouseup', { clientX: 60, clientY: 60 }),
    )

    expect(vueFlowMocks.addSelectedNodes).toHaveBeenCalledWith([previousNode])
    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
  })

  it('toggles previously selected nodes inside a Shift marquee', () => {
    const harness = createHarness()
    const previousInsideNode: TestFlowItem = { id: 'node-a' }
    const previousOutsideNode: TestFlowItem = { id: 'node-c' }
    const newMarqueeNode: TestFlowItem = { id: 'node-b' }
    const selectedEdge: TestFlowItem = { id: 'edge-a', source: 'node-a', target: 'node-b' }
    setSelectedElements(previousInsideNode, previousOutsideNode)
    vueFlowMocks.findNode.mockImplementation((nodeId: string) =>
      nodeId === previousOutsideNode.id ? previousOutsideNode : undefined,
    )

    harness.adapter.paneSelectionBindings.onPointerDownCapture(
      createPanePointerEvent({ shiftKey: true }),
    )
    setSelectedElements(previousInsideNode, newMarqueeNode, selectedEdge)
    harness.adapter.vueFlowSelectionBindings.onSelectionEnd(
      new MouseEvent('mouseup', { clientX: 60, clientY: 60 }),
    )

    expect(vueFlowMocks.removeSelectedElements).toHaveBeenNthCalledWith(1, [selectedEdge])
    expect(vueFlowMocks.removeSelectedElements).toHaveBeenNthCalledWith(2, [previousInsideNode])
    expect(vueFlowMocks.addSelectedNodes).toHaveBeenCalledWith([previousOutsideNode])
  })

  it('keeps Shift blank click as a regular pane clear', () => {
    const harness = createHarness()
    const previousNode: TestFlowItem = { id: 'node-a' }
    setSelectedElements(previousNode)

    harness.adapter.paneSelectionBindings.onPointerDownCapture(
      createPanePointerEvent({ shiftKey: true }),
    )
    setSelectedElements()
    harness.adapter.vueFlowSelectionBindings.onSelectionEnd(
      new MouseEvent('mouseup', { clientX: 10, clientY: 10 }),
    )

    expect(vueFlowMocks.addSelectedNodes).not.toHaveBeenCalled()
    expect(vueFlowMocks.removeSelectedElements).not.toHaveBeenCalled()
  })

  it('filters native edge selection changes only while marquee selection is active', () => {
    const harness = createHarness()
    const edgeSelectChange: EdgeChange = { id: 'edge-a', type: 'select', selected: true }
    const edgeRemoveChange: EdgeChange = {
      id: 'edge-b',
      type: 'remove',
      source: 'node-a',
      target: 'node-b',
      sourceHandle: null,
      targetHandle: null,
    }

    harness.adapter.vueFlowSelectionBindings.onEdgesChange([edgeSelectChange])
    expect(harness.onEdgesChange).toHaveBeenLastCalledWith([edgeSelectChange])

    harness.adapter.vueFlowSelectionBindings.onSelectionStart(
      new MouseEvent('mousedown', { clientX: 10, clientY: 10 }),
    )
    harness.onEdgesChange.mockClear()

    harness.adapter.vueFlowSelectionBindings.onEdgesChange([edgeSelectChange, edgeRemoveChange])

    expect(harness.onEdgesChange).toHaveBeenCalledWith([edgeRemoveChange])

    harness.adapter.vueFlowSelectionBindings.onSelectionEnd(
      new MouseEvent('mouseup', { clientX: 20, clientY: 20 }),
    )
    harness.onEdgesChange.mockClear()

    harness.adapter.vueFlowSelectionBindings.onEdgesChange([edgeSelectChange])

    expect(harness.onEdgesChange).toHaveBeenLastCalledWith([edgeSelectChange])
  })
})
