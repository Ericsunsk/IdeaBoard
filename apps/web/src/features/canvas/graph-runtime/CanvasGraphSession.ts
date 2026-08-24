import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import { CanvasGraphStore } from './CanvasGraphStore'

/**
 * Owns the canonical graph for exactly one attached Canvas Board.  Flow is a
 * projection of this store; consumers must never reconstruct a store from
 * Flow arrays while a session is active.
 */
export class CanvasGraphSession {
  readonly boardId: string
  readonly #store: CanvasGraphStore
  #disposed = false

  private constructor(boardId: string, snapshot: CanvasBoardSnapshot) {
    const normalizedBoardId = boardId.trim()
    if (!normalizedBoardId) throw new Error('Canvas Graph Session boardId is required')
    this.boardId = normalizedBoardId
    this.#store = CanvasGraphStore.fromSnapshot(snapshot)
  }

  static hydrate(boardId: string, snapshot: CanvasBoardSnapshot): CanvasGraphSession {
    return new CanvasGraphSession(boardId, snapshot)
  }

  get store(): CanvasGraphStore {
    this.assertActive()
    return this.#store
  }

  replace(snapshot: CanvasBoardSnapshot): void {
    this.assertActive()
    this.#store.hydrate(snapshot)
  }

  exportSnapshot(base?: CanvasBoardSnapshot): CanvasBoardSnapshot {
    this.assertActive()
    return this.#store.exportSnapshot(base)
  }

  dispose(): void {
    this.#disposed = true
  }

  private assertActive(): void {
    if (this.#disposed) throw new Error('Canvas Graph Session is detached')
  }
}
