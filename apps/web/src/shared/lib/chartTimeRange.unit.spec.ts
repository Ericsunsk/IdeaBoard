import { describe, expect, it } from 'vitest'
import { buildChartTimeBuckets, shouldShowChartTimeTick } from './chartTimeRange'

describe('chart time range helpers', () => {
  it('builds inclusive daily buckets ending on the current local date', () => {
    expect(
      buildChartTimeBuckets({
        days: 7,
        now: new Date(2026, 6, 6, 12),
      }),
    ).toEqual([
      { date: '2026-06-30', label: '06/30' },
      { date: '2026-07-01', label: '07/01' },
      { date: '2026-07-02', label: '07/02' },
      { date: '2026-07-03', label: '07/03' },
      { date: '2026-07-04', label: '07/04' },
      { date: '2026-07-05', label: '07/05' },
      { date: '2026-07-06', label: '07/06' },
    ])
  })

  it('builds intraday buckets with caller-owned date and label formatting', () => {
    expect(
      buildChartTimeBuckets({
        days: 1,
        now: new Date(2026, 6, 6, 12),
        bucketHours: 6,
        formatDateKey: () => 'current-day',
        formatIntradayLabel: (startHour) => `slot-${startHour}`,
      }),
    ).toEqual([
      { date: 'current-dayT00', label: 'slot-0' },
      { date: 'current-dayT06', label: 'slot-6' },
      { date: 'current-dayT12', label: 'slot-12' },
      { date: 'current-dayT18', label: 'slot-18' },
    ])
  })

  it('keeps endpoints while applying caller-owned interval or month-start density', () => {
    const showMonthlyTick = (index: number) =>
      shouldShowChartTimeTick({
        index,
        count: 30,
        density: { mode: 'interval', interval: 6 },
      })

    expect([0, 1, 6, 29].map(showMonthlyTick)).toEqual([true, false, true, true])
    expect(
      shouldShowChartTimeTick({
        index: 31,
        count: 365,
        label: '2026-02-01',
        density: { mode: 'month-start' },
      }),
    ).toBe(true)
    expect(
      shouldShowChartTimeTick({
        index: 32,
        count: 365,
        label: '02/02',
        density: { mode: 'month-start' },
      }),
    ).toBe(false)
    expect(
      shouldShowChartTimeTick({
        index: 3,
        count: 7,
        density: { mode: 'month-start' },
      }),
    ).toBe(true)
  })
})
