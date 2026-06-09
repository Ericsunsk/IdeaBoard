import { useGetAdminStatistics } from '../../../../shared/api/generated-query/admin-statistics/admin-statistics'
import {
  GetAdminStatisticsRange,
  type AdminStatistics,
  type GetAdminStatisticsRange as GetAdminStatisticsRangeValue,
} from '../../../../shared/api/generated-query/model'
import {
  pickAdminTimeRangeOptions,
  type AdminTimeRangeOption,
} from '../../shared/lib/adminTimeRange'
import { createMockAdminStatistics, useAdminMockMode } from '../../shared/lib/adminMockData'

const usageTrendRanges: readonly GetAdminStatisticsRangeValue[] = [
  GetAdminStatisticsRange['7d'],
  GetAdminStatisticsRange['30d'],
  GetAdminStatisticsRange['1y'],
]
const qualityTrendRanges: readonly GetAdminStatisticsRangeValue[] = [
  GetAdminStatisticsRange['1d'],
  GetAdminStatisticsRange['7d'],
  GetAdminStatisticsRange['30d'],
]

export function useUsageStatisticsData() {
  const isMockMode = useAdminMockMode()
  const timeRange = ref<GetAdminStatisticsRangeValue>(GetAdminStatisticsRange['7d'])
  const qualityTimeRange = ref<GetAdminStatisticsRangeValue>(GetAdminStatisticsRange['7d'])
  const rangeOptions: AdminTimeRangeOption[] = pickAdminTimeRangeOptions(usageTrendRanges)
  const qualityRangeOptions: AdminTimeRangeOption[] = pickAdminTimeRangeOptions(qualityTrendRanges)

  const statisticsParams = computed(() => ({ range: timeRange.value }))
  const statisticsQuery = useGetAdminStatistics(statisticsParams, {
    query: {
      enabled: computed(() => !isMockMode.value),
      placeholderData: (previous) => previous,
      staleTime: 60_000,
    },
  })
  const qualityStatisticsParams = computed(() => ({ range: qualityTimeRange.value }))
  const isQualityRangeShared = computed(() => qualityTimeRange.value === timeRange.value)
  const qualityStatisticsQuery = useGetAdminStatistics(qualityStatisticsParams, {
    query: {
      enabled: computed(() => !isMockMode.value && !isQualityRangeShared.value),
      placeholderData: (previous) => previous,
      staleTime: 60_000,
    },
  })

  const mockAdminStatistics = computed(() => createMockAdminStatistics(timeRange.value))
  const mockQualityStatistics = computed(() => createMockAdminStatistics(qualityTimeRange.value))
  const adminStatistics = computed<AdminStatistics | null>(() =>
    isMockMode.value ? mockAdminStatistics.value : (statisticsQuery.data.value?.data ?? null),
  )
  const qualityStatistics = computed<AdminStatistics | null>(() =>
    isMockMode.value
      ? mockQualityStatistics.value
      : isQualityRangeShared.value
        ? adminStatistics.value
        : (qualityStatisticsQuery.data.value?.data ?? null),
  )
  const isStatisticsSettled = computed(
    () => isMockMode.value || statisticsQuery.isFetched.value || statisticsQuery.isError.value,
  )
  const isQualityStatisticsSettled = computed(() =>
    isMockMode.value
      ? true
      : isQualityRangeShared.value
        ? isStatisticsSettled.value
        : qualityStatisticsQuery.isFetched.value || qualityStatisticsQuery.isError.value,
  )

  return {
    adminStatistics,
    qualityStatistics,
    isStatisticsSettled,
    isQualityStatisticsSettled,
    timeRange,
    qualityTimeRange,
    rangeOptions,
    qualityRangeOptions,
  }
}
