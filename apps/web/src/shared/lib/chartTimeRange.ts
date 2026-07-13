export type ChartTimeBucket = {
  date: string
  label: string
}

export type BuildChartTimeBucketsInput = {
  days: number
  now?: Date
  bucketHours?: number
  formatDateKey?: (date: Date) => string
  formatDailyLabel?: (date: string) => string
  formatIntradayLabel?: (startHour: number) => string
}

export type ChartTimeTickDensity =
  { mode: 'all' } | { mode: 'interval'; interval: number } | { mode: 'month-start' }

export type ChartTimeTickInput = {
  index: number
  count: number
  label?: string
  density: ChartTimeTickDensity
  maxVisibleCount?: number
}

export function buildChartTimeBuckets(input: BuildChartTimeBucketsInput): ChartTimeBucket[] {
  const now = input.now ?? new Date()
  const formatDate = input.formatDateKey ?? formatDateKey
  if (input.bucketHours) {
    return buildIntradayBuckets(now, input.bucketHours, formatDate, input.formatIntradayLabel)
  }

  return Array.from({ length: input.days }, (_, index) => {
    const dateValue = new Date(now)
    dateValue.setDate(now.getDate() - (input.days - 1 - index))
    const date = formatDate(dateValue)
    return {
      date,
      label: input.formatDailyLabel?.(date) ?? date.slice(5).replace('-', '/'),
    }
  })
}

export function shouldShowChartTimeTick(input: ChartTimeTickInput): boolean {
  if (input.index < 0 || input.index >= input.count || input.count <= 0) return false
  if (input.count <= (input.maxVisibleCount ?? 7) || input.density.mode === 'all') return true
  if (input.index === 0 || input.index === input.count - 1) return true
  if (input.density.mode === 'month-start') return isMonthStartLabel(input.label)

  const interval = Math.max(1, Math.floor(input.density.interval))
  return input.index % interval === 0
}

function buildIntradayBuckets(
  now: Date,
  bucketHours: number,
  formatDate: (date: Date) => string,
  formatLabel = (startHour: number) => `${pad2(startHour)}:00`,
): ChartTimeBucket[] {
  const bucketCount = Math.ceil(24 / bucketHours)
  const dayStart = new Date(now)
  dayStart.setHours(0, 0, 0, 0)
  const date = formatDate(dayStart)

  return Array.from({ length: bucketCount }, (_, index) => {
    const startHour = index * bucketHours
    return {
      date: `${date}T${pad2(startHour)}`,
      label: formatLabel(startHour),
    }
  })
}

function formatDateKey(date: Date): string {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
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

function pad2(value: number): string {
  return value.toString().padStart(2, '0')
}
