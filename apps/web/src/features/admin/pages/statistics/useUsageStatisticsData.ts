import type { AdminStatisticsResponse } from '../../../../shared/api/adminStatistics'
import { useGetAdminStatistics } from '../../../../shared/api/generated-query/admin-statistics/admin-statistics'
import type { AdminStatisticsRange } from '../../../../shared/query/adminQueries'

type RangeOption = { label: string; key: AdminStatisticsRange }

export function useUsageStatisticsData() {
  const timeRange = ref<AdminStatisticsRange>('7d')
  const rangeOptions: RangeOption[] = [
    { label: '周', key: '7d' },
    { label: '月', key: '30d' },
    { label: '年', key: '1y' },
  ]

  const statisticsParams = computed(() => ({ range: timeRange.value }))
  const statisticsQuery = useGetAdminStatistics(statisticsParams, {
    query: {
      placeholderData: (previous) => previous,
      staleTime: 60_000,
    },
  })

  const adminStatistics = computed<AdminStatisticsResponse | null>(
    () => statisticsQuery.data.value?.data ?? null,
  )

  return {
    adminStatistics,
    timeRange,
    rangeOptions,
  }
}
