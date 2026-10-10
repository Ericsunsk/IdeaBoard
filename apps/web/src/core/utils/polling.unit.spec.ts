import { describe, expect, it } from 'vitest'
import { advancePollDelay, createPollDelay, sleep } from './polling'

describe('advancePollDelay', () => {
  it('applies exponential backoff capped at the maximum', () => {
    const policy = { maxDelayMs: 8_000, backoffFactor: 1.6 }
    expect(advancePollDelay(1_200, policy)).toBe(1_920)
    expect(advancePollDelay(1_920, policy)).toBe(3_072)
    expect(advancePollDelay(7_900, policy)).toBe(8_000)
    expect(advancePollDelay(8_000, policy)).toBe(8_000)
  })

  it('keeps a fixed cadence when no factor is given', () => {
    expect(advancePollDelay(500, { maxDelayMs: 500 })).toBe(500)
  })

  it('never stalls on a zero initial delay', () => {
    expect(advancePollDelay(0, { maxDelayMs: 100, backoffFactor: 2 })).toBe(1)
  })
})

describe('createPollDelay', () => {
  it('returns the current delay then advances, and resets on success', () => {
    const delay = createPollDelay({ initialDelayMs: 800, maxDelayMs: 2_000, backoffFactor: 2 })
    expect(delay.next()).toBe(800)
    expect(delay.next()).toBe(1_600)
    expect(delay.next()).toBe(2_000)
    delay.reset()
    expect(delay.current()).toBe(800)
  })
})

describe('sleep', () => {
  it('resolves after the requested delay', async () => {
    await expect(sleep(0)).resolves.toBeUndefined()
  })
})
