import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { renderWithUser, screen, waitFor, within } from '@/test/render'
import type { AdminStatistics } from '../../session/resource'
import AdminStatisticsPage from './AdminStatisticsPage.vue'

vi.mock('chart.js/auto', () => ({
  default: class {
    update() {}
    destroy() {}
  },
}))

describe('AdminStatisticsPage chart ranges', () => {
  it('changes each chart period independently and reuses requests for matching periods', async () => {
    const requests: string[] = []
    useMswHandlers(
      http.get('/api/control/v1/platform/statistics', ({ request }) => {
        requests.push(new URL(request.url).searchParams.get('range') ?? '')
        return HttpResponse.json({ data: makeStatistics() })
      }),
    )
    const { user } = renderStatisticsPage()
    await waitFor(() => expect(requests).toEqual(['7d']))
    const trend = screen.getByRole('radiogroup', { name: '调用量与消耗趋势' })
    const quality = screen.getByRole('radiogroup', { name: '服务质量趋势' })

    await user.click(within(trend).getByRole('radio', { name: '月' }))
    expect(within(trend).getByRole('radio', { name: '月' })).toHaveAttribute('aria-checked', 'true')
    expect(within(quality).getByRole('radio', { name: '周' })).toHaveAttribute(
      'aria-checked',
      'true',
    )
    await waitFor(() => expect(requests).toEqual(['7d', '30d']))

    await user.click(within(quality).getByRole('radio', { name: '年' }))
    expect(within(trend).getByRole('radio', { name: '月' })).toHaveAttribute('aria-checked', 'true')
    await waitFor(() => expect(requests).toEqual(['7d', '30d', '1y']))

    await user.click(within(quality).getByRole('radio', { name: '月' }))
    expect(within(trend).getByRole('radio', { name: '月' })).toHaveAttribute('aria-checked', 'true')
    expect(within(quality).getByRole('radio', { name: '月' })).toHaveAttribute(
      'aria-checked',
      'true',
    )
    expect(requests).toEqual(['7d', '30d', '1y'])
  })

  it('keeps the other chart and overview visible when one period fails to load', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/statistics', ({ request }) => {
        if (new URL(request.url).searchParams.get('range') === '30d') {
          return HttpResponse.json(
            { error: { code: 'internal_error', message: '统计查询失败', request_id: 'test' } },
            { status: 500 },
          )
        }
        return HttpResponse.json({ data: makeStatistics() })
      }),
    )
    const { user } = renderStatisticsPage()
    const trend = screen.getByRole('radiogroup', { name: '调用量与消耗趋势' })
    const quality = screen.getByRole('radiogroup', { name: '服务质量趋势' })
    await user.click(within(trend).getByRole('radio', { name: '月' }))
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.getByText('今日调用')).toBeVisible()
    expect(screen.getByText('模型风险矩阵')).toBeVisible()
    expect(within(quality).getByRole('radio', { name: '周' })).toHaveAttribute(
      'aria-checked',
      'true',
    )

    await user.click(within(trend).getByRole('radio', { name: '周' }))
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  })
})

function renderStatisticsPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return renderWithUser(AdminStatisticsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
}

function makeStatistics(): AdminStatistics {
  return {
    summary: {
      total_calls: 0,
      today_calls: 0,
      today_success: 0,
      today_failed: 0,
      today_cancelled: 0,
      yesterday_calls: 0,
      yesterday_success: 0,
      yesterday_failed: 0,
      yesterday_cancelled: 0,
      this_month_calls: 0,
      this_month_cancelled: 0,
      pending: 0,
      running: 0,
      completed: 0,
      failed: 0,
      cancelled: 0,
    },
    trend: [],
    model_health: [],
    quality_trend: [],
    slow_models: [],
    bottleneck_sources: [],
    failure_categories: [],
    backfill_status: {
      range_start: '',
      range_end: '',
      missing_days: 0,
      backfilled_days: 0,
    },
  }
}
