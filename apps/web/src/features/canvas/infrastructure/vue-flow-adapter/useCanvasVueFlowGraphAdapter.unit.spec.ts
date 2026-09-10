import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphActions,
} from './useCanvasVueFlowGraphAdapter'

type SelectableNode = Node & { selected?: boolean }
type SelectableEdge = Edge & { selected?: boolean }
type MeasuredNode = Node & {
  dimensions?: { width: number; height: number }
  handleBounds?: unknown
  resizing?: boolean
}

describe('useCanvasVueFlowGraphAdapter', () => {
  it('applies an incremental projection without creating a Flow command', () => {
    const unchangedNode: Node = { id: 'node-2', position: { x: 20, y: 0 }, data: {} }
    const changedNode: Node = { id: 'node-1', position: { x: 0, y: 0 }, data: {} }
    const nodes = shallowRef<Node[]>([changedNode, unchangedNode])
    const edges = shallowRef<Edge[]>([])
    const beforeCommand = vi.fn()
    const afterCommand = vi.fn()
    const runtimeNode = {
      ...changedNode,
      computedPosition: { x: 0, y: 0, z: 0 },
      handleBounds: { source: [], target: [] },
      dragging: false,
      initialized: true,
    }
    const updateNode = vi.fn((id, update) => {
      if (id !== runtimeNode.id) return
      Object.assign(runtimeNode, typeof update === 'function' ? update(runtimeNode) : update)
    })
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { updateNode },
      beforeCommand,
      afterCommand,
    })
    const projectedNode = { ...changedNode, position: { x: 40, y: 0 } }

    adapter.applyIncrementalProjection({
      nodes: [projectedNode, unchangedNode],
      changedNodeIds: ['node-1'],
    })

    expect(nodes.value).toEqual([projectedNode, unchangedNode])
    expect(nodes.value[1]).toBe(unchangedNode)
    expect(updateNode).toHaveBeenCalledWith('node-1', expect.any(Function), { replace: false })
    expect(runtimeNode).toMatchObject({
      computedPosition: { x: 0, y: 0, z: 0 },
      handleBounds: { source: [], target: [] },
      initialized: true,
      position: { x: 40, y: 0 },
    })
    expect(beforeCommand).not.toHaveBeenCalled()
    expect(afterCommand).not.toHaveBeenCalled()
  })

  it('synchronizes a drag commit through Vue Flow renderer actions', () => {
    const currentNode: Node = { id: 'node-1', position: { x: 0, y: 0 }, data: {} }
    const projectedNode: Node = { ...currentNode, position: { x: 40, y: 20 } }
    const nodes = shallowRef<Node[]>([currentNode])
    const edges = shallowRef<Edge[]>([])
    const updateNode = vi.fn()
    const setNodes = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { updateNode, setNodes },
    })

    adapter.applyIncrementalProjection({
      nodes: [projectedNode],
      changedNodeIds: ['node-1'],
      nodeMutations: { added: [], updated: [projectedNode], removedIds: [] },
    })

    expect(nodes.value).toEqual([projectedNode])
    expect(updateNode).toHaveBeenCalledWith('node-1', expect.any(Function), { replace: false })
    expect(setNodes).not.toHaveBeenCalled()
  })

  it('syncs active drag positions into the controlled model without renderer fields', () => {
    const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({ nodes, edges, actions: { applyNodeChanges } })

    adapter.syncControlledNodeChanges([
      {
        id: 'node-1',
        type: 'position',
        position: { x: 40, y: 20 },
        from: { x: 0, y: 0 },
        dragging: true,
      },
    ])

    expect(nodes.value[0]).toMatchObject({ position: { x: 40, y: 20 } })
    expect(nodes.value[0]).not.toHaveProperty('dragging')
    expect(applyNodeChanges).not.toHaveBeenCalled()
  })

  it('keeps animated edge state out of the semantic graph on every write path', () => {
    const nodes = shallowRef<Node[]>([])
    const edges = shallowRef<Edge[]>([])
    const setEdges = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({ nodes, edges, actions: { setEdges } })
    const renderEdge: Edge = {
      id: 'edge-1',
      source: 'node-1',
      target: 'image-1',
      targetHandle: 'in:image',
      animated: true,
    }

    adapter.replaceGraph({ nodes: [], edges: [renderEdge] })
    expect(edges.value[0]).not.toHaveProperty('animated')
    expect(setEdges).toHaveBeenLastCalledWith([expect.not.objectContaining({ animated: true })])

    adapter.replaceEdges([renderEdge])
    expect(edges.value[0]).not.toHaveProperty('animated')
    expect(setEdges).toHaveBeenLastCalledWith([expect.not.objectContaining({ animated: true })])

    adapter.addEdges({ ...renderEdge, id: 'edge-2' })
    expect(edges.value).toEqual([
      expect.objectContaining({ id: 'edge-1' }),
      expect.objectContaining({ id: 'edge-2' }),
    ])
    expect(edges.value.every((edge) => !('animated' in edge))).toBe(true)
  })

  it('replaces the renderer projection during full recovery', () => {
    const nodes = shallowRef<Node[]>([{ id: 'stale', position: { x: 0, y: 0 }, data: {} }])
    const edges = shallowRef<Edge[]>([{ id: 'stale-edge', source: 'stale', target: 'stale' }])
    const setNodes = vi.fn()
    const setEdges = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({ nodes, edges, actions: { setNodes, setEdges } })

    const canonicalNode: Node = { id: 'canonical', position: { x: 20, y: 20 }, data: {} }
    adapter.replaceFullProjection({ nodes: [canonicalNode], edges: [] })

    expect(setNodes).toHaveBeenCalledWith([canonicalNode])
    expect(setEdges).toHaveBeenCalledWith([])
    expect(nodes.value).toEqual([canonicalNode])
    expect(edges.value).toEqual([])
  })

  it('runs a node command through Vue Flow actions and exposes hook results', () => {
    const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }])
    const edges = shallowRef<Edge[]>([])
    const nodeToAdd: Node = { id: 'node-2', position: { x: 20, y: 0 }, data: {} }
    const beforeCommand = vi.fn(() => ({ historyToken: 'before-node-add' }))
    const afterCommand = vi.fn()
    const addNodes = vi.fn()

    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { addNodes },
      beforeCommand,
      afterCommand,
    })

    const result = adapter.addNodes(nodeToAdd, {
      metadata: { reason: 'unit-test' },
    })

    expect(addNodes).toHaveBeenCalledOnce()
    expect(addNodes).toHaveBeenCalledWith(nodeToAdd)
    expect(nodes.value.map((node) => node.id)).toEqual(['node-1', 'node-2'])
    expect(result).toMatchObject({
      command: 'addNodes',
      didChange: true,
      metadata: { reason: 'unit-test' },
    })
    expect(result.before.nodes.map((node) => node.id)).toEqual(['node-1'])
    expect(result.after.nodes.map((node) => node.id)).toEqual(['node-1', 'node-2'])
    expect(beforeCommand).toHaveBeenCalledWith(
      expect.objectContaining({
        command: 'addNodes',
        metadata: { reason: 'unit-test' },
      }),
    )
    expect(afterCommand).toHaveBeenCalledWith(
      expect.objectContaining({ command: 'addNodes', didChange: true }),
      { historyToken: 'before-node-add' },
    )
  })

  it('runs global and command hooks around the graph commit in order', () => {
    const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }])
    const edges = shallowRef<Edge[]>([])
    const events: string[] = []
    const nodeToAdd: Node = { id: 'node-2', position: { x: 20, y: 0 }, data: {} }
    const addNodes = vi.fn(() => events.push('commit'))
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { addNodes },
      beforeCommand: (context) => {
        events.push(`global-before:${context.next.nodes.length}`)
        return 'global-token'
      },
      afterCommand: (_result, beforeValue) => {
        events.push(`global-after:${String(beforeValue)}:${nodes.value.length}`)
      },
    })

    const result = adapter.addNodes(nodeToAdd, {
      beforeCommand: (context) => {
        events.push(`command-before:${context.before.nodes.length}`)
        return 'command-token'
      },
      afterCommand: (_result, beforeValue) => {
        events.push(`command-after:${String(beforeValue)}:${nodes.value.length}`)
      },
    })

    expect(result).toMatchObject({ command: 'addNodes', didChange: true })
    expect(events).toEqual([
      'global-before:2',
      'command-before:1',
      'commit',
      'global-after:global-token:2',
      'command-after:command-token:2',
    ])
  })

  it('runs an edge command through Vue Flow actions and skips hooks for no-op targets', () => {
    const nodes = shallowRef<Node[]>([
      { id: 'node-1', position: { x: 0, y: 0 }, data: {} },
      { id: 'node-2', position: { x: 20, y: 0 }, data: {} },
    ])
    const edges = shallowRef<Edge[]>([
      { id: 'edge-1', source: 'node-1', target: 'node-2' },
      { id: 'edge-2', source: 'node-2', target: 'node-1' },
    ])
    const removeEdges = vi.fn()
    const afterCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { removeEdges },
    })

    const result = adapter.removeEdges('edge-1', { afterCommand })
    const noOpResult = adapter.removeEdges('missing-edge', { afterCommand })

    expect(removeEdges).toHaveBeenCalledOnce()
    expect(removeEdges).toHaveBeenCalledWith(['edge-1'])
    expect(edges.value.map((edge) => edge.id)).toEqual(['edge-2'])
    expect(result).toMatchObject({ command: 'removeEdges', didChange: true })
    expect(result.before.edges.map((edge) => edge.id)).toEqual(['edge-1', 'edge-2'])
    expect(result.after.edges.map((edge) => edge.id)).toEqual(['edge-2'])
    expect(noOpResult).toMatchObject({ command: 'removeEdges', didChange: false })
    expect(afterCommand).toHaveBeenCalledOnce()
  })

  it('applies native node selection as a visual-only command result', () => {
    const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn((changes) => {
      for (const change of changes) {
        if (change.type !== 'select') continue
        const node = nodes.value.find((item) => item.id === change.id)
        if (node) (node as SelectableNode).selected = change.selected
      }
      return nodes.value
    }) as unknown as NonNullable<CanvasVueFlowGraphActions['applyNodeChanges']>
    const beforeCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
      beforeCommand,
    })

    const result = adapter.applyNodeChanges([{ id: 'node-1', type: 'select', selected: true }])

    expect(applyNodeChanges).toHaveBeenCalledOnce()
    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: false })
    expect(result.after.nodes[0]).toMatchObject({ id: 'node-1', selected: true })
    expect(beforeCommand).not.toHaveBeenCalled()
  })

  it('uses Vue Flow native node change results without cloning unchanged nodes', () => {
    const unchangedNode: Node = { id: 'node-2', position: { x: 20, y: 0 }, data: {} }
    const nodeToSelect: Node = { id: 'node-1', position: { x: 0, y: 0 }, data: {} }
    const nativeNodes = [nodeToSelect, unchangedNode]
    const nodes = shallowRef<Node[]>(nativeNodes)
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn((changes) => {
      expect(changes).toEqual([{ id: 'node-1', type: 'select', selected: true }])
      ;(nodeToSelect as SelectableNode).selected = true
      return nativeNodes
    }) as unknown as NonNullable<CanvasVueFlowGraphActions['applyNodeChanges']>
    const afterCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
      afterCommand,
    })

    const result = adapter.applyNodeChanges([{ id: 'node-1', type: 'select', selected: true }])

    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: false })
    expect(applyNodeChanges).toHaveBeenCalledOnce()
    expect(nodes.value).toBe(nativeNodes)
    expect(nodes.value[1]).toBe(unchangedNode)
    expect(nodes.value[0]).toMatchObject({ id: 'node-1', selected: true })
    expect(afterCommand).not.toHaveBeenCalled()
  })

  it('skips Vue Flow native node changes that do not target the current graph', () => {
    const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn()
    const afterCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
      afterCommand,
    })

    const result = adapter.applyNodeChanges([
      { id: 'missing-node', type: 'select', selected: true },
    ])

    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: false })
    expect(applyNodeChanges).not.toHaveBeenCalled()
    expect(afterCommand).not.toHaveBeenCalled()
  })

  it('skips repeated native node dimension changes that match current node state', () => {
    const nodes = shallowRef<Node[]>([
      {
        id: 'node-1',
        position: { x: 0, y: 0 },
        data: {},
        dimensions: { width: 100, height: 80 },
        handleBounds: { source: [], target: [] },
        resizing: false,
      } as MeasuredNode,
    ])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn()
    const afterCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
      afterCommand,
    })

    const result = adapter.applyNodeChanges([
      {
        id: 'node-1',
        type: 'dimensions',
        dimensions: { width: 100, height: 80 },
        handleBounds: { source: [], target: [] },
        resizing: false,
      } as NodeChange,
    ])

    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: false })
    expect(applyNodeChanges).not.toHaveBeenCalled()
    expect(afterCommand).not.toHaveBeenCalled()
  })

  it('applies native node dimension changes as visual-only command results', () => {
    const measuredNode = {
      id: 'node-1',
      position: { x: 0, y: 0 },
      data: {},
      dimensions: { width: 100, height: 80 },
    } as MeasuredNode
    const nodes = shallowRef<Node[]>([measuredNode])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn((changes) => {
      const change = changes[0]
      if (change?.type === 'dimensions') {
        measuredNode.dimensions = change.dimensions
      }
      return nodes.value
    }) as unknown as NonNullable<CanvasVueFlowGraphActions['applyNodeChanges']>
    const afterCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
      afterCommand,
    })

    const result = adapter.applyNodeChanges([
      {
        id: 'node-1',
        type: 'dimensions',
        dimensions: { width: 120, height: 80 },
      } as NodeChange,
    ])

    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: false })
    expect(applyNodeChanges).toHaveBeenCalledOnce()
    expect(afterCommand).not.toHaveBeenCalled()
  })

  it('classifies repeated handle bounds and resizing changes as no-op native node changes', () => {
    const nodes = shallowRef<Node[]>([
      {
        id: 'node-1',
        position: { x: 0, y: 0 },
        data: {},
        handleBounds: { source: [{ id: 'out' }], target: [] },
        resizing: true,
      } as MeasuredNode,
    ])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
    })

    const repeatedHandleBoundsResult = adapter.applyNodeChanges([
      {
        id: 'node-1',
        type: 'dimensions',
        handleBounds: { source: [{ id: 'out' }], target: [] },
      } as unknown as NodeChange,
    ])
    const repeatedResizingResult = adapter.applyNodeChanges([
      {
        id: 'node-1',
        type: 'dimensions',
        resizing: true,
      } as NodeChange,
    ])

    expect(repeatedHandleBoundsResult).toMatchObject({
      command: 'applyNodeChanges',
      didChange: false,
    })
    expect(repeatedResizingResult).toMatchObject({
      command: 'applyNodeChanges',
      didChange: false,
    })
    expect(applyNodeChanges).not.toHaveBeenCalled()
  })

  it('applies changed handle bounds and resizing without leaking them into the controlled model', () => {
    const measuredNode = {
      id: 'node-1',
      position: { x: 0, y: 0 },
      data: {},
      handleBounds: { source: [], target: [] },
      resizing: false,
    } as MeasuredNode
    const nodes = shallowRef<Node[]>([measuredNode])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn((changes) => {
      for (const change of changes) {
        if (change.type !== 'dimensions') continue
        measuredNode.handleBounds = change.handleBounds ?? measuredNode.handleBounds
        measuredNode.resizing = change.resizing ?? measuredNode.resizing
      }
      return nodes.value
    }) as unknown as NonNullable<CanvasVueFlowGraphActions['applyNodeChanges']>
    const afterCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
      afterCommand,
    })

    const result = adapter.applyNodeChanges([
      {
        id: 'node-1',
        type: 'dimensions',
        handleBounds: { source: [{ id: 'out' }], target: [] },
        resizing: true,
      } as unknown as NodeChange,
    ])

    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: false })
    expect(result.after.nodes[0]).not.toHaveProperty('handleBounds')
    expect(result.after.nodes[0]).not.toHaveProperty('resizing')
    expect(applyNodeChanges).toHaveBeenCalledOnce()
    expect(afterCommand).not.toHaveBeenCalled()
  })

  it('classifies unchanged node position changes as no-op native node changes', () => {
    const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 10, y: 20 }, data: {} }])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
    })

    const result = adapter.applyNodeChanges([
      { id: 'node-1', type: 'position', position: { x: 10, y: 20 } } as NodeChange,
    ])

    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: false })
    expect(applyNodeChanges).not.toHaveBeenCalled()
  })

  it('classifies changed node position as a graph-affecting command result', () => {
    const positionedNode = { id: 'node-1', position: { x: 10, y: 20 }, data: {} }
    const nodes = shallowRef<Node[]>([positionedNode])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn((changes) => {
      const change = changes[0]
      if (change?.type === 'position' && change.position) {
        positionedNode.position = change.position
      }
      return nodes.value
    }) as unknown as NonNullable<CanvasVueFlowGraphActions['applyNodeChanges']>
    const beforeCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
      beforeCommand,
    })

    const result = adapter.applyNodeChanges([
      { id: 'node-1', type: 'position', position: { x: 12, y: 24 } } as NodeChange,
    ])

    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: true })
    expect(result.after.nodes[0]).toMatchObject({ position: { x: 12, y: 24 } })
    expect(applyNodeChanges).toHaveBeenCalledOnce()
    expect(beforeCommand).toHaveBeenCalledOnce()
  })

  it('applies native edge selection as a visual-only command result', () => {
    const nodes = shallowRef<Node[]>([
      { id: 'node-1', position: { x: 0, y: 0 }, data: {} },
      { id: 'node-2', position: { x: 20, y: 0 }, data: {} },
    ])
    const edges = shallowRef<Edge[]>([{ id: 'edge-1', source: 'node-1', target: 'node-2' }])
    const applyEdgeChanges = vi.fn((changes: EdgeChange[]) => {
      const change = changes[0]
      if (change?.type === 'select') {
        edges.value[0] = {
          ...(edges.value[0] as SelectableEdge),
          selected: change.selected,
        } as unknown as Edge
      }
      return edges.value
    }) as unknown as NonNullable<CanvasVueFlowGraphActions['applyEdgeChanges']>
    const afterCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyEdgeChanges },
      afterCommand,
    })

    const result = adapter.applyEdgeChanges([{ id: 'edge-1', type: 'select', selected: true }])

    expect(result).toMatchObject({ command: 'applyEdgeChanges', didChange: false })
    expect(result.after.edges[0]).toMatchObject({ selected: true })
    expect(applyEdgeChanges).toHaveBeenCalledOnce()
    expect(afterCommand).not.toHaveBeenCalled()
  })

  it('applies node and edge changes as one graph command', () => {
    const nodes = shallowRef<Node[]>([
      { id: 'node-1', position: { x: 0, y: 0 }, data: {} },
      { id: 'node-2', position: { x: 20, y: 0 }, data: {} },
    ])
    const edges = shallowRef<Edge[]>([{ id: 'edge-1', source: 'node-1', target: 'node-2' }])
    const applyNodeChanges = vi.fn()
    const applyEdgeChanges = vi.fn()
    const afterCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges, applyEdgeChanges },
      afterCommand,
    })

    const result = adapter.applyGraphChanges(
      {
        nodes: [{ id: 'node-1', type: 'remove' }],
        edges: [
          {
            id: 'edge-1',
            type: 'remove',
            source: 'node-1',
            target: 'node-2',
            sourceHandle: null,
            targetHandle: null,
          },
        ],
      },
      { metadata: { reason: 'native-delete' } },
    )

    expect(result).toMatchObject({
      command: 'applyGraphChanges',
      didChange: true,
      metadata: { reason: 'native-delete' },
    })
    expect(nodes.value.map((node) => node.id)).toEqual(['node-2'])
    expect(edges.value).toHaveLength(0)
    expect(applyNodeChanges).toHaveBeenCalledOnce()
    expect(applyEdgeChanges).toHaveBeenCalledOnce()
    expect(afterCommand).toHaveBeenCalledOnce()
  })
})
