import { cleanup } from '@testing-library/vue'
import { defineComponent, nextTick, type Ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../../../test/render'
import { setUserLocale } from '../../../../shared/i18n'
import UsagePane from './UsagePane.vue'

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
  displayRange: { value: '30d' as '7d' | '30d' | '1y' | undefined, __v_isRef: true },
  isInitialPending: { value: false, __v_isRef: true },
  isSwitching: { value: false, __v_isRef: true },
  error: { value: null as unknown, __v_isRef: true },
  retry: vi.fn(),
}))

let selectedRangeRef: Ref<'7d' | '30d' | '1y'> | undefined

vi.mock('../../application/usage/useUsage', () => {
  const handler = (range: Ref<'7d' | '30d' | '1y'>) => {
    selectedRangeRef = range
    return usageState
  }
  return {
    useUsage: handler,
  }
})

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
  return renderWithUser(UsagePane, {
    props: { active: true },
    global: { stubs: { Button: ButtonStub, ChartRenderer: chartStub } },
  })
}

describe('UsagePane', () => {
  afterEach(async () => {
    await setUserLocale('en')
  })

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
    usageState.isInitialPending.value = false
    usageState.error.value = null

    renderUsagePane()

    expect(screen.getByRole('heading', { name: 'Usage' })).toBeVisible()
    expect(screen.queryByText('Lifetime 数据 · Asia/Shanghai')).not.toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Usage summary' })).toHaveTextContent('1,234')
    expect(screen.getByRole('region', { name: 'Usage summary' })).toHaveTextContent('12.5')
    expect(screen.getByRole('region', { name: 'Usage summary' })).not.toHaveTextContent('Ideas')
    expect(screen.getByText('Tasks run')).toBeVisible()
    expect(screen.getByText('Active days')).toBeVisible()
  })

  it('updates range and summary labels after the locale changes', async () => {
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
    usageState.isInitialPending.value = false
    usageState.error.value = null

    renderUsagePane()
    expect(screen.getByRole('radio', { name: 'Week' })).toBeVisible()
    expect(screen.getByText('Total used')).toBeVisible()

    await setUserLocale('es')
    await nextTick()

    expect(screen.getByRole('radio', { name: 'Semana' })).toBeVisible()
    expect(screen.getByText('Total usado')).toBeVisible()
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
    usageState.isInitialPending.value = false
    usageState.error.value = null

    renderUsagePane()

    const panel = screen.getByRole('region', { name: 'Usage summary' })
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
    usageState.isInitialPending.value = false
    usageState.error.value = null

    renderUsagePane()

    const summary = within(screen.getByRole('region', { name: 'Usage summary' }))
    expect(summary.getByText('Total used')).toBeVisible()
    expect(summary.getAllByText('0')).toHaveLength(5)
    expect(summary.queryByText('0 Ideas')).not.toBeInTheDocument()
    expect(screen.getByTestId('usage-trend-chart')).toHaveAttribute('data-points', '30')
    expect(screen.getByTestId('usage-trend-chart')).toHaveAttribute('data-datasets', '1')
    const table = screen.getByRole('table', { name: 'Last 30 days ideas usage details' })
    expect(table).toHaveClass('sr-only')
    expect(table.querySelectorAll('tbody tr')).toHaveLength(30)
    expect(
      Array.from(table.querySelectorAll('tbody td')).every(
        (cell) => cell.textContent === '0 Ideas',
      ),
    ).toBe(true)
    expect(screen.queryByText('Nothing here yet')).not.toBeInTheDocument()
  })

  it('shows a local skeleton while the first request is pending', () => {
    usageState.usage.value = undefined
    usageState.isInitialPending.value = true
    usageState.error.value = null

    renderUsagePane()

    const skeleton = screen.getByLabelText('Loading usage statistics…')
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
    expect(screen.getByLabelText('Loading ideas-used trend…')).toBeVisible()
    const trendSkeleton = screen.getByTestId('usage-trend-skeleton')
    expect(trendSkeleton).toHaveClass('h-full')
    expect(trendSkeleton.parentElement).toHaveClass('h-[240px]', 'md:h-[278px]')
    expect(screen.queryByRole('region', { name: 'Usage summary' })).not.toBeInTheDocument()
  })

  it('uses identical cell wrapper heights while loading and loaded', () => {
    usageState.usage.value = undefined
    usageState.isInitialPending.value = true
    usageState.error.value = null
    renderUsagePane()
    const loadingClasses = Array.from(
      screen.getByLabelText('Loading usage statistics…').querySelectorAll('article'),
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
    usageState.isInitialPending.value = false
    renderUsagePane()
    const loadedClasses = Array.from(
      screen.getByRole('region', { name: 'Usage summary' }).querySelectorAll('article'),
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
    usageState.isInitialPending.value = false
    usageState.error.value = null

    renderUsagePane()

    expect(screen.getByRole('region', { name: 'Last 30 days ideas-used trend' })).toBeVisible()
    expect(screen.getByRole('radio', { name: 'Month' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByTestId('usage-trend-chart')).toHaveClass('h-full', 'w-full', 'min-w-0')
    expect(screen.getByTestId('usage-trend-chart').parentElement).toHaveClass(
      'h-[240px]',
      'md:h-[278px]',
    )
    expect(screen.getByTestId('usage-trend-chart')).toHaveAttribute('data-points', '30')
    const chartImage = screen.getByRole('img', { name: 'Last 30 days ideas-used trend' })
    const table = screen.getByRole('table', { name: 'Last 30 days ideas usage details' })
    expect(chartImage).toHaveAttribute('aria-describedby', table.querySelector('caption')?.id)
    expect(table.querySelectorAll('tbody tr')).toHaveLength(30)
    expect(table).toHaveTextContent('2026-07-30')
    expect(table).toHaveTextContent('99 Ideas')
  })

  it('shows an inline retry when loading fails', async () => {
    usageState.usage.value = undefined
    usageState.isInitialPending.value = false
    usageState.error.value = new Error('网络异常')
    usageState.retry.mockReset()

    const { user } = renderUsagePane()

    expect(screen.getByRole('alert')).toHaveTextContent('Please try again later.')
    await user.click(screen.getByRole('button', { name: 'Reload' }))
    expect(usageState.retry).toHaveBeenCalledOnce()
  })

  it('renders a native segmented range switcher in the chart card header', async () => {
    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: 99,
        peak_daily_credits: 42,
        average_daily_credits: 3.3,
        task_count: 5,
        active_days: 4,
      },
      trend: buildZeroTrend(),
    }
    usageState.displayRange.value = '30d'
    usageState.isInitialPending.value = false
    usageState.isSwitching.value = false
    usageState.error.value = null

    const { user } = renderUsagePane()
    const group = screen.getByRole('radiogroup', { name: 'Trend time range' })
    expect(group).not.toHaveClass('p-selectbutton')
    const trendSection = screen.getByRole('region', { name: 'Last 30 days ideas-used trend' })
    const chartHeading = screen.getByRole('heading', { name: 'Ideas used' })
    const trendHeader = chartHeading.parentElement?.parentElement
    if (!trendHeader) throw new Error('ChartCard header is missing')
    expect(trendHeader).toHaveClass('flex', 'flex-wrap', 'items-start', 'justify-between', 'gap-3')
    expect(trendHeader).toContainElement(group)
    expect(trendSection).toContainElement(trendHeader)
    expect(chartHeading).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Usage' }).closest('header')).not.toContainElement(
      group,
    )
    const month = screen.getByRole('radio', { name: 'Month' })
    expect(month).toHaveAttribute('aria-checked', 'true')
    expect(month).toHaveClass('bg-ib-surface', 'font-semibold')

    const year = screen.getByRole('radio', { name: 'Year' })
    year.focus()
    await user.keyboard('{Enter}')
    expect(year).toHaveAttribute('aria-checked', 'true')
    expect(selectedRangeRef?.value).toBe('1y')

    const week = screen.getByRole('radio', { name: 'Week' })
    week.focus()
    await user.keyboard(' ')
    expect(week).toHaveAttribute('aria-checked', 'true')
    expect(selectedRangeRef?.value).toBe('7d')
  })

  it('keeps the last successful chart and summary visible when a range switch fails', async () => {
    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: 99,
        peak_daily_credits: 42,
        average_daily_credits: 3.3,
        task_count: 5,
        active_days: 4,
      },
      trend: buildZeroTrend(),
    }
    usageState.displayRange.value = '30d'
    usageState.isInitialPending.value = false
    usageState.isSwitching.value = false
    usageState.error.value = new Error('周趋势加载失败')
    usageState.retry.mockReset()

    const { user } = renderUsagePane()

    expect(screen.getByRole('region', { name: 'Usage summary' })).toHaveTextContent('99')
    expect(screen.getByTestId('usage-trend-chart')).toHaveAttribute('data-points', '30')
    expect(screen.getByRole('radio', { name: 'Month' })).toHaveAttribute('aria-checked', 'true')
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Please try again later.')
    await user.click(screen.getByRole('button', { name: 'Retry current range' }))
    expect(usageState.retry).toHaveBeenCalledOnce()
  })
})

function buildZeroTrend() {
  return Array.from({ length: 30 }, (_, index) => ({
    date: `2026-07-${String(index + 1).padStart(2, '0')}`,
    credits: 0,
  }))
}
