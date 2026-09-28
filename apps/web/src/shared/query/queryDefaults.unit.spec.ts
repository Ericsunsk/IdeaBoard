import { describe, expect, it } from 'vitest'
import { accountScoped, adminPolling, keepPrevious, noRetry, STALE_TIME_MS } from './queryDefaults'

describe('query presets', () => {
  it('documents the canonical freshness values', () => {
    expect(STALE_TIME_MS.account).toBe(15_000)
    expect(STALE_TIME_MS.usage).toBe(30_000)
    expect(STALE_TIME_MS.statistics).toBe(60_000)
    expect(STALE_TIME_MS.static).toBe(Infinity)
  })

  it('noRetry() fails fast without touching freshness', () => {
    expect(noRetry()).toEqual({ retry: false })
  })

  it('accountScoped() defaults to 15s freshness and keeps global retry', () => {
    expect(accountScoped()).toEqual({ staleTime: 15_000 })
    expect(accountScoped(STALE_TIME_MS.usage)).toEqual({ staleTime: 30_000 })
    expect('retry' in accountScoped()).toBe(false)
  })

  it('adminPolling() selects the active/idle cadence from cached data', () => {
    const polling = adminPolling(
      5_000,
      30_000,
      (data) => data != null && (data as { on: boolean }).on,
    )

    expect(polling.retry).toBe(false)
    expect(polling.refetchIntervalInBackground).toBe(false)
    expect(polling.refetchInterval({ state: { data: { on: true } } })).toBe(5_000)
    expect(polling.refetchInterval({ state: { data: { on: false } } })).toBe(30_000)
    expect(polling.refetchInterval({ state: { data: undefined } })).toBe(30_000)
  })

  it('adminPolling() defaults to the active cadence', () => {
    const polling = adminPolling(4_000, false)

    expect(polling.refetchInterval({ state: { data: undefined } })).toBe(4_000)
  })

  it('keepPrevious() returns the previous page by identity', () => {
    const { placeholderData } = keepPrevious()
    const previous = { items: [1, 2, 3] }

    expect(placeholderData(previous)).toBe(previous)
    expect(placeholderData(undefined)).toBeUndefined()
  })
})
