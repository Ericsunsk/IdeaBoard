import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { shallowRef } from 'vue'
import { useFlowHistory } from './useFlowHistory'
import { useFlowTransactions } from './useFlowTransactions'

function createTransactionsHarness() {
  const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }])
  const edges = shallowRef<Edge[]>([])
  const flowHistory = useFlowHistory({
    nodes,
    edges,
    commitNodes: (nextNodes) => {
      nodes.value = nextNodes as Node[]
    },
    commitEdges: (nextEdges) => {
      edges.value = nextEdges as Edge[]
    },
  })
  const captureHistory = vi.fn(flowHistory.capture)
  const pushHistory = vi.fn()
  const markDirty = vi.fn()
  const refreshSnapshot = vi.fn()
  const commitNodes = vi.fn((nextNodes: Node[]) => {
    nodes.value = nextNodes
  })
  const commitEdges = vi.fn((nextEdges: Edge[]) => {
    edges.value = nextEdges
  })
  const commitGraph = vi.fn((nextGraph: { nodes: Node[]; edges: Edge[] }) => {
    nodes.value = nextGraph.nodes
    edges.value = nextGraph.edges
  })
  const transactions = useFlowTransactions({
    nodes,
    edges,
    captureHistory,
    pushHistory,
    beginDragHistory: vi.fn(),
    commitDragHistory: vi.fn(),
    markDirty,
    refreshSnapshot,
    commitGraph,
    commitNodes,
    commitEdges,
  })

  return {
    nodes,
    edges,
    captureHistory,
    pushHistory,
    markDirty,
    refreshSnapshot,
    commitGraph,
    commitNodes,
    commitEdges,
    transactions,
  }
}

describe('useFlowTransactions', () => {
  it('does not capture history for no-op mutations', () => {
    const { nodes, captureHistory, pushHistory, transactions } = createTransactionsHarness()

    transactions.commitNodeMutation((currentNodes) => currentNodes)

    expect(captureHistory).not.toHaveBeenCalled()
    expect(pushHistory).not.toHaveBeenCalled()
    expect(nodes.value).toHaveLength(1)
  })

  it('captures history lazily only after a mutation changes the graph', () => {
    const { captureHistory, commitNodes, pushHistory, transactions } = createTransactionsHarness()

    transactions.commitNodeMutation((currentNodes) => [
      ...currentNodes,
      { id: 'node-2', position: { x: 10, y: 0 }, data: {} },
    ])

    expect(captureHistory).toHaveBeenCalledTimes(1)
    expect(commitNodes).toHaveBeenCalledTimes(1)
    expect(pushHistory).toHaveBeenCalledTimes(1)
  })

  it('does not capture history when history recording is disabled', () => {
    const { captureHistory, pushHistory, transactions } = createTransactionsHarness()

    transactions.commitNodeMutation(
      (currentNodes) => [...currentNodes, { id: 'node-2', position: { x: 10, y: 0 }, data: {} }],
      { recordHistory: false },
    )

    expect(captureHistory).not.toHaveBeenCalled()
    expect(pushHistory).not.toHaveBeenCalled()
  })

  it('commits graph mutations atomically without separate node or edge commits', () => {
    const {
      nodes,
      edges,
      captureHistory,
      pushHistory,
      markDirty,
      refreshSnapshot,
      commitGraph,
      commitNodes,
      commitEdges,
      transactions,
    } = createTransactionsHarness()
    const nextNode = { id: 'node-2', position: { x: 10, y: 0 }, data: {} }
    const nextEdge = { id: 'edge-1', source: 'node-1', target: 'node-2' }

    transactions.commitGraphMutation((currentNodes) => ({
      nodes: [...currentNodes, nextNode],
      edges: [nextEdge],
    }))

    expect(commitGraph).toHaveBeenCalledTimes(1)
    expect(commitGraph).toHaveBeenCalledWith({ nodes: nodes.value, edges: edges.value })
    expect(commitNodes).not.toHaveBeenCalled()
    expect(commitEdges).not.toHaveBeenCalled()
    expect(captureHistory).toHaveBeenCalledTimes(1)
    expect(pushHistory).toHaveBeenCalledTimes(1)
    expect(markDirty).toHaveBeenCalledTimes(1)
    expect(refreshSnapshot).toHaveBeenCalledTimes(1)
    expect(nodes.value).toEqual([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }, nextNode])
    expect(edges.value).toEqual([nextEdge])
  })
})
