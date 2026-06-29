import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useUsageStatisticsData } from './useUsageStatisticsData'

const mockState = vi.hoisted(() => ({
  statisticsReturns: [] as unknown[],
}))

vi.mock('./statisticsResource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./statisticsResource')>()),
  useGetAdminStatistics: vi.fn(() => mockState.statisticsReturns.shift()),
}))

beforeEach(() => {
  mockState.statisticsReturns = [makeStatisticsQuery(), makeStatisticsQuery()]
})

describe('useUsageStatisticsData', () => {
  it('exposes statistics API errors so the page does not render missing data as zeroes', () => {
    const error = new Error('statistics failed')
    mockState.statisticsReturns = [
      makeStatisticsQuery({ error, isError: true }),
      makeStatisticsQuery(),
    ]

    const statistics = useUsageStatisticsData()

    expect(statistics.adminStatistics.value).toBeNull()
    expect(statistics.isStatisticsError.value).toBe(true)
    expect(statistics.statisticsError.value).toBe(error)
    expect(statistics.isStatisticsSettled.value).toBe(true)
  })
})

function makeStatisticsQuery(options: { error?: Error; isError?: boolean } = {}) {
  return {
    data: ref(undefined),
    error: ref(options.error ?? null),
    isError: ref(options.isError ?? false),
    isFetched: ref(false),
  }
}
