import { useDocumentVisibility } from '@vueuse/core'
import { computed, toValue } from 'vue'

type QueryPollingInterval = number | false

type QueryPollingOptions = {
  visibleInterval: QueryPollingInterval | (() => QueryPollingInterval)
  hiddenInterval?: QueryPollingInterval
  isEnabled?: () => boolean
}

/** Reactive Query option: visibility and enablement changes reinstall or stop its timer. */
export function useQueryPolling(options: QueryPollingOptions) {
  const visibility = useDocumentVisibility()
  const isPageVisible = computed(() => visibility.value === 'visible')

  const refetchInterval = computed(() => {
    if (options.isEnabled && !options.isEnabled()) return false
    if (!isPageVisible.value) return options.hiddenInterval ?? false
    return toValue(options.visibleInterval)
  })

  return {
    isPageVisible,
    refetchInterval,
  }
}
