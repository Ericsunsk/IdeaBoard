import { describe, expect, it, vi } from 'vitest'
import { runWithDeadline, withSignal } from './request'

describe('withSignal', () => {
  it('returns empty object when signal is null or undefined', () => {
    expect(withSignal(null)).toEqual({})
    expect(withSignal(undefined)).toEqual({})
  })

  it('returns object with signal when signal is provided', () => {
    const controller = new AbortController()
    expect(withSignal(controller.signal)).toEqual({ signal: controller.signal })
  })
})

describe('runWithDeadline', () => {
  it('resolves when operation completes before deadline', async () => {
    const result = await runWithDeadline(Date.now() + 1000, undefined, async () => 42)
    expect(result).toBe(42)
  })

  it('rejects with TimeoutError when deadline is already passed', async () => {
    await expect(runWithDeadline(Date.now() - 10, undefined, async () => 42)).rejects.toMatchObject(
      { name: 'TimeoutError' },
    )
  })

  it('times out even when the operation ignores its abort signal', async () => {
    vi.useFakeTimers()
    try {
      const result = runWithDeadline(
        Date.now() + 50,
        undefined,
        () => new Promise<never>(() => undefined),
      )
      const rejected = expect(result).rejects.toMatchObject({ name: 'TimeoutError' })
      await vi.advanceTimersByTimeAsync(50)
      await rejected
    } finally {
      vi.useRealTimers()
    }
  })

  it('rejects when parent signal is already aborted', async () => {
    const parent = new AbortController()
    parent.abort(new Error('parent aborted'))
    await expect(runWithDeadline(Date.now() + 1000, parent.signal, async () => 42)).rejects.toThrow(
      'parent aborted',
    )
  })

  it('aborts operation when parent signal aborts during execution', async () => {
    const parent = new AbortController()
    const promise = runWithDeadline(Date.now() + 1000, parent.signal, async (signal) => {
      return new Promise<number>((_, reject) => {
        signal.addEventListener('abort', () => reject(signal.reason))
      })
    })
    parent.abort(new Error('cancelled by caller'))
    await expect(promise).rejects.toThrow('cancelled by caller')
  })
})
