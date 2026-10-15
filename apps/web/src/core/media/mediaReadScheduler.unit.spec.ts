import { expect, test, vi } from 'vitest'
import { createDeferred } from '../../test/deferred'
import { scheduleMediaRead } from './mediaReadScheduler'

test('queued cancellation skips work and active cancellation retains its slot until transport settles', async () => {
  const controllers = Array.from({ length: 5 }, () => new AbortController())
  const deferred = controllers.map(() => createDeferred<void>())
  const starts = controllers.map((_, index) => vi.fn(() => deferred[index]!.promise))
  const reads = controllers.map((controller, index) =>
    scheduleMediaRead(controller.signal, starts[index]!),
  )
  expect(starts.at(-1)).not.toHaveBeenCalled()
  controllers[0]!.abort()
  expect(starts.at(-1)).not.toHaveBeenCalled()
  controllers.at(-1)!.abort()
  const cancelled = expect(reads.at(-1)).rejects.toMatchObject({ name: 'AbortError' })
  for (const read of deferred) read.resolve()
  await cancelled
  await Promise.all(reads.slice(0, -1))
  expect(starts.at(-1)).not.toHaveBeenCalled()
})

test('a synchronous adapter failure releases capacity', async () => {
  await expect(
    scheduleMediaRead(new AbortController().signal, () => {
      throw new Error('failed')
    }),
  ).rejects.toThrow('failed')
  await expect(scheduleMediaRead(new AbortController().signal, async () => 'ready')).resolves.toBe(
    'ready',
  )
})
