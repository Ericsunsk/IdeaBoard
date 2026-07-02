import type { Ref } from 'vue'

export type AsyncActionLoadingHandle = {
  destroy: () => void
}

export type AsyncActionFeedback<T = unknown> = {
  loading?: () => AsyncActionLoadingHandle | undefined
  success?: (result: T) => void
  error?: (error: unknown) => void
  finally?: () => void
}

type AsyncActionOptions<T = unknown> = {
  loading?: Ref<boolean>
  before?: () => void
  after?: () => void
  feedback?: AsyncActionFeedback<T>
  rethrow?: boolean
}

export function useAsyncAction() {
  async function runAsyncAction<T>(
    action: () => T | Promise<T>,
    options: AsyncActionOptions<T> = {},
  ): Promise<T | undefined> {
    options.before?.()
    if (options.loading) {
      options.loading.value = true
    }
    const loadingFeedback = options.feedback?.loading?.()

    try {
      const result = await action()
      options.feedback?.success?.(result)
      return result
    } catch (error) {
      if (options.feedback?.error) {
        options.feedback.error(error)
      } else if (!options.rethrow) {
        console.error('[useAsyncAction] unhandled async action error', error)
      }

      if (options.rethrow) {
        throw error
      }
      return undefined
    } finally {
      loadingFeedback?.destroy()
      if (options.loading) {
        options.loading.value = false
      }
      options.after?.()
      options.feedback?.finally?.()
    }
  }

  return {
    runAsyncAction,
  }
}
