import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useAsyncAction } from './useAsyncAction'

describe('useAsyncAction', () => {
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

  it('logs unhandled errors when the action does not rethrow', async () => {
    const error = new Error('boom')
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { runAsyncAction } = useAsyncAction()

    const result = await runAsyncAction(async () => {
      throw error
    })

    expect(result).toBeUndefined()
    expect(consoleError).toHaveBeenCalledWith(
      '[useAsyncAction] unhandled async action error',
      error,
    )
    consoleError.mockRestore()
  })
})
