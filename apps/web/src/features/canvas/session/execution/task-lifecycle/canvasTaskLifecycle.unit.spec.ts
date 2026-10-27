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

    lifecycle.cancelTasks([' task-1 ', 'task-1', '', 'task-2'])
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

    lifecycle.cancelTasks(['task-1'])
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

    lifecycle.cancelTasks(['task-1'])
    for (let index = 0; index < CANVAS_TASK_CANCEL_RETRY_DELAYS_MS.length + 1; index += 1) {
      await flushRejection()
    }

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

    lifecycle.cancelTasks(['task-1'])
    for (let index = 0; index < CANVAS_TASK_CANCEL_RETRY_DELAYS_MS.length + 1; index += 1) {
      await flushRejection()
    }

    expect(warning).toHaveBeenCalledWith('[canvas] failed to cancel task', {
      taskId: 'task-1',
      error,
    })
    warning.mockRestore()
  })

  it('keeps exhausted cancellations pending and retries them on demand', async () => {
    const error = new Error('permanent')
    const cancel = vi.fn<(taskId: string) => Promise<void>>().mockRejectedValue(error)
    const lifecycle = createCanvasTaskLifecycle({
      cancel,
      scheduleRetry: (callback) => callback(),
      onFinalFailure: () => undefined,
    })

    lifecycle.cancelTasks(['task-1'])
    for (let index = 0; index < CANVAS_TASK_CANCEL_RETRY_DELAYS_MS.length + 1; index += 1) {
      await flushRejection()
    }

    // 留档：远端可能仍在跑，最终失败不止于 warn
    expect(lifecycle.pendingCancelTaskIds()).toEqual(['task-1'])
    const settled = cancel.mock.calls.length
    cancel.mockResolvedValueOnce(undefined)
    lifecycle.retryPendingCancels()
    await flushRejection()
    expect(cancel).toHaveBeenCalledTimes(settled + 1)
    expect(lifecycle.pendingCancelTaskIds()).toEqual([])
  })

  it('piggybacks pending cancellations onto the next cancel call', async () => {
    const error = new Error('permanent')
    const cancel = vi.fn<(taskId: string) => Promise<void>>().mockRejectedValue(error)
    const lifecycle = createCanvasTaskLifecycle({
      cancel,
      scheduleRetry: (callback) => callback(),
      onFinalFailure: () => undefined,
    })

    lifecycle.cancelTasks(['task-1'])
    for (let index = 0; index < CANVAS_TASK_CANCEL_RETRY_DELAYS_MS.length + 1; index += 1) {
      await flushRejection()
    }
    expect(lifecycle.pendingCancelTaskIds()).toEqual(['task-1'])

    cancel.mockResolvedValue(undefined)
    lifecycle.cancelTasks(['task-2'])
    await flushRejection()
    await flushRejection()
    expect(cancel).toHaveBeenCalledWith('task-1')
    expect(cancel).toHaveBeenCalledWith('task-2')
    expect(lifecycle.pendingCancelTaskIds()).toEqual([])
  })
})
