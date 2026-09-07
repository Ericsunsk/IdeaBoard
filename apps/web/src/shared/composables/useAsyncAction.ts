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
    let outcome: AsyncActionRunOutcome<T> = { actionResult: undefined, actionSucceeded: false }
    const lifecycleErrors: unknown[] = []

    try {
      options.before?.()
      loadingFeedback = options.feedback?.loading?.()
      outcome = await executeGuardedAction(action, options.feedback, lifecycleErrors)
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
    return outcome.actionSucceeded ? outcome.actionResult : undefined
  }

  return {
    isPending,
    runAsyncAction,
  }
}

type AsyncActionRunOutcome<T> = {
  actionResult: T | undefined
  actionSucceeded: boolean
}

async function executeGuardedAction<T>(
  action: () => T | Promise<T>,
  feedback: AsyncActionFeedback<T> | undefined,
  lifecycleErrors: unknown[],
): Promise<AsyncActionRunOutcome<T>> {
  let actionResult: T | undefined
  let actionSucceeded = false
  try {
    actionResult = await action()
    actionSucceeded = true
  } catch (error) {
    if (feedback?.error) {
      try {
        feedback.error(error)
      } catch (feedbackError) {
        lifecycleErrors.push(error, feedbackError)
      }
    } else {
      lifecycleErrors.push(error)
    }
  }

  if (actionSucceeded) {
    try {
      feedback?.success?.(actionResult as T)
    } catch (error) {
      lifecycleErrors.push(error)
    }
  }
  return { actionResult, actionSucceeded }
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
