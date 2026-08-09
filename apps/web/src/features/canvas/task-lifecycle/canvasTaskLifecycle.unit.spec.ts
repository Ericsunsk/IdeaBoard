import { describe, expect, it, vi } from 'vitest'
import {
  CANVAS_TASK_CANCEL_RETRY_DELAYS_MS,
  createCanvasTaskLifecycle,
} from './canvasTaskLifecycle'

function flushRejection() {
  return new Promise<void>((resolve) => queueMicrotask(resolve))
}

describe('createCanvasTaskLifecycle', () => {
  it('cancels each normalized task id once', async () => {
    const cancel = vi.fn(async () => undefined)
    const lifecycle = createCanvasTaskLifecycle({ cancel })

    lifecycle.cancelDeletedRunningTasks([' task-1 ', 'task-1', '', 'task-2'])
    await flushRejection()

    expect(cancel).toHaveBeenCalledTimes(2)
    expect(cancel).toHaveBeenNthCalledWith(1, 'task-1')
    expect(cancel).toHaveBeenNthCalledWith(2, 'task-2')
  })

  it('retries a failed cancellation with the bounded retry policy', async () => {
    const cancel = vi
      .fn<(taskId: string) => Promise<void>>()
      .mockRejectedValueOnce(new Error('temporary'))
      .mockResolvedValueOnce(undefined)
    const scheduleRetry = vi.fn<(callback: () => void, delayMs: number) => void>((callback) => {
      callback()
    })
    const lifecycle = createCanvasTaskLifecycle({ cancel, scheduleRetry })

    lifecycle.cancelDeletedRunningTasks(['task-1'])
    await flushRejection()
    await flushRejection()

    expect(scheduleRetry).toHaveBeenCalledWith(
      expect.any(Function),
      CANVAS_TASK_CANCEL_RETRY_DELAYS_MS[0],
    )
    expect(cancel).toHaveBeenCalledTimes(2)
  })

  it('reports final failure after all retries are exhausted', async () => {
    const error = new Error('permanent')
    const cancel = vi.fn<(taskId: string) => Promise<void>>().mockRejectedValue(error)
    const scheduleRetry = vi.fn<(callback: () => void, delayMs: number) => void>((callback) => {
      callback()
    })
    const onFinalFailure = vi.fn()
    const lifecycle = createCanvasTaskLifecycle({ cancel, scheduleRetry, onFinalFailure })

    lifecycle.cancelDeletedRunningTasks(['task-1'])
    await flushRejection()
    await flushRejection()
    await flushRejection()

    expect(cancel).toHaveBeenCalledTimes(CANVAS_TASK_CANCEL_RETRY_DELAYS_MS.length + 1)
    expect(scheduleRetry).toHaveBeenNthCalledWith(
      1,
      expect.any(Function),
      CANVAS_TASK_CANCEL_RETRY_DELAYS_MS[0],
    )
    expect(scheduleRetry).toHaveBeenNthCalledWith(
      2,
      expect.any(Function),
      CANVAS_TASK_CANCEL_RETRY_DELAYS_MS[1],
    )
    expect(onFinalFailure).toHaveBeenCalledWith({ taskId: 'task-1', error })
  })

  it('warns when the default final-failure reporter is reached', async () => {
    const error = new Error('permanent')
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const lifecycle = createCanvasTaskLifecycle({
      cancel: vi.fn<(taskId: string) => Promise<void>>().mockRejectedValue(error),
      scheduleRetry: (callback) => callback(),
    })

    lifecycle.cancelDeletedRunningTasks(['task-1'])
    await flushRejection()
    await flushRejection()
    await flushRejection()

    expect(warning).toHaveBeenCalledWith('[canvas] failed to stop deleted task', {
      taskId: 'task-1',
      error,
    })
    warning.mockRestore()
  })
})
