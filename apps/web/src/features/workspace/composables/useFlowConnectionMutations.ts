import type { Ref } from 'vue'
import type { Connection, Edge, EdgeUpdateEvent } from '@vue-flow/core'
import { resolveFlowEdgeToneFromHandles } from '../utils/flowEdgeVisual'

type UseFlowConnectionMutationsOptions = {
  edges: Ref<Array<Record<string, any>>>
  isValidConnection: (connection: Connection) => boolean
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
    if (!options.isValidConnection(connection)) {
      options.onWarning('该连线不符合当前连接语义')
      return
    }
    if (!connection.source || !connection.target) return
    const tone = resolveFlowEdgeToneFromHandles(connection.sourceHandle, connection.targetHandle)
    const nextEdge = {
      ...connection,
      id: `edge-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      source: connection.source,
      target: connection.target,
      animated: false,
      class: `tone-${tone}`,
      data: { tone },
    } as Edge
    const before = options.captureHistory()
    const existingEdges = options.edges.value as unknown as Edge[]
    const filteredEdges = existingEdges.filter(edge => {
      if (edge.target === connection.target && edge.targetHandle === connection.targetHandle) return false
      if (
        edge.source === connection.source
        && edge.target === connection.target
        && edge.sourceHandle === connection.sourceHandle
        && edge.targetHandle === connection.targetHandle
      ) return false
      return true
    })
    options.commitEdges([...filteredEdges, nextEdge])
    options.pushHistory(before)
    options.markDirty()
    options.refreshSnapshot()
    options.markFlowExecutionNodeDirty(connection.target, { includeSelf: true })
  }

  function onEdgeUpdate(event: EdgeUpdateEvent) {
    const { edge, connection } = event
    if (!options.isValidConnection(connection)) {
      options.onWarning('该连线不符合当前连接语义')
      return
    }
    if (!connection.source || !connection.target) return
    const before = options.captureHistory()
    const tone = resolveFlowEdgeToneFromHandles(connection.sourceHandle, connection.targetHandle)
    const nextEdges = (options.edges.value as unknown as Edge[])
      .filter(item => (
        item.id === edge.id
        || !(
          item.target === connection.target
          && (item.targetHandle ?? null) === (connection.targetHandle ?? null)
        )
      ))
      .map((item) => {
        if (item.id !== edge.id) return item
        const nextData = { ...((item.data ?? {}) as Record<string, unknown>), tone }
        return {
          ...item,
          source: connection.source!,
          target: connection.target!,
          sourceHandle: connection.sourceHandle ?? null,
          targetHandle: connection.targetHandle ?? null,
          class: `tone-${tone}`,
          data: nextData,
        } as Edge
      })
    options.commitEdges(nextEdges)
    options.pushHistory(before)
    options.markDirty()
    options.refreshSnapshot()
    options.markFlowExecutionNodeDirty(connection.target, { includeSelf: true })
  }

  return {
    onConnect,
    onEdgeUpdate,
  }
}
