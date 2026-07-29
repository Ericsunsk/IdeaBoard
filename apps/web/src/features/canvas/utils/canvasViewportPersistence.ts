import {
  resolveCanvasViewportState,
  writeCanvasViewportState,
  type CanvasFlowViewportState,
  type CanvasViewportStorageIdentity,
} from './canvasViewportStorage'

export type CanvasViewportPersistenceViewport = CanvasFlowViewportState
export type CanvasViewportPersistenceIdentity = CanvasViewportStorageIdentity

export type CanvasViewportPersistence = {
  restore: () => Promise<CanvasViewportPersistenceViewport>
  scheduleDelayedPersist: (viewport?: CanvasViewportPersistenceViewport) => void
  persistViewportEnd: (viewport?: CanvasViewportPersistenceViewport) => void
  persistNow: (viewport?: CanvasViewportPersistenceViewport) => void
  flush: () => void
  dispose: () => void
}

export type CanvasViewportPersistenceOptions = {
  getIdentity: () => CanvasViewportPersistenceIdentity
  getViewport: () => CanvasViewportPersistenceViewport
  setViewport: (
    viewport: CanvasViewportPersistenceViewport,
    options?: { duration?: number },
  ) => Promise<unknown> | unknown
  delayMs?: number
}

type PendingViewportPersistence = {
  identity: CanvasViewportPersistenceIdentity
  identityKey: string
  viewport: CanvasViewportPersistenceViewport
  viewportKey: string
}

const DEFAULT_DELAY_MS = 180

function viewportKey(viewport: CanvasViewportPersistenceViewport) {
  return `${viewport.x}:${viewport.y}:${viewport.zoom}`
}

function identityKey(identity: CanvasViewportPersistenceIdentity) {
  return `${identity.userUid ?? ''}:${identity.boardId ?? ''}`
}

export function createCanvasViewportPersistence(
  options: CanvasViewportPersistenceOptions,
): CanvasViewportPersistence {
  const delayMs = options.delayMs ?? DEFAULT_DELAY_MS
  let lastCommittedViewportKey: string | null = null
  let lastCommittedIdentityKey: string | null = null
  let pendingDelayed: PendingViewportPersistence | null = null
  // Persistence is browser storage-backed; DOM timers return numeric handles.
  let delayedTimer: number | null = null
  let isDisposed = false

  function capturePending(viewport = options.getViewport()): PendingViewportPersistence {
    const identity = options.getIdentity()
    const capturedIdentity = {
      userUid: identity.userUid,
      boardId: identity.boardId,
    }
    return {
      identity: capturedIdentity,
      identityKey: identityKey(capturedIdentity),
      viewport: { ...viewport },
      viewportKey: viewportKey(viewport),
    }
  }

  function clearTimer() {
    if (delayedTimer === null) return
    window.clearTimeout(delayedTimer)
    delayedTimer = null
  }

  function commit(
    viewport = options.getViewport(),
    identity = options.getIdentity(),
    committedIdentityKey = identityKey(identity),
  ) {
    const key = viewportKey(viewport)
    if (key === lastCommittedViewportKey && committedIdentityKey === lastCommittedIdentityKey) {
      return
    }
    const stored = writeCanvasViewportState(identity, viewport)
    if (!stored) return
    lastCommittedViewportKey = viewportKey(stored)
    lastCommittedIdentityKey = committedIdentityKey
  }

  function consumePending() {
    const pending = pendingDelayed
    pendingDelayed = null
    return pending
  }

  async function restore() {
    const identity = options.getIdentity()
    const currentIdentityKey = identityKey(identity)
    const nextViewport = resolveCanvasViewportState(identity)
    lastCommittedViewportKey = viewportKey(nextViewport)
    lastCommittedIdentityKey = currentIdentityKey
    try {
      await options.setViewport(nextViewport, { duration: 0 })
      lastCommittedViewportKey = viewportKey(options.getViewport())
      lastCommittedIdentityKey = currentIdentityKey
    } catch (error) {
      console.warn('[canvas] restore viewport failed', error)
    }
    return nextViewport
  }

  function scheduleDelayedPersist(viewport = options.getViewport()) {
    if (isDisposed) return
    pendingDelayed = capturePending(viewport)
    clearTimer()
    delayedTimer = window.setTimeout(() => {
      delayedTimer = null
      const pending = consumePending()
      if (!pending) return
      commit(pending.viewport, pending.identity, pending.identityKey)
    }, delayMs)
  }

  function persistViewportEnd(viewport = options.getViewport()) {
    const nextIdentity = options.getIdentity()
    const nextIdentityKey = identityKey(nextIdentity)
    const nextViewportKey = viewportKey(viewport)
    const pending = pendingDelayed
    if (
      pending &&
      pending.viewportKey === nextViewportKey &&
      pending.identityKey === nextIdentityKey
    ) {
      return
    }
    if (pending) {
      clearTimer()
      pendingDelayed = null
    }
    commit(viewport, nextIdentity, nextIdentityKey)
  }

  function persistNow(viewport = options.getViewport()) {
    clearTimer()
    pendingDelayed = null
    commit(viewport)
  }

  function flush() {
    clearTimer()
    const pending = consumePending()
    if (pending) {
      commit(pending.viewport, pending.identity, pending.identityKey)
      return
    }
    commit()
  }

  function dispose() {
    if (isDisposed) return
    flush()
    isDisposed = true
  }

  return {
    restore,
    scheduleDelayedPersist,
    persistViewportEnd,
    persistNow,
    flush,
    dispose,
  }
}
