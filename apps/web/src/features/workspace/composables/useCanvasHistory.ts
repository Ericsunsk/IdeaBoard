import { ref } from 'vue'
import type { CanvasBoardSnapshot, ImageNodeOutputItem } from '../types/canvasNodes'

interface UseCanvasHistoryOptions {
  createSnapshot: () => CanvasBoardSnapshot
  applySnapshot: (snapshot: CanvasBoardSnapshot) => Promise<void> | void
  maxEntries?: number
  debounceMs?: number
}

interface HistoryEntry {
  snapshot: CanvasBoardSnapshot
  serialized: string
}

function cloneOutputImages(outputImages: ImageNodeOutputItem[]): ImageNodeOutputItem[] {
  return outputImages.map(output => ({ ...output }))
}

function cloneBoardSnapshot(snapshot: CanvasBoardSnapshot): CanvasBoardSnapshot {
  return {
    messages: snapshot.messages.map(item => ({ ...item })),
    imageNodes: snapshot.imageNodes.map(node => ({
      ...node,
      outputImages: cloneOutputImages(node.outputImages),
    })),
    promptNodes: snapshot.promptNodes.map(node => ({ ...node })),
    modelNodes: snapshot.modelNodes.map(node => ({
      ...node,
      angleSpec: { ...node.angleSpec },
      outputImages: cloneOutputImages(node.outputImages),
    })),
    viewpointNodes: snapshot.viewpointNodes.map(node => ({
      ...node,
      outputImages: cloneOutputImages(node.outputImages),
    })),
    maskNodes: snapshot.maskNodes.map(node => ({ ...node })),
    colorNodes: snapshot.colorNodes.map(node => ({
      ...node,
      colors: [...node.colors],
      outputImages: cloneOutputImages(node.outputImages),
    })),
    editNodes: snapshot.editNodes.map(node => ({
      ...node,
      outputImages: cloneOutputImages(node.outputImages),
    })),
    fusionNodes: snapshot.fusionNodes.map(node => ({
      ...node,
      outputImages: cloneOutputImages(node.outputImages),
    })),
    enhanceNodes: snapshot.enhanceNodes.map(node => ({
      ...node,
      outputImages: cloneOutputImages(node.outputImages),
    })),
    wearNodes: snapshot.wearNodes.map(node => ({
      ...node,
      outputImages: cloneOutputImages(node.outputImages),
    })),
    connections: snapshot.connections.map(connection => ({ ...connection })),
    viewport: { ...snapshot.viewport },
  }
}

function serializeSnapshot(snapshot: CanvasBoardSnapshot): string {
  const { viewport: _viewport, ...snapshotWithoutViewport } = snapshot
  return JSON.stringify(snapshotWithoutViewport)
}

function snapshotHasImageUrlReference(snapshot: CanvasBoardSnapshot, url: string): boolean {
  return (
    snapshot.messages.some(item => item.url === url) ||
    snapshot.imageNodes.some(node => node.sourceImageUrl === url) ||
    snapshot.maskNodes.some(node => node.sourceImageUrl === url)
  )
}

interface HasImageUrlReferenceOptions {
  recentUndoSteps?: number
  includeRedo?: boolean
  includeCurrent?: boolean
}

const MASK_DATA_URL_TOKEN_PREFIX = '__mask_data_ref__:'
const MASK_DATA_URL_MIN_LENGTH = 256

function isLargeImageDataUrl(value: string): boolean {
  return value.startsWith('data:image/') && value.length > MASK_DATA_URL_MIN_LENGTH
}

export function useCanvasHistory(options: UseCanvasHistoryOptions) {
  const maxEntries = options.maxEntries ?? 50
  const debounceMs = options.debounceMs ?? 120

  const canUndo = ref(false)
  const canRedo = ref(false)
  const undoDepth = ref(0)
  const redoDepth = ref(0)
  const historyVersion = ref(0)

  const undoStack: HistoryEntry[] = []
  const redoStack: HistoryEntry[] = []
  let lastCommittedEntry: HistoryEntry | null = null
  let pendingTimer: number | null = null
  let isRestoring = false
  let isBatching = false
  let hasPendingBatchChange = false
  let nextMaskDataUrlTokenId = 1
  const maskDataUrlValueToToken = new Map<string, string>()
  const maskDataUrlTokenToValue = new Map<string, string>()

  function tokenizeMaskDataUrl(value: string): string {
    if (!isLargeImageDataUrl(value)) return value
    const existingToken = maskDataUrlValueToToken.get(value)
    if (existingToken) return existingToken
    const token = `${MASK_DATA_URL_TOKEN_PREFIX}${nextMaskDataUrlTokenId++}`
    maskDataUrlValueToToken.set(value, token)
    maskDataUrlTokenToValue.set(token, value)
    return token
  }

  function detokenizeMaskDataUrl(value: string): string {
    if (!value.startsWith(MASK_DATA_URL_TOKEN_PREFIX)) return value
    return maskDataUrlTokenToValue.get(value) ?? value
  }

  function normalizeSnapshotForStorage(snapshot: CanvasBoardSnapshot): CanvasBoardSnapshot {
    const nextSnapshot = cloneBoardSnapshot(snapshot)
    nextSnapshot.maskNodes = nextSnapshot.maskNodes.map(node => ({
      ...node,
      maskDataUrl: tokenizeMaskDataUrl(node.maskDataUrl || ''),
      executionMaskDataUrl: tokenizeMaskDataUrl(node.executionMaskDataUrl || ''),
    }))
    return nextSnapshot
  }

  function restoreSnapshotForApply(snapshot: CanvasBoardSnapshot): CanvasBoardSnapshot {
    const nextSnapshot = cloneBoardSnapshot(snapshot)
    nextSnapshot.maskNodes = nextSnapshot.maskNodes.map(node => ({
      ...node,
      maskDataUrl: detokenizeMaskDataUrl(node.maskDataUrl || ''),
      executionMaskDataUrl: detokenizeMaskDataUrl(node.executionMaskDataUrl || ''),
    }))
    nextSnapshot.viewport = { ...options.createSnapshot().viewport }
    return nextSnapshot
  }

  // Keep token pool bounded to snapshots still reachable from history state.
  function cleanupMaskDataUrlTokens() {
    const referencedTokens = new Set<string>()
    const collectSnapshotTokens = (snapshot: CanvasBoardSnapshot | null) => {
      if (!snapshot) return
      snapshot.maskNodes.forEach((node) => {
        const maskDataUrl = node.maskDataUrl || ''
        const executionMaskDataUrl = node.executionMaskDataUrl || ''
        if (maskDataUrl.startsWith(MASK_DATA_URL_TOKEN_PREFIX)) {
          referencedTokens.add(maskDataUrl)
        }
        if (executionMaskDataUrl.startsWith(MASK_DATA_URL_TOKEN_PREFIX)) {
          referencedTokens.add(executionMaskDataUrl)
        }
      })
    }

    collectSnapshotTokens(lastCommittedEntry?.snapshot ?? null)
    undoStack.forEach((entry) => collectSnapshotTokens(entry.snapshot))
    redoStack.forEach((entry) => collectSnapshotTokens(entry.snapshot))

    Array.from(maskDataUrlTokenToValue.keys()).forEach((token) => {
      if (referencedTokens.has(token)) return
      const originalValue = maskDataUrlTokenToValue.get(token)
      if (originalValue) {
        maskDataUrlValueToToken.delete(originalValue)
      }
      maskDataUrlTokenToValue.delete(token)
    })
  }

  function syncUndoState() {
    undoDepth.value = undoStack.length
    canUndo.value = undoStack.length > 0
    redoDepth.value = redoStack.length
    canRedo.value = redoStack.length > 0
  }

  function bumpHistoryVersion() {
    historyVersion.value += 1
  }

  function clearPendingTimer() {
    if (pendingTimer === null) return
    window.clearTimeout(pendingTimer)
    pendingTimer = null
  }

  function createHistoryEntry(snapshot: CanvasBoardSnapshot): HistoryEntry {
    const normalizedSnapshot = normalizeSnapshotForStorage(snapshot)
    return {
      snapshot: normalizedSnapshot,
      serialized: serializeSnapshot(normalizedSnapshot),
    }
  }

  function resetHistoryBaseline(snapshot?: CanvasBoardSnapshot | null) {
    clearPendingTimer()
    isBatching = false
    hasPendingBatchChange = false
    if (!isRestoring) {
      undoStack.length = 0
      redoStack.length = 0
      syncUndoState()
    }

    lastCommittedEntry = createHistoryEntry(snapshot ?? options.createSnapshot())
    cleanupMaskDataUrlTokens()
    bumpHistoryVersion()
  }

  function commitCurrentSnapshot() {
    if (isRestoring || !lastCommittedEntry) return
    const currentEntry = createHistoryEntry(options.createSnapshot())
    if (currentEntry.serialized === lastCommittedEntry.serialized) return

    undoStack.push(lastCommittedEntry)
    redoStack.length = 0
    if (undoStack.length > maxEntries) {
      undoStack.splice(0, undoStack.length - maxEntries)
    }
    syncUndoState()

    lastCommittedEntry = currentEntry
    cleanupMaskDataUrlTokens()
    bumpHistoryVersion()
  }

  function notifySnapshotChanged() {
    if (isRestoring || !lastCommittedEntry) return
    if (isBatching) {
      hasPendingBatchChange = true
      return
    }
    clearPendingTimer()
    pendingTimer = window.setTimeout(() => {
      pendingTimer = null
      commitCurrentSnapshot()
    }, debounceMs)
  }

  function beginBatch() {
    clearPendingTimer()
    isBatching = true
  }

  function endBatch(commit = true) {
    clearPendingTimer()
    if (!isBatching) {
      if (!commit) {
        hasPendingBatchChange = false
      }
      return
    }

    isBatching = false
    if (!commit) {
      hasPendingBatchChange = false
      return
    }

    hasPendingBatchChange = false
    commitCurrentSnapshot()
  }

  async function undo() {
    clearPendingTimer()
    isBatching = false
    hasPendingBatchChange = false
    commitCurrentSnapshot()

    const previousEntry = undoStack.pop()
    if (!previousEntry) {
      syncUndoState()
      cleanupMaskDataUrlTokens()
      return false
    }

    if (lastCommittedEntry) {
      redoStack.push(lastCommittedEntry)
    }
    syncUndoState()
    isRestoring = true
    try {
      const restoreTarget = restoreSnapshotForApply(previousEntry.snapshot)
      await options.applySnapshot(restoreTarget)
      lastCommittedEntry = previousEntry
      bumpHistoryVersion()
      return true
    } finally {
      isRestoring = false
      cleanupMaskDataUrlTokens()
    }
  }

  async function redo() {
    clearPendingTimer()
    isBatching = false
    hasPendingBatchChange = false

    const nextEntry = redoStack.pop()
    if (!nextEntry || !lastCommittedEntry) {
      syncUndoState()
      cleanupMaskDataUrlTokens()
      return false
    }

    undoStack.push(lastCommittedEntry)
    if (undoStack.length > maxEntries) {
      undoStack.splice(0, undoStack.length - maxEntries)
    }
    syncUndoState()

    isRestoring = true
    try {
      const restoreTarget = restoreSnapshotForApply(nextEntry.snapshot)
      await options.applySnapshot(restoreTarget)
      lastCommittedEntry = nextEntry
      bumpHistoryVersion()
      return true
    } finally {
      isRestoring = false
      cleanupMaskDataUrlTokens()
    }
  }

  function hasImageUrlReference(url: string, queryOptions?: HasImageUrlReferenceOptions): boolean {
    const targetURL = url.trim()
    if (!targetURL) return false
    const recentUndoSteps = queryOptions?.recentUndoSteps
    const includeRedo = queryOptions?.includeRedo ?? true
    const includeCurrent = queryOptions?.includeCurrent ?? true
    const normalizedRecentUndoSteps = typeof recentUndoSteps === 'number' && Number.isFinite(recentUndoSteps)
      ? Math.max(0, Math.floor(recentUndoSteps))
      : null
    const undoCandidates = normalizedRecentUndoSteps !== null
      ? undoStack.slice(Math.max(0, undoStack.length - normalizedRecentUndoSteps))
      : undoStack

    if (includeCurrent && lastCommittedEntry && snapshotHasImageUrlReference(lastCommittedEntry.snapshot, targetURL)) {
      return true
    }
    if (undoCandidates.some(entry => snapshotHasImageUrlReference(entry.snapshot, targetURL))) {
      return true
    }
    if (!includeRedo) return false
    return redoStack.some(entry => snapshotHasImageUrlReference(entry.snapshot, targetURL))
  }

  return {
    canUndo,
    canRedo,
    undoDepth,
    redoDepth,
    historyVersion,
    notifySnapshotChanged,
    beginBatch,
    endBatch,
    resetHistoryBaseline,
    undo,
    redo,
    hasImageUrlReference,
  }
}
