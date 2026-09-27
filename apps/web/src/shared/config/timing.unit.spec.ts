import { describe, expect, it } from 'vitest'
import {
  EVENT_STREAM_RECONNECT_BACKOFF_FACTOR,
  EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS,
  EVENT_STREAM_RECONNECT_MAX_DELAY_MS,
  MEDIA_METADATA_TTL_MS,
  SESSION_FRESH_THRESHOLD_MS,
  TASK_POLL_BACKOFF_FACTOR,
  TASK_POLL_INITIAL_DELAY_MS,
  TASK_POLL_MAX_DELAY_MS,
} from './timing'

describe('shared timing waits', () => {
  it('keeps the canonical transport wait values', () => {
    expect(TASK_POLL_INITIAL_DELAY_MS).toBe(1_200)
    expect(TASK_POLL_MAX_DELAY_MS).toBe(8_000)
    expect(TASK_POLL_BACKOFF_FACTOR).toBe(1.6)
    expect(EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS).toBe(1_000)
    expect(EVENT_STREAM_RECONNECT_MAX_DELAY_MS).toBe(30_000)
    expect(EVENT_STREAM_RECONNECT_BACKOFF_FACTOR).toBe(1.6)
    expect(MEDIA_METADATA_TTL_MS).toBe(5 * 60 * 1_000)
    expect(SESSION_FRESH_THRESHOLD_MS).toBe(300_000)
  })

  it('keeps backoff windows well-formed (initial at or below max)', () => {
    expect(TASK_POLL_INITIAL_DELAY_MS).toBeLessThanOrEqual(TASK_POLL_MAX_DELAY_MS)
    expect(EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS).toBeLessThanOrEqual(
      EVENT_STREAM_RECONNECT_MAX_DELAY_MS,
    )
  })
})
