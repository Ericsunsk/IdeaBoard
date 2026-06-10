import type { Ref } from 'vue'
import { formatDisplayErrorMessage } from '../utils/errorMessages'
import { useAppToast } from './useAppToast'

type AsyncActionErrorMessage = string | ((error: unknown) => string)

type AsyncActionOptions = {
  loading?: Ref<boolean>
  before?: () => void
  after?: () => void
  errorMessage?: AsyncActionErrorMessage
  onError?: (error: unknown) => void
  rethrow?: boolean
}

function resolveErrorMessage(error: unknown, message: AsyncActionErrorMessage): string {
  return typeof message === 'function' ? message(error) : formatDisplayErrorMessage(error, message)
}

export function useAsyncAction() {
  const appToast = useAppToast()

  async function runAsyncAction<T>(
    action: () => T | Promise<T>,
    options: AsyncActionOptions = {},
  ): Promise<T | undefined> {
    options.before?.()
    if (options.loading) {
      options.loading.value = true
    }

    try {
      return await action()
    } catch (error) {
      if (options.onError) {
        options.onError(error)
      } else if (options.errorMessage) {
        appToast.error(resolveErrorMessage(error, options.errorMessage))
      }

      if (options.rethrow) {
        throw error
      }
      return undefined
    } finally {
      if (options.loading) {
        options.loading.value = false
      }
      options.after?.()
    }
  }

  return {
    runAsyncAction,
  }
}
