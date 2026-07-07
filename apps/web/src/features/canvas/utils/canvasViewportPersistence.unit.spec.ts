import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createCanvasViewportPersistence,
  type CanvasViewportPersistenceIdentity,
  type CanvasViewportPersistenceViewport,
} from './canvasViewportPersistence'

function readStoredViewport(boardId = 'board-a', userUid = 'user-uid') {
  const raw = localStorage.getItem(
    `ideaboard:canvas:view:${encodeURIComponent(userUid)}:${encodeURIComponent(boardId)}`,
  )
  return raw ? (JSON.parse(raw) as { x: number; y: number; zoom: number; updatedAt: number }) : null
}

function createPersistenceHarness() {
  const identity: CanvasViewportPersistenceIdentity = {
    userUid: 'user-uid',
    boardId: 'board-a',
  }
  let viewport: CanvasViewportPersistenceViewport = { x: 0, y: 0, zoom: 1 }
  const getIdentity = vi.fn(() => identity)
  const getViewport = vi.fn(() => viewport)
  const setViewport = vi.fn((nextViewport: CanvasViewportPersistenceViewport) => {
    viewport = nextViewport
  })
  const persistence = createCanvasViewportPersistence({
    getIdentity,
    getViewport,
    setViewport,
    delayMs: 20,
  })

  return {
    identity,
    get viewport() {
      return viewport
    },
    set viewport(nextViewport: CanvasViewportPersistenceViewport) {
      viewport = nextViewport
    },
    getIdentity,
    getViewport,
    setViewport,
    persistence,
  }
}

describe('createCanvasViewportPersistence', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('restores the default viewport when stored browser-local data is invalid', async () => {
    localStorage.setItem(
      'ideaboard:canvas:view:user-uid:board-a',
      JSON.stringify({ x: 'bad', y: -24, zoom: 1.75, updatedAt: 10 }),
    )
    const harness = createPersistenceHarness()

    await harness.persistence.restore()

    expect(harness.setViewport).toHaveBeenCalledWith({ x: 0, y: 0, zoom: 1 }, { duration: 0 })
    expect(harness.viewport).toEqual({ x: 0, y: 0, zoom: 1 })
  })

  it('scopes delayed writes to the user and board that produced the event', () => {
    vi.useFakeTimers()
    const harness = createPersistenceHarness()

    harness.viewport = { x: 12, y: 24, zoom: 1.25 }
    harness.persistence.scheduleDelayedPersist()
    harness.identity.boardId = 'board-b'
    harness.viewport = { x: 99, y: 88, zoom: 1 }

    vi.advanceTimersByTime(20)

    expect(readStoredViewport('board-a')).toMatchObject({ x: 12, y: 24, zoom: 1.25 })
    expect(readStoredViewport('board-b')).toBeNull()
  })

  it('dedupes equivalent writes and persists ordinary viewport ends immediately', () => {
    const harness = createPersistenceHarness()
    const setItemSpy = vi.spyOn(localStorage, 'setItem')

    harness.persistence.persistNow({ x: 4, y: 8, zoom: 1.5 })
    harness.persistence.persistNow({ x: 4, y: 8, zoom: 1.5 })

    expect(readStoredViewport()).toMatchObject({ x: 4, y: 8, zoom: 1.5 })
    expect(setItemSpy).toHaveBeenCalledTimes(1)
  })

  it('flushes pending delayed persistence and disposes without a later write', () => {
    vi.useFakeTimers()
    const harness = createPersistenceHarness()

    harness.viewport = { x: 10, y: 20, zoom: 1.25 }
    harness.persistence.scheduleDelayedPersist()
    harness.persistence.dispose()

    expect(readStoredViewport()).toMatchObject({ x: 10, y: 20, zoom: 1.25 })

    harness.viewport = { x: 30, y: 40, zoom: 1.5 }
    harness.persistence.scheduleDelayedPersist()
    vi.advanceTimersByTime(20)

    expect(readStoredViewport()).toMatchObject({ x: 10, y: 20, zoom: 1.25 })
  })

  it('keeps running when storage writes fail', () => {
    const harness = createPersistenceHarness()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const setItemSpy = vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded')
    })

    try {
      expect(() => harness.persistence.persistNow({ x: 1, y: 2, zoom: 1.25 })).not.toThrow()
      expect(readStoredViewport()).toBeNull()
    } finally {
      setItemSpy.mockRestore()
      warnSpy.mockRestore()
    }
  })
})
