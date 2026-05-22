import { ref } from 'vue'

type FlowHistorySnapshot = {
  nodes: unknown[]
  edges: unknown[]
}

type FlowHistoryPatch = {
  id: string
  before?: unknown
  after?: unknown
}

type FlowHistoryEntry = {
  nodePatches: FlowHistoryPatch[]
  edgePatches: FlowHistoryPatch[]
  beforeNodeOrder: string[]
  afterNodeOrder: string[]
  beforeEdgeOrder: string[]
  afterEdgeOrder: string[]
}

type ArrayRef<T> = { value: T[] }

type UseFlowHistoryOptions = {
  nodes: ArrayRef<unknown>
  edges: ArrayRef<unknown>
  maxSteps?: number
  onRestore?: () => void
  commitNodes: (nodes: unknown[]) => void
  commitEdges: (edges: unknown[]) => void
}

function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object'
}

function getHistoryItemId(item: unknown): string | null {
  if (!isRecord(item) || typeof item.id !== 'string') return null
  return item.id
}

function createHistoryItemMap(items: unknown[]): Map<string, unknown> {
  const result = new Map<string, unknown>()
  for (const item of items) {
    const id = getHistoryItemId(item)
    if (id) result.set(id, item)
  }
  return result
}

function createHistoryItemOrder(items: unknown[]): string[] {
  return items.map((item) => getHistoryItemId(item)).filter((id): id is string => !!id)
}

function isSameJson(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

function isSameOrder(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  return a.every((id, index) => id === b[index])
}

export function useFlowHistory(options: UseFlowHistoryOptions) {
  const maxSteps = Math.max(1, options.maxSteps ?? 10)
  const flowHistoryPast = ref<FlowHistoryEntry[]>([])
  const flowHistoryFuture = ref<FlowHistoryEntry[]>([])
  const dragBeforeSnapshot = ref<FlowHistorySnapshot | null>(null)

  function capture(): FlowHistorySnapshot {
    const serialized = JSON.stringify({
      nodes: options.nodes.value as unknown[],
      edges: options.edges.value as unknown[],
    })
    const snapshot = JSON.parse(serialized) as FlowHistorySnapshot
    return {
      nodes: snapshot.nodes,
      edges: snapshot.edges,
    }
  }

  function pushBoundedStack<T>(current: T[], value: T): T[] {
    const next = current.slice()
    next.push(value)
    return next.slice(-maxSteps)
  }

  function createHistoryPatches(
    before: FlowHistorySnapshot,
    after: FlowHistorySnapshot,
  ): FlowHistoryEntry | null {
    const nodePatches: FlowHistoryPatch[] = []
    const edgePatches: FlowHistoryPatch[] = []
    const beforeNodesById = createHistoryItemMap(before.nodes)
    const afterNodesById = createHistoryItemMap(after.nodes)
    const beforeEdgesById = createHistoryItemMap(before.edges)
    const afterEdgesById = createHistoryItemMap(after.edges)
    const beforeNodeOrder = createHistoryItemOrder(before.nodes)
    const afterNodeOrder = createHistoryItemOrder(after.nodes)
    const beforeEdgeOrder = createHistoryItemOrder(before.edges)
    const afterEdgeOrder = createHistoryItemOrder(after.edges)

    for (const id of new Set([...beforeNodesById.keys(), ...afterNodesById.keys()])) {
      const beforeNode = beforeNodesById.get(id)
      const afterNode = afterNodesById.get(id)
      if (isSameJson(beforeNode, afterNode)) continue
      nodePatches.push({
        id,
        before: beforeNode === undefined ? undefined : cloneJson(beforeNode),
        after: afterNode === undefined ? undefined : cloneJson(afterNode),
      })
    }
    for (const id of new Set([...beforeEdgesById.keys(), ...afterEdgesById.keys()])) {
      const beforeEdge = beforeEdgesById.get(id)
      const afterEdge = afterEdgesById.get(id)
      if (isSameJson(beforeEdge, afterEdge)) continue
      edgePatches.push({
        id,
        before: beforeEdge === undefined ? undefined : cloneJson(beforeEdge),
        after: afterEdge === undefined ? undefined : cloneJson(afterEdge),
      })
    }
    const hasOrderChange =
      !isSameOrder(beforeNodeOrder, afterNodeOrder) || !isSameOrder(beforeEdgeOrder, afterEdgeOrder)
    if (nodePatches.length === 0 && edgePatches.length === 0 && !hasOrderChange) return null
    return {
      nodePatches,
      edgePatches,
      beforeNodeOrder,
      afterNodeOrder,
      beforeEdgeOrder,
      afterEdgeOrder,
    }
  }

  function clearBaseline() {
    flowHistoryPast.value = []
    flowHistoryFuture.value = []
    dragBeforeSnapshot.value = null
  }

  function push(before: FlowHistorySnapshot, after = capture()): boolean {
    const entry = createHistoryPatches(before, after)
    if (!entry) return false
    flowHistoryPast.value = pushBoundedStack(flowHistoryPast.value, entry)
    flowHistoryFuture.value = []
    return true
  }

  function applyCollectionPatches(
    currentItems: unknown[],
    patches: FlowHistoryPatch[],
    order: string[],
    direction: 'forward' | 'backward',
  ): unknown[] {
    const itemsById = createHistoryItemMap(currentItems)
    for (const patch of patches) {
      const nextValue = direction === 'forward' ? patch.after : patch.before
      if (nextValue === undefined) {
        itemsById.delete(patch.id)
        continue
      }
      itemsById.set(patch.id, cloneJson(nextValue))
    }
    const orderedItems: unknown[] = []
    for (const id of order) {
      const item = itemsById.get(id)
      if (item !== undefined) orderedItems.push(item)
      itemsById.delete(id)
    }
    orderedItems.push(...itemsById.values())
    return orderedItems
  }

  function applyHistoryEntry(entry: FlowHistoryEntry, direction: 'forward' | 'backward') {
    const nextNodes = applyCollectionPatches(
      options.nodes.value as unknown[],
      entry.nodePatches,
      direction === 'forward' ? entry.afterNodeOrder : entry.beforeNodeOrder,
      direction,
    )
    const nextEdges = applyCollectionPatches(
      options.edges.value as unknown[],
      entry.edgePatches,
      direction === 'forward' ? entry.afterEdgeOrder : entry.beforeEdgeOrder,
      direction,
    )
    options.commitNodes(nextNodes)
    options.commitEdges(nextEdges)
    options.onRestore?.()
  }

  function undo() {
    if (flowHistoryPast.value.length === 0) return
    const entry = flowHistoryPast.value[flowHistoryPast.value.length - 1]
    flowHistoryPast.value = flowHistoryPast.value.slice(0, -1)
    flowHistoryFuture.value = pushBoundedStack(flowHistoryFuture.value, entry)
    applyHistoryEntry(entry, 'backward')
  }

  function redo() {
    if (flowHistoryFuture.value.length === 0) return
    const entry = flowHistoryFuture.value[flowHistoryFuture.value.length - 1]
    flowHistoryFuture.value = flowHistoryFuture.value.slice(0, -1)
    flowHistoryPast.value = pushBoundedStack(flowHistoryPast.value, entry)
    applyHistoryEntry(entry, 'forward')
  }

  function onNodeDragStart() {
    dragBeforeSnapshot.value = capture()
  }

  function onNodeDragStop(onChanged?: () => void) {
    if (!dragBeforeSnapshot.value) return
    const before = dragBeforeSnapshot.value
    const after = capture()
    dragBeforeSnapshot.value = null
    if (!push(before, after)) return
    onChanged?.()
  }

  return {
    capture,
    push,
    undo,
    redo,
    clearBaseline,
    onNodeDragStart,
    onNodeDragStop,
  }
}
