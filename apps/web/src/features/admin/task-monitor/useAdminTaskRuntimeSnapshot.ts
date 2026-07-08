import { computed, ref, watch } from 'vue'
import { useGetAdminMonitor } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'
import type { ApiResponseMonitor } from '../../../shared/api/generated-query/model'
import { useAdminQueryPolling } from '../shared/composables/useAdminQueryPolling'
import { useAdminTaskMonitorStream } from './useAdminTaskMonitorStream'

export function useAdminTaskRuntimeSnapshot() {
  const stream = useAdminTaskMonitorStream()
  const lastGoodSnapshot = ref<ApiResponseMonitor | null>(null)
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
      if (polling.isPageVisible.value && status !== 'connecting') void monitorQuery.refetch()
    },
    { flush: 'sync' },
  )

  watch(
    stream.snapshot,
    (data) => {
      if (data) lastGoodSnapshot.value = data
    },
    { immediate: true },
  )

  watch(
    () => monitorQuery.data.value,
    (data) => {
      if (data) lastGoodSnapshot.value = data
    },
    { immediate: true },
  )

  const liveSnapshot = computed(
    () =>
      (stream.isConnected.value ? stream.snapshot.value : null) ||
      (!monitorQuery.isError.value ? monitorQuery.data.value : null) ||
      null,
  )
  const activeSnapshot = computed(() => liveSnapshot.value || lastGoodSnapshot.value)
  const monitorData = computed(() => activeSnapshot.value?.data?.monitor || null)
  const monitorStats = computed(() => activeSnapshot.value?.data?.stats || null)
  const isStale = computed(() => !liveSnapshot.value && !!lastGoodSnapshot.value)
  const isError = computed(() => !activeSnapshot.value && !stream.isConnected.value && monitorQuery.isError.value)
  const error = computed(() => (isError.value ? (monitorQuery.error.value ?? null) : null))

  async function retry() {
    stream.reconnect()
    await monitorQuery.refetch()
  }

  return {
    monitorData,
    monitorStats,
    monitorStreamStatus: stream.status,
    isStale,
    isError,
    error,
    retry,
  }
}
