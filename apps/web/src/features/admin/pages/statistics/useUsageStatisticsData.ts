import { computed, ref } from 'vue'
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
  const timeRange = ref<GetAdminStatisticsRangeValue>(GetAdminStatisticsRange['7d'])
  const qualityTimeRange = ref<GetAdminStatisticsRangeValue>(GetAdminStatisticsRange['7d'])
  const rangeOptions: AdminTimeRangeOption[] = pickAdminTimeRangeOptions(usageTrendRanges)
  const qualityRangeOptions: AdminTimeRangeOption[] = pickAdminTimeRangeOptions(qualityTrendRanges)

  const statisticsParams = computed(() => ({ range: timeRange.value }))
  const statisticsQuery = useGetAdminStatistics(statisticsParams, {
    query: {
      placeholderData: (previous) => previous,
      staleTime: 60_000,
    },
  })
  const qualityStatisticsParams = computed(() => ({ range: qualityTimeRange.value }))
  const isQualityRangeShared = computed(() => qualityTimeRange.value === timeRange.value)
  const qualityStatisticsQuery = useGetAdminStatistics(qualityStatisticsParams, {
    query: {
      enabled: computed(() => !isQualityRangeShared.value),
      placeholderData: (previous) => previous,
      staleTime: 60_000,
    },
  })

  const adminStatistics = computed<AdminStatistics | null>(() =>
    statisticsQuery.data.value?.data ?? null,
  )
  const qualityStatistics = computed<AdminStatistics | null>(() =>
    isQualityRangeShared.value
      ? adminStatistics.value
      : (qualityStatisticsQuery.data.value?.data ?? null),
  )
  const isStatisticsSettled = computed(
    () => statisticsQuery.isFetched.value || statisticsQuery.isError.value,
  )
  const isQualityStatisticsSettled = computed(() =>
    isQualityRangeShared.value
      ? isStatisticsSettled.value
      : qualityStatisticsQuery.isFetched.value || qualityStatisticsQuery.isError.value,
  )
  const isStatisticsError = computed(() => statisticsQuery.isError.value)
  const statisticsError = computed(() =>
    isStatisticsError.value ? (statisticsQuery.error.value ?? null) : null,
  )
  const isQualityStatisticsError = computed(() =>
    isQualityRangeShared.value ? isStatisticsError.value : qualityStatisticsQuery.isError.value,
  )
  const qualityStatisticsError = computed(() =>
    isQualityStatisticsError.value
      ? isQualityRangeShared.value
        ? statisticsError.value
        : (qualityStatisticsQuery.error.value ?? null)
      : null,
  )

  return {
    adminStatistics,
    qualityStatistics,
    isStatisticsSettled,
    isQualityStatisticsSettled,
    isStatisticsError,
    statisticsError,
    isQualityStatisticsError,
    qualityStatisticsError,
    timeRange,
    qualityTimeRange,
    rangeOptions,
    qualityRangeOptions,
  }
}
