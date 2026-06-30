import { shallowRef } from 'vue'
import { measureCanvasPerf } from '../utils/canvasPerfProbe'

type FlowHistoryItemIndex = {
  itemsById: Map<string, FlowHistoryIndexedItem>
  order: string[]
}

type FlowHistoryIndexedItem = {
  value: unknown
  json: string
}

declare const flowHistorySnapshotBrand: unique symbol

export type FlowHistorySnapshot = {
  readonly [flowHistorySnapshotBrand]: 'FlowHistorySnapshot'
}

type FlowHistorySnapshotState = {
  nodeIndex: FlowHistoryItemIndex
  edgeIndex: FlowHistoryItemIndex
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

const snapshotStates = new WeakMap<FlowHistorySnapshot, FlowHistorySnapshotState>()

function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function stringifyJson(value: unknown): string {
  return JSON.stringify(value) ?? 'null'
}

function parseJson<T = unknown>(json: string): T {
  return JSON.parse(json) as T
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object'
}

function getHistoryItemId(item: unknown): string | null {
  if (!isRecord(item) || typeof item.id !== 'string') return null
  return item.id
}

function createHistoryItemIndex(items: unknown[]): FlowHistoryItemIndex {
  const itemsById = new Map<string, FlowHistoryIndexedItem>()
  const order: string[] = []
  for (const item of items) {
    const id = getHistoryItemId(item)
    if (!id) continue
    itemsById.set(id, {
      value: item,
      json: stringifyJson(item),
    })
    order.push(id)
  }
  return { itemsById, order }
}

function createFlowHistorySnapshot(nodes: unknown[], edges: unknown[]): FlowHistorySnapshot {
  const snapshot = Object.freeze({}) as FlowHistorySnapshot
  snapshotStates.set(snapshot, {
    nodeIndex: createHistoryItemIndex(nodes),
    edgeIndex: createHistoryItemIndex(edges),
  })
  return snapshot
}

function readFlowHistorySnapshotState(snapshot: FlowHistorySnapshot): FlowHistorySnapshotState {
  const state = snapshotStates.get(snapshot)
  if (!state) {
    throw new Error('Invalid flow history snapshot')
  }
  return state
}

function isSameOrder(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  return a.every((id, index) => id === b[index])
}

function createChangedItemPatch(
  id: string,
  beforeItem: FlowHistoryIndexedItem,
  afterItem: FlowHistoryIndexedItem,
): FlowHistoryPatch | null {
  const beforeJson = beforeItem.json
  const afterJson = afterItem.json
  if (beforeJson === afterJson) return null
  return {
    id,
    before: parseJson(beforeJson),
    after: parseJson(afterJson),
  }
}

function createCollectionPatches(
  beforeIndex: FlowHistoryItemIndex,
  afterIndex: FlowHistoryItemIndex,
): FlowHistoryPatch[] {
  const patches: FlowHistoryPatch[] = []
  for (const [id, beforeItem] of beforeIndex.itemsById) {
    if (!afterIndex.itemsById.has(id)) {
      patches.push({ id, before: parseJson(beforeItem.json) })
      continue
    }
    const afterItem = afterIndex.itemsById.get(id)
    if (!afterItem) continue
    const patch = createChangedItemPatch(id, beforeItem, afterItem)
    if (patch) patches.push(patch)
  }
  for (const [id, afterItem] of afterIndex.itemsById) {
    if (beforeIndex.itemsById.has(id)) continue
    patches.push({ id, after: parseJson(afterItem.json) })
  }
  return patches
}

export type FlowHistory = {
  capture: () => FlowHistorySnapshot
  push: (before: FlowHistorySnapshot, after?: FlowHistorySnapshot) => boolean
  undo: () => void
  redo: () => void
  clearBaseline: () => void
  onNodeDragStart: () => void
  onNodeDragStop: (onChanged?: () => void) => void
}

export function useFlowHistory(options: UseFlowHistoryOptions): FlowHistory {
  const maxSteps = Math.max(1, options.maxSteps ?? 10)
  const flowHistoryPast = shallowRef<FlowHistoryEntry[]>([])
  const flowHistoryFuture = shallowRef<FlowHistoryEntry[]>([])
  const dragBeforeSnapshot = shallowRef<FlowHistorySnapshot | null>(null)

  function capture(): FlowHistorySnapshot {
    return measureCanvasPerf(
      'history.capture',
      () => ({
        nodes: options.nodes.value.length,
        edges: options.edges.value.length,
      }),
      () => {
        const nodes = options.nodes.value as unknown[]
        const edges = options.edges.value as unknown[]
        return createFlowHistorySnapshot(nodes, edges)
      },
    )
  }

  function createHistoryPatches(
    before: FlowHistorySnapshot,
    after: FlowHistorySnapshot,
  ): FlowHistoryEntry | null {
    const beforeState = readFlowHistorySnapshotState(before)
    const afterState = readFlowHistorySnapshotState(after)
    const beforeNodeIndex = beforeState.nodeIndex
    const afterNodeIndex = afterState.nodeIndex
    const beforeEdgeIndex = beforeState.edgeIndex
    const afterEdgeIndex = afterState.edgeIndex
    const nodePatches = createCollectionPatches(beforeNodeIndex, afterNodeIndex)
    const edgePatches = createCollectionPatches(beforeEdgeIndex, afterEdgeIndex)
    const hasOrderChange =
      !isSameOrder(beforeNodeIndex.order, afterNodeIndex.order) ||
      !isSameOrder(beforeEdgeIndex.order, afterEdgeIndex.order)

    if (nodePatches.length === 0 && edgePatches.length === 0 && !hasOrderChange) return null
    return {
      nodePatches,
      edgePatches,
      beforeNodeOrder: beforeNodeIndex.order,
      afterNodeOrder: afterNodeIndex.order,
      beforeEdgeOrder: beforeEdgeIndex.order,
      afterEdgeOrder: afterEdgeIndex.order,
    }
  }

  function pushBoundedStack<T>(current: T[], value: T): T[] {
    const next = current.slice()
    next.push(value)
    return next.slice(-maxSteps)
  }

  function clearBaseline() {
    flowHistoryPast.value = []
    flowHistoryFuture.value = []
    dragBeforeSnapshot.value = null
  }

  function push(before: FlowHistorySnapshot, after = capture()): boolean {
    return measureCanvasPerf(
      'history.push',
      () => ({
        beforeNodes: readFlowHistorySnapshotState(before).nodeIndex.order.length,
        afterNodes: readFlowHistorySnapshotState(after).nodeIndex.order.length,
        beforeEdges: readFlowHistorySnapshotState(before).edgeIndex.order.length,
        afterEdges: readFlowHistorySnapshotState(after).edgeIndex.order.length,
      }),
      () => {
        const entry = createHistoryPatches(before, after)
        if (!entry) return false
        flowHistoryPast.value = pushBoundedStack(flowHistoryPast.value, entry)
        flowHistoryFuture.value = []
        return true
      },
    )
  }

  function applyCollectionPatches(
    currentItems: unknown[],
    patches: FlowHistoryPatch[],
    order: string[],
    direction: 'forward' | 'backward',
  ): unknown[] {
    const itemsById = createHistoryItemIndex(currentItems).itemsById
    for (const patch of patches) {
      const nextValue = direction === 'forward' ? patch.after : patch.before
      if (nextValue === undefined) {
        itemsById.delete(patch.id)
        continue
      }
      itemsById.set(patch.id, {
        value: cloneJson(nextValue),
        json: stringifyJson(nextValue),
      })
    }
    const orderedItems: unknown[] = []
    for (const id of order) {
      const item = itemsById.get(id)
      if (item !== undefined) orderedItems.push(item.value)
      itemsById.delete(id)
    }
    orderedItems.push(...Array.from(itemsById.values(), (item) => item.value))
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
