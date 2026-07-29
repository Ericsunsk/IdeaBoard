import { describe, expect, it, vi } from 'vitest'
import { ref, watch } from 'vue'
import { useAsyncAction } from './useAsyncAction'

describe('useAsyncAction', () => {
  it('blocks duplicate submissions while the committed action is pending', async () => {
    const pendingAction = Promise.withResolvers<string>()
    const action = vi.fn(() => pendingAction.promise)
    const { isPending, runAsyncAction } = useAsyncAction()

    const firstRun = runAsyncAction(action)
    const duplicateRun = runAsyncAction(action)

    expect(isPending.value).toBe(true)
    expect(action).toHaveBeenCalledOnce()
    await expect(duplicateRun).resolves.toBeUndefined()

    pendingAction.resolve('saved')

    await expect(firstRun).resolves.toBe('saved')
    expect(isPending.value).toBe(false)
  })

  it('runs feedback lifecycle around successful actions', async () => {
    const loadingHandle = { destroy: vi.fn() }
    const events: string[] = []
    const loading = ref(false)
    const { runAsyncAction } = useAsyncAction()

    const result = await runAsyncAction(
      async () => {
        expect(loading.value).toBe(true)
        events.push('action')
        return 'saved'
      },
      {
        loading,
        feedback: {
          loading: () => {
            events.push('loading')
            return loadingHandle
          },
          success: (value) => {
            events.push(`success:${value}`)
          },
          finally: () => {
            events.push('finally')
          },
        },
      },
    )

    expect(result).toBe('saved')
    expect(loading.value).toBe(false)
    expect(loadingHandle.destroy).toHaveBeenCalledOnce()
    expect(events).toEqual(['loading', 'action', 'success:saved', 'finally'])
  })

  it('does not reinterpret a success feedback failure as an action failure', async () => {
    const feedbackError = vi.fn()
    const feedbackFinally = vi.fn()
    const successError = new Error('success feedback failed')
    const { isPending, runAsyncAction } = useAsyncAction()

    await expect(
      runAsyncAction(async () => 'saved', {
        feedback: {
          success: () => {
            throw successError
          },
          error: feedbackError,
          finally: feedbackFinally,
        },
      }),
    ).rejects.toBe(successError)

    expect(feedbackError).not.toHaveBeenCalled()
    expect(feedbackFinally).toHaveBeenCalledOnce()
    expect(isPending.value).toBe(false)
  })

  it('runs feedback error around failed actions', async () => {
    const error = new Error('boom')
    const loadingHandle = { destroy: vi.fn() }
    const feedbackError = vi.fn()
    const { runAsyncAction } = useAsyncAction()

    const result = await runAsyncAction(
      async () => {
        throw error
      },
      {
        feedback: {
          loading: () => loadingHandle,
          error: feedbackError,
        },
      },
    )

    expect(result).toBeUndefined()
    expect(feedbackError).toHaveBeenCalledWith(error)
    expect(loadingHandle.destroy).toHaveBeenCalledOnce()
  })

  it('settles every cleanup step once even when one cleanup step fails', async () => {
    const cleanupError = new Error('loading cleanup failed')
    const loading = ref(false)
    const loadingHandle = {
      destroy: vi.fn(() => {
        throw cleanupError
      }),
    }
    const after = vi.fn()
    const feedbackFinally = vi.fn()
    const { isPending, runAsyncAction } = useAsyncAction()

    await expect(
      runAsyncAction(async () => 'saved', {
        loading,
        after,
        feedback: {
          loading: () => loadingHandle,
          finally: feedbackFinally,
        },
      }),
    ).rejects.toBe(cleanupError)

    expect(loadingHandle.destroy).toHaveBeenCalledOnce()
    expect(after).toHaveBeenCalledOnce()
    expect(feedbackFinally).toHaveBeenCalledOnce()
    expect(loading.value).toBe(false)
    expect(isPending.value).toBe(false)
  })

  it('keeps the action pending while cleanup blocks reentrant submissions', async () => {
    const action = vi.fn(async () => 'saved')
    const { isPending, runAsyncAction } = useAsyncAction()
    let reentrantRun: Promise<string | undefined> | undefined

    await expect(
      runAsyncAction(action, {
        after: () => {
          expect(isPending.value).toBe(true)
          reentrantRun = runAsyncAction(action)
        },
      }),
    ).resolves.toBe('saved')

    await expect(reentrantRun).resolves.toBeUndefined()
    expect(action).toHaveBeenCalledOnce()
    expect(isPending.value).toBe(false)
  })

  it('settles aggregate and scoped pending state atomically for synchronous observers', async () => {
    const loading = ref(false)
    const action = vi.fn(async () => 'saved')
    const { isPending, runAsyncAction } = useAsyncAction()
    let aggregatePendingWhenScopeSettled: boolean | undefined
    const stopWatching = watch(
      loading,
      (pending) => {
        if (!pending) {
          aggregatePendingWhenScopeSettled = isPending.value
        }
      },
      { flush: 'sync' },
    )

    await expect(runAsyncAction(action, { loading })).resolves.toBe('saved')
    expect(aggregatePendingWhenScopeSettled).toBe(false)
    expect(action).toHaveBeenCalledOnce()

    stopWatching()
  })

  it('cleans up a lifecycle that fails before the action starts', async () => {
    const setupError = new Error('before failed')
    const action = vi.fn()
    const loading = ref(false)
    const feedbackFinally = vi.fn()
    const { isPending, runAsyncAction } = useAsyncAction()

    await expect(
      runAsyncAction(action, {
        loading,
        before: () => {
          throw setupError
        },
        feedback: {
          finally: feedbackFinally,
        },
      }),
    ).rejects.toBe(setupError)

    expect(action).not.toHaveBeenCalled()
    expect(feedbackFinally).toHaveBeenCalledOnce()
    expect(loading.value).toBe(false)
    expect(isPending.value).toBe(false)
  })

  it('rethrows action failures when no error feedback adapter handles them', async () => {
    const error = new Error('boom')
    const { isPending, runAsyncAction } = useAsyncAction()

    await expect(
      runAsyncAction(async () => {
        throw error
      }),
    ).rejects.toBe(error)

    expect(isPending.value).toBe(false)
  })

  it('preserves both action and cleanup failures', async () => {
    const actionError = new Error('action failed')
    const cleanupError = new Error('cleanup failed')
    const { runAsyncAction } = useAsyncAction()

    const failure = await runAsyncAction(
      async () => {
        throw actionError
      },
      {
        feedback: {
          loading: () => ({
            destroy: () => {
              throw cleanupError
            },
          }),
        },
      },
    ).catch((error: unknown) => error)

    expect(failure).toBeInstanceOf(AggregateError)
    expect((failure as AggregateError).errors).toEqual([actionError, cleanupError])
  })
})
