import { computed, ref, type Ref } from 'vue'

export type AsyncActionLoadingHandle = {
  destroy: () => void
}

export type AsyncActionFeedback<T = unknown> = {
  loading?: () => AsyncActionLoadingHandle | undefined
  success?: (result: T) => void
  error?: ((error: unknown) => void) | undefined
  finally?: () => void
}

type AsyncActionOptions<T = unknown> = {
  loading?: Ref<boolean>
  before?: () => void
  after?: () => void
  feedback?: AsyncActionFeedback<T>
}

export function useAsyncAction() {
  const defaultPending = ref(false)
  const activeActionCount = ref(0)
  const isPending = computed(() => activeActionCount.value > 0)

  async function runAsyncAction<T>(
    action: () => T | Promise<T>,
    options: AsyncActionOptions<T> = {},
  ): Promise<T | undefined> {
    const pending = options.loading ?? defaultPending
    if (pending.value) return undefined

    pending.value = true
    activeActionCount.value += 1
    let loadingFeedback: AsyncActionLoadingHandle | undefined
    let actionResult: T | undefined
    let actionSucceeded = false
    const lifecycleErrors: unknown[] = []

    try {
      options.before?.()
      loadingFeedback = options.feedback?.loading?.()

      try {
        actionResult = await action()
        actionSucceeded = true
      } catch (error) {
        if (options.feedback?.error) {
          try {
            options.feedback.error(error)
          } catch (feedbackError) {
            lifecycleErrors.push(error, feedbackError)
          }
        } else {
          lifecycleErrors.push(error)
        }
      }

      if (actionSucceeded) {
        try {
          options.feedback?.success?.(actionResult as T)
        } catch (error) {
          lifecycleErrors.push(error)
        }
      }
    } catch (error) {
      lifecycleErrors.push(error)
    }

    runCleanupStep(() => loadingFeedback?.destroy(), lifecycleErrors)
    runCleanupStep(options.after, lifecycleErrors)
    runCleanupStep(options.feedback?.finally, lifecycleErrors)
    activeActionCount.value = Math.max(0, activeActionCount.value - 1)
    runCleanupStep(() => {
      pending.value = false
    }, lifecycleErrors)

    throwLifecycleErrors(lifecycleErrors)
    return actionSucceeded ? actionResult : undefined
  }

  return {
    isPending,
    runAsyncAction,
  }
}

function runCleanupStep(step: (() => void) | undefined, errors: unknown[]) {
  try {
    step?.()
  } catch (error) {
    errors.push(error)
  }
}

function throwLifecycleErrors(errors: unknown[]) {
  if (errors.length === 1) {
    throw errors[0]
  }
  if (errors.length > 1) {
    throw new AggregateError(errors, 'Async action lifecycle failed')
  }
}
