import { computed, watch } from 'vue'
import { useGetAdminMonitor } from '../../../../shared/api/generated-query/admin-monitor/admin-monitor'
import { useAdminQueryPolling } from '../../shared/composables/useAdminQueryPolling'
import { mockMonitorData, mockMonitorStats, useAdminMockMode } from '../../shared/lib/adminMockData'
import { useTaskMonitorStream } from './useTaskMonitorStream'

export function useTaskRuntimeMonitor() {
  const isMockMode = useAdminMockMode()

  if (isMockMode.value) {
    return {
      monitorData: computed(() => mockMonitorData),
      monitorStats: computed(() => mockMonitorStats),
      monitorStreamStatus: computed(() => 'connected'),
    }
  }

  const stream = useTaskMonitorStream()
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

  watch(
    stream.status,
    (status) => {
      if (status === 'connected') return
      stream.snapshot.value = null
      if (polling.isPageVisible.value && status !== 'connecting') void monitorQuery.refetch()
    },
    { flush: 'sync' },
  )

  watch(
    () => monitorQuery.data.value,
    (data) => {
      if (data && !stream.isConnected.value) stream.snapshot.value = null
    },
  )

  const monitorData = computed(
    () =>
      (stream.isConnected.value ? stream.snapshot.value?.data?.monitor : null) ||
      monitorQuery.data.value?.data?.monitor ||
      null,
  )
  const monitorStats = computed(
    () =>
      (stream.isConnected.value ? stream.snapshot.value?.data?.stats : null) ||
      monitorQuery.data.value?.data?.stats ||
      null,
  )

  return {
    monitorData,
    monitorStats,
    monitorStreamStatus: stream.status,
  }
}
