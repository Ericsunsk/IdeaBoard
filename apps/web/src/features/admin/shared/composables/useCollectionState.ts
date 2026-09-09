import { computed, ref, shallowRef } from 'vue'

export type AdminCollectionLoadOptions = {
  preserveData?: boolean
}

type AdminCollectionStateOptions<TPage> = {
  load: (signal: AbortSignal) => Promise<TPage>
  isEmpty: (page: TPage) => boolean
  onInitialError?: (error: unknown) => void
}

/**
 * Owns the request lifecycle shared by Admin collections. Resource adapters keep
 * their domain query, pagination, rows, and feedback outside this seam.
 */
export function useCollectionState<TPage>(options: AdminCollectionStateOptions<TPage>) {
  const data = shallowRef<TPage>()
  const loading = ref(true)
  const initialError = ref(false)
  const backgroundError = ref(false)
  const hasLoadedResult = ref(false)
  const isEmpty = computed(() => Boolean(data.value && options.isEmpty(data.value)))
  let requestEpoch = 0
  let activeRequest: AbortController | undefined

  async function reload({ preserveData = false }: AdminCollectionLoadOptions = {}) {
    const epoch = ++requestEpoch
    activeRequest?.abort()
    const controller = new AbortController()
    activeRequest = controller

    if (!preserveData) data.value = undefined
    loading.value = true
    initialError.value = false
    backgroundError.value = false

    try {
      const page = await options.load(controller.signal)
      if (controller.signal.aborted || epoch !== requestEpoch) return undefined

      data.value = page
      hasLoadedResult.value = true
      return page
    } catch (error) {
      if (controller.signal.aborted || epoch !== requestEpoch) return undefined

      if (hasLoadedResult.value && preserveData) backgroundError.value = true
      else {
        initialError.value = true
        options.onInitialError?.(error)
      }
      return undefined
    } finally {
      if (epoch === requestEpoch) loading.value = false
    }
  }

  function cancel() {
    requestEpoch += 1
    activeRequest?.abort()
    activeRequest = undefined
    loading.value = false
  }

  function reset() {
    cancel()
    data.value = undefined
    hasLoadedResult.value = false
    initialError.value = false
    backgroundError.value = false
  }

  function updateData(update: (current: TPage) => TPage) {
    if (data.value) data.value = update(data.value)
  }

  return {
    data,
    loading,
    initialError,
    backgroundError,
    hasLoadedResult,
    isEmpty,
    reload,
    cancel,
    reset,
    updateData,
  }
}
