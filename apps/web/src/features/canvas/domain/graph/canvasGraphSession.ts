import type { BoardSnapshot } from '../../../board'
import {
  CanvasGraphStore,
  type CanvasGraphCommitEvent,
  type CanvasGraphCommitPatch,
  type CanvasGraphPatchDirection,
  type CanvasGraphTransactionOptions,
  type CanvasGraphTransactionSummary,
} from './canvasGraphStore'
import {
  noopCanvasGraphStoreTelemetry,
  type CanvasGraphStoreTelemetry,
} from './canvasGraphStoreTelemetry'

/**
 * Owns the canonical graph for exactly one attached Canvas Board.  Flow is a
 * projection of this store; consumers must never reconstruct a store from
 * Flow arrays while a session is active.
 */
export class CanvasGraphSession {
  readonly boardId: string
  readonly #store: CanvasGraphStore
  readonly #unsubscribeCommitListeners = new Set<() => void>()
  #disposed = false

  private constructor(
    boardId: string,
    snapshot: BoardSnapshot,
    telemetry: CanvasGraphStoreTelemetry = noopCanvasGraphStoreTelemetry,
  ) {
    const normalizedBoardId = boardId.trim()
    if (!normalizedBoardId) throw new Error('Canvas Graph Session boardId is required')
    this.boardId = normalizedBoardId
    this.#store = CanvasGraphStore.fromSnapshot(snapshot, telemetry)
  }

  static hydrate(
    boardId: string,
    snapshot: BoardSnapshot,
    telemetry: CanvasGraphStoreTelemetry = noopCanvasGraphStoreTelemetry,
  ): CanvasGraphSession {
    return new CanvasGraphSession(boardId, snapshot, telemetry)
  }

  get store(): CanvasGraphStore {
    this.assertActive()
    return this.#store
  }

  replace(snapshot: BoardSnapshot): void {
    this.assertActive()
    this.#store.hydrate(snapshot)
  }

  exportSnapshot(base?: BoardSnapshot): BoardSnapshot {
    this.assertActive()
    return this.#store.exportSnapshot(base)
  }

  /** Subscribe to canonical graph commits for this Board attachment. */
  subscribe(listener: (event: CanvasGraphCommitEvent) => void): () => void {
    this.assertActive()
    const unsubscribeStore = this.#store.subscribe((event) => {
      if (this.#disposed) return
      listener(event)
    })
    let unsubscribed = false
    const unsubscribe = () => {
      if (unsubscribed) return
      unsubscribed = true
      this.#unsubscribeCommitListeners.delete(unsubscribe)
      unsubscribeStore()
    }
    this.#unsubscribeCommitListeners.add(unsubscribe)
    return unsubscribe
  }

  applyCommitPatch(
    patch: CanvasGraphCommitPatch,
    direction: CanvasGraphPatchDirection,
    options?: CanvasGraphTransactionOptions,
  ): CanvasGraphTransactionSummary {
    this.assertActive()
    return this.#store.applyCommitPatch(patch, direction, options)
  }

  dispose(): void {
    this.#unsubscribeCommitListeners.forEach((unsubscribe) => unsubscribe())
    this.#disposed = true
  }

  private assertActive(): void {
    if (this.#disposed) throw new Error('Canvas Graph Session is detached')
  }
}
