import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { clearStoredAuth } from '../../../../core/utils/auth'
import { primeAuthenticatedWorkspace, selectWorkspace } from '../../testing'
import {
  isBoardAccessLost,
  isBoardAccessStale,
  isConfirmedAccessLoss,
  markBoardAccessLost,
  revalidateBoardAccess,
} from './accessLoss'
import { observeOpenBoardAccessLoss, subscribeBoardAccessLost } from './convergeAccessLoss'

const WORKSPACE = 'ws_loss123'
const OTHER_WORKSPACE = 'ws_loss456'
const BOARD = 'board_loss1'

function codedError(code: string, status?: number): Error {
  const error = new Error(code) as Error & { code?: string; status?: number }
  error.code = code
  if (status !== undefined) error.status = status
  return error
}

function primeScope() {
  primeAuthenticatedWorkspace(WORKSPACE)
}

class MockEventSource {
  static instances: MockEventSource[] = []
  onerror: ((event: Event) => void) | null = null
  isClosed = false
  readonly url: string
  private listeners: Record<string, EventListener[]> = {}

  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    const fn: EventListener =
      typeof listener === 'function' ? listener : listener.handleEvent.bind(listener)
    this.listeners[type] = [...(this.listeners[type] ?? []), fn]
  }

  close() {
    this.isClosed = true
  }

  emit(type: string, data: unknown) {
    const event = new MessageEvent(type, {
      data: typeof data === 'string' ? data : JSON.stringify(data),
    })
    ;(this.listeners[type] ?? []).forEach((listener) => listener(event))
  }
}

function boardEnvelope(boardId: string, type: string): Record<string, unknown> {
  return {
    id: 'evt_loss123',
    type,
    version: 1,
    occurred_at: new Date().toISOString(),
    actor: { type: 'service', id: 'board-access' },
    resource: { type: 'board', id: boardId },
    workspace_id: WORKSPACE,
    data: { board_id: boardId, member_id: 'usr_someone', access_version: 7 },
  }
}

describe('access loss classifier', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
  })

  test('confirms revocation on board and workspace loss codes', () => {
    expect(isConfirmedAccessLoss(codedError('board_forbidden', 403))).toBe(true)
    expect(isConfirmedAccessLoss(codedError('board_not_found', 404))).toBe(true)
    expect(isConfirmedAccessLoss(codedError('workspace_not_found', 404))).toBe(true)
  })

  test('confirms bare revalidation 403/404 without a wire code', () => {
    expect(isConfirmedAccessLoss({ status: 403 })).toBe(true)
    expect(isConfirmedAccessLoss({ status: 404 })).toBe(true)
  })

  test('preserves network errors, 5xx, and unknown codes', () => {
    expect(isConfirmedAccessLoss(new Error('socket hang up'))).toBe(false)
    expect(isConfirmedAccessLoss({ status: 500 })).toBe(false)
    expect(isConfirmedAccessLoss(codedError('service_unavailable', 503))).toBe(false)
    expect(isConfirmedAccessLoss(codedError('board_access_conflict', 412))).toBe(false)
    expect(isConfirmedAccessLoss(codedError('board_revision_conflict', 409))).toBe(false)
    expect(isConfirmedAccessLoss(null)).toBe(false)
  })
})

describe('access loss registry', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
  })

  test('revalidation marks on revoked and clears on ok', async () => {
    primeScope()
    expect(isBoardAccessLost(BOARD)).toBe(false)
    const revoked = await revalidateBoardAccess(BOARD, {
      fetchMeta: (async () => {
        throw codedError('board_forbidden', 403)
      }) as never,
    })
    expect(revoked).toBe('revoked')
    expect(isBoardAccessLost(BOARD)).toBe(true)
    const ok = await revalidateBoardAccess(BOARD, {
      fetchMeta: (async () => ({ id: BOARD })) as never,
    })
    expect(ok).toBe('ok')
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })

  test('an old denied response cannot revoke fresh access after A→B→A', async () => {
    primeScope()
    let reject!: (error: Error) => void
    const pending = revalidateBoardAccess(BOARD, {
      fetchMeta: vi.fn(
        () =>
          new Promise<never>((_resolve, fail) => {
            reject = fail
          }),
      ),
    })
    selectWorkspace(OTHER_WORKSPACE)
    selectWorkspace(WORKSPACE)
    expect(
      await revalidateBoardAccess(BOARD, { fetchMeta: vi.fn().mockResolvedValue({ id: BOARD }) }),
    ).toBe('ok')
    reject(codedError('board_not_found', 404))
    expect(await pending).toBe('preserved')
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })

  test('an old successful response cannot clear a new loss after A→B→A', async () => {
    primeScope()
    let resolve!: (value: never) => void
    const pending = revalidateBoardAccess(BOARD, {
      fetchMeta: vi.fn(
        () =>
          new Promise<never>((done) => {
            resolve = done
          }),
      ),
    })
    selectWorkspace(OTHER_WORKSPACE)
    selectWorkspace(WORKSPACE)
    markBoardAccessLost(BOARD)
    resolve({ id: BOARD } as never)
    expect(await pending).toBe('preserved')
    expect(isBoardAccessLost(BOARD)).toBe(true)
  })

  test('workspace switches isolate and clear marks', () => {
    primeScope()
    markBoardAccessLost(BOARD)
    selectWorkspace(OTHER_WORKSPACE)
    expect(isBoardAccessLost(BOARD)).toBe(false)
    selectWorkspace(WORKSPACE)
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })

  test('sign-out clears marks so a new session starts clean', () => {
    primeScope()
    markBoardAccessLost(BOARD)
    clearStoredAuth()
    expect(isBoardAccessLost(BOARD)).toBe(false)
    primeScope()
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })

  test('success clears a previous mark', async () => {
    primeScope()
    markBoardAccessLost(BOARD)
    const verdict = await revalidateBoardAccess(BOARD, {
      fetchMeta: (async () => ({ id: BOARD })) as never,
    })
    expect(verdict).toBe('ok')
    expect(isBoardAccessLost(BOARD)).toBe(false)
    expect(isBoardAccessStale(BOARD)).toBe(false)
  })
})

describe('access revalidation', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
  })

  test('revoked verdicts mark the registry', async () => {
    primeScope()
    const verdict = await revalidateBoardAccess(BOARD, {
      fetchMeta: (async () => {
        throw codedError('board_not_found', 404)
      }) as never,
    })
    expect(verdict).toBe('revoked')
    expect(isBoardAccessLost(BOARD)).toBe(true)
  })

  test('ordinary failures preserve without marking', async () => {
    primeScope()
    for (const error of [
      new Error('socket hang up'),
      codedError('service_unavailable', 503),
      codedError('board_access_conflict', 412),
    ]) {
      const verdict = await revalidateBoardAccess(BOARD, {
        fetchMeta: (async () => {
          throw error
        }) as never,
      })
      expect(verdict).toBe('preserved')
    }
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })

  test('success clears a previous mark', async () => {
    primeScope()
    markBoardAccessLost(BOARD)
    const verdict = await revalidateBoardAccess(BOARD, {
      fetchMeta: (async () => ({ id: BOARD })) as never,
    })
    expect(verdict).toBe('ok')
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })

  test('does not revalidate or revoke a departed workspace', async () => {
    primeScope()
    const fetchMeta = vi.fn(async () => {
      throw codedError('board_forbidden', 403)
    })
    const verdict = await revalidateBoardAccess(BOARD, {
      workspaceId: OTHER_WORKSPACE,
      fetchMeta,
    })
    expect(verdict).toBe('preserved')
    expect(fetchMeta).not.toHaveBeenCalled()
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })

  test('drops successful reads after a scope switch', async () => {
    primeScope()
    let resolve!: () => void
    const gate = new Promise<void>((done) => {
      resolve = done
    })
    const pending = revalidateBoardAccess(BOARD, {
      fetchMeta: (async () => {
        await gate
        return { id: BOARD }
      }) as never,
    })
    selectWorkspace(OTHER_WORKSPACE)
    resolve()
    expect(await pending).toBe('preserved')
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })

  test('mid-flight scope switches drop the verdict instead of polluting', async () => {
    primeScope()
    let reject!: (error: unknown) => void
    const gate = new Promise<never>((_, rejectFetch) => {
      reject = rejectFetch
    })
    const pending = revalidateBoardAccess(BOARD, { fetchMeta: (() => gate) as never })
    selectWorkspace(OTHER_WORKSPACE)
    reject(codedError('board_forbidden', 403))
    expect(await pending).toBe('preserved')
    expect(isBoardAccessLost(BOARD)).toBe(false)
    selectWorkspace(WORKSPACE)
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })
})

describe('open board loss observation', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    MockEventSource.instances = []
    vi.stubGlobal('EventSource', MockEventSource)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  test('directed and roster signals revalidate only the open board', async () => {
    primeScope()
    const revalidate = vi.fn(async () => 'preserved' as const)
    const revoked: string[] = []
    const stopSubscription = subscribeBoardAccessLost((boardId) => revoked.push(boardId))
    const stop = observeOpenBoardAccessLoss({
      getBoardId: () => BOARD,
      revalidate,
    })
    try {
      const source = MockEventSource.instances[MockEventSource.instances.length - 1]
      expect(source).toBeDefined()
      if (!source) return
      // Rapid duplicate signals coalesce into one revalidation; a later
      // signal after settlement revalidates again. Frames for other
      // boards never revalidate.
      source.emit('board.access_changed', boardEnvelope(BOARD, 'board.access_changed'))
      source.emit('board.access_changed', boardEnvelope(BOARD, 'board.access_changed'))
      source.emit('board.access_changed', boardEnvelope('board_other9', 'board.access_changed'))
      await vi.waitFor(() => expect(revalidate).toHaveBeenCalledTimes(1))
      expect(revalidate).toHaveBeenCalledWith(BOARD, {
        workspaceId: undefined,
        fetchMeta: undefined,
      })
      await flushPromises()
      source.emit('board.access_changed', boardEnvelope(BOARD, 'board.access_changed'))
      await vi.waitFor(() => expect(revalidate).toHaveBeenCalledTimes(2))
      expect(revoked).toEqual([])
    } finally {
      stop()
      stopSubscription()
    }
  })

  test('revoked verdicts reach the surface once', async () => {
    primeScope()
    const revalidate = vi.fn(async () => 'revoked' as const)
    const revoked: string[] = []
    const stopSubscription = subscribeBoardAccessLost((boardId) => revoked.push(boardId))
    const stop = observeOpenBoardAccessLoss({
      getBoardId: () => BOARD,
      revalidate,
    })
    try {
      const source = MockEventSource.instances[MockEventSource.instances.length - 1]
      source?.emit('board.access_changed', boardEnvelope(BOARD, 'board.access_changed'))
      await vi.waitFor(() => expect(revoked).toEqual([BOARD]))
    } finally {
      stop()
      stopSubscription()
    }
  })

  test('gap notices revalidate the open board for missed revocations', async () => {
    primeScope()
    const revalidate = vi.fn(async () => 'preserved' as const)
    const stop = observeOpenBoardAccessLoss({
      getBoardId: () => BOARD,
      revalidate,
    })
    try {
      const source = MockEventSource.instances[MockEventSource.instances.length - 1]
      if (!source) return
      // Gap frames arrive on the dedicated `gap` channel, not a type. The
      // payload is intentionally empty: the transport ignores gap payloads
      // (the `{truncated: true}` wire shape is pinned by the
      // workspaceEventStream specs).
      source.emit('gap', {})
      await vi.waitFor(() =>
        expect(revalidate).toHaveBeenCalledWith(BOARD, {
          workspaceId: undefined,
          fetchMeta: undefined,
        }),
      )
      expect(isBoardAccessStale(BOARD)).toBe(true)
      // A later successful revalidation clears the stale flag again.
      const cleared = await revalidateBoardAccess(BOARD, {
        fetchMeta: (async () => ({ id: BOARD })) as never,
      })
      expect(cleared).toBe('ok')
      expect(isBoardAccessStale(BOARD)).toBe(false)
    } finally {
      stop()
    }
  })

  test('late verdicts after a board switch never converge the new board', async () => {
    primeScope()
    let resolve!: (verdict: 'revoked') => void
    const gate = new Promise<'revoked'>((resolveGate) => {
      resolve = resolveGate
    })
    let current: string | null = BOARD
    const revoked: string[] = []
    const stopSubscription = subscribeBoardAccessLost((boardId) => revoked.push(boardId))
    const stop = observeOpenBoardAccessLoss({
      getBoardId: () => current,
      revalidate: () => gate,
    })
    try {
      const source = MockEventSource.instances[MockEventSource.instances.length - 1]
      source?.emit('board.access_changed', boardEnvelope(BOARD, 'board.access_changed'))
      current = 'board_other9'
      resolve('revoked')
      await gate
      await new Promise((tick) => setTimeout(tick, 20))
      expect(revoked).toEqual([])
    } finally {
      stop()
      stopSubscription()
    }
  })
})
