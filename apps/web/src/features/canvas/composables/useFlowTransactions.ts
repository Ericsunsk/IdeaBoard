import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
export type { FlowHistorySnapshot } from './useFlowHistory'
import type { FlowHistorySnapshot } from './useFlowHistory'

export type CommitMutationOptions = {
  recordHistory?: boolean
  historyBefore?: FlowHistorySnapshot
  markDirty?: boolean
}

type FlowGraph = {
  nodes: Node[]
  edges: Edge[]
}

export type FlowTransactions = {
  captureHistory: () => FlowHistorySnapshot
  beginNodeDragTransaction: () => void
  commitNodeDragTransaction: () => void
  commitDirty: (commitOptions?: Pick<CommitMutationOptions, 'markDirty'>) => void
  commitGraphMutation: (
    mutator: (nodes: Node[], edges: Edge[]) => FlowGraph,
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
  commitGraph: (nextGraph: FlowGraph) => void
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

  function captureHistoryBeforeCommit(commitOptions?: CommitMutationOptions) {
    if (commitOptions?.recordHistory === false) return null
    return commitOptions?.historyBefore ?? options.captureHistory()
  }

  function recordHistory(historyBefore: FlowHistorySnapshot | null) {
    if (historyBefore) {
      options.pushHistory(historyBefore)
    }
  }

  function commitGraphMutation(
    mutator: (nodes: Node[], edges: Edge[]) => FlowGraph,
    commitOptions?: CommitMutationOptions,
  ) {
    const { nodes: nextNodes, edges: nextEdges } = mutator(options.nodes.value, options.edges.value)
    if (nextNodes === options.nodes.value && nextEdges === options.edges.value) return
    const resolvedHistoryBefore = captureHistoryBeforeCommit(commitOptions)
    options.commitGraph({ nodes: nextNodes, edges: nextEdges })
    recordHistory(resolvedHistoryBefore)
    commitDirty(commitOptions)
  }

  function commitNodeMutation(
    mutator: (nodes: Node[]) => Node[],
    commitOptions?: CommitMutationOptions,
  ) {
    const nextNodes = mutator(options.nodes.value)
    if (nextNodes === options.nodes.value) return
    const resolvedHistoryBefore = captureHistoryBeforeCommit(commitOptions)
    options.commitNodes(nextNodes)
    recordHistory(resolvedHistoryBefore)
    commitDirty(commitOptions)
  }

  function commitEdgeMutation(
    mutator: (edges: Edge[]) => Edge[],
    commitOptions?: CommitMutationOptions,
  ) {
    const nextEdges = mutator(options.edges.value)
    if (nextEdges === options.edges.value) return
    const resolvedHistoryBefore = captureHistoryBeforeCommit(commitOptions)
    options.commitEdges(nextEdges)
    recordHistory(resolvedHistoryBefore)
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
