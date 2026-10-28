import { nextTick, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mockAppToast, type AppToastMocks } from '../../test/toast'
import { useFailureToast } from './useFailureToast'

vi.mock('./useAppToast')

describe('useFailureToast', () => {
  let toast: AppToastMocks

  beforeEach(() => {
    toast = mockAppToast()
  })

  it('toasts a new error object once and ignores repeats of the same instance', async () => {
    const error = ref<unknown>(null)
    useFailureToast(error, (value) => `failed: ${String(value)}`)

    error.value = new Error('boom')
    await nextTick()
    expect(toast.error).toHaveBeenCalledTimes(1)
    expect(toast.error).toHaveBeenCalledWith('failed: Error: boom')

    const failure = error.value
    error.value = failure
    await nextTick()
    expect(toast.error).toHaveBeenCalledTimes(1)

    error.value = null
    await nextTick()
    error.value = failure
    await nextTick()
    expect(toast.error).toHaveBeenCalledTimes(2)
  })

  it('skips toast when the skip predicate matches (403 neutral state)', async () => {
    const error = ref<unknown>(null)
    useFailureToast(error, (value) => `failed: ${String(value)}`, {
      skip: (value) => (value as { code?: string })?.code === 'billing_account_forbidden',
    })

    error.value = Object.assign(new Error('forbidden'), { code: 'billing_account_forbidden' })
    await nextTick()
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('supports the boolean form with resolveError for admin list errors', async () => {
    const isFailed = ref(false)
    const queryError = ref<unknown>(null)
    useFailureToast(
      isFailed,
      (error) => {
        expect(error).toBe(queryError.value)
        return 'load failed'
      },
      { shouldFireOnMount: false, resolveError: () => queryError.value },
    )

    queryError.value = new Error('503')
    isFailed.value = true
    await nextTick()
    expect(toast.error).toHaveBeenCalledWith('load failed')
  })

  it('does not toast on mount when shouldFireOnMount is false', async () => {
    const isFailed = ref(true)
    useFailureToast(isFailed, () => 'load failed', { shouldFireOnMount: false })
    await nextTick()
    expect(toast.error).not.toHaveBeenCalled()

    isFailed.value = false
    await nextTick()
    isFailed.value = true
    await nextTick()
    expect(toast.error).toHaveBeenCalledWith('load failed')
  })

  it('lets admin feedback factories toast internally via a void format', async () => {
    const isFailed = ref(false)
    const feedbackFailed = vi.fn()
    useFailureToast(
      isFailed,
      (error) => {
        feedbackFailed(error)
      },
      { shouldFireOnMount: false, resolveError: () => new Error('detail') },
    )

    isFailed.value = true
    await nextTick()
    expect(feedbackFailed).toHaveBeenCalledOnce()
    expect(toast.error).not.toHaveBeenCalled()
  })
})
