import { useDocumentVisibility } from '@vueuse/core'

type PollingInterval = number | false

type AdminQueryPollingOptions = {
  visibleInterval: PollingInterval | (() => PollingInterval)
  hiddenInterval?: PollingInterval
  isEnabled?: () => boolean
}

function resolveInterval(interval: PollingInterval | (() => PollingInterval)): PollingInterval {
  if (typeof interval === 'function') return interval()
  return interval
}

export function useAdminQueryPolling(options: AdminQueryPollingOptions) {
  const visibility = useDocumentVisibility()
  const isPageVisible = computed(() => visibility.value === 'visible')

  const refetchInterval = () => {
    if (options.isEnabled && !options.isEnabled()) return false
    if (!isPageVisible.value) return options.hiddenInterval ?? false
    return resolveInterval(options.visibleInterval)
  }

  return {
    isPageVisible,
    refetchInterval,
  }
}
