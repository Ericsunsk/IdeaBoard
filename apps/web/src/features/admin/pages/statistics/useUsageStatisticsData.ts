import { useGetAdminStatistics } from '../../../../shared/api/generated-query/admin-statistics/admin-statistics'
import type { AdminStatistics } from '../../../../shared/api/generated-query/model'
import type { AdminStatisticsRange } from '../../shared/queries/adminActions'

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

  const adminStatistics = computed<AdminStatistics | null>(
    () => statisticsQuery.data.value?.data ?? null,
  )

  return {
    adminStatistics,
    timeRange,
    rangeOptions,
  }
}
