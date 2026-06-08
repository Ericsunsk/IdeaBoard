import type { GetAdminStatisticsRange } from '../../../../shared/api/generated-query/model'

export type AdminTimeRangeOption = {
  label: string
  key: GetAdminStatisticsRange
  days: number
  bucketHours?: number
}

export type AdminTimeRangeBucket = {
  date: string
  label: string
}

const adminTimeRangeOptions: readonly AdminTimeRangeOption[] = [
  { label: '日', key: '1d', days: 1, bucketHours: 4 },
  { label: '周', key: '7d', days: 7 },
  { label: '月', key: '30d', days: 30 },
  { label: '年', key: '1y', days: 365 },
]

const adminTimeRangeByKey = new Map(adminTimeRangeOptions.map((option) => [option.key, option]))

export function pickAdminTimeRangeOptions(
  ranges: readonly GetAdminStatisticsRange[],
): AdminTimeRangeOption[] {
  const allowed = new Set(ranges)
  return adminTimeRangeOptions.filter((option) => allowed.has(option.key))
}

function getAdminTimeRangeOption(range: GetAdminStatisticsRange): AdminTimeRangeOption {
  return adminTimeRangeByKey.get(range) ?? adminTimeRangeByKey.get('7d')!
}

export function buildAdminTimeRangeBuckets(
  range: GetAdminStatisticsRange,
  now = new Date(),
): AdminTimeRangeBucket[] {
  const option = getAdminTimeRangeOption(range)
  if (option.bucketHours) return buildIntradayBuckets(now, option.bucketHours)

  const days = option.days
  return Array.from({ length: days }, (_, idx) => {
    const d = new Date(now)
    d.setDate(now.getDate() - (days - 1 - idx))
    const date = formatDateKey(d)
    return {
      date,
      label: date.slice(5).replace('-', '/'),
    }
  })
}

export function shouldShowAdminTimeRangeTick(input: {
  index: number
  count: number
  range: GetAdminStatisticsRange
  label?: string
}): boolean {
  const { index, count, range, label } = input
  if (count <= 7) return true
  if (range === '30d') return index === 0 || index === count - 1 || index % 6 === 0
  if (range === '1y') return index === 0 || index === count - 1 || isMonthStartLabel(label)
  return true
}

function isMonthStartLabel(label: string | undefined): boolean {
  const value = (label || '').trim()
  if (!value) return false
  const mmdd = /^(\d{2})\/(\d{2})$/.exec(value)
  if (mmdd) return mmdd[2] === '01'
  const ymd = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (ymd) return ymd[3] === '01'
  return false
}

function buildIntradayBuckets(now: Date, bucketHours: number): AdminTimeRangeBucket[] {
  const bucketCount = Math.ceil(24 / bucketHours)
  const dayStart = new Date(now)
  dayStart.setHours(0, 0, 0, 0)
  const date = formatDateKey(dayStart)

  return Array.from({ length: bucketCount }, (_, idx) => {
    const startHour = idx * bucketHours
    return {
      date: `${date}T${pad2(startHour)}`,
      label: `${pad2(startHour)}:00`,
    }
  })
}

function formatDateKey(date: Date): string {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

function pad2(value: number): string {
  return value.toString().padStart(2, '0')
}
