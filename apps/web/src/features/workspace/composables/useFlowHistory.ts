import { ref } from 'vue'

type FlowHistorySnapshot = {
  nodes: unknown[]
  edges: unknown[]
}

type ArrayRef<T> = { value: T[] }

type UseFlowHistoryOptions = {
  nodes: ArrayRef<unknown>
  edges: ArrayRef<unknown>
  maxSteps?: number
  onRestore?: () => void
  commitNodes?: (nodes: unknown[]) => void
  commitEdges?: (edges: unknown[]) => void
}

function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

const HISTORY_BLOB_TOKEN_PREFIX = '__FLOW_HISTORY_BLOB__:'
const HISTORY_BLOB_DATA_KEYS = new Set(['maskDataUrl', 'executionMaskDataUrl'])
const HISTORY_BLOB_MIN_LENGTH = 2048

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object'
}

function cloneFlowHistorySnapshot(snapshot: FlowHistorySnapshot): FlowHistorySnapshot {
  return {
    nodes: cloneJson(snapshot.nodes) as unknown[],
    edges: cloneJson(snapshot.edges) as unknown[],
  }
}

function isSameFlowHistorySnapshot(a: FlowHistorySnapshot, b: FlowHistorySnapshot): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

export function useFlowHistory(options: UseFlowHistoryOptions) {
  const maxSteps = options.maxSteps ?? 10
  const flowHistoryPast = ref<FlowHistorySnapshot[]>([])
  const flowHistoryFuture = ref<FlowHistorySnapshot[]>([])
  const dragBeforeSnapshot = ref<FlowHistorySnapshot | null>(null)
  const blobStore = new Map<string, string>()
  const blobTokenByValue = new Map<string, string>()
  let nextBlobId = 1

  function encodeHistoryBlob(value: string): string {
    const existingToken = blobTokenByValue.get(value)
    if (existingToken) return existingToken
    const token = `${HISTORY_BLOB_TOKEN_PREFIX}${nextBlobId}`
    nextBlobId += 1
    blobStore.set(token, value)
    blobTokenByValue.set(value, token)
    return token
  }

  function decodeHistoryBlobsInPlace(value: unknown): void {
    if (Array.isArray(value)) {
      for (let i = 0; i < value.length; i += 1) {
        const item = value[i]
        if (typeof item === 'string' && item.startsWith(HISTORY_BLOB_TOKEN_PREFIX)) {
          value[i] = blobStore.get(item) ?? item
          continue
        }
        decodeHistoryBlobsInPlace(item)
      }
      return
    }
    if (!isRecord(value)) return
    for (const key of Object.keys(value)) {
      const field = value[key]
      if (typeof field === 'string' && field.startsWith(HISTORY_BLOB_TOKEN_PREFIX)) {
        value[key] = blobStore.get(field) ?? field
        continue
      }
      decodeHistoryBlobsInPlace(field)
    }
  }

  function collectHistoryBlobTokens(value: unknown, tokens: Set<string>): void {
    if (Array.isArray(value)) {
      value.forEach(item => collectHistoryBlobTokens(item, tokens))
      return
    }
    if (typeof value === 'string') {
      if (value.startsWith(HISTORY_BLOB_TOKEN_PREFIX)) tokens.add(value)
      return
    }
    if (!isRecord(value)) return
    Object.values(value).forEach(field => collectHistoryBlobTokens(field, tokens))
  }

  function pruneBlobStore() {
    const retainedTokens = new Set<string>()
    for (const snapshot of flowHistoryPast.value) collectHistoryBlobTokens(snapshot, retainedTokens)
    for (const snapshot of flowHistoryFuture.value) collectHistoryBlobTokens(snapshot, retainedTokens)
    if (dragBeforeSnapshot.value) collectHistoryBlobTokens(dragBeforeSnapshot.value, retainedTokens)
    for (const [token, value] of blobStore.entries()) {
      if (retainedTokens.has(token)) continue
      blobStore.delete(token)
      blobTokenByValue.delete(value)
    }
  }

  function cloneFlowNodesForHistory(nodes: unknown[]): unknown[] {
    const serialized = JSON.stringify(nodes, (key, value) => {
      if (
        typeof value === 'string'
        && HISTORY_BLOB_DATA_KEYS.has(key)
        && value.startsWith('data:')
        && value.length >= HISTORY_BLOB_MIN_LENGTH
      ) {
        return encodeHistoryBlob(value)
      }
      return value
    })
    return JSON.parse(serialized) as unknown[]
  }

  function capture(): FlowHistorySnapshot {
    return {
      nodes: cloneFlowNodesForHistory(options.nodes.value as unknown[]),
      edges: cloneJson(options.edges.value as unknown[]),
    }
  }

  function clearBaseline() {
    flowHistoryPast.value = []
    flowHistoryFuture.value = []
    dragBeforeSnapshot.value = null
    pruneBlobStore()
  }

  function push(before: FlowHistorySnapshot) {
    const current = capture()
    if (isSameFlowHistorySnapshot(before, current)) {
      pruneBlobStore()
      return
    }
    const nextPast = flowHistoryPast.value.slice()
    nextPast.push(cloneFlowHistorySnapshot(before))
    flowHistoryPast.value = nextPast.slice(-maxSteps)
    flowHistoryFuture.value = []
    pruneBlobStore()
  }

  function restore(snapshot: FlowHistorySnapshot) {
    const restored = cloneFlowHistorySnapshot(snapshot)
    decodeHistoryBlobsInPlace(restored.nodes)
    if (options.commitNodes) {
      options.commitNodes(restored.nodes)
    } else {
      options.nodes.value = restored.nodes
    }
    if (options.commitEdges) {
      options.commitEdges(restored.edges)
    } else {
      options.edges.value = restored.edges
    }
    options.onRestore?.()
  }

  function undo() {
    if (flowHistoryPast.value.length === 0) return
    const before = flowHistoryPast.value[flowHistoryPast.value.length - 1]
    const current = capture()
    flowHistoryPast.value = flowHistoryPast.value.slice(0, -1)
    const nextFuture = flowHistoryFuture.value.slice()
    nextFuture.push(current)
    flowHistoryFuture.value = nextFuture.slice(-maxSteps)
    restore(before)
    pruneBlobStore()
  }

  function redo() {
    if (flowHistoryFuture.value.length === 0) return
    const next = flowHistoryFuture.value[flowHistoryFuture.value.length - 1]
    const current = capture()
    flowHistoryFuture.value = flowHistoryFuture.value.slice(0, -1)
    const nextPast = flowHistoryPast.value.slice()
    nextPast.push(current)
    flowHistoryPast.value = nextPast.slice(-maxSteps)
    restore(next)
    pruneBlobStore()
  }

  function onNodeDragStart() {
    dragBeforeSnapshot.value = capture()
  }

  function onNodeDragStop(onChanged?: () => void) {
    if (!dragBeforeSnapshot.value) return
    const before = dragBeforeSnapshot.value
    dragBeforeSnapshot.value = null
    push(before)
    if (isSameFlowHistorySnapshot(before, capture())) return
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
    isSameSnapshot: isSameFlowHistorySnapshot,
  }
}
