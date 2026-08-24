import type { Edge, Node } from '@vue-flow/core'
import type {
  CanvasGraphCommitPatch,
  CanvasGraphEdgeRecord,
  CanvasGraphNodeRecord,
} from '../graph-runtime'
import {
  canvasConnectionToFlowEdge,
  canvasNodeToFlowNode,
  type FlowAdapterEdge,
  type FlowAdapterNode,
} from '../utils/flowSnapshotAdapter'
import { recordCanvasInteractionCount } from '../utils/canvasInteractionMetrics'
import { measureCanvasPerf } from '../utils/canvasPerfProbe'

export type CanvasFlowGraphProjection = Readonly<{
  nodes: Node[]
  edges: Edge[]
}>

export type CanvasGraphCommitProjectionKind = 'incremental' | 'full'

export type CanvasGraphCommitProjection = Readonly<{
  kind: CanvasGraphCommitProjectionKind
  graph: CanvasFlowGraphProjection
  changedNodeIds: readonly string[]
  changedEdgeIds: readonly string[]
  invalidatedNodeIds: readonly string[]
  nodeMutations: Readonly<{
    added: readonly Node[]
    updated: readonly Node[]
    removedIds: readonly string[]
  }>
  edgeMutations: Readonly<{
    added: readonly Edge[]
    updated: readonly Edge[]
    removedIds: readonly string[]
  }>
}>

export type CanvasGraphCommitProjectionFailure = Readonly<{
  cause: unknown
  projection: CanvasGraphCommitProjection
}>

export type CanvasGraphCommitProjectionController = Readonly<{
  apply: (input: {
    current: CanvasFlowGraphProjection
    patch: CanvasGraphCommitPatch
    readNode?: (nodeId: string) => CanvasGraphNodeRecord | undefined
    projectNode?: (node: Node) => Node
  }) => CanvasGraphCommitProjection
  recover: () => CanvasGraphCommitProjection | null
  needsRecovery: () => boolean
}>

type OrderedRecordPatch<Record extends { id: string }> = Readonly<{
  records: readonly Readonly<{ id: string; before?: Record; after?: Record }>[]
  order?: readonly Readonly<{
    id: string
    after?: Readonly<{ previousId?: string; nextId?: string }>
  }>[]
}>

function mapById<T extends { id: string }>(items: readonly T[]): Map<string, T> {
  return new Map(items.map((item) => [item.id, item]))
}

function hasOrderChange<Record extends { id: string }>(patch: OrderedRecordPatch<Record>): boolean {
  return (patch.order?.length ?? 0) > 0
}

function applyOrderPatches<T extends { id: string }>(
  items: readonly T[],
  patches: OrderedRecordPatch<T>['order'],
): T[] {
  if (!patches || patches.length === 0) return items as T[]
  const nextItems = [...items]
  for (const { id, after } of patches) {
    if (!after) continue
    const currentIndex = nextItems.findIndex((item) => item.id === id)
    if (currentIndex < 0) continue
    const [item] = nextItems.splice(currentIndex, 1)
    const nextIndex = after.nextId
      ? nextItems.findIndex((candidate) => candidate.id === after.nextId)
      : -1
    if (nextIndex >= 0) {
      nextItems.splice(nextIndex, 0, item)
      continue
    }
    const previousIndex = after.previousId
      ? nextItems.findIndex((candidate) => candidate.id === after.previousId)
      : -1
    if (previousIndex >= 0) {
      nextItems.splice(previousIndex + 1, 0, item)
      continue
    }
    nextItems.push(item)
  }
  return nextItems
}

function mergeProjectedNode(current: Node | undefined, record: CanvasGraphNodeRecord): Node {
  const projected = canvasNodeToFlowNode(record.kind, record.snapshot, record.selected)
  if (!current) return projected as Node
  const currentTransient = current as Node & {
    dimensions?: { width?: number; height?: number }
    selected?: boolean
  }
  const projectedTransient = projected as FlowAdapterNode & {
    dimensions?: { width?: number; height?: number }
    selected?: boolean
  }

  const currentSnapshot = (current.data as { snapshotNode?: unknown } | undefined)?.snapshotNode
  const projectedSnapshot = (projected.data as { snapshotNode?: unknown } | undefined)?.snapshotNode
  const sameSnapshotExceptPosition = (() => {
    if (!currentSnapshot || !projectedSnapshot) return false
    if (typeof currentSnapshot !== 'object' || typeof projectedSnapshot !== 'object') return false
    const { x: _currentX, y: _currentY, ...currentRest } = currentSnapshot as Record<string, unknown>
    const { x: _projectedX, y: _projectedY, ...projectedRest } = projectedSnapshot as Record<string, unknown>
    return JSON.stringify(currentRest) === JSON.stringify(projectedRest)
  })()
  if (
    sameSnapshotExceptPosition &&
    current.position.x === projected.position.x &&
    current.position.y === projected.position.y &&
    currentTransient.dimensions?.width === projectedTransient.dimensions?.width &&
    currentTransient.dimensions?.height === projectedTransient.dimensions?.height &&
    (currentTransient.selected ?? false) === (projectedTransient.selected ?? false)
  ) {
    // Native Vue Flow drag already applied the visual position. Reusing the
    // existing node/array avoids a second controlled-model rewrite at commit.
    return current
  }

  // Vue Flow owns measurement/drag internals. Preserve those transient fields
  // while replacing the canonical Canvas-derived fields for this ID only.
  return {
    ...current,
    ...projected,
    data: projected.data,
    position: projected.position,
    dimensions: projected.dimensions,
  } as Node
}

function mergeProjectedEdge(current: Edge | undefined, record: CanvasGraphEdgeRecord): Edge {
  const projected = canvasConnectionToFlowEdge(record.connection) as Edge
  if (!current) return (record.selected ? { ...projected, selected: true } : projected) as Edge

  // `animated` is execution runtime state and is deliberately not copied from
  // canonical graph records. The execution projection adds it after this step.
  const { animated: _animated, ...semanticCurrent } = current
  return {
    ...semanticCurrent,
    ...projected,
    data: projected.data,
    ...(record.selected ? { selected: true } : { selected: false }),
  } as Edge
}

function projectCollection<Record extends { id: string }, FlowItem extends { id: string }>(input: {
  current: readonly FlowItem[]
  patch: OrderedRecordPatch<Record>
  project: (current: FlowItem | undefined, record: Record) => FlowItem
}): { items: FlowItem[]; changedIds: string[]; needsRecovery: boolean } {
  if (input.patch.records.length === 0 && !hasOrderChange(input.patch)) {
    return { items: input.current as FlowItem[], changedIds: [], needsRecovery: false }
  }

  const currentById = mapById(input.current)
  const nextById = new Map(currentById)
  const changedIds: string[] = []
  let needsRecovery = false

  for (const recordPatch of input.patch.records) {
    const current = currentById.get(recordPatch.id)
    if (!recordPatch.after) {
      if (!current && recordPatch.before) needsRecovery = true
      if (current) {
        nextById.delete(recordPatch.id)
        changedIds.push(recordPatch.id)
      }
      continue
    }

    if (recordPatch.before && !current) needsRecovery = true
    if (!recordPatch.before && current) needsRecovery = true
    nextById.set(recordPatch.id, input.project(current, recordPatch.after))
    changedIds.push(recordPatch.id)
  }

  let nextItems = input.current.filter((item) => nextById.has(item.id)).map((item) => nextById.get(item.id)!)
  for (const recordPatch of input.patch.records) {
    if (!recordPatch.after || currentById.has(recordPatch.id)) continue
    nextItems.push(nextById.get(recordPatch.id)!)
  }
  nextItems = applyOrderPatches(nextItems, input.patch.order)
  return { items: nextItems, changedIds, needsRecovery }
}

function projectMutations<T extends { id: string }>(input: {
  items: readonly T[]
  patches: readonly Readonly<{ id: string; before?: unknown; after?: unknown }>[]
  extraUpdatedIds?: ReadonlySet<string>
}): { added: T[]; updated: T[]; removedIds: string[] } {
  const itemsById = mapById(input.items)
  const added: T[] = []
  const updated: T[] = []
  const removedIds: string[] = []
  const updatedIds = new Set(input.extraUpdatedIds)

  input.patches.forEach(({ id, before, after }) => {
    if (!after) {
      if (before) removedIds.push(id)
      return
    }
    const item = itemsById.get(id)
    if (!item) return
    if (!before) added.push(item)
    else updatedIds.add(id)
  })
  updatedIds.forEach((id) => {
    if (added.some((item) => item.id === id)) return
    const item = itemsById.get(id)
    if (item) updated.push(item)
  })
  return { added, updated, removedIds }
}

/**
 * Creates a structural-sharing Flow projection for one canonical graph commit.
 * It never mutates the graph store; an incomplete renderer projection is
 * explicitly marked for a later full recovery instead of corrupting facts.
 */
export function projectCanvasGraphCommit(input: {
  current: CanvasFlowGraphProjection
  patch: CanvasGraphCommitPatch
  /** Resolves node UI dependencies affected by an edge topology change. */
  readNode?: (nodeId: string) => CanvasGraphNodeRecord | undefined
  /** Applies presentation-only node state (for example group z-index). */
  projectNode?: (node: Node) => Node
}): CanvasGraphCommitProjection & { needsRecovery: boolean } {
  return measureCanvasPerf(
    'graph-projection.incremental',
    () => ({
      nodes: input.current.nodes.length,
      edges: input.current.edges.length,
      nodeRecords: input.patch.nodes.records.length,
      edgeRecords: input.patch.edges.records.length,
    }),
    () => {
      recordCanvasInteractionCount('incrementalGraphProjectionCount')
      const nodes = projectCollection({
        current: input.current.nodes,
        patch: input.patch.nodes,
        project: (current, record) => input.projectNode?.(mergeProjectedNode(current, record)) ?? mergeProjectedNode(current, record),
      })
      const edges = projectCollection({
        current: input.current.edges,
        patch: input.patch.edges,
        project: mergeProjectedEdge,
      })
      const invalidatedNodeIds = new Set(nodes.changedIds)
      input.patch.edges.records.forEach(({ before, after }) => {
        for (const record of [before, after]) {
          if (!record) continue
          invalidatedNodeIds.add(`${record.connection.fromNodeType}-${record.connection.fromNodeId}`)
          invalidatedNodeIds.add(`${record.connection.toNodeType}-${record.connection.toNodeId}`)
        }
      })
      input.patch.groups.records.forEach(({ before, after }) => {
        for (const record of [before, after]) {
          record?.nodeIds.forEach((nodeId) => invalidatedNodeIds.add(nodeId))
        }
      })

      let projectedNodes = nodes.items
      if (input.readNode) {
        const nodeIndexes = new Map(projectedNodes.map((node, index) => [node.id, index]))
        for (const nodeId of invalidatedNodeIds) {
          if (nodes.changedIds.includes(nodeId)) continue
          const index = nodeIndexes.get(nodeId)
          const record = input.readNode(nodeId)
          if (index === undefined || !record) continue
          if (projectedNodes === nodes.items) projectedNodes = projectedNodes.slice()
          const projected = mergeProjectedNode(projectedNodes[index], record)
          projectedNodes[index] = input.projectNode?.(projected) ?? projected
        }
      }
      const nodeMutations = projectMutations({
        items: projectedNodes,
        patches: input.patch.nodes.records,
        extraUpdatedIds: invalidatedNodeIds,
      })
      const edgeMutations = projectMutations({
        items: edges.items,
        patches: input.patch.edges.records,
      })
      return {
        kind: 'incremental' as const,
        graph: { nodes: projectedNodes, edges: edges.items },
        changedNodeIds: [...invalidatedNodeIds],
        changedEdgeIds: edges.changedIds,
        invalidatedNodeIds: [...invalidatedNodeIds],
        nodeMutations,
        edgeMutations,
        needsRecovery: nodes.needsRecovery || edges.needsRecovery,
      }
    },
  )
}

/**
 * Isolates renderer failure handling from canonical graph facts. Once an apply
 * fails, the next attempt is a caller-provided full projection from the live
 * graph session; successful recovery clears the flag.
 */
export function createCanvasGraphCommitProjectionController(options: {
  projectFull: () => CanvasFlowGraphProjection
  render: (projection: CanvasGraphCommitProjection) => void
  onFailure?: (failure: CanvasGraphCommitProjectionFailure) => void
  scheduleMicrotask?: (task: () => void) => void
}): CanvasGraphCommitProjectionController {
  let recoveryRequired = false
  let recoveryScheduled = false
  const scheduleMicrotask = options.scheduleMicrotask ?? queueMicrotask

  function scheduleRecovery() {
    if (recoveryScheduled) return
    recoveryScheduled = true
    scheduleMicrotask(() => {
      recoveryScheduled = false
      if (!recoveryRequired) return
      try {
        recover(true)
      } catch {
        // The graph has already committed. Keep the recovery flag so a later
        // commit or an explicit retry can safely re-project the live session.
      }
    })
  }

  function render(projection: CanvasGraphCommitProjection, scheduleOnFailure = true) {
    try {
      options.render(projection)
      recoveryRequired = false
    } catch (cause) {
      recoveryRequired = true
      options.onFailure?.({ cause, projection })
      if (scheduleOnFailure) scheduleRecovery()
      throw cause
    }
  }

  function recover(suppressRetrySchedule = false): CanvasGraphCommitProjection | null {
    if (!recoveryRequired) return null
    const graph = measureCanvasPerf('graph-projection.full-recovery', undefined, () => {
      recordCanvasInteractionCount('fullGraphProjectionCount')
      return options.projectFull()
    })
    const projection: CanvasGraphCommitProjection = {
      kind: 'full',
      graph,
      changedNodeIds: graph.nodes.map((node) => node.id),
      changedEdgeIds: graph.edges.map((edge) => edge.id),
      invalidatedNodeIds: graph.nodes.map((node) => node.id),
      nodeMutations: { added: graph.nodes, updated: [], removedIds: [] },
      edgeMutations: { added: graph.edges, updated: [], removedIds: [] },
    }
    render(projection, !suppressRetrySchedule)
    return projection
  }

  return {
    apply(input) {
      if (recoveryRequired) {
        const recovered = recover()
        if (recovered) return recovered
      }
      const projection = projectCanvasGraphCommit(input)
      if (projection.needsRecovery) {
        recoveryRequired = true
        const recovered = recover()
        if (recovered) return recovered
      }
      render(projection)
      return projection
    },
    recover,
    needsRecovery: () => recoveryRequired,
  }
}

export type { FlowAdapterEdge, FlowAdapterNode }
