import { useGetAdminMonitor } from '../../../../shared/api/generated-query/admin-monitor/admin-monitor'
import { useAdminQueryPolling } from '../../shared/composables/useAdminQueryPolling'
import { useAdminMonitorStream } from './useAdminMonitorStream'

export function useAdminMonitorData() {
  const stream = useAdminMonitorStream()
  const polling = useAdminQueryPolling({
    visibleInterval: 10_000,
    hiddenInterval: false,
    isEnabled: () => !stream.isConnected.value,
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
    monitorStreamStatus: stream.status,
  }
}
