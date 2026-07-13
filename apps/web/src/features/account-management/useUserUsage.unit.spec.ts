import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser } from '../../test/render'
import type { AccountUsage, UserUsageRange } from './userUsageResource'
import { useUserUsage } from './useUserUsage'

const fetchUserUsageMock = vi.hoisted(() => vi.fn())

vi.mock('./userUsageResource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./userUsageResource')>()
  return { ...actual, fetchUserUsage: fetchUserUsageMock }
})

const UsageHost = defineComponent({
  setup() {
    const range = ref<UserUsageRange>('30d')
    return { range, ...useUserUsage(range, true) }
  },
  template: `
    <button type="button" @click="range = '7d'">周</button>
    <button type="button" @click="range = '30d'">月</button>
    <button type="button" @click="range = '1y'">年</button>
    <button type="button" @click="retry()">重试</button>
    <output data-testid="selected-range">{{ range }}</output>
    <output data-testid="display-range">{{ displayRange || '' }}</output>
    <output data-testid="last-credit">{{ usage?.trend[usage.trend.length - 1]?.credits ?? '' }}</output>
    <output data-testid="request-state">{{ isInitialPending ? 'initial' : isSwitching ? 'switching' : error ? 'error' : 'ready' }}</output>
  `,
})

describe('useUserUsage', () => {
  beforeEach(() => {
    fetchUserUsageMock.mockReset()
  })

  it('keeps the last successful range when a switch fails and retries only the selected range', async () => {
    fetchUserUsageMock
      .mockResolvedValueOnce(buildUsage('30d', 30))
      .mockRejectedValueOnce(new Error('周趋势加载失败'))
      .mockResolvedValueOnce(buildUsage('7d', 7))
    const { user } = renderUsageHost()

    await waitFor(() => expect(screen.getByTestId('last-credit')).toHaveTextContent('30'))
    await user.click(screen.getByRole('button', { name: '周' }))

    await waitFor(() => expect(screen.getByTestId('request-state')).toHaveTextContent('error'))
    expect(screen.getByTestId('selected-range')).toHaveTextContent('7d')
    expect(screen.getByTestId('display-range')).toHaveTextContent('30d')
    expect(screen.getByTestId('last-credit')).toHaveTextContent('30')
    expect(fetchUserUsageMock).toHaveBeenCalledTimes(2)

    await user.click(screen.getByRole('button', { name: '重试' }))
    await waitFor(() => expect(screen.getByTestId('display-range')).toHaveTextContent('7d'))
    expect(screen.getByTestId('last-credit')).toHaveTextContent('7')
    expect(fetchUserUsageMock).toHaveBeenCalledTimes(3)
  })

  it('aborts an obsolete switch and never lets its late response replace the selected range', async () => {
    const week = deferred<AccountUsage>()
    const year = deferred<AccountUsage>()
    fetchUserUsageMock
      .mockResolvedValueOnce(buildUsage('30d', 30))
      .mockImplementationOnce((_range: UserUsageRange, signal?: AbortSignal) => {
        expect(signal).toBeInstanceOf(AbortSignal)
        return week.promise
      })
      .mockImplementationOnce((_range: UserUsageRange, signal?: AbortSignal) => {
        expect(signal).toBeInstanceOf(AbortSignal)
        return year.promise
      })
    const { user } = renderUsageHost()

    await waitFor(() => expect(screen.getByTestId('display-range')).toHaveTextContent('30d'))
    await user.click(screen.getByRole('button', { name: '周' }))
    await waitFor(() => expect(fetchUserUsageMock).toHaveBeenCalledTimes(2))
    const weekSignal = fetchUserUsageMock.mock.calls[1]?.[1] as AbortSignal
    await user.click(screen.getByRole('button', { name: '年' }))
    await waitFor(() => expect(fetchUserUsageMock).toHaveBeenCalledTimes(3))
    expect(weekSignal.aborted).toBe(true)

    week.resolve(buildUsage('7d', 7))
    await Promise.resolve()
    expect(screen.getByTestId('display-range')).toHaveTextContent('30d')
    expect(screen.getByTestId('last-credit')).toHaveTextContent('30')

    year.resolve(buildUsage('1y', 365))
    await waitFor(() => expect(screen.getByTestId('display-range')).toHaveTextContent('1y'))
    expect(screen.getByTestId('last-credit')).toHaveTextContent('365')
  })

  it('uses a 30 second stale window and refetches stale data when the window regains focus', async () => {
    const initial = buildUsage('30d', 30)
    fetchUserUsageMock.mockResolvedValue(initial)
    const { queryClient } = renderUsageHost()

    await waitFor(() => expect(fetchUserUsageMock).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByTestId('request-state')).toHaveTextContent('ready'))
    const queryKey = ['user', 'usage', 'current', '30d'] as const
    const query = queryClient.getQueryCache().find({ queryKey })
    expect(query?.options).toMatchObject({
      staleTime: 30_000,
      refetchOnWindowFocus: true,
      retry: false,
    })

    queryClient.getQueryCache().onFocus()
    await Promise.resolve()
    expect(fetchUserUsageMock).toHaveBeenCalledTimes(1)

    queryClient.setQueryData(queryKey, queryClient.getQueryData(queryKey), {
      updatedAt: Date.now() - 31_000,
    })
    await waitFor(() => expect(query?.isStale()).toBe(true))
    queryClient.getQueryCache().onFocus()
    await waitFor(() => expect(fetchUserUsageMock).toHaveBeenCalledTimes(2))
  })
})

function renderUsageHost() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { gcTime: Number.POSITIVE_INFINITY } },
  })
  const result = renderWithUser(UsageHost, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
  return { ...result, queryClient }
}

function buildUsage(range: UserUsageRange, credits: number): AccountUsage {
  const days = range === '7d' ? 7 : range === '30d' ? 30 : 365
  return {
    timezone: 'Asia/Shanghai',
    lifetime: {
      total_credits: 999,
      peak_daily_credits: 100,
      average_daily_credits: 10,
      task_count: 20,
      active_days: 5,
    },
    trend: Array.from({ length: days }, (_, index) => ({
      date: `2026-01-${String((index % 28) + 1).padStart(2, '0')}`,
      credits: index === days - 1 ? credits : 0,
    })),
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}
