// P2.5 cache timing spike exercises real observer freshness and garbage collection.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryObserver } from '@tanstack/vue-query'
import { clearStoredAuth } from '../../../../core/utils/auth'
import { addRemoteBoardFixture, primeAuthenticatedWorkspace } from '../../testing/syncFixtures'
import {
  createMockWorkspaceBoardApiRequest,
  createMockWorkspaceBoardApiState,
  makeBoardState,
} from '../../testing/factories'
import {
  designDocumentOptions,
  designListOptions,
  newDesignQueryClient,
} from '../../testing/syncDesignHarness'
import { queryClient } from '../../../../core/query/queryClient'
import { accountScoped, STALE_TIME_MS } from '../../../../core/query/queryDefaults'

const WORKSPACE_ID = 'ws_test123'
const mocks = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('../../../../api/client', () => ({ api: { request: mocks.request } }))

describe('board cache timing', () => {
  let client: ReturnType<typeof newDesignQueryClient>
  const stops: Array<() => void> = []
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-11T00:00:00Z'))
    localStorage.clear()
    clearStoredAuth()
    primeAuthenticatedWorkspace(WORKSPACE_ID)
    vi.clearAllMocks()
    const state = createMockWorkspaceBoardApiState()
    addRemoteBoardFixture(state, WORKSPACE_ID, {
      id: 'board-a',
      name: 'Server Board',
      createdAt: 100,
      updatedAt: 1000,
      state: makeBoardState({ updatedAt: 1000 }),
    })
    mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(state))
    client = newDesignQueryClient()
  })
  afterEach(() => {
    stops.splice(0).forEach((stop) => stop())
    client.clear()
    vi.useRealTimers()
  })
  it('global query defaults own document freshness, gc, retry, and focus policy', () => {
    expect(queryClient.getDefaultOptions().queries).toMatchObject({
      staleTime: 5_000,
      gcTime: 10 * 60 * 1000,
      retry: 1,
      refetchOnWindowFocus: false,
    })
  })

  it('accountScoped() defaults to 15s freshness and keeps the global retry policy', () => {
    expect(STALE_TIME_MS.account).toBe(15_000)
    expect(accountScoped()).toEqual({ staleTime: 15_000 })
    expect('retry' in accountScoped()).toBe(false)
  })

  describe('board query freshness design spike', () => {
    it('catalog list rides accountScoped() 15s freshness across navigation', async () => {
      const options = designListOptions(WORKSPACE_ID)
      const first = new QueryObserver(client, options)
      const leave = first.subscribe(() => undefined)
      stops.push(leave)
      await vi.advanceTimersByTimeAsync(0)
      expect(first.getCurrentResult().data?.[0]?.id).toBe('board-a')
      expect(first.getCurrentResult().isStale).toBe(false)
      expect(mocks.request).toHaveBeenCalledTimes(1)
      leave()

      await vi.advanceTimersByTimeAsync(14_999)
      const freshVisit = new QueryObserver(client, options)
      const leaveFresh = freshVisit.subscribe(() => undefined)
      stops.push(leaveFresh)
      await vi.advanceTimersByTimeAsync(0)
      expect(freshVisit.getCurrentResult().isStale).toBe(false)
      expect(mocks.request).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(2)
      expect(freshVisit.getCurrentResult().isStale).toBe(true)
      leaveFresh()

      const staleVisit = new QueryObserver(client, options)
      stops.push(staleVisit.subscribe(() => undefined))
      await vi.advanceTimersByTimeAsync(0)
      expect(mocks.request).toHaveBeenCalledTimes(2)
      expect(staleVisit.getCurrentResult().isStale).toBe(false)
    })

    it('per-board document stays at 5s freshness across navigation', async () => {
      const options = designDocumentOptions(WORKSPACE_ID, 'board-a')
      const first = new QueryObserver(client, options)
      const leave = first.subscribe(() => undefined)
      stops.push(leave)
      await vi.advanceTimersByTimeAsync(0)
      expect(first.getCurrentResult().data?.boardId).toBe('board-a')
      expect(mocks.request).toHaveBeenCalledTimes(1)
      leave()

      await vi.advanceTimersByTimeAsync(4_999)
      const freshVisit = new QueryObserver(client, options)
      const leaveFresh = freshVisit.subscribe(() => undefined)
      stops.push(leaveFresh)
      await vi.advanceTimersByTimeAsync(0)
      expect(freshVisit.getCurrentResult().isStale).toBe(false)
      expect(mocks.request).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(2)
      expect(freshVisit.getCurrentResult().isStale).toBe(true)
      leaveFresh()

      const staleVisit = new QueryObserver(client, options)
      stops.push(staleVisit.subscribe(() => undefined))
      await vi.advanceTimersByTimeAsync(0)
      expect(mocks.request).toHaveBeenCalledTimes(2)
      expect(staleVisit.getCurrentResult().data?.revision).toBe(1)
    })

    it('retains inactive snapshots for 10m and collects them at the inherited gc boundary', async () => {
      const options = designDocumentOptions(WORKSPACE_ID, 'board-a')
      const observer = new QueryObserver(client, options)
      const leave = observer.subscribe(() => undefined)
      stops.push(leave)
      await vi.advanceTimersByTimeAsync(0)
      leave()

      await vi.advanceTimersByTimeAsync(10 * 60 * 1000 - 1)
      expect(client.getQueryData(options.queryKey)).toMatchObject({ boardId: 'board-a' })
      expect(mocks.request).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(1)
      expect(
        client.getQueryCache().find({ queryKey: options.queryKey, exact: true }),
      ).toBeUndefined()
      const returnVisit = new QueryObserver(client, options)
      stops.push(returnVisit.subscribe(() => undefined))
      await vi.advanceTimersByTimeAsync(0)
      expect(mocks.request).toHaveBeenCalledTimes(2)
      expect(returnVisit.getCurrentResult().data?.boardId).toBe('board-a')
    })
  })
})
