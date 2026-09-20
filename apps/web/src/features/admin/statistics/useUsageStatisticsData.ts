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

export function useUsageStatisticsData() {
  const timeRange = ref<GetAdminStatisticsRangeValue>(GetAdminStatisticsRange['7d'])
  const rangeOptions: AdminTimeRangeOption[] = pickAdminTimeRangeOptions(usageTrendRanges)

  const statisticsParams = computed(() => ({ range: timeRange.value }))
  const statisticsResource = useAdminStatisticsResource(statisticsParams, {
    keepPreviousData: true,
  })

  const adminStatistics = computed<AdminStatistics | null>(
    () => statisticsResource.statistics.value,
  )
  const isStatisticsSettled = computed(() => statisticsResource.isSettled.value)
  const isStatisticsError = computed(() => statisticsResource.isError.value)
  const statisticsError = computed(() => statisticsResource.error.value)

  return {
    adminStatistics,
    isStatisticsSettled,
    isStatisticsError,
    statisticsError,
    timeRange,
    rangeOptions,
  }
}
