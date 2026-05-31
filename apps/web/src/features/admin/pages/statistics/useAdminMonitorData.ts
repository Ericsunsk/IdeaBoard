import { useGetAdminMonitor } from '../../../../shared/api/generated-query/admin-monitor/admin-monitor'

export function useAdminMonitorData() {
  const monitorQuery = useGetAdminMonitor({
    query: {
      refetchInterval: 10_000,
    },
  })

  const monitorData = computed(() => monitorQuery.data.value?.data?.monitor || null)

  return {
    monitorData,
  }
}
