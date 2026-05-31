import { useGetAdminMonitor } from '../../../../shared/api/generated-query/admin-monitor/admin-monitor'
import { useAdminQueryPolling } from '../../shared/composables/useAdminQueryPolling'

export function useAdminMonitorData() {
  const polling = useAdminQueryPolling({
    visibleInterval: 10_000,
    hiddenInterval: false,
  })

  const monitorQuery = useGetAdminMonitor({
    query: {
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: false,
    },
  })

  watch(polling.isPageVisible, (isVisible, wasVisible) => {
    if (isVisible && !wasVisible) void monitorQuery.refetch()
  })

  const monitorData = computed(() => monitorQuery.data.value?.data?.monitor || null)

  return {
    monitorData,
  }
}
