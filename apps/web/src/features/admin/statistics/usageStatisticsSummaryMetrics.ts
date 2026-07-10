import { computed, type Ref } from 'vue'
import type { AdminStatistics } from './statisticsResource'
import {
  buildDeltaFoot,
  formatPercentValue,
  formatSignedPercentPoint,
} from './usageStatisticsChartBase'

export function useUsageStatisticsSummaryMetrics(adminStatistics: Ref<AdminStatistics | null>) {
  const todayCalls = computed(() => adminStatistics.value?.summary?.today_calls || 0)
  const todaySuccess = computed(() => adminStatistics.value?.summary?.today_success || 0)
  const todayFailed = computed(() => adminStatistics.value?.summary?.today_failed || 0)
  const monthCalls = computed(() => adminStatistics.value?.summary?.this_month_calls || 0)
  const yesterdayCalls = computed(() => adminStatistics.value?.summary?.yesterday_calls || 0)
  const yesterdaySuccess = computed(() => adminStatistics.value?.summary?.yesterday_success || 0)
  const yesterdayFailed = computed(() => adminStatistics.value?.summary?.yesterday_failed || 0)

  const todayCallsFoot = computed(() =>
    buildDeltaFoot(todayCalls.value, yesterdayCalls.value, '昨日'),
  )
  const todayFailedFoot = computed(() =>
    buildDeltaFoot(todayFailed.value, yesterdayFailed.value, '昨日'),
  )
  const todayAttemptedCalls = computed(() => todaySuccess.value + todayFailed.value)
  const todaySuccessRate = computed(() =>
    todayAttemptedCalls.value === 0
      ? '0.0%'
      : `${((todaySuccess.value / todayAttemptedCalls.value) * 100).toFixed(1)}%`,
  )
  const yesterdayAttemptedCalls = computed(() => yesterdaySuccess.value + yesterdayFailed.value)
  const todaySuccessRateFoot = computed(() => {
    if (yesterdayAttemptedCalls.value === 0) return '昨日无可比数据'
    const yesterdayRate = yesterdaySuccess.value / yesterdayAttemptedCalls.value
    const currentRate =
      todayAttemptedCalls.value === 0 ? 0 : todaySuccess.value / todayAttemptedCalls.value
    return `${formatSignedPercentPoint((currentRate - yesterdayRate) * 100)} 较昨日`
  })
  const monthCallsFoot = computed(() => {
    const totalCalls = adminStatistics.value?.summary?.total_calls || 0
    if (totalCalls <= 0) return '累计无调用'
    return `占总调用 ${formatPercentValue(monthCalls.value, totalCalls)}`
  })

  return {
    todayCalls,
    todayFailed,
    todayCallsFoot,
    todayFailedFoot,
    todaySuccessRate,
    todaySuccessRateFoot,
    monthCalls,
    monthCallsFoot,
  }
}
