import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { shallowRef } from 'vue'
import { useFlowTransactions } from './useFlowTransactions'

function createTransactionsHarness() {
  const nodes = shallowRef<Node[]>([{ id: 'node-1', position: { x: 0, y: 0 }, data: {} }])
  const edges = shallowRef<Edge[]>([])
  const captureHistory = vi.fn(() => ({
    nodes: nodes.value,
    edges: edges.value,
  }))
  const pushHistory = vi.fn()
  const markDirty = vi.fn()
  const refreshSnapshot = vi.fn()
  const commitNodes = vi.fn((nextNodes: Node[]) => {
    nodes.value = nextNodes
  })
  const commitEdges = vi.fn((nextEdges: Edge[]) => {
    edges.value = nextEdges
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
      (currentNodes) => [
        ...currentNodes,
        { id: 'node-2', position: { x: 10, y: 0 }, data: {} },
      ],
      { recordHistory: false },
    )

    expect(captureHistory).not.toHaveBeenCalled()
    expect(pushHistory).not.toHaveBeenCalled()
  })
})
