import {
  GetAdminStatisticsRange,
  type GetAdminStatisticsRange as GetAdminStatisticsRangeValue,
} from '../../../../shared/api/generated-query/model'
import {
  buildChartTimeBuckets,
  shouldShowChartTimeTick,
  type ChartTimeBucket,
  type ChartTimeTickDensity,
} from '../../../../shared/lib/chartTimeRange'

export type AdminTimeRangeOption = {
  label: string
  key: GetAdminStatisticsRangeValue
  days: number
  bucketHours?: number
}

export type AdminTimeRangeBucket = ChartTimeBucket

const adminTimeRangeOptions: readonly AdminTimeRangeOption[] = [
  { label: '日', key: GetAdminStatisticsRange['1d'], days: 1, bucketHours: 4 },
  { label: '周', key: GetAdminStatisticsRange['7d'], days: 7 },
  { label: '月', key: GetAdminStatisticsRange['30d'], days: 30 },
  { label: '年', key: GetAdminStatisticsRange['1y'], days: 365 },
]

const adminTimeRangeByKey = new Map(adminTimeRangeOptions.map((option) => [option.key, option]))

export function pickAdminTimeRangeOptions(
  ranges: readonly GetAdminStatisticsRangeValue[],
): AdminTimeRangeOption[] {
  const allowed = new Set(ranges)
  return adminTimeRangeOptions.filter((option) => allowed.has(option.key))
}

function getAdminTimeRangeOption(range: GetAdminStatisticsRangeValue): AdminTimeRangeOption {
  return adminTimeRangeByKey.get(range) ?? adminTimeRangeByKey.get(GetAdminStatisticsRange['7d'])!
}

export function buildAdminTimeRangeBuckets(
  range: GetAdminStatisticsRangeValue,
  now = new Date(),
): AdminTimeRangeBucket[] {
  const option = getAdminTimeRangeOption(range)
  return buildChartTimeBuckets({
    days: option.days,
    now,
    bucketHours: option.bucketHours,
  })
}

export function shouldShowAdminTimeRangeTick(input: {
  index: number
  count: number
  range: GetAdminStatisticsRangeValue
  label?: string
}): boolean {
  return shouldShowChartTimeTick({
    index: input.index,
    count: input.count,
    label: input.label,
    density: getAdminTimeRangeTickDensity(input.range),
  })
}

function getAdminTimeRangeTickDensity(range: GetAdminStatisticsRangeValue): ChartTimeTickDensity {
  if (range === GetAdminStatisticsRange['30d']) return { mode: 'interval', interval: 6 }
  if (range === GetAdminStatisticsRange['1y']) return { mode: 'month-start' }
  return { mode: 'all' }
}
