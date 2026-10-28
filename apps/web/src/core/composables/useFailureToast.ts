import { watch, type WatchSource } from 'vue'
import { useAppToast } from './useAppToast'

export interface UseFailureToastOptions {
  /** Return true to keep this failure silent. */
  skip?: ((error: unknown) => boolean) | undefined
  /** Resolve the error when the watched source is boolean. */
  resolveError?: (() => unknown) | undefined
  /** Check the initial value immediately; defaults to true. */
  shouldFireOnMount?: boolean | undefined
}

/**
 * Report a new truthy error or a false-to-true transition.
 * A void formatter can delegate feedback to a feature-owned factory.
 */
export function useFailureToast(
  source: WatchSource<unknown>,
  format: (error: unknown) => string | void,
  options: UseFailureToastOptions = {},
): void {
  const toast = useAppToast()
  const { skip, resolveError, shouldFireOnMount = true } = options
  watch(
    source,
    (value, previous) => {
      if (!value || value === previous) return
      const error = typeof value === 'boolean' ? resolveError?.() : value
      if (skip?.(error)) return
      const message = format(error)
      if (typeof message === 'string' && message) toast.error(message)
    },
    { immediate: shouldFireOnMount },
  )
}
