import type { Ref } from 'vue'
import type { Connection, Edge, EdgeUpdateEvent } from '@vue-flow/core'

type UseFlowConnectionMutationsOptions = {
  edges: Ref<Array<Record<string, any>>>
  isValidConnection: (connection: Connection) => boolean
  normalizeConnection?: (connection: Connection, existingEdges: readonly Edge[]) => Connection | null
  captureHistory: () => { nodes: unknown[]; edges: unknown[] }
  pushHistory: (snapshot: { nodes: unknown[]; edges: unknown[] }) => void
  commitEdges: (nextEdges: Edge[]) => void
  markDirty: () => void
  refreshSnapshot: () => void
  onWarning: (message: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export function useFlowConnectionMutations(options: UseFlowConnectionMutationsOptions) {
  function onConnect(connection: Connection) {
    const existingEdges = options.edges.value as unknown as Edge[]
    const resolvedConnection = options.normalizeConnection
      ? options.normalizeConnection(connection, existingEdges)
      : connection
    if (!resolvedConnection || !options.isValidConnection(resolvedConnection)) {
      options.onWarning('该连线不符合当前连接语义')
      return
    }
    if (!resolvedConnection.source || !resolvedConnection.target) return
    const nextEdge = {
      ...resolvedConnection,
      id: `edge-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      source: resolvedConnection.source,
      target: resolvedConnection.target,
      animated: false,
    } as Edge
    const before = options.captureHistory()
    const filteredEdges = existingEdges.filter((edge) => {
      if (
        edge.target === resolvedConnection.target &&
        edge.targetHandle === resolvedConnection.targetHandle
      )
        return false
      if (
        edge.source === resolvedConnection.source &&
        edge.target === resolvedConnection.target &&
        edge.sourceHandle === resolvedConnection.sourceHandle &&
        edge.targetHandle === resolvedConnection.targetHandle
      )
        return false
      return true
    })
    options.commitEdges([...filteredEdges, nextEdge])
    options.pushHistory(before)
    options.markDirty()
    options.refreshSnapshot()
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  function onEdgeUpdate(event: EdgeUpdateEvent) {
    const { edge, connection } = event
    const existingEdges = (options.edges.value as unknown as Edge[]).filter(
      (item) => item.id !== edge.id,
    )
    const resolvedConnection = options.normalizeConnection
      ? options.normalizeConnection(connection, existingEdges)
      : connection
    if (!resolvedConnection || !options.isValidConnection(resolvedConnection)) {
      options.onWarning('该连线不符合当前连接语义')
      return
    }
    if (!resolvedConnection.source || !resolvedConnection.target) return
    const before = options.captureHistory()
    const nextEdges = (options.edges.value as unknown as Edge[])
      .filter(
        (item) =>
          item.id === edge.id ||
          !(
            item.target === resolvedConnection.target &&
            (item.targetHandle ?? null) === (resolvedConnection.targetHandle ?? null)
          ),
      )
      .map((item) => {
        if (item.id !== edge.id) return item
        return {
          ...item,
          source: resolvedConnection.source!,
          target: resolvedConnection.target!,
          sourceHandle: resolvedConnection.sourceHandle ?? null,
          targetHandle: resolvedConnection.targetHandle ?? null,
        } as Edge
      })
    options.commitEdges(nextEdges)
    options.pushHistory(before)
    options.markDirty()
    options.refreshSnapshot()
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  return {
    onConnect,
    onEdgeUpdate,
  }
}
