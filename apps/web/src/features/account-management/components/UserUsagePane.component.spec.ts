import { cleanup } from '@testing-library/vue'
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import UserUsagePane from './UserUsagePane.vue'

const usageState = vi.hoisted(() => ({
  usage: {
    value: undefined as
      | {
          timezone: 'Asia/Shanghai'
          lifetime: {
            total_credits: number
            peak_daily_credits: number
            average_daily_credits: number
            task_count: number
            active_days: number
          }
          trend: { date: string; credits: number }[]
        }
      | undefined,
    __v_isRef: true,
  },
  isPending: { value: false, __v_isRef: true },
  error: { value: null as unknown, __v_isRef: true },
  refresh: vi.fn(),
}))

vi.mock('../useUserUsage', () => ({
  useUserUsage: () => usageState,
}))

const ButtonStub = defineComponent({
  template: '<button type="button"><slot /></button>',
})

const chartStub = {
  props: {
    data: { type: Object, required: true },
    options: { type: Object, required: true },
    canvasProps: { type: Object, required: true },
  },
  template:
    '<div data-testid="usage-trend-chart" :data-points="String(data.datasets[0].data.length)" :data-datasets="String(data.datasets.length)"><canvas v-bind="canvasProps" /></div>',
}

function renderUsagePane() {
  return renderWithUser(UserUsagePane, {
    props: { active: true },
    global: { stubs: { Button: ButtonStub, Chart: chartStub } },
  })
}

describe('UserUsagePane', () => {
  it('renders lifetime values in one consolidated summary panel', () => {
    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: 1234,
        peak_daily_credits: 500,
        average_daily_credits: 12.5,
        task_count: 9,
        active_days: 4,
      },
      trend: buildZeroTrend(),
    }
    usageState.isPending.value = false
    usageState.error.value = null

    renderUsagePane()

    expect(screen.getByRole('heading', { name: '用量统计' })).toBeVisible()
    expect(screen.getByRole('region', { name: '用量摘要' })).toHaveTextContent('1,234')
    expect(screen.getByRole('region', { name: '用量摘要' })).toHaveTextContent('12.5')
    expect(screen.getByText('调用次数')).toBeVisible()
    expect(screen.getByText('累计活跃天数')).toBeVisible()
  })

  it('keeps the responsive 2x2 plus spanning-row contract without viewport font sizing', () => {
    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: Number.MAX_SAFE_INTEGER,
        peak_daily_credits: 500,
        average_daily_credits: 12.5,
        task_count: 9,
        active_days: 4,
      },
      trend: buildZeroTrend(),
    }
    usageState.isPending.value = false
    usageState.error.value = null

    renderUsagePane()

    const panel = screen.getByRole('region', { name: '用量摘要' })
    const cells = panel.querySelectorAll('article')
    expect(cells).toHaveLength(5)
    expect(cells[0]).toHaveClass('max-[640px]:border-b')
    expect(cells[4]).toHaveClass('max-[640px]:col-span-2')
    const value = cells[0]?.querySelector('p')
    expect(value).toHaveClass('text-[34px]', 'max-[900px]:text-[30px]', 'max-[640px]:text-[28px]')
    expect(value).toHaveClass('break-all', 'whitespace-normal', 'leading-tight')
    expect(value).not.toHaveClass('overflow-hidden', 'text-ellipsis', 'whitespace-nowrap')
    expect(value).toHaveTextContent('9,007,199,254,740,991')
    expect(value?.className).not.toContain('clamp')
    expect(value?.className).not.toContain('vw')
  })

  it('keeps the five zero metrics visible when no records exist', () => {
    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: 0,
        peak_daily_credits: 0,
        average_daily_credits: 0,
        task_count: 0,
        active_days: 0,
      },
      trend: buildZeroTrend(),
    }
    usageState.isPending.value = false
    usageState.error.value = null

    renderUsagePane()

    expect(screen.getByRole('region', { name: '用量摘要' })).toHaveTextContent('总消耗')
    expect(screen.getAllByText('0')).toHaveLength(5)
    expect(screen.getByTestId('usage-trend-chart')).toHaveAttribute('data-points', '30')
    expect(screen.getByTestId('usage-trend-chart')).toHaveAttribute('data-datasets', '1')
    const table = screen.getByRole('table', { name: '最近 30 天点子消耗明细' })
    expect(table).toHaveClass('sr-only')
    expect(table.querySelectorAll('tbody tr')).toHaveLength(30)
    expect(
      Array.from(table.querySelectorAll('tbody td')).every((cell) => cell.textContent === '0 点子'),
    ).toBe(true)
    expect(screen.queryByText('暂无数据')).not.toBeInTheDocument()
  })

  it('shows a local skeleton while the first request is pending', () => {
    usageState.usage.value = undefined
    usageState.isPending.value = true
    usageState.error.value = null

    renderUsagePane()

    const skeleton = screen.getByLabelText('正在加载用量统计')
    expect(skeleton).toBeVisible()
    const cells = skeleton.querySelectorAll('article')
    expect(cells).toHaveLength(5)
    expect(cells[0]).toHaveClass(
      'min-h-[108px]',
      'min-w-0',
      'p-5',
      'max-[640px]:min-h-[100px]',
      'max-[640px]:p-4',
    )
    expect(cells[4]).toHaveClass('max-[640px]:col-span-2')
    expect(screen.getByLabelText('正在加载点子消耗趋势')).toHaveClass('min-h-[334px]')
    expect(screen.queryByRole('region', { name: '用量摘要' })).not.toBeInTheDocument()
  })

  it('uses identical cell wrapper heights while loading and loaded', () => {
    usageState.usage.value = undefined
    usageState.isPending.value = true
    usageState.error.value = null
    renderUsagePane()
    const loadingClasses = Array.from(
      screen.getByLabelText('正在加载用量统计').querySelectorAll('article'),
    ).map((cell) => cell.className)
    cleanup()

    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: 1234,
        peak_daily_credits: 500,
        average_daily_credits: 12.5,
        task_count: 9,
        active_days: 4,
      },
      trend: buildZeroTrend(),
    }
    usageState.isPending.value = false
    renderUsagePane()
    const loadedClasses = Array.from(
      screen.getByRole('region', { name: '用量摘要' }).querySelectorAll('article'),
    ).map((cell) => cell.className)

    expect(loadedClasses).toEqual(loadingClasses)
    expect(loadedClasses[0]).toContain('min-h-[108px]')
    expect(loadedClasses[0]).toContain('max-[640px]:min-h-[100px]')
  })

  it('renders the server-owned monthly credit trend below the lifetime summary', () => {
    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: 99,
        peak_daily_credits: 42,
        average_daily_credits: 3.3,
        task_count: 5,
        active_days: 4,
      },
      trend: buildZeroTrend().map((point, index) => ({
        ...point,
        credits: index === 29 ? 99 : 0,
      })),
    }
    usageState.isPending.value = false
    usageState.error.value = null

    renderUsagePane()

    expect(screen.getByRole('region', { name: '点子消耗趋势' })).toBeVisible()
    expect(screen.getByText('最近 30 天')).toBeVisible()
    expect(screen.getByTestId('usage-trend-chart')).toHaveClass('h-[260px]')
    expect(screen.getByTestId('usage-trend-chart')).toHaveAttribute('data-points', '30')
    const chartImage = screen.getByRole('img', { name: '最近 30 天点子消耗趋势' })
    const table = screen.getByRole('table', { name: '最近 30 天点子消耗明细' })
    expect(chartImage).toHaveAttribute('aria-describedby', table.querySelector('caption')?.id)
    expect(table.querySelectorAll('tbody tr')).toHaveLength(30)
    expect(table).toHaveTextContent('2026-07-30')
    expect(table).toHaveTextContent('99 点子')
  })

  it('shows an inline retry when loading fails', async () => {
    usageState.usage.value = undefined
    usageState.isPending.value = false
    usageState.error.value = new Error('网络异常')
    usageState.refresh.mockReset()

    const { user } = renderUsagePane()

    expect(screen.getByRole('alert')).toHaveTextContent('网络异常')
    await user.click(screen.getByRole('button', { name: '重新加载' }))
    expect(usageState.refresh).toHaveBeenCalledOnce()
  })
})

function buildZeroTrend() {
  return Array.from({ length: 30 }, (_, index) => ({
    date: `2026-07-${String(index + 1).padStart(2, '0')}`,
    credits: 0,
  }))
}
