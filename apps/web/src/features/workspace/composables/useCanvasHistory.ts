import { ref } from 'vue'
import type { CanvasBoardSnapshot } from '../types/canvasNodes'

interface UseCanvasHistoryOptions {
  createSnapshot: () => CanvasBoardSnapshot
  applySnapshot: (snapshot: CanvasBoardSnapshot) => Promise<void> | void
  maxEntries?: number
  debounceMs?: number
}

function cloneBoardSnapshot(snapshot: CanvasBoardSnapshot): CanvasBoardSnapshot {
  return {
    messages: snapshot.messages.map(item => ({ ...item })),
    imageNodes: snapshot.imageNodes.map(node => ({
      ...node,
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    promptNodes: snapshot.promptNodes.map(node => ({ ...node })),
    modelNodes: snapshot.modelNodes.map(node => ({
      ...node,
      angleSpec: { ...node.angleSpec },
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    viewpointNodes: snapshot.viewpointNodes.map(node => ({
      ...node,
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    maskNodes: snapshot.maskNodes.map(node => ({ ...node })),
    colorNodes: snapshot.colorNodes.map(node => ({
      ...node,
      colors: [...node.colors],
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    editNodes: snapshot.editNodes.map(node => ({
      ...node,
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    fusionNodes: snapshot.fusionNodes.map(node => ({
      ...node,
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    enhanceNodes: snapshot.enhanceNodes.map(node => ({
      ...node,
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    wearNodes: snapshot.wearNodes.map(node => ({
      ...node,
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    connections: snapshot.connections.map(connection => ({ ...connection })),
    viewport: { ...snapshot.viewport },
  }
}

function serializeSnapshot(snapshot: CanvasBoardSnapshot): string {
  return JSON.stringify(snapshot)
}

export function useCanvasHistory(options: UseCanvasHistoryOptions) {
  const maxEntries = options.maxEntries ?? 50
  const debounceMs = options.debounceMs ?? 120

  const canUndo = ref(false)
  const canRedo = ref(false)
  const undoDepth = ref(0)
  const redoDepth = ref(0)

  const undoStack: CanvasBoardSnapshot[] = []
  const redoStack: CanvasBoardSnapshot[] = []
  let lastCommittedSnapshot: CanvasBoardSnapshot | null = null
  let lastCommittedSerialized = ''
  let pendingTimer: number | null = null
  let isRestoring = false
  let isBatching = false
  let hasPendingBatchChange = false

  function syncUndoState() {
    undoDepth.value = undoStack.length
    canUndo.value = undoStack.length > 0
    redoDepth.value = redoStack.length
    canRedo.value = redoStack.length > 0
  }

  function clearPendingTimer() {
    if (pendingTimer === null) return
    window.clearTimeout(pendingTimer)
    pendingTimer = null
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

    const baseline = cloneBoardSnapshot(snapshot ?? options.createSnapshot())
    lastCommittedSnapshot = baseline
    lastCommittedSerialized = serializeSnapshot(baseline)
  }

  function commitCurrentSnapshot() {
    if (isRestoring || !lastCommittedSnapshot) return
    const currentSnapshot = cloneBoardSnapshot(options.createSnapshot())
    const currentSerialized = serializeSnapshot(currentSnapshot)
    if (currentSerialized === lastCommittedSerialized) return

    undoStack.push(cloneBoardSnapshot(lastCommittedSnapshot))
    redoStack.length = 0
    if (undoStack.length > maxEntries) {
      undoStack.splice(0, undoStack.length - maxEntries)
    }
    syncUndoState()

    lastCommittedSnapshot = currentSnapshot
    lastCommittedSerialized = currentSerialized
  }

  function notifySnapshotChanged() {
    if (isRestoring || !lastCommittedSnapshot) return
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

    const shouldCommit = hasPendingBatchChange
    hasPendingBatchChange = false
    if (shouldCommit) {
      commitCurrentSnapshot()
      return
    }

    commitCurrentSnapshot()
  }

  async function undo() {
    clearPendingTimer()
    isBatching = false
    hasPendingBatchChange = false
    commitCurrentSnapshot()

    const previousSnapshot = undoStack.pop()
    if (!previousSnapshot) {
      syncUndoState()
      return false
    }

    if (lastCommittedSnapshot) {
      redoStack.push(cloneBoardSnapshot(lastCommittedSnapshot))
    }
    syncUndoState()
    isRestoring = true
    try {
      const restoreTarget = cloneBoardSnapshot(previousSnapshot)
      await options.applySnapshot(restoreTarget)
      lastCommittedSnapshot = restoreTarget
      lastCommittedSerialized = serializeSnapshot(restoreTarget)
      return true
    } finally {
      isRestoring = false
    }
  }

  async function redo() {
    clearPendingTimer()
    isBatching = false
    hasPendingBatchChange = false

    const nextSnapshot = redoStack.pop()
    if (!nextSnapshot || !lastCommittedSnapshot) {
      syncUndoState()
      return false
    }

    undoStack.push(cloneBoardSnapshot(lastCommittedSnapshot))
    if (undoStack.length > maxEntries) {
      undoStack.splice(0, undoStack.length - maxEntries)
    }
    syncUndoState()

    isRestoring = true
    try {
      const restoreTarget = cloneBoardSnapshot(nextSnapshot)
      await options.applySnapshot(restoreTarget)
      lastCommittedSnapshot = restoreTarget
      lastCommittedSerialized = serializeSnapshot(restoreTarget)
      return true
    } finally {
      isRestoring = false
    }
  }

  return {
    canUndo,
    canRedo,
    undoDepth,
    redoDepth,
    notifySnapshotChanged,
    beginBatch,
    endBatch,
    resetHistoryBaseline,
    undo,
    redo,
  }
}
