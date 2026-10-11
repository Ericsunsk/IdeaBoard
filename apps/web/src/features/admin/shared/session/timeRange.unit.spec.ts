import { describe, expect, it } from 'vitest'
import { GetAdminStatisticsRange } from '../../statistics/session/resource'
import { buildAdminTimeRangeBuckets, shouldShowAdminTimeRangeTick } from './timeRange'

describe('admin time range adapter', () => {
  it('maps generated Admin ranges to shared bucket and tick strategies', () => {
    expect(
      buildAdminTimeRangeBuckets(GetAdminStatisticsRange['1d'], new Date(2026, 6, 6, 12)),
    ).toEqual([
      { date: '2026-07-06T00', label: '00:00' },
      { date: '2026-07-06T04', label: '04:00' },
      { date: '2026-07-06T08', label: '08:00' },
      { date: '2026-07-06T12', label: '12:00' },
      { date: '2026-07-06T16', label: '16:00' },
      { date: '2026-07-06T20', label: '20:00' },
    ])
    expect(
      shouldShowAdminTimeRangeTick({
        index: 6,
        count: 30,
        range: GetAdminStatisticsRange['30d'],
      }),
    ).toBe(true)
    expect(
      shouldShowAdminTimeRangeTick({
        index: 1,
        count: 30,
        range: GetAdminStatisticsRange['30d'],
      }),
    ).toBe(false)
    expect(
      shouldShowAdminTimeRangeTick({
        index: 31,
        count: 365,
        range: GetAdminStatisticsRange['1y'],
        label: '2026-02-01',
      }),
    ).toBe(true)
  })
})
