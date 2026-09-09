import { computed, ref } from 'vue'
import {
  GetAdminStatisticsRange,
  type AdminStatistics,
  type GetAdminStatisticsRangeValue,
  useAdminStatisticsResource,
} from './resource'
import { pickAdminTimeRangeOptions, type AdminTimeRangeOption } from '../shared/lib/timeRange'

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
  const statisticsResource = useAdminStatisticsResource(statisticsParams, {
    keepPreviousData: true,
  })
  const qualityStatisticsParams = computed(() => ({ range: qualityTimeRange.value }))
  const isQualityRangeShared = computed(() => qualityTimeRange.value === timeRange.value)
  const qualityStatisticsResource = useAdminStatisticsResource(qualityStatisticsParams, {
    enabled: computed(() => !isQualityRangeShared.value),
    keepPreviousData: true,
  })

  const adminStatistics = computed<AdminStatistics | null>(
    () => statisticsResource.statistics.value,
  )
  const qualityStatistics = computed<AdminStatistics | null>(() =>
    isQualityRangeShared.value ? adminStatistics.value : qualityStatisticsResource.statistics.value,
  )
  const isStatisticsSettled = computed(() => statisticsResource.isSettled.value)
  const isQualityStatisticsSettled = computed(() =>
    isQualityRangeShared.value
      ? isStatisticsSettled.value
      : qualityStatisticsResource.isSettled.value,
  )
  const isStatisticsError = computed(() => statisticsResource.isError.value)
  const statisticsError = computed(() => statisticsResource.error.value)
  const isQualityStatisticsError = computed(() =>
    isQualityRangeShared.value ? isStatisticsError.value : qualityStatisticsResource.isError.value,
  )
  const qualityStatisticsError = computed(() =>
    isQualityStatisticsError.value
      ? isQualityRangeShared.value
        ? statisticsError.value
        : qualityStatisticsResource.error.value
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
