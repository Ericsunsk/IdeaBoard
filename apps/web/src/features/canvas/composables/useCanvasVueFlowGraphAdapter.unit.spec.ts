import type { Edge, Node } from '@vue-flow/core'
import { shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useCanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'

describe('useCanvasVueFlowGraphAdapter', () => {
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

  it('updates an edge through the Vue Flow setEdges action', () => {
    const nodes = shallowRef<Node[]>([
      { id: 'node-1', position: { x: 0, y: 0 }, data: {} },
      { id: 'node-2', position: { x: 20, y: 0 }, data: {} },
    ])
    const edges = shallowRef<Edge[]>([
      { id: 'edge-1', source: 'node-1', target: 'node-2', animated: false },
    ])
    const setEdges = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { setEdges },
    })

    const result = adapter.updateEdge('edge-1', { animated: true })

    expect(result).toMatchObject({ command: 'updateEdge', didChange: true })
    expect(edges.value[0]).toMatchObject({ id: 'edge-1', animated: true })
    expect(setEdges).toHaveBeenCalledOnce()
    expect(setEdges).toHaveBeenCalledWith(edges.value)
  })

  it('applies node changes without mutating the command before graph', () => {
    const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }])
    const edges = shallowRef<Edge[]>([])
    const applyNodeChanges = vi.fn()
    const beforeCommand = vi.fn()
    const adapter = useCanvasVueFlowGraphAdapter({
      nodes,
      edges,
      actions: { applyNodeChanges },
      beforeCommand,
    })

    const result = adapter.applyNodeChanges([
      { id: 'node-1', type: 'select', selected: true },
    ])

    expect(applyNodeChanges).toHaveBeenCalledOnce()
    expect(result).toMatchObject({ command: 'applyNodeChanges', didChange: true })
    expect(result.before.nodes[0]).not.toHaveProperty('selected')
    expect(result.after.nodes[0]).toMatchObject({ id: 'node-1', selected: true })
    expect(beforeCommand).toHaveBeenCalledWith(
      expect.objectContaining({
        before: expect.objectContaining({
          nodes: [expect.not.objectContaining({ selected: true })],
        }),
      }),
    )
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
