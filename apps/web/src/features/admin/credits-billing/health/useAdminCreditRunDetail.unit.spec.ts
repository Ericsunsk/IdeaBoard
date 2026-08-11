import { describe, expect, it, vi } from 'vitest'
import { useAdminCreditRunDetail } from './useAdminCreditRunDetail'

describe('useAdminCreditRunDetail', () => {
  it('polls queued or running Run detail every three seconds and stops at a terminal status', async () => {
    vi.useFakeTimers()
    const load = vi
      .fn()
      .mockResolvedValueOnce({ id: 21, status: 'running' })
      .mockResolvedValueOnce({ id: 21, status: 'completed' })
    const detail = useAdminCreditRunDetail(load)

    await detail.open(21)
    await vi.advanceTimersByTimeAsync(3_000)

    expect(load).toHaveBeenCalledTimes(2)
    expect(detail.run.value).toEqual({ id: 21, status: 'completed' })

    await vi.advanceTimersByTimeAsync(6_000)
    expect(load).toHaveBeenCalledTimes(2)
    vi.useRealTimers()
  })

  it('aborts and rejects completion after the detail closes', async () => {
    vi.useFakeTimers()
    let resolveRequest: (run: { id: number; status: string }) => void = () => undefined
    const load = vi.fn(
      (_runId: number, signal: AbortSignal) =>
        new Promise<{ id: number; status: string }>((resolve) => {
          resolveRequest = resolve
          expect(signal.aborted).toBe(false)
        }),
    )
    const detail = useAdminCreditRunDetail(load)

    const request = detail.open(21)
    detail.close()
    resolveRequest({ id: 21, status: 'running' })
    await request

    expect(load.mock.calls[0]?.[1].aborted).toBe(true)
    expect(detail.run.value).toBeUndefined()
    expect(detail.loading.value).toBe(false)
    await vi.advanceTimersByTimeAsync(3_000)
    expect(load).toHaveBeenCalledTimes(1)
    vi.useRealTimers()
  })
})
