import { computed, ref, watch } from 'vue'
import { useAdminMonitorResource, type MonitorResponse } from '../infrastructure/resource'
import { useQueryPolling } from '../../shared/composables/useQueryPolling'
import { useAdminMonitorStream } from './useAdminMonitorStream'

export function useAdminTaskRuntimeSnapshot() {
  const stream = useAdminMonitorStream()
  const lastGoodSnapshot = ref<MonitorResponse | null>(null)
  const polling = useQueryPolling({
    visibleInterval: 10_000,
    hiddenInterval: false,
    isEnabled: () => !stream.isConnected.value,
  })

  const monitorQuery = useAdminMonitorResource({
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
  const monitorData = computed(() => activeSnapshot.value?.monitor || null)
  const monitorStats = computed(() => activeSnapshot.value?.stats || null)
  const isStale = computed(() => !liveSnapshot.value && !!lastGoodSnapshot.value)
  const isError = computed(
    () => !activeSnapshot.value && !stream.isConnected.value && monitorQuery.isError.value,
  )
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
