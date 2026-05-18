import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'

export type FlowHistorySnapshot = {
  nodes: unknown[]
  edges: unknown[]
}

export type CommitMutationOptions = {
  recordHistory?: boolean
  historyBefore?: FlowHistorySnapshot
  markDirty?: boolean
}

export type FlowTransactions = {
  captureHistory: () => FlowHistorySnapshot
  beginNodeDragTransaction: () => void
  commitNodeDragTransaction: () => void
  commitDirty: (commitOptions?: Pick<CommitMutationOptions, 'markDirty'>) => void
  commitGraphMutation: (
    mutator: (nodes: Node[], edges: Edge[]) => { nodes: Node[]; edges: Edge[] },
    commitOptions?: CommitMutationOptions,
  ) => void
  commitNodeMutation: (
    mutator: (nodes: Node[]) => Node[],
    commitOptions?: CommitMutationOptions,
  ) => void
  commitEdgeMutation: (
    mutator: (edges: Edge[]) => Edge[],
    commitOptions?: CommitMutationOptions,
  ) => void
}

type UseFlowTransactionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  captureHistory: () => FlowHistorySnapshot
  pushHistory: (snapshot: FlowHistorySnapshot) => void
  beginDragHistory: () => void
  commitDragHistory: (onCommit: () => void) => void
  markDirty: () => void
  refreshSnapshot: () => void
  commitNodes: (nextNodes: Node[]) => void
  commitEdges: (nextEdges: Edge[]) => void
}

export function useFlowTransactions(options: UseFlowTransactionsOptions): FlowTransactions {
  function commitDirty(commitOptions?: Pick<CommitMutationOptions, 'markDirty'>) {
    if (commitOptions?.markDirty !== false) {
      options.markDirty()
    }
    options.refreshSnapshot()
  }

  function recordHistory(historyBefore: FlowHistorySnapshot, commitOptions?: CommitMutationOptions) {
    if (commitOptions?.recordHistory !== false) {
      options.pushHistory(historyBefore)
    }
  }

  function commitGraphMutation(
    mutator: (nodes: Node[], edges: Edge[]) => { nodes: Node[]; edges: Edge[] },
    commitOptions?: CommitMutationOptions,
  ) {
    const historyBefore = commitOptions?.historyBefore ?? options.captureHistory()
    const { nodes: nextNodes, edges: nextEdges } = mutator(options.nodes.value, options.edges.value)
    if (nextNodes === options.nodes.value && nextEdges === options.edges.value) return
    options.commitNodes(nextNodes)
    options.commitEdges(nextEdges)
    recordHistory(historyBefore, commitOptions)
    commitDirty(commitOptions)
  }

  function commitNodeMutation(
    mutator: (nodes: Node[]) => Node[],
    commitOptions?: CommitMutationOptions,
  ) {
    const historyBefore = commitOptions?.historyBefore ?? options.captureHistory()
    const nextNodes = mutator(options.nodes.value)
    if (nextNodes === options.nodes.value) return
    options.commitNodes(nextNodes)
    recordHistory(historyBefore, commitOptions)
    commitDirty(commitOptions)
  }

  function commitEdgeMutation(
    mutator: (edges: Edge[]) => Edge[],
    commitOptions?: CommitMutationOptions,
  ) {
    const historyBefore = commitOptions?.historyBefore ?? options.captureHistory()
    const nextEdges = mutator(options.edges.value)
    if (nextEdges === options.edges.value) return
    options.commitEdges(nextEdges)
    recordHistory(historyBefore, commitOptions)
    commitDirty(commitOptions)
  }

  function beginNodeDragTransaction() {
    options.beginDragHistory()
  }

  function commitNodeDragTransaction() {
    options.commitDragHistory(() => {
      options.markDirty()
    })
  }

  return {
    captureHistory: options.captureHistory,
    beginNodeDragTransaction,
    commitNodeDragTransaction,
    commitDirty,
    commitGraphMutation,
    commitNodeMutation,
    commitEdgeMutation,
  }
}
