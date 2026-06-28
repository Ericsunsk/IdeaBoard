import { describe, expect, it, vi } from 'vitest'
import { ImageProcessingScheduler } from './imageProcessingScheduler'

describe('ImageProcessingScheduler', () => {
  it('debounces latest-only work and only runs the newest task', async () => {
    vi.useFakeTimers()
    try {
      const scheduler = new ImageProcessingScheduler({ debounceMs: 20 })
      const runner = vi.fn((context) => context.revision)

      const first = scheduler.schedule(runner, { taskId: 'preview' })
      const second = scheduler.schedule(runner, { taskId: 'preview' })

      await expect(first).resolves.toMatchObject({
        status: 'stale',
        revision: 1,
      })
      expect(runner).not.toHaveBeenCalled()

      await vi.advanceTimersByTimeAsync(20)

      await expect(second).resolves.toMatchObject({
        status: 'completed',
        revision: 2,
        value: 2,
      })
      expect(runner).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('cancels active tasks by public task id', async () => {
    const scheduler = new ImageProcessingScheduler({ latestOnly: false })
    const captured: { signal?: AbortSignal } = {}
    const task = scheduler.schedule(
      (context) =>
        new Promise<string>((resolve) => {
          captured.signal = context.signal
          void resolve
        }),
      { taskId: 'apply' },
    )

    await Promise.resolve()
    expect(scheduler.cancel('apply')).toBe(1)
    expect(captured.signal?.aborted).toBe(true)
    await expect(task).resolves.toMatchObject({
      status: 'cancelled',
      taskId: 'apply',
    })
  })

  it('allows non-latest tasks to complete independently', async () => {
    const scheduler = new ImageProcessingScheduler({ latestOnly: false })

    const first = scheduler.schedule((context) => context.revision, { taskId: 'final' })
    const second = scheduler.schedule((context) => context.revision, { taskId: 'final' })

    await expect(first).resolves.toMatchObject({
      status: 'completed',
      revision: 1,
      value: 1,
    })
    await expect(second).resolves.toMatchObject({
      status: 'completed',
      revision: 2,
      value: 2,
    })
  })

  it('marks an active latest-only task stale when a newer revision starts', async () => {
    const scheduler = new ImageProcessingScheduler()
    const resolvers: Array<(value: string) => void> = []
    const first = scheduler.schedule(
      () =>
        new Promise<string>((resolve) => {
          resolvers.push(resolve)
        }),
      { taskId: 'preview' },
    )

    await Promise.resolve()
    const second = scheduler.schedule(() => 'next', { taskId: 'preview' })

    await expect(first).resolves.toMatchObject({
      status: 'stale',
      revision: 1,
    })
    resolvers[0]?.('old')
    await expect(second).resolves.toMatchObject({
      status: 'completed',
      revision: 2,
      value: 'next',
    })
  })
})
