import { describe, expect, it } from 'vitest'
import { SESSION_FRESH_THRESHOLD_MS } from '../../../shared/config/timing'
import { SESSION_TRUST_WINDOW_MS, isSessionFresh } from './freshness'

const NOW = 1_000_000

describe('session freshness policy', () => {
  it('shares a single trust window with the shared transport waits', () => {
    expect(SESSION_TRUST_WINDOW_MS).toBe(SESSION_FRESH_THRESHOLD_MS)
    expect(SESSION_TRUST_WINDOW_MS).toBe(300_000)
  })

  it('trusts a recently validated session', () => {
    expect(isSessionFresh(NOW - 60_000, NOW)).toBe(true)
  })

  it('treats a session validated exactly one window ago as stale (boundary)', () => {
    expect(isSessionFresh(NOW - SESSION_TRUST_WINDOW_MS, NOW)).toBe(false)
    expect(isSessionFresh(NOW - SESSION_TRUST_WINDOW_MS + 1, NOW)).toBe(true)
    expect(isSessionFresh(NOW - SESSION_TRUST_WINDOW_MS - 1, NOW)).toBe(false)
  })

  it('treats a never-validated session as stale', () => {
    expect(isSessionFresh(0, NOW)).toBe(false)
    expect(isSessionFresh(-1, NOW)).toBe(false)
  })
})
